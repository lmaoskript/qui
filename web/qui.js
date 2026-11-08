import { Align, Dir, Flag, Key, Kind, Mod, Op, Role, Size, Themes, Tokens, Version } from "./wire.gen.js";

const HEADER = 64;
const SLOT = 48;
const OVERSCAN_ROWS = 6;
const MAX_LIST_PX = 15000000; // Firefox discards element heights above about 17.9M px.
const listPitch = (node) => (node.est || 32) + (node.gap || 0);
const isHugeList = (node) => node.count * listPitch(node) > MAX_LIST_PX;
const DRAG_THRESHOLD = 4;
const MAGIC = 0x31495551;
const decoder = new TextDecoder();
const encoder = new TextEncoder();

const namedKeys = {
  Backspace: Key.Backspace, Tab: Key.Tab, Enter: Key.Enter, Escape: Key.Escape, " ": Key.Space, Delete: Key.Delete,
  ArrowLeft: Key.Left, ArrowRight: Key.Right, ArrowUp: Key.Up, ArrowDown: Key.Down,
  Home: Key.Home, End: Key.End, PageUp: Key.PageUp, PageDown: Key.PageDown,
};
const fieldEditKeys = new Set([Key.Left, Key.Right, Key.Home, Key.End, Key.Backspace, Key.Delete]);
const fieldCtrlKeys = new Set([..."acxvzy"].map((c) => c.codePointAt(0)).concat([...fieldEditKeys]));

function keyCode(event) {
  if (namedKeys[event.key] !== undefined) return namedKeys[event.key];
  if (/^F([1-9]|1[0-2])$/.test(event.key)) return Key.F1 + Number(event.key.slice(1)) - 1;
  if (/^Digit\d$/.test(event.code)) return event.code.charCodeAt(5);
  if ([...event.key].length === 1) return event.key.toLowerCase().codePointAt(0);
  return undefined;
}

const flexAlign = { [Align.Start]: "flex-start", [Align.Center]: "center", [Align.End]: "flex-end", [Align.Stretch]: "stretch", [Align.Between]: "space-between" };
const selfAlign = { [Align.Start]: "start", [Align.Center]: "center", [Align.End]: "end", [Align.Stretch]: "stretch", [Align.Between]: "start" };

const baseCss = `
.qui-root { position: relative; width: 100%; height: 100%; display: grid; overflow: hidden;
  font: 14px/20px var(--qui-sans); color: var(--qui-Ink); background: var(--qui-Background); }
.qui-root * { box-sizing: border-box; min-width: 0; min-height: 0; margin: 0; }
.qui-btn { padding: 6px 12px; border: 0; border-radius: 6px; background: var(--qui-Item); color: var(--qui-Ink);
  font: inherit; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.qui-btn:hover { box-shadow: inset 0 0 0 999px var(--qui-Hover); }
.qui-btn:active { box-shadow: inset 0 0 0 999px var(--qui-Active); }
.qui-btn:focus-visible { outline: 2px solid var(--qui-Accent); outline-offset: 1px; }
.qui-text { white-space: pre-wrap; overflow-wrap: anywhere; }
.qui-input { display: block; padding: 5px 9px; border: 1px solid var(--qui-Line); border-radius: 6px; background: var(--qui-Inset);
  color: var(--qui-Ink); font: inherit; outline: none; resize: none; }
.qui-input:focus { border-color: var(--qui-Accent); }
.qui-input::placeholder { color: var(--qui-Muted); opacity: 1; }
.qui-list { overflow-y: auto; display: block; }
.qui-huge { overflow-anchor: none; }
.qui-end { display: flex; flex-direction: column; }
.qui-end > * { flex-shrink: 0; }
.qui-root, .qui-root * { scrollbar-width: thin; scrollbar-color: var(--qui-Scrollbar) transparent; }
.qui-root canvas { display: block; }
.qui-root [data-qh], .qui-list, .qui-text { pointer-events: auto; }
.qui-root [data-qh]:not(.qui-input) { user-select: none; -webkit-user-select: none; touch-action: pan-y; }
`;

class Reader {
  constructor(bytes) {
    this.bytes = bytes;
    this.view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    this.at = 0;
  }
  get more() { return this.at < this.bytes.length; }
  u8() { return this.view.getUint8(this.at++); }
  u32() { const v = this.view.getUint32(this.at, true); this.at += 4; return v; }
  f32() { const v = this.view.getFloat32(this.at, true); this.at += 4; return v; }
  str() {
    const n = this.u32();
    const out = decoder.decode(this.bytes.subarray(this.at, this.at + n));
    this.at += n;
    return out;
  }
}

function color(value) {
  if ((value & 0x00ffffff) === 0 && value >>> 24 !== 0) return `var(--qui-${Tokens[value >>> 24]})`;
  return `rgba(${value >>> 24},${(value >>> 16) & 255},${(value >>> 8) & 255},${(value & 255) / 255})`;
}

function hashRange(bytes, start, end, seed) {
  let h = 2166136261 ^ seed;
  for (let i = start; i < end; i++) h = Math.imul(h ^ bytes[i], 16777619);
  return h >>> 0;
}

function decode(bytes, length) {
  const r = new Reader(bytes.subarray(0, length));
  const root = { children: [] };
  const stack = [root];
  let theme = 0;
  let current = null;
  let propsStart = 0;
  const seal = () => {
    if (!current) return;
    current.sig = hashRange(r.bytes, propsStart, r.at, 0);
    current = null;
  };
  while (r.more) {
    const before = r.at;
    const op = r.u8();
    if (op === Op.Frame) {
      const version = r.u8();
      theme = r.u8();
      if (version !== Version) console.error(`qui: app uses wire version ${version} but the patcher expects ${Version}`);
      continue;
    }
    if (op === Op.Open || op === Op.End) {
      if (current) {
        current.sig = hashRange(r.bytes, propsStart, before, 0);
        current = null;
      }
    }
    if (op === Op.End) {
      stack.pop();
      continue;
    }
    if (op === Op.Open) {
      const kind = r.u8();
      const lo = r.u32();
      const hi = r.u32();
      const node = { kind, lo, hi, key: (hi & 0x1fffff) * 4294967296 + lo, dir: Dir.Column, children: [], item: -1 };
      stack[stack.length - 1].children.push(node);
      stack.push(node);
      current = node;
      propsStart = r.at;
      continue;
    }
    const node = stack[stack.length - 1];
    switch (op) {
      case Op.Dir: node.dir = r.u8(); break;
      case Op.Width: node.wMode = r.u8(); node.wVal = r.f32(); break;
      case Op.Height: node.hMode = r.u8(); node.hVal = r.f32(); break;
      case Op.MinW: node.minW = r.f32(); break;
      case Op.MaxW: node.maxW = r.f32(); break;
      case Op.MinH: node.minH = r.f32(); break;
      case Op.MaxH: node.maxH = r.f32(); break;
      case Op.Pad: node.pad = [r.f32(), r.f32(), r.f32(), r.f32()]; break;
      case Op.Gap: node.gap = r.f32(); break;
      case Op.Align: node.align = r.u8(); break;
      case Op.Justify: node.justify = r.u8(); break;
      case Op.Pos: node.x = r.f32(); node.y = r.f32(); node.absolute = true; break;
      case Op.Bg: node.bg = r.u32(); break;
      case Op.Fg: node.fg = r.u32(); break;
      case Op.Border: node.border = r.u32(); node.borderW = r.f32(); break;
      case Op.Radius: node.radius = r.f32(); break;
      case Op.Opacity: node.opacity = r.f32(); break;
      case Op.Clip: node.clip = true; break;
      case Op.Scroll: node.scroll = true; break;
      case Op.Hit: node.hit = r.u8(); break;
      case Op.Role: node.role = r.u8(); break;
      case Op.Label: node.label = r.str(); break;
      case Op.Text: node.text = r.str(); break;
      case Op.FontSize: node.fontSize = r.f32(); break;
      case Op.Weight: node.weight = r.u8(); break;
      case Op.Mono: node.mono = true; break;
      case Op.Ellipsis: node.ellipsis = true; break;
      case Op.Virtual: node.count = r.u32(); node.est = r.f32(); node.bottom = r.u8() !== 0; break;
      case Op.Item: node.item = r.u32(); break;
      case Op.Src: node.src = r.str(); break;
      case Op.Fps: node.fps = r.f32(); break;
      case Op.Placeholder: node.placeholder = r.str(); break;
      case Op.Multiline: node.multiline = r.u8(); break;
      case Op.Password: node.password = true; break;
      case Op.Focus: node.focus = r.u8(); break;
      case Op.Z: node.z = r.u32() | 0; break;
      case Op.Gradient: node.gradient = { from: r.u32(), to: r.u32(), angle: r.f32() }; break;
      case Op.Shadow: node.shadow = r.u32(); node.shadowBlur = r.f32(); node.shadowY = r.f32(); break;
      case Op.Tracking: node.tracking = r.f32(); break;
      case Op.ScrollTo: node.scrollTo = r.f32(); break;
      case Op.Uniform: {
        const nameLength = r.u8();
        const name = decoder.decode(r.bytes.subarray(r.at, r.at + nameLength));
        r.at += nameLength;
        const n = r.u8();
        const values = [];
        for (let i = 0; i < n; i++) values.push(r.f32());
        (node.uniforms ??= []).push({ name, values });
        break;
      }
      default:
        console.error(`qui: unknown wire op ${op}, dropping the rest of the frame`);
        return { root, theme };
    }
  }
  seal();
  return { root, theme };
}

function sizeCss(mode, value, property) {
  if (mode === Size.Px) return `${property}:${value}px;`;
  if (mode === Size.Pct) return `${property}:${value}%;`;
  return "";
}

function css(node, parent) {
  let s = "position:relative;";
  const row = parent.dir === Dir.Row && parent.kind !== Kind.List;
  const column = parent.dir === Dir.Column && parent.kind !== Kind.List;
  const wMode = node.wMode ?? Size.Fit;
  const hMode = node.hMode ?? Size.Fit;
  s += sizeCss(wMode, node.wVal, "width") + sizeCss(hMode, node.hVal, "height");
  if (node.kind === Kind.Input && wMode === Size.Fit) s += "width:200px;";

  if (node.absolute) {
    s += `position:absolute;left:${node.x}px;top:${node.y}px;`;
    if ((node.kind === Kind.Box || node.kind === Kind.Shader) && !node.hit && !node.scroll) s += "pointer-events:none;";
    if (wMode === Size.Fill) s += "width:100%;";
    if (hMode === Size.Fill) s += "height:100%;";
  } else if (row) {
    s += wMode === Size.Fill ? `flex:${node.wVal} 1 0;` : wMode === Size.Fit ? "flex:0 1 auto;" : "flex:0 0 auto;";
    if (hMode === Size.Fill) s += "align-self:stretch;";
  } else if (column) {
    s += hMode === Size.Fill ? `flex:${node.hVal} 1 0;` : hMode === Size.Fit ? "flex:0 0 auto;" : "flex:0 0 auto;";
    if (wMode === Size.Fill) s += "align-self:stretch;";
  } else if (parent.kind === Kind.List) {
    s += "width:100%;";
    if (parent.gap) s += `margin-bottom:${parent.gap}px;`;
  } else {
    s += "grid-area:1/1;";
    s += `justify-self:${wMode === Size.Fill ? "stretch" : selfAlign[parent.align ?? 0]};`;
    s += `align-self:${hMode === Size.Fill ? "stretch" : selfAlign[parent.justify ?? 0]};`;
  }

  if (node.minW) s += `min-width:${node.minW}px;`;
  if (node.maxW) s += `max-width:${node.maxW}px;`;
  if (node.minH) s += `min-height:${node.minH}px;`;
  if (node.maxH) s += `max-height:${node.maxH}px;`;
  if (node.pad) s += `padding:${node.pad[0]}px ${node.pad[1]}px ${node.pad[2]}px ${node.pad[3]}px;`;
  if (node.bg) s += `background:${color(node.bg)};`;
  if (node.fg) s += `color:${color(node.fg)};`;
  if (node.gradient) {
    const { from, to, angle } = node.gradient;
    const image = `linear-gradient(${angle}deg,${color(from)},${color(to)})`;
    if (node.kind === Kind.Text) {
      s += `background-image:${image};-webkit-background-clip:text;background-clip:text;color:transparent;`;
      if (node.tracking < 0) s += `padding-right:${-node.tracking}px;margin-right:${node.tracking}px;`;
    } else {
      s += `background:${image};`;
    }
  }
  if (node.shadow) s += `box-shadow:0 ${node.shadowY}px ${node.shadowBlur}px ${color(node.shadow)};`;
  if (node.border) s += `border:${node.borderW}px solid ${color(node.border)};`;
  if (node.radius) s += `border-radius:${node.radius}px;`;
  if (node.opacity) s += `opacity:${node.opacity};`;
  if (node.clip) s += "overflow:hidden;";
  if (node.scroll) s += "overflow-y:auto;pointer-events:auto;";
  if (node.z) s += `z-index:${node.z};`;

  if (node.kind === Kind.Box) {
    if (node.dir === Dir.Stack) s += "display:grid;";
    else s += `display:flex;flex-direction:${node.dir === Dir.Row ? "row" : "column"};`;
    if (node.gap) s += `gap:${node.gap}px;`;
    if (node.dir !== Dir.Stack) {
      s += `align-items:${flexAlign[node.align ?? 0]};justify-content:${flexAlign[node.justify ?? 0]};`;
    }
  }
  if (node.kind === Kind.Text || node.kind === Kind.Button || node.kind === Kind.Input) {
    const size = node.fontSize || 14;
    s += `font-size:${size}px;line-height:${Math.ceil(size * 1.4)}px;`;
    s += node.weight ? "font-weight:600;" : "font-weight:400;";
    if (node.mono) s += "font-family:var(--qui-mono);";
    if (node.tracking) s += `letter-spacing:${node.tracking}px;`;
    if (node.ellipsis) s += "white-space:nowrap;overflow:hidden;text-overflow:ellipsis;";
    if (node.kind === Kind.Text && node.justify) s += `text-align:${node.justify === Align.Center ? "center" : node.justify === Align.End ? "right" : "left"};`;
  }
  return s;
}

function tagFor(node) {
  if (node.kind === Kind.Button) return node.role === Role.Link ? "a" : "button";
  if (node.kind === Kind.Shader) return "canvas";
  if (node.kind === Kind.Input) return node.multiline ? "textarea" : "input";
  if (node.kind === Kind.Text) return node.role === Role.Heading ? "h2" : "span";
  if (node.role === Role.Main) return "main";
  if (node.role === Role.Nav) return "nav";
  return "div";
}

const shaderVertex = `#version 300 es
out vec2 v_uv;
void main() {
  vec2 corner = vec2(float(gl_VertexID & 1), float((gl_VertexID >> 1) & 1));
  v_uv = corner;
  gl_Position = vec4(corner.x * 2.0 - 1.0, 1.0 - corner.y * 2.0, 0.0, 1.0);
}`;

const shaderHeader = `#version 300 es
precision highp float;
precision highp int;
uniform float u_time;
uniform vec2 u_resolution;
in vec2 v_uv;
out vec4 qui_out;
`;

class ShaderView {
  constructor(canvas) {
    this.canvas = canvas;
    this.gl = canvas.getContext("webgl2", { antialias: false, depth: false, stencil: false, alpha: true, premultipliedAlpha: true });
    this.visible = true;
    this.start = performance.now();
    this.last = 0;
    this.observer = new IntersectionObserver(([entry]) => {
      this.visible = entry.isIntersecting;
      if (this.visible) this.schedule();
    });
    this.observer.observe(canvas);
    this.resize = new ResizeObserver(() => this.draw(true));
    this.resize.observe(canvas);
  }

  update(node) {
    this.fps = node.fps || 0;
    this.uniforms = node.uniforms || [];
    if (node.src !== this.src) {
      this.src = node.src;
      this.compile();
    }
    this.draw(true);
    this.schedule();
  }

  compile() {
    const gl = this.gl;
    if (!gl) return;
    this.error = undefined;
    const build = (type, source) => {
      const shader = gl.createShader(type);
      gl.shaderSource(shader, source);
      gl.compileShader(shader);
      if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
        this.error = gl.getShaderInfoLog(shader);
        console.error(`qui: shader failed to compile, showing its background instead:\n${this.error}`);
        gl.deleteShader(shader);
        return null;
      }
      return shader;
    };
    const vs = build(gl.VERTEX_SHADER, shaderVertex);
    const fs = build(gl.FRAGMENT_SHADER, `${shaderHeader}#line 1\n${this.src}\nvoid main() { qui_out = shade(v_uv); }\n`);
    if (this.program) gl.deleteProgram(this.program);
    this.program = null;
    if (!vs || !fs) {
      if (vs) gl.deleteShader(vs);
      if (fs) gl.deleteShader(fs);
      return;
    }
    const program = gl.createProgram();
    gl.attachShader(program, vs);
    gl.attachShader(program, fs);
    gl.linkProgram(program);
    gl.deleteShader(vs);
    gl.deleteShader(fs);
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      this.error = gl.getProgramInfoLog(program);
      console.error(`qui: shader failed to link: ${this.error}`);
      gl.deleteProgram(program);
      return;
    }
    this.program = program;
    this.locations = new Map();
    const count = gl.getProgramParameter(program, gl.ACTIVE_UNIFORMS);
    for (let i = 0; i < count; i++) {
      const info = gl.getActiveUniform(program, i);
      this.locations.set(info.name, { location: gl.getUniformLocation(program, info.name), type: info.type });
    }
  }

  draw(force) {
    const gl = this.gl;
    if (!gl || !this.program) return;
    const dpr = devicePixelRatio || 1;
    const w = Math.max(1, Math.round(this.canvas.clientWidth * dpr));
    const h = Math.max(1, Math.round(this.canvas.clientHeight * dpr));
    if (this.canvas.width !== w || this.canvas.height !== h) {
      this.canvas.width = w;
      this.canvas.height = h;
    }
    if (!force && !this.visible) return;
    gl.viewport(0, 0, w, h);
    gl.useProgram(this.program);
    const time = gl.getUniformLocation(this.program, "u_time");
    if (time) gl.uniform1f(time, (performance.now() - this.start) / 1000);
    const resolution = gl.getUniformLocation(this.program, "u_resolution");
    if (resolution) gl.uniform2f(resolution, w, h);
    for (const { name, values } of this.uniforms) {
      const entry = this.locations.get(name);
      if (!entry) continue;
      if (entry.type === gl.INT) gl.uniform1i(entry.location, values[0]);
      else gl[`uniform${values.length}fv`](entry.location, values);
    }
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
  }

  schedule() {
    if (this.pending || !this.fps || !this.visible || this.dead) return;
    this.pending = requestAnimationFrame((now) => {
      this.pending = 0;
      if (now - this.last >= 1000 / this.fps - 1) {
        this.last = now;
        this.draw(false);
      }
      this.schedule();
    });
  }

  destroy() {
    this.dead = true;
    if (this.pending) cancelAnimationFrame(this.pending);
    this.observer.disconnect();
    this.resize.disconnect();
    this.gl?.getExtension("WEBGL_lose_context")?.loseContext();
  }
}

class Patcher {
  constructor(root) {
    this.root = root;
    root.classList.add("qui-root");
    this.nodes = new Map();
    this.entries = new Map();
    this.oneShots = [];
    this.keys = [];
    this.lists = new Set();
    this.rafPending = false;
    this.lastTime = performance.now();
    this.pointer = [-1, -1];
    this.down = false;
    this.press = null;
    this.swallowClick = false;
    this.preventKeys = new Set();
    this.engineMs = 0;
    this.themeIndex = -1;
    this.wake = null;
    this.bind();
  }

  bind() {
    const target = (event) => event.target.closest?.("[data-qh]");
    const entry = (el) => this.entries.get(el.__key);
    const flag = (el, bit, on) => {
      const e = entry(el);
      if (!e) return;
      const before = e.flags;
      e.flags = on ? e.flags | bit : e.flags & ~bit;
      if (e.flags !== before) this.requestFrame();
    };
    const shot = (el, bit) => {
      const e = entry(el);
      if (!e) return;
      e.flags |= bit;
      this.oneShots.push([e, bit]);
      this.requestFrame();
    };
    this.root.addEventListener("click", (e) => {
      if (this.swallowClick) {
        this.swallowClick = false;
        return;
      }
      const el = target(e);
      if (el) shot(el, Flag.Clicked);
    });
    this.root.addEventListener("contextmenu", (e) => { const el = target(e); if (el) shot(el, Flag.RightClicked); });
    this.root.addEventListener("pointerover", (e) => { const el = target(e); if (el) flag(el, Flag.Hovered, true); });
    this.root.addEventListener("pointerout", (e) => { const el = target(e); if (el) flag(el, Flag.Hovered, false); });
    this.root.addEventListener("pointerdown", (e) => {
      if (e.button !== 0) return;
      this.down = true;
      this.tapped = true;
      this.swallowClick = false;
      this.movePointer(e);
      this.requestFrame();
      const el = target(e);
      if (!el) return;
      flag(el, Flag.Pressed, true);
      this.press = { el, startX: e.clientX, startY: e.clientY, x: e.clientX, y: e.clientY, dragging: false };
    });
    const release = () => {
      this.down = false;
      this.requestFrame();
      const press = this.press;
      this.press = null;
      if (!press) return;
      flag(press.el, Flag.Pressed, false);
      if (!press.dragging) return;
      flag(press.el, Flag.Dragging, false);
      shot(press.el, Flag.DragEnd);
      this.swallowClick = true;
    };
    window.addEventListener("pointerup", release);
    window.addEventListener("pointercancel", release);
    this.root.addEventListener("focusin", (e) => { const el = target(e); if (el) flag(el, Flag.Focused, true); });
    this.root.addEventListener("focusout", (e) => { const el = target(e); if (el) flag(el, Flag.Focused, false); });
    window.addEventListener("pointermove", (e) => {
      this.movePointer(e);
      const press = this.press;
      if (!press) return;
      this.requestFrame();
      if (!press.dragging && Math.hypot(e.clientX - press.startX, e.clientY - press.startY) >= DRAG_THRESHOLD) {
        press.dragging = true;
        shot(press.el, Flag.DragStart);
        flag(press.el, Flag.Dragging, true);
      }
      const state = entry(press.el);
      if (press.dragging && state) {
        state.dx += e.clientX - press.x;
        state.dy += e.clientY - press.y;
      }
      press.x = e.clientX;
      press.y = e.clientY;
    }, { passive: true });
    this.root.addEventListener("input", (e) => {
      if (!e.target.classList?.contains("qui-input")) return;
      const field = e.target;
      const state = entry(field);
      if (!state) return;
      field.__reported = field.value;
      state.text = field.value;
      this.autosize(field);
      shot(field, Flag.Changed);
    });
    this.root.addEventListener("compositionstart", (e) => { e.target.__composing = true; });
    this.root.addEventListener("compositionend", (e) => { e.target.__composing = false; });
    window.addEventListener("keydown", (e) => {
      if (e.isComposing || e.keyCode === 229) return;
      const code = keyCode(e);
      if (code === undefined || code === Key.Tab) return;
      const chord = [e.ctrlKey && "ctrl", e.shiftKey && "shift", e.altKey && "alt", e.metaKey && "meta", e.key.toLowerCase()].filter(Boolean).join("+");
      if (this.preventKeys.has(chord)) e.preventDefault();
      const mods = (e.shiftKey ? Mod.Shift : 0) | (e.ctrlKey ? Mod.Ctrl : 0) | (e.altKey ? Mod.Alt : 0) | (e.metaKey ? Mod.Meta : 0);
      if (e.target.classList?.contains("qui-input")) {
        if (this.fieldKey(e.target, e, code, mods, shot)) return;
      } else if (e.target.tagName === "BUTTON" && !mods && (code === Key.Enter || code === Key.Space)) {
        return;
      }
      this.keys.push(code, mods);
      this.requestFrame();
    });
    new ResizeObserver(() => this.requestFrame()).observe(this.root);
  }

  fieldKey(field, event, code, mods, shot) {
    const shift = (mods & Mod.Shift) !== 0, ctrl = (mods & Mod.Ctrl) !== 0;
    if (code === Key.Escape || (mods & (Mod.Alt | Mod.Meta))) return false;
    if (code === Key.Enter) {
      const submit = !field.__multi || (field.__multi === 2 ? !shift : ctrl);
      if (submit) {
        if (field.__multi) event.preventDefault();
        shot(field, Flag.Submitted);
      }
      return true;
    }
    if (code === Key.Up || code === Key.Down) {
      if (!field.__multi) event.preventDefault();
      return Boolean(field.__multi);
    }
    if (ctrl) return fieldCtrlKeys.has(code);
    return code < Key.Left || fieldEditKeys.has(code);
  }

  movePointer(event) {
    const origin = this.root.getBoundingClientRect();
    this.pointer = [event.clientX - origin.left, event.clientY - origin.top];
  }

  autosize(field) {
    if (!field.__multi || field.__fixedHeight) return;
    field.style.height = "auto";
    field.style.height = `${field.scrollHeight + field.offsetHeight - field.clientHeight}px`;
  }

  requestFrame() {
    if (this.rafPending || !this.wake) return;
    this.rafPending = true;
    requestAnimationFrame(() => {
      this.rafPending = false;
      this.wake();
    });
  }

  setTheme(index) {
    if (index === this.themeIndex) return;
    this.themeIndex = index;
    for (const [name, value] of Object.entries(Themes[index] ?? Themes[0])) this.root.style.setProperty(`--qui-${name}`, value);
  }

  create(node) {
    const el = document.createElement(tagFor(node));
    el.__key = node.key;
    el.__kind = node.kind;
    this.nodes.set(node.key, el);
    if (node.kind === Kind.Button) el.className = "qui-btn";
    if (node.kind === Kind.Text) el.className = "qui-text";
    if (node.kind === Kind.Input) {
      el.className = "qui-input";
      if (node.multiline) el.rows = 1;
      el.spellcheck = false;
    }
    if (node.kind === Kind.Shader) el.__shader = new ShaderView(el);
    if (node.kind === Kind.List) {
      el.className = "qui-list";
      el.__top = el.appendChild(document.createElement("div"));
      el.__bottom = el.appendChild(document.createElement("div"));
      el.addEventListener("scroll", () => this.onListScroll(el), { passive: true });
    }
    return el;
  }

  forget(el) {
    this.nodes.delete(el.__key);
    this.entries.delete(el.__key);
    el.__shader?.destroy();
    for (const child of el.children) if (child.__key !== undefined) this.forget(child);
  }

  listPosition(el, node, scrollTop = el.scrollTop) {
    const pitch = listPitch(node);
    if (!isHugeList(node)) return scrollTop / pitch;
    const viewport = el.clientHeight;
    return (scrollTop / Math.max(1, MAX_LIST_PX - viewport)) * ((node.count * pitch - viewport) / pitch);
  }

  listRange(el, node) {
    const est = node.est || 32;
    const overscan = OVERSCAN_ROWS;
    const position = Math.floor(this.listPosition(el, node));
    const first = Math.max(0, Math.min(node.count - 1, position - overscan));
    const visible = Math.min(node.count - first, Math.ceil(el.clientHeight / est) + overscan * 2);
    return [first, Math.max(0, visible)];
  }

  placeSpacers(el, node, scrollTop = el.scrollTop) {
    const est = node.est || 32;
    const pitch = listPitch(node);
    const huge = isHugeList(node);
    let top = el.__first * pitch;
    let bottom = Math.max(0, node.count - el.__last) * pitch;
    if (huge) {
      const start = el.__top.offsetTop + el.__top.offsetHeight;
      const rendered = el.__bottom.offsetTop - start;
      const position = this.listPosition(el, node, scrollTop);
      const index = Math.max(el.__first, Math.min(el.__last - 1, Math.floor(position)));
      const anchor = el.__last > el.__first ? el.children[1 + index - el.__first] : null;
      const fraction = Math.max(0, Math.min(1, position - index));
      const anchored = scrollTop - fraction * (anchor?.offsetHeight ?? est) - (anchor ? anchor.offsetTop - start : 0);
      const atEnd = el.__last === node.count && scrollTop >= MAX_LIST_PX - el.clientHeight - 1;
      top = atEnd ? MAX_LIST_PX - rendered : anchored;
      bottom = Math.max(0, MAX_LIST_PX - top - rendered);
    }
    el.classList.toggle("qui-huge", huge);
    el.classList.toggle("qui-end", !!node.bottom);
    el.__top.style.height = `${Math.max(0, top)}px`;
    el.__top.style.marginTop = top < 0 ? `${top}px` : node.bottom ? "auto" : "0px";
    el.__bottom.style.height = `${bottom}px`;
    el.__bottom.style.marginTop = `${-(node.gap || 0)}px`;
  }

  onListScroll(el) {
    const entry = this.entries.get(el.__key);
    if (!entry || !el.__node) return;
    if (isHugeList(el.__node)) this.placeSpacers(el, el.__node);
    const [first, count] = this.listRange(el, el.__node);
    if (first !== entry.first || count !== entry.count) {
      entry.first = first;
      entry.count = count;
      this.requestFrame();
    }
  }

  entryFor(node, el) {
    let entry = this.entries.get(node.key);
    if (!entry) {
      entry = { lo: node.lo, hi: node.hi, el, flags: 0, first: 0, count: 0, dx: 0, dy: 0 };
      this.entries.set(node.key, entry);
    }
    return entry;
  }

  patch(el, node, parent) {
    const signature = node.sig ^ (parent.dir * 2654435761) ^ (parent.kind === Kind.List ? 7 : 0);
    if (el.__sig !== signature) {
      el.__sig = signature;
      el.style.cssText = css(node, parent);
      if (node.kind === Kind.Text || node.kind === Kind.Button) {
        if (el.textContent !== node.text) el.textContent = node.text ?? "";
      }
      if (node.label) el.setAttribute("aria-label", node.label);
      else el.removeAttribute("aria-label");
      if (node.role === Role.Image) el.setAttribute("role", "img");
      if (node.hit || node.kind === Kind.Button || node.kind === Kind.Input) el.setAttribute("data-qh", "");
      else el.removeAttribute("data-qh");
      if (node.kind === Kind.Shader) this.patchShader(el, node);
      if (node.kind === Kind.Input) this.patchField(el, node);
    }
    if (node.focus && document.activeElement !== el && (node.focus === 2 || !el.__autofocused)) {
      el.__autofocused = true;
      el.focus();
    }
    if (node.hit || node.kind === Kind.Button || node.kind === Kind.List || node.kind === Kind.Input) this.entryFor(node, el);
    if (node.kind === Kind.List) this.patchList(el, node);
    else this.reconcile(el, node, null, null);
    if (node.scrollTo !== undefined) el.scrollTop = node.scrollTo;
  }

  patchShader(el, node) {
    const view = el.__shader;
    const before = view.src;
    view.update(node);
    if (view.src !== before) this.requestFrame();
    if (!view.error) {
      this.entries.delete(node.key);
      return;
    }
    const entry = this.entryFor(node, el);
    entry.error = view.error;
    entry.flags |= Flag.ShaderError;
  }

  patchField(el, node) {
    el.__multi = node.multiline ?? 0;
    el.__fixedHeight = (node.hMode ?? Size.Fit) !== Size.Fit;
    el.placeholder = node.placeholder ?? "";
    if (!node.multiline) el.type = node.password ? "password" : "text";
    const value = node.text ?? "";
    if (value !== el.__reported && value !== el.value && !el.__composing) {
      el.value = value;
      el.__reported = value;
    }
    this.autosize(el);
  }

  patchList(el, node) {
    el.__node = node;
    const items = node.children;
    el.__first = items.length ? items[0].item : 0;
    el.__last = items.length ? items[items.length - 1].item + 1 : 0;
    const wasNearBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 2;
    const scrollTop = el.scrollTop;
    this.reconcile(el, node, el.__top.nextSibling, el.__bottom);
    this.placeSpacers(el, node, scrollTop);
    if (isHugeList(node) && el.scrollTop !== scrollTop) el.scrollTop = scrollTop;
    if (node.bottom && (wasNearBottom || !el.__settled)) {
      el.scrollTop = el.scrollHeight;
      this.placeSpacers(el, node);
    }
    el.__settled = true;
    const entry = this.entries.get(node.key);
    const [wantFirst, wantCount] = this.listRange(el, node);
    if (wantFirst !== entry.first || wantCount !== entry.count) {
      entry.first = wantFirst;
      entry.count = wantCount;
      this.requestFrame();
    }
  }

  reconcile(parentEl, parent, start, stop) {
    let cursor = start ?? (stop ? null : parentEl.firstElementChild);
    for (const child of parent.children) {
      let el = this.nodes.get(child.key);
      if (el && el.__kind !== child.kind) {
        if (el === cursor) cursor = cursor.nextElementSibling;
        el.remove();
        this.forget(el);
        el = undefined;
      }
      if (!el) el = this.create(child);
      if (el === cursor) cursor = cursor.nextElementSibling;
      else parentEl.insertBefore(el, cursor ?? stop ?? null);
      this.patch(el, child, parent);
    }
    while (cursor && cursor !== stop) {
      const next = cursor.nextElementSibling;
      if (cursor.__key !== undefined) {
        this.forget(cursor);
        cursor.remove();
      }
      cursor = next;
    }
  }

  apply(bytes, length) {
    const began = performance.now();
    const { root, theme } = decode(bytes, length);
    this.setTheme(theme);
    root.dir = Dir.Stack;
    root.kind = Kind.Box;
    this.reconcile(this.root, root, null, null);
    this.engineMs = performance.now() - began;
  }

  takeResults() {
    const now = performance.now();
    const n = this.entries.size;
    let capacity = 16;
    while (capacity < n * 2) capacity *= 2;
    const encoded = [];
    let textBytes = 0;
    for (const entry of this.entries.values()) {
      const text = entry.text ?? entry.error;
      if (text === undefined) continue;
      const bytes = encoder.encode(text);
      entry.first = textBytes;
      entry.count = bytes.length;
      entry.text = undefined;
      encoded.push(bytes);
      textBytes += bytes.length;
    }
    const keysAt = HEADER + capacity * SLOT;
    const textAt = keysAt + this.keys.length * 4;
    const table = new Uint8Array(textAt + textBytes);
    const view = new DataView(table.buffer);
    view.setUint32(32, keysAt, true);
    view.setUint32(36, this.keys.length / 2, true);
    view.setUint32(40, textAt, true);
    this.keys.forEach((value, index) => view.setUint32(keysAt + index * 4, value, true));
    let textCursor = textAt;
    for (const bytes of encoded) {
      table.set(bytes, textCursor);
      textCursor += bytes.length;
    }
    const hadKeys = this.keys.length > 0;
    this.keys.length = 0;
    view.setUint32(0, MAGIC, true);
    view.setUint32(4, capacity, true);
    view.setFloat32(8, this.root.clientWidth, true);
    view.setFloat32(12, this.root.clientHeight, true);
    view.setFloat32(16, devicePixelRatio || 1, true);
    view.setFloat32(20, (now - this.lastTime) / 1000, true);
    view.setFloat32(24, this.pointer[0], true);
    view.setFloat32(28, this.pointer[1], true);
    view.setUint32(44, this.down || this.tapped ? 1 : 0, true);
    this.tapped = false;
    view.setFloat32(48, this.engineMs, true);
    const origin = this.root.getBoundingClientRect();
    this.lastTime = now;
    for (const entry of this.entries.values()) {
      let i = entry.lo & (capacity - 1);
      while (view.getUint32(HEADER + i * SLOT, true) !== 0 || view.getUint32(HEADER + i * SLOT + 4, true) !== 0) i = (i + 1) & (capacity - 1);
      const at = HEADER + i * SLOT;
      view.setUint32(at, entry.lo, true);
      view.setUint32(at + 4, entry.hi, true);
      view.setUint32(at + 8, entry.flags, true);
      view.setUint32(at + 12, entry.first, true);
      view.setUint32(at + 16, entry.count, true);
      const rect = entry.el.getBoundingClientRect();
      view.setFloat32(at + 24, rect.left - origin.left, true);
      view.setFloat32(at + 28, rect.top - origin.top, true);
      view.setFloat32(at + 32, rect.width, true);
      view.setFloat32(at + 36, rect.height, true);
      view.setFloat32(at + 40, entry.dx, true);
      view.setFloat32(at + 44, entry.dy, true);
      entry.dx = entry.dy = 0;
    }
    if (this.oneShots.length || hadKeys) {
      for (const [entry, bit] of this.oneShots) entry.flags &= ~bit;
      this.oneShots.length = 0;
      this.requestFrame();
    }
    return table;
  }
}

/* Loads a Go wasm app built with GOOS=js. wasm_exec.js must already be loaded. */
export async function start({ wasm, root = document.body, sans = "system-ui, sans-serif", mono = "ui-monospace, monospace", preventKeys = [] }) {
  const style = document.createElement("style");
  style.textContent = baseCss;
  document.head.append(style);
  root.style.setProperty("--qui-sans", sans);
  root.style.setProperty("--qui-mono", mono);

  const patcher = new Patcher(root);
  patcher.preventKeys = new Set(preventKeys);
  let firstFrame;
  const ready = new Promise((resolve) => { firstFrame = resolve; });
  globalThis.__qui = {
    connect(title, wake) {
      if (title) document.title = title;
      patcher.wake = wake;
      patcher.requestFrame();
    },
    apply(bytes, length) {
      patcher.apply(bytes, length);
      firstFrame();
    },
    takeResults: () => patcher.takeResults(),
    invalidate: () => patcher.requestFrame(),
  };

  const go = new Go();
  const { instance } = await WebAssembly.instantiateStreaming(fetch(wasm), go.importObject);
  go.run(instance);
  await ready;
}
