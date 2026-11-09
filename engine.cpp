//go:build cgo

#include "engine.hpp"
#include "system_fonts.hpp"

#include <algorithm>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <limits>

namespace qui {

using namespace wire;

IRect intersect(IRect a, IRect b) {
  const int x0 = std::max(a.x, b.x), y0 = std::max(a.y, b.y);
  const int x1 = std::min(a.x + a.w, b.x + b.w), y1 = std::min(a.y + a.h, b.y + b.h);
  return {x0, y0, std::max(0, x1 - x0), std::max(0, y1 - y0)};
}

IRect unite(IRect a, IRect b) {
  if (a.empty()) return b;
  if (b.empty()) return a;
  const int x0 = std::min(a.x, b.x), y0 = std::min(a.y, b.y);
  const int x1 = std::max(a.x + a.w, b.x + b.w), y1 = std::max(a.y + a.h, b.y + b.h);
  return {x0, y0, x1 - x0, y1 - y0};
}

namespace {

class Reader {
 public:
  Reader(const uint8_t *data, size_t len) : p_(data), end_(data + len) {}
  bool more() const { return p_ < end_; }
  bool ok() const { return ok_; }

  template <class T>
  T read() {
    T value{};
    if (static_cast<size_t>(end_ - p_) < sizeof(T)) {
      ok_ = false;
      p_ = end_;
      return value;
    }
    std::memcpy(&value, p_, sizeof(T));
    p_ += sizeof(T);
    return value;
  }

  std::string_view bytes(size_t n) {
    if (static_cast<size_t>(end_ - p_) < n) {
      ok_ = false;
      p_ = end_;
      return {};
    }
    std::string_view out(reinterpret_cast<const char *>(p_), n);
    p_ += n;
    return out;
  }

  std::string_view str() { return bytes(read<uint32_t>()); }

 private:
  const uint8_t *p_, *end_;
  bool ok_ = true;
};

float clamp_size(float value, const float min_value, const float max_value) {
  if (max_value > 0) value = std::min(value, max_value);
  return std::max(value, min_value);
}

constexpr float kOverscan = 200;
constexpr float kDefaultRange = 32;
constexpr float kInputWidth = 200;

template <class F>
void each_child(std::vector<Node> &nodes, const Node &node, F &&visit) {
  for (int32_t c = node.first_child; c >= 0; c = nodes[c].next_sibling) visit(nodes[c]);
}

bool in_flow(const Node &node) { return !node.has_pos; }

bool is_text_leaf(const Node &node) { return node.kind == KindText || node.kind == KindButton; }

constexpr float kScrollbarZone = 12;

}  // namespace

float ListState::height(uint32_t index) const {
  const auto it = heights.find(index);
  return it != heights.end() ? it->second : est;
}

void visual_offsets(const TextLine &line, std::vector<float> &out) {
  const auto &glyphs = line.shaped->glyphs;
  const uint32_t count = line.last - line.first;
  out.resize(count);
  if (count == 0) return;
  const float origin = glyphs[line.first].x;
  for (uint32_t i = 0; i < count; ++i) out[i] = glyphs[line.first + i].x - origin;
  if (!line.shaped->has_rtl) return;
  const auto advance = [&](uint32_t i) { return glyphs[line.first + i].x_advance; };
  for (const uint8_t level : {uint8_t{2}, uint8_t{1}}) {
    for (uint32_t from = 0; from < count;) {
      if (glyphs[line.first + from].level < level) {
        ++from;
        continue;
      }
      uint32_t to = from;
      float low = std::numeric_limits<float>::max(), high = std::numeric_limits<float>::lowest();
      for (; to < count && glyphs[line.first + to].level >= level; ++to) {
        low = std::min(low, out[to]);
        high = std::max(high, out[to] + advance(to));
      }
      for (uint32_t i = from; i < to; ++i) out[i] = low + high - out[i] - advance(i);
      from = to;
    }
  }
}

float ListState::average() const {
  if (heights.empty()) return est + gap;
  float known = 0;
  for (const auto &kv : heights) known += kv.second + gap;
  return known / static_cast<float>(heights.size());
}

void ListState::jump(float fraction) {
  if (count == 0) return;
  const float pitch = average();
  const float position = fraction * std::max(1.f, pitch * static_cast<float>(count) - viewport) / pitch;
  anchor = std::min(static_cast<uint32_t>(position), count - 1);
  offset = (position - static_cast<float>(anchor)) * height(anchor);
  stuck = bottom && fraction >= 0.999f;
  jumped = true;
}

void ListState::scroll_by(float delta) {
  offset += delta;
  while (anchor + 1 < count && offset >= height(anchor) + gap) {
    offset -= height(anchor) + gap;
    ++anchor;
  }
  while (anchor > 0 && offset < 0) {
    --anchor;
    offset += height(anchor) + gap;
  }
  if (anchor == 0 && offset < 0) offset = 0;
  if (delta > 0) return;
  stuck = false;
}

void ListState::compute_range() {
  if (count == 0) {
    first = visible = 0;
    return;
  }
  if (bottom && stuck) {
    uint32_t top = count;
    float covered = 0;
    while (top > 0 && covered < viewport + kOverscan) covered += height(--top) + gap;
    first = top;
    visible = count - top;
    return;
  }
  uint32_t top = std::min(anchor, count - 1);
  float above = offset;
  while (top > 0 && above < kOverscan) above += height(--top) + gap;
  uint32_t last = std::min(anchor, count - 1);
  float below = height(last) - offset;
  while (last + 1 < count && below < viewport + kOverscan) below += height(++last) + gap;
  first = top;
  visible = last - top + 1;
}

Engine::Engine() {
  sans_regular_ = system_font(false, false);
  sans_bold_ = system_font(false, true);
  mono_regular_ = system_font(true, false);
  mono_bold_ = system_font(true, true);
  nodes.reserve(4096);
  lines.reserve(4096);
  frame.instances.reserve(16384);
  previous.instances.reserve(16384);
}

void Engine::decode(const uint8_t *wire, size_t len) {
  nodes.clear();
  lines.clear();
  uniforms.clear();
  shaper.trim();

  Node root;
  root.kind = KindBox;
  root.dir = DirStack;
  root.w_mode = root.h_mode = SizePx;
  nodes.push_back(root);

  std::vector<int32_t> stack{0};
  Reader in(wire, len);

  while (in.more() && in.ok()) {
    const uint8_t op = in.read<uint8_t>();
    if (op == OpFrame) {
      const uint8_t version = in.read<uint8_t>();
      theme = in.read<uint8_t>();
      if (version != kVersion)
        std::fprintf(stderr, "qui: app uses wire version %u but the engine expects %u\n", version, kVersion);
      if (theme >= kThemeCount) theme = 0;
      continue;
    }
    if (op == OpOpen) {
      const auto index = static_cast<int32_t>(nodes.size());
      Node &node = nodes.emplace_back();
      node.kind = in.read<uint8_t>();
      node.id = in.read<uint64_t>();
      node.parent = stack.back();
      if (node.kind == KindButton) {
        node.pad[0] = node.pad[2] = 6;
        node.pad[1] = node.pad[3] = 12;
        node.radius = 6;
        node.bg = Item;
        node.align = node.justify = Center;
        node.hit = 1;
      }
      if (node.kind == KindInput) {
        node.pad[0] = node.pad[2] = 5;
        node.pad[1] = node.pad[3] = 9;
        node.radius = 6;
        node.bg = Inset;
        node.border = Line;
        node.border_w = 1;
        node.align = Center;
        node.hit = 1;
      }
      if (node.kind == KindList) node.clip = true;
      Node &parent = nodes[node.parent];
      if (parent.last_child >= 0)
        nodes[parent.last_child].next_sibling = index;
      else
        parent.first_child = index;
      parent.last_child = index;
      stack.push_back(index);
      continue;
    }
    if (op == OpEnd) {
      if (Node &closing = nodes[stack.back()]; closing.border && closing.border_w > 0)
        for (float &side : closing.pad) side += closing.border_w;
      if (stack.size() > 1) stack.pop_back();
      continue;
    }

    Node &node = nodes[stack.back()];
    switch (op) {
      case OpDir: node.dir = in.read<uint8_t>(); break;
      case OpWidth:
        node.w_mode = in.read<uint8_t>();
        node.w_val = in.read<float>();
        break;
      case OpHeight:
        node.h_mode = in.read<uint8_t>();
        node.h_val = in.read<float>();
        break;
      case OpMinW: node.min_w = in.read<float>(); break;
      case OpMaxW: node.max_w = in.read<float>(); break;
      case OpMinH: node.min_h = in.read<float>(); break;
      case OpMaxH: node.max_h = in.read<float>(); break;
      case OpPad:
        for (float &p : node.pad) p = in.read<float>();
        break;
      case OpGap: node.gap = in.read<float>(); break;
      case OpAlign: node.align = in.read<uint8_t>(); break;
      case OpJustify: node.justify = in.read<uint8_t>(); break;
      case OpPos:
        node.pos_x = in.read<float>();
        node.pos_y = in.read<float>();
        node.has_pos = true;
        break;
      case OpBg: node.bg = in.read<uint32_t>(); break;
      case OpFg: node.fg = in.read<uint32_t>(); break;
      case OpBorder:
        node.border = in.read<uint32_t>();
        node.border_w = in.read<float>();
        break;
      case OpRadius: node.radius = in.read<float>(); break;
      case OpOpacity: node.opacity = in.read<float>(); break;
      case OpClip: node.clip = true; break;
      case OpScroll: node.scroll = node.clip = true; break;
      case OpHit: node.hit = in.read<uint8_t>(); break;
      case OpRole: node.role = in.read<uint8_t>(); break;
      case OpLabel: node.label = in.str(); break;
      case OpText: node.text = in.str(); break;
      case OpFontSize: node.font_size = in.read<float>(); break;
      case OpWeight: node.weight = in.read<uint8_t>(); break;
      case OpMono: node.mono = true; break;
      case OpEllipsis: node.ellipsis = true; break;
      case OpVirtual:
        node.count = in.read<uint32_t>();
        node.est = in.read<float>();
        node.bottom = in.read<uint8_t>() != 0;
        break;
      case OpItem: node.item = in.read<uint32_t>(); break;
      case OpSrc: node.src = in.str(); break;
      case OpFps: node.fps = in.read<float>(); break;
      case OpPlaceholder: node.placeholder = in.str(); break;
      case OpMultiline:
        node.multiline = in.read<uint8_t>();
        node.align = Start;
        break;
      case OpPassword: node.password = true; break;
      case OpFocus: node.focus_mode = in.read<uint8_t>(); break;
      case OpZ: node.z = in.read<int32_t>(); break;
      case OpGradient:
        node.gradient_from = in.read<uint32_t>();
        node.gradient_to = in.read<uint32_t>();
        node.gradient_angle = in.read<float>();
        node.has_gradient = true;
        break;
      case OpShadow:
        node.shadow = in.read<uint32_t>();
        node.shadow_blur = in.read<float>();
        node.shadow_y = in.read<float>();
        break;
      case OpTracking: node.tracking = in.read<float>(); break;
      case OpGestures: node.gestures = in.read<uint8_t>(); break;
      case OpScrollTo:
        node.scroll_to = in.read<float>();
        node.has_scroll_to = true;
        break;
      case OpUniform: {
        UniformRef uniform;
        uniform.name = in.bytes(in.read<uint8_t>());
        const uint8_t n = in.read<uint8_t>();
        for (uint8_t i = 0; i < n; ++i) {
          const float v = in.read<float>();
          if (i < 4) uniform.v[i] = v;
        }
        uniform.n = std::min<uint8_t>(n, 4);
        if (node.uniform_count == 0) node.uniform_first = static_cast<uint32_t>(uniforms.size());
        uniforms.push_back(uniform);
        ++node.uniform_count;
        break;
      }
      default:
        std::fprintf(stderr, "qui: unknown wire op %u, dropping the rest of the frame\n", op);
        return;
    }
  }
  if (!in.ok()) std::fprintf(stderr, "qui: truncated wire frame\n");
}

FontInstanceId Engine::font_for(const Node &node) {
  const float size = (node.font_size > 0 ? node.font_size : kDefaultFontSize) * dpr;
  const int rounded = static_cast<int>(size + 0.5f);
  const uint64_t key = (static_cast<uint64_t>(rounded) << 2) | (node.mono ? 2u : 0u) | (node.weight ? 1u : 0u);
  if (const auto it = font_cache_.find(key); it != font_cache_.end()) return it->second;
  const SystemFont &face = node.mono ? (node.weight ? mono_bold_ : mono_regular_)
                                     : (node.weight ? sans_bold_ : sans_regular_);
  const FontInstanceId id = fonts.load_font_face(face.path, face.index, static_cast<float>(rounded), true, !node.mono, face.weight);
  font_cache_[key] = id;
  return id;
}

uint32_t Engine::resolve(uint32_t color) const {
  if ((color & 0x00ffffffu) == 0 && (color >> 24) != 0) {
    const uint32_t token = color >> 24;
    if (token <= static_cast<uint32_t>(kTokenCount)) return kThemes[theme][token];
  }
  return color;
}

void Engine::intrinsic_width(Node &node) {
  const float pad_x = node.pad[1] + node.pad[3];
  float content = 0;
  if (node.kind == KindInput) {
    node.font = font_for(node);
    content = kInputWidth;
  } else if (is_text_leaf(node)) {
    node.font = font_for(node);
    std::string_view rest = node.text;
    for (;;) {
      const size_t nl = rest.find('\n');
      const std::string_view para = rest.substr(0, nl);
      const ShapedLine &shaped = shaper.shape_line_single(para, {}, {node.font, 0xffffffffu, node.tracking == 0, node.tracking * dpr});
      content = std::max(content, shaped.total_advance / dpr);
      if (nl == std::string_view::npos) {
        if (rest.size() == node.text.size()) node.shaped = &shaped;
        break;
      }
      rest.remove_prefix(nl + 1);
    }
  } else {
    int flow = 0;
    each_child(nodes, node, [&](const Node &c) {
      if (!in_flow(c)) return;
      ++flow;
      content = node.dir == DirRow ? content + c.iw : std::max(content, c.iw);
    });
    if (node.dir == DirRow && flow > 1) content += node.gap * static_cast<float>(flow - 1);
  }
  node.iw = node.w_mode == SizePx ? node.w_val : content + pad_x;
  node.iw = clamp_size(node.iw, node.min_w, node.max_w);
}

void Engine::distribute_width(Node &node) {
  const float inner = std::max(0.f, node.w - node.pad[1] - node.pad[3]);
  if (node.kind == KindList) {
    each_child(nodes, node, [&](Node &c) { c.w = inner; });
    return;
  }
  each_child(nodes, node, [&](Node &c) {
    if (in_flow(c)) return;
    c.w = c.w_mode == SizePx ? c.w_val : c.w_mode == SizePct ? inner * c.w_val / 100 : c.w_mode == SizeFill ? inner : c.iw;
    c.w = clamp_size(c.w, c.min_w, c.max_w);
  });
  if (node.dir != DirRow) {
    each_child(nodes, node, [&](Node &c) {
      if (!in_flow(c)) return;
      float w = c.iw;
      switch (c.w_mode) {
        case SizePx: w = c.w_val; break;
        case SizePct: w = inner * c.w_val / 100; break;
        case SizeFill: w = inner; break;
        default: w = node.align == Stretch ? inner : std::min(c.iw, inner); break;
      }
      c.w = clamp_size(w, c.min_w, c.max_w);
    });
    return;
  }

  float fixed = 0, weights = 0, fit_base = 0;
  int flow = 0;
  each_child(nodes, node, [&](Node &c) {
    if (!in_flow(c)) return;
    ++flow;
    switch (c.w_mode) {
      case SizePx: c.w = clamp_size(c.w_val, c.min_w, c.max_w); fixed += c.w; break;
      case SizePct: c.w = clamp_size(inner * c.w_val / 100, c.min_w, c.max_w); fixed += c.w; break;
      case SizeFill: weights += c.w_val; break;
      default: c.w = c.iw; fixed += c.w; fit_base += c.iw; break;
    }
  });
  float free_space = inner - fixed - (flow > 1 ? node.gap * static_cast<float>(flow - 1) : 0);
  if (free_space < 0 && fit_base > 0) {
    const float shrink = std::min(-free_space, fit_base);
    each_child(nodes, node, [&](Node &c) {
      if (in_flow(c) && c.w_mode == SizeFit)
        c.w = clamp_size(c.iw - c.iw / fit_base * shrink, c.min_w, c.max_w);
    });
    free_space = 0;
  }
  each_child(nodes, node, [&](Node &c) {
    if (in_flow(c) && c.w_mode == SizeFill)
      c.w = clamp_size(weights > 0 ? std::max(0.f, free_space) * c.w_val / weights : 0, c.min_w, c.max_w);
  });
}

void Engine::wrap(Node &node, float inner_width) {
  node.line_first = static_cast<uint32_t>(lines.size());
  const float limit = node.kind == KindInput && !node.multiline ? std::numeric_limits<float>::infinity() : inner_width * dpr;
  std::string_view rest = node.text;
  uint32_t para_byte = 0;
  for (;;) {
    const size_t nl = rest.find('\n');
    const std::string_view para = rest.substr(0, nl);
    const ShapedLine &shaped = node.shaped ? *node.shaped : shaper.shape_line_single(para, {}, {node.font, 0xffffffffu, node.tracking == 0, node.tracking * dpr});
    const auto count = static_cast<uint32_t>(shaped.glyphs.size());
    const uint32_t para_end = para_byte + static_cast<uint32_t>(para.size());

    if (node.ellipsis || shaped.total_advance <= limit + 0.5f || count == 0) {
      TextLine line{&shaped, 0, count, shaped.total_advance, false, para_byte, para_byte, para_end};
      if (node.ellipsis && shaped.total_advance > limit + 0.5f) {
        const ShapedLine &dots = shaper.shape_line_single("\xe2\x80\xa6", {}, {node.font, 0xffffffffu, true});
        const float room = std::max(0.f, limit - dots.total_advance);
        uint32_t last = 0;
        while (last < count && shaped.glyphs[last].x + shaped.glyphs[last].x_advance <= room) ++last;
        line.last = last;
        line.width = (last ? shaped.glyphs[last - 1].x + shaped.glyphs[last - 1].x_advance : 0) + dots.total_advance;
        line.ellipsis = true;
      }
      lines.push_back(line);
    } else {
      uint32_t start = 0;
      while (start < count) {
        const float origin = shaped.glyphs[start].x;
        uint32_t last_break = start;
        uint32_t end = count;
        for (uint32_t i = start; i < count; ++i) {
          const ShapedGlyph &g = shaped.glyphs[i];
          if (g.x + g.x_advance - origin > limit && i > start) {
            end = last_break > start ? last_break : i;
            break;
          }
          if (g.cluster_start_byte < para.size() && para[g.cluster_start_byte] == ' ') last_break = i + 1;
        }
        uint32_t visible = end;
        while (visible > start && para[shaped.glyphs[visible - 1].cluster_start_byte] == ' ') --visible;
        const float width = visible > start
                                ? shaped.glyphs[visible - 1].x + shaped.glyphs[visible - 1].x_advance - origin
                                : 0;
        const uint32_t line_first = start;
        const uint32_t line_start = para_byte + shaped.glyphs[line_first].cluster_start_byte;
        start = end;
        while (start < count && para[shaped.glyphs[start].cluster_start_byte] == ' ') ++start;
        const uint32_t line_end = start < count ? para_byte + shaped.glyphs[start].cluster_start_byte : para_end;
        lines.push_back({&shaped, line_first, visible, width, false, para_byte, line_start, line_end});
      }
    }
    if (node.ellipsis || nl == std::string_view::npos) break;
    rest.remove_prefix(nl + 1);
    para_byte = para_end + 1;
  }
  node.line_count = static_cast<uint32_t>(lines.size()) - node.line_first;
}

void Engine::intrinsic_height(Node &node) {
  const float pad_y = node.pad[0] + node.pad[2];
  float content = 0;
  if (is_text_leaf(node) || node.kind == KindInput) {
    wrap(node, std::max(0.f, node.w - node.pad[1] - node.pad[3]));
    const float size = node.font_size > 0 ? node.font_size : kDefaultFontSize;
    content = line_height_for(size) * static_cast<float>(node.line_count);
  } else {
    int flow = 0;
    each_child(nodes, node, [&](const Node &c) {
      if (!in_flow(c)) return;
      ++flow;
      content = node.dir == DirColumn ? content + c.ih : std::max(content, c.ih);
    });
    if (node.dir == DirColumn && flow > 1) content += node.gap * static_cast<float>(flow - 1);
    if (node.kind == KindList) content = 0;
  }
  node.ih = node.h_mode == SizePx ? node.h_val : content + pad_y;
  node.ih = clamp_size(node.ih, node.min_h, node.max_h);
}

void Engine::distribute_height(Node &node) {
  const float inner = std::max(0.f, node.h - node.pad[0] - node.pad[2]);
  if (node.kind == KindList) {
    each_child(nodes, node, [&](Node &c) { c.h = c.ih; });
    return;
  }
  each_child(nodes, node, [&](Node &c) {
    if (in_flow(c)) return;
    c.h = c.h_mode == SizePx ? c.h_val : c.h_mode == SizePct ? inner * c.h_val / 100 : c.h_mode == SizeFill ? inner : c.ih;
    c.h = clamp_size(c.h, c.min_h, c.max_h);
  });
  if (node.dir != DirColumn) {
    each_child(nodes, node, [&](Node &c) {
      if (!in_flow(c)) return;
      float h = c.ih;
      switch (c.h_mode) {
        case SizePx: h = c.h_val; break;
        case SizePct: h = inner * c.h_val / 100; break;
        case SizeFill: h = inner; break;
        default: h = node.dir == DirRow && node.align == Stretch ? inner : c.ih; break;
      }
      c.h = clamp_size(h, c.min_h, c.max_h);
    });
    return;
  }

  float fixed = 0, weights = 0;
  int flow = 0;
  each_child(nodes, node, [&](Node &c) {
    if (!in_flow(c)) return;
    ++flow;
    switch (c.h_mode) {
      case SizePx: c.h = clamp_size(c.h_val, c.min_h, c.max_h); fixed += c.h; break;
      case SizePct: c.h = clamp_size(inner * c.h_val / 100, c.min_h, c.max_h); fixed += c.h; break;
      case SizeFill: weights += c.h_val; break;
      default: c.h = c.ih; fixed += c.h; break;
    }
  });
  const float free_space = std::max(0.f, inner - fixed - (flow > 1 ? node.gap * static_cast<float>(flow - 1) : 0));
  each_child(nodes, node, [&](Node &c) {
    if (in_flow(c) && c.h_mode == SizeFill && !node.scroll)
      c.h = clamp_size(weights > 0 ? free_space * c.h_val / weights : 0, c.min_h, c.max_h);
    else if (in_flow(c) && c.h_mode == SizeFill)
      c.h = c.ih;
  });
}

void Engine::place_list(Node &node) {
  ListState &state = lists[node.id];
  const bool first_layout = !state.seen;
  state.seen = true;
  state.frame = frame_number;
  state.bottom = node.bottom;
  state.gap = node.gap;
  state.est = node.est > 0 ? node.est : 32;
  state.count = node.count;
  state.viewport = std::max(0.f, node.h - node.pad[0] - node.pad[2]);
  if (state.anchor >= state.count) state.anchor = state.count ? state.count - 1 : 0;

  const float left = node.x + node.pad[3];
  const float top = node.y + node.pad[0];
  const float bottom = top + state.viewport;

  std::vector<Node *> items;
  each_child(nodes, node, [&](Node &c) {
    if (c.item >= 0) {
      state.heights[static_cast<uint32_t>(c.item)] = c.h;
      items.push_back(&c);
    }
  });

  if (items.empty()) {
    if (first_layout) wants_another_frame = true;
    return;
  }

  if (node.bottom && state.stuck) {
    float y = bottom;
    float total = 0;
    for (auto it = items.rbegin(); it != items.rend(); ++it) {
      Node &c = **it;
      y -= c.h;
      c.x = left;
      c.y = y;
      total += c.h + node.gap;
      if (y <= top) {
        state.anchor = static_cast<uint32_t>(c.item);
        state.offset = top - y;
      }
      y -= node.gap;
    }
    if (y + node.gap > top && items.front()->item > 0) wants_another_frame = true;
    if (first_layout) wants_another_frame = true;
    return;
  }

  int anchor_index = -1;
  for (size_t i = 0; i < items.size(); ++i) {
    if (items[i]->item == state.anchor) {
      anchor_index = static_cast<int>(i);
      break;
    }
  }
  const bool stale = anchor_index < 0 && state.jumped;
  if (anchor_index < 0) {
    wants_another_frame = true;
    anchor_index = 0;
    if (!stale) {
      state.anchor = static_cast<uint32_t>(items[0]->item);
      state.offset = 0;
    }
  } else {
    state.jumped = false;
  }

  const auto position = [&](float anchor_y) {
    float y = anchor_y;
    for (size_t i = static_cast<size_t>(anchor_index); i < items.size(); ++i) {
      items[i]->x = left;
      items[i]->y = y;
      y += items[i]->h + node.gap;
    }
    y = anchor_y;
    for (int i = anchor_index - 1; i >= 0; --i) {
      y -= items[static_cast<size_t>(i)]->h + node.gap;
      items[static_cast<size_t>(i)]->x = left;
      items[static_cast<size_t>(i)]->y = y;
    }
  };

  float anchor_y = top - state.offset;
  position(anchor_y);
  if (stale) return;

  Node &last = *items.back();
  const bool reached_end = last.item + 1 == static_cast<int64_t>(state.count);
  if (reached_end && last.y + last.h < bottom) {
    const float shift = bottom - (last.y + last.h);
    Node &first = *items.front();
    const float allowed = first.item == 0 ? std::max(0.f, top - first.y) : shift;
    const float applied = std::min(shift, allowed);
    if (applied > 0) {
      anchor_y += applied;
      position(anchor_y);
      state.offset -= applied;
      state.scroll_by(0);
    }
  }
  state.stuck = reached_end && last.y + last.h <= bottom + 1;

  if ((!reached_end && last.y + last.h < bottom) || (items.front()->item > 0 && items.front()->y > top))
    wants_another_frame = true;
  if (first_layout) wants_another_frame = true;
}

void Engine::place_children(Node &node) {
  if (node.kind == KindList) {
    place_list(node);
    return;
  }
  const float left = node.x + node.pad[3], top = node.y + node.pad[0];
  const float inner_w = std::max(0.f, node.w - node.pad[1] - node.pad[3]);
  const float inner_h = std::max(0.f, node.h - node.pad[0] - node.pad[2]);

  float scroll = 0;
  if (node.scroll) {
    ScrollState &state = scrolls[node.id];
    state.frame = frame_number;
    state.viewport = inner_h;
    int flow = 0;
    float sum = 0;
    each_child(nodes, node, [&](const Node &c) {
      if (in_flow(c)) {
        ++flow;
        sum += c.h;
      }
    });
    state.content = sum + (flow > 1 ? node.gap * static_cast<float>(flow - 1) : 0);
    if (node.has_scroll_to) state.offset = node.scroll_to;
    state.offset = std::clamp(state.offset, 0.f, std::max(0.f, state.content - state.viewport));
    scroll = state.offset;
  }

  each_child(nodes, node, [&](Node &c) {
    if (!in_flow(c)) {
      c.x = left + c.pos_x;
      c.y = top + c.pos_y;
    }
  });

  if (node.dir == DirStack) {
    each_child(nodes, node, [&](Node &c) {
      if (!in_flow(c)) return;
      const float free_x = inner_w - c.w, free_y = inner_h - c.h;
      c.x = left + (node.align == Center ? free_x / 2 : node.align == End ? free_x : 0);
      c.y = top - scroll + (node.justify == Center ? free_y / 2 : node.justify == End ? free_y : 0);
    });
    return;
  }

  const bool row = node.dir == DirRow;
  const float main_size = row ? inner_w : inner_h;
  const float cross_size = row ? inner_h : inner_w;
  float total = 0;
  int flow = 0;
  each_child(nodes, node, [&](const Node &c) {
    if (!in_flow(c)) return;
    total += row ? c.w : c.h;
    ++flow;
  });
  total += flow > 1 ? node.gap * static_cast<float>(flow - 1) : 0;
  const float free_space = main_size - total;

  float cursor = 0, gap = node.gap;
  if (node.justify == Center) cursor = free_space / 2;
  else if (node.justify == End) cursor = free_space;
  else if (node.justify == Between && flow > 1 && free_space > 0) gap += free_space / static_cast<float>(flow - 1);

  each_child(nodes, node, [&](Node &c) {
    if (!in_flow(c)) return;
    const float child_cross = row ? c.h : c.w;
    const float offset = node.align == Center ? (cross_size - child_cross) / 2 : node.align == End ? cross_size - child_cross : 0;
    if (row) {
      c.x = left + cursor;
      c.y = top + offset;
    } else {
      c.x = left + offset;
      c.y = top + cursor - scroll;
    }
    cursor += (row ? c.w : c.h) + gap;
  });
}

void Engine::layout(float width, float height, float scale) {
  dpr = scale;
  ++frame_number;
  wants_another_frame = false;
  Node &root = nodes[0];
  root.w_val = width;
  root.h_val = height;
  root.w = width;
  root.h = height;
  for (int side = 0; side < 4; ++side) root.pad[side] = insets[side];

  const int count = static_cast<int>(nodes.size());
  for (int i = 0; i < count; ++i)
    if (nodes[static_cast<size_t>(i)].kind == KindInput) sync_input(nodes[static_cast<size_t>(i)], i);
  for (int i = count - 1; i >= 0; --i) intrinsic_width(nodes[static_cast<size_t>(i)]);
  for (int i = 0; i < count; ++i) distribute_width(nodes[static_cast<size_t>(i)]);
  for (int i = count - 1; i >= 0; --i) intrinsic_height(nodes[static_cast<size_t>(i)]);
  for (int i = 0; i < count; ++i) distribute_height(nodes[static_cast<size_t>(i)]);
  for (int i = 0; i < count; ++i) place_children(nodes[static_cast<size_t>(i)]);
  prune();
}

void Engine::prune() {
  if (frame_number % 256 != 0) return;
  const auto stale = [&](uint64_t frame_seen) { return frame_number - frame_seen > 600; };
  std::erase_if(lists, [&](const auto &kv) { return stale(kv.second.frame); });
  std::erase_if(scrolls, [&](const auto &kv) { return stale(kv.second.frame); });
  std::erase_if(inputs, [&](const auto &kv) { return stale(kv.second.frame); });
}

void Engine::add_instance(const Instance &instance, int page, IRect clip) {
  auto &batches = frame.batches;
  const bool same_clip = !batches.empty() && batches.back().clip.x == clip.x && batches.back().clip.y == clip.y &&
                         batches.back().clip.w == clip.w && batches.back().clip.h == clip.h && batches.back().round == round_clip_;
  const bool page_ok = !batches.empty() && (page < 0 || batches.back().page < 0 || batches.back().page == page);
  if (batches.empty() || !same_clip || !page_ok || batches.back().shader >= 0) {
    Batch batch;
    batch.first = static_cast<uint32_t>(frame.instances.size());
    batch.clip = clip;
    batch.round = round_clip_;
    batch.page = page;
    batches.push_back(batch);
  } else if (page >= 0) {
    batches.back().page = page;
  }
  frame.instances.push_back(instance);
  ++batches.back().count;
}

void Engine::add_rect(float x, float y, float w, float h, uint32_t color, float radius, float stroke, IRect clip) {
  const auto rgba = __builtin_bswap32(color);
  Instance instance{x, y, w, h, radius, stroke, 0, 0, rgba, InstSolid};
  if (radius > 0 || stroke > 0) instance.flags |= InstRounded;
  add_instance(instance, -1, clip);
}

void Engine::add_gradient(float x, float y, float w, float h, uint32_t from, uint32_t to, float angle, float radius, IRect clip) {
  Instance instance{x, y, w, h, radius, angle, encode_color_high(to), encode_color_low(to), __builtin_bswap32(from), InstSolid | InstGradient};
  if (radius > 0) instance.flags |= InstRounded;
  add_instance(instance, -1, clip);
}

void Engine::paint_text(const Node &node, float opacity, IRect clip, float dx, float dy, uint32_t color_override) {
  const float size = node.font_size > 0 ? node.font_size : kDefaultFontSize;
  const float line_height = line_height_for(size) * dpr;
  const FontMetrics *metrics = fonts.metrics(node.font);
  if (!metrics) return;

  uint32_t color = resolve(color_override ? color_override : node.fg ? node.fg : static_cast<uint32_t>(Ink));
  const auto alpha = static_cast<uint32_t>(static_cast<float>(color & 0xffu) * opacity + 0.5f);
  color = (color & 0xffffff00u) | std::min(alpha, 255u);
  const uint32_t rgba = __builtin_bswap32(color);

  const auto faded = [&](uint32_t c) {
    c = resolve(c);
    return (c & 0xffffff00u) | std::min(static_cast<uint32_t>(static_cast<float>(c & 0xffu) * opacity + 0.5f), 255u);
  };
  const float angle = node.gradient_angle * 3.14159265f / 180;
  const bool gradient = node.has_gradient && !color_override;
  const bool vertical = std::abs(std::cos(angle)) > std::abs(std::sin(angle));
  const bool reversed = vertical ? std::cos(angle) > 0 : std::sin(angle) < 0;
  const uint32_t gradient_from = gradient ? faded(reversed ? node.gradient_to : node.gradient_from) : 0;
  const uint32_t gradient_to = gradient ? faded(reversed ? node.gradient_from : node.gradient_to) : 0;
  const float gradient_start = (vertical ? node.y : node.x) * dpr + (vertical ? dy : dx);
  const float gradient_span = std::max(1.f, (vertical ? node.h : node.w) * dpr);
  const auto gradient_at = [&](float position) {
    return mix_colors(gradient_from, gradient_to, std::clamp((position - gradient_start) / gradient_span, 0.f, 1.f));
  };

  const float inner_w = (node.w - node.pad[1] - node.pad[3]) * dpr;
  const float inner_h = (node.h - node.pad[0] - node.pad[2]) * dpr;
  const float text_h = line_height * static_cast<float>(node.line_count);
  const float free_h = inner_h - text_h;
  float y = (node.y + node.pad[0]) * dpr + dy + (node.align == Center ? free_h / 2 : node.align == End ? free_h : 0);

  const float ink = metrics->ascender - metrics->descender;
  const auto emit = [&](const ShapedGlyph &g, float origin) {
    float position = origin + g.x_offset;
    uint32_t quarter = 0;
    if (fonts.subpixel(g.font)) {
      const long quarters = std::lround(position * 4);
      quarter = static_cast<uint32_t>(quarters & 3);
      position = static_cast<float>(quarters - static_cast<long>(quarter)) / 4;
    } else {
      position = std::round(position);
    }
    const CachedGlyph *glyph = fonts.get_glyph(g.font, g.glyph_id, quarter);
    if (!glyph || !glyph->has_bitmap) return;
    const float gx = position + static_cast<float>(glyph->bitmap_left);
    const float gy = std::round(y + (line_height - ink) / 2 + metrics->ascender - g.y_offset) - static_cast<float>(glyph->bitmap_top);
    const IRect bounds{static_cast<int>(gx), static_cast<int>(gy), static_cast<int>(glyph->bitmap_width), static_cast<int>(glyph->bitmap_height)};
    if (intersect(bounds, clip).empty()) return;
    Instance instance{gx, gy, static_cast<float>(glyph->bitmap_width), static_cast<float>(glyph->bitmap_height),
                      static_cast<float>(glyph->atlas_x), static_cast<float>(glyph->atlas_y), 0, 0,
                      glyph->color_bitmap ? (0x00ffffffu | (std::min(alpha, 255u) << 24)) : rgba,
                      glyph->color_bitmap ? InstColorGlyph : InstGlyph};
    if (gradient && !glyph->color_bitmap) {
      const float start = vertical ? gy : gx;
      const uint32_t end = gradient_at(start + (vertical ? instance.h : instance.w));
      instance.rgba = __builtin_bswap32(gradient_at(start));
      instance.u1 = encode_color_high(end);
      instance.v1 = encode_color_low(end);
      instance.flags |= vertical ? InstGradientV : InstGradient;
    }
    add_instance(instance, static_cast<int>(glyph->atlas_page), clip);
  };

  for (uint32_t i = 0; i < node.line_count; ++i) {
    const TextLine &line = lines[node.line_first + i];
    const float free_w = inner_w - line.width;
    const float x = (node.x + node.pad[3]) * dpr + dx + (node.justify == Center ? free_w / 2 : node.justify == End ? free_w : 0);
    const float origin = line.first < line.shaped->glyphs.size() ? line.shaped->glyphs[line.first].x : 0;
    float end = x;
    const bool bidi = line.shaped->has_rtl;
    if (bidi) visual_offsets(line, visual_);
    for (uint32_t g = line.first; g < line.last; ++g) {
      const ShapedGlyph &shaped = line.shaped->glyphs[g];
      emit(shaped, x + (bidi ? visual_[g - line.first] : shaped.x - origin));
      end = x + shaped.x - origin + shaped.x_advance;
    }
    if (line.ellipsis) {
      const ShapedLine &dots = shaper.shape_line_single("\xe2\x80\xa6", {}, {node.font, 0xffffffffu, true});
      for (const ShapedGlyph &shaped : dots.glyphs) emit(shaped, end + shaped.x);
    }
    y += line_height;
  }
}

void Engine::paint_node(int index, float opacity, IRect clip) {
  const Node &node = nodes[static_cast<size_t>(index)];
  const uint64_t hovered = ui.hovered, pressed = ui.pressed;
  const float alpha = opacity * node.opacity;

  const auto snap = [&](float v) { return std::round(v * dpr); };
  const float x0 = snap(node.x), y0 = snap(node.y);
  const float x1 = snap(node.x + node.w), y1 = snap(node.y + node.h);
  const float w = x1 - x0, h = y1 - y0;
  const IRect bounds{static_cast<int>(x0), static_cast<int>(y0), static_cast<int>(w), static_cast<int>(h)};
  const IRect visible = intersect(bounds, clip);

  const auto fade = [&](uint32_t c) {
    const auto a = static_cast<uint32_t>(static_cast<float>(c & 0xffu) * alpha + 0.5f);
    return (c & 0xffffff00u) | std::min(a, 255u);
  };

  int32_t own_scroll = -1, own_hit = -1;
  if (index != 0 && (node.hit || node.scroll || node.gestures || node.kind == KindList)) {
    HitEntry hit;
    hit.id = node.id;
    hit.x = node.x;
    hit.y = node.y;
    hit.w = node.w;
    hit.h = node.h;
    hit.cx0 = static_cast<float>(clip.x) / dpr;
    hit.cy0 = static_cast<float>(clip.y) / dpr;
    hit.cx1 = static_cast<float>(clip.x + clip.w) / dpr;
    hit.cy1 = static_cast<float>(clip.y + clip.h) / dpr;
    hit.press = node.hit != 0;
    hit.scroll = node.scroll || node.kind == KindList || (node.kind == KindInput && node.multiline);
    hit.focusable = node.kind == KindButton || node.kind == KindInput;
    hit.scroll_parent = scroll_parent_;
    hit.parent = hit_parent_;
    hit.gestures = node.gestures;
    hits.push_back(hit);
    own_hit = static_cast<int32_t>(hits.size()) - 1;
    if (hit.scroll) own_scroll = own_hit;
  }

  const float radius = node.radius * dpr;
  if (node.shadow) {
    const float blur = std::max(1.f, std::round(node.shadow_blur * dpr));
    const float offset = std::round(node.shadow_y * dpr);
    const IRect area{static_cast<int>(x0 - blur), static_cast<int>(y0 + offset - blur), static_cast<int>(w + blur * 2), static_cast<int>(h + blur * 2)};
    if (!intersect(area, clip).empty()) {
      const Instance instance{x0 - blur, y0 + offset - blur, w + blur * 2, h + blur * 2, radius, blur, 0, offset, __builtin_bswap32(fade(resolve(node.shadow))), InstShadow};
      add_instance(instance, -1, clip);
    }
  }

  if (visible.empty() && index != 0) return;

  if (node.kind == KindShader) {
    if (node.bg) add_rect(x0, y0, w, h, fade(resolve(node.bg)), radius, 0, clip);
    ShaderDraw draw;
    draw.id = node.id;
    draw.src.assign(node.src);
    draw.hash = std::hash<std::string>{}(draw.src);
    draw.rect = bounds;
    draw.fps = node.fps;
    draw.opacity = alpha;
    for (uint32_t i = 0; i < node.uniform_count; ++i) {
      const UniformRef &ref = uniforms[node.uniform_first + i];
      ShaderUniform uniform;
      uniform.name.assign(ref.name);
      std::memcpy(uniform.v, ref.v, sizeof(uniform.v));
      uniform.n = ref.n;
      draw.uniforms.push_back(std::move(uniform));
    }
    Batch batch;
    batch.first = static_cast<uint32_t>(frame.instances.size());
    batch.clip = clip;
    batch.round = radius > 0 ? RoundClip{bounds.x, bounds.y, bounds.w, bounds.h, radius} : round_clip_;
    batch.shader = static_cast<int>(frame.shaders.size());
    frame.batches.push_back(batch);
    frame.shaders.push_back(std::move(draw));
  } else if (node.has_gradient && node.kind != KindText) {
    add_gradient(x0, y0, w, h, fade(resolve(node.gradient_from)), fade(resolve(node.gradient_to)), node.gradient_angle * 3.14159265f / 180, radius, clip);
  } else if (node.bg) {
    add_rect(x0, y0, w, h, fade(resolve(node.bg)), radius, 0, clip);
  }

  if (node.kind == KindButton && (node.id == hovered || node.id == pressed)) {
    const uint32_t tint = resolve(node.id == pressed ? Active : Hover);
    add_rect(x0, y0, w, h, fade(tint), radius, 0, clip);
  }
  if (node.border && node.border_w > 0) {
    const uint32_t border = node.kind == KindInput && node.id == ui.focused ? static_cast<uint32_t>(Accent) : node.border;
    add_rect(x0, y0, w, h, fade(resolve(border)), radius, std::max(1.f, std::round(node.border_w * dpr)), clip);
  }
  if (node.kind == KindButton && node.id == ui.focused && ui.focus_ring) {
    add_rect(x0, y0, w, h, fade(resolve(Accent)), radius, std::round(2 * dpr), clip);
  }

  if (is_text_leaf(node)) paint_text(node, alpha, clip);
  if (node.kind == KindInput) paint_input(node, alpha, clip);

  IRect child_clip = clip;
  const RoundClip outer_round = round_clip_;
  if (node.clip) {
    const int border = node.border && node.border_w > 0 ? static_cast<int>(std::max(1.f, std::round(node.border_w * dpr))) : 0;
    const IRect inner{bounds.x + border, bounds.y + border, bounds.w - border * 2, bounds.h - border * 2};
    child_clip = intersect(clip, inner);
    if (radius > static_cast<float>(border)) round_clip_ = {inner.x, inner.y, inner.w, inner.h, radius - static_cast<float>(border)};
  }
  const int32_t outer_scroll = scroll_parent_, outer_hit = hit_parent_;
  if (own_scroll >= 0) scroll_parent_ = own_scroll;
  if (own_hit >= 0) hit_parent_ = own_hit;
  paint_children(node, alpha, child_clip);
  scroll_parent_ = outer_scroll;
  hit_parent_ = outer_hit;

  if (node.kind == KindList) {
    const auto it = lists.find(node.id);
    if (it != lists.end() && it->second.count) {
      const ListState &state = it->second;
      const float average = state.average();
      const float total = average * static_cast<float>(state.count);
      const float along = (static_cast<float>(state.anchor) + state.offset / std::max(1.f, state.height(state.anchor))) * average;
      const float range = std::max(1.f, total - state.viewport);
      add_scrollbar(node, alpha, child_clip, std::clamp(along / range, 0.f, 1.f), state.viewport / std::max(1.f, total));
    }
  } else if (node.scroll) {
    const auto it = scrolls.find(node.id);
    if (it != scrolls.end() && it->second.content > it->second.viewport) {
      const ScrollState &state = it->second;
      add_scrollbar(node, alpha, child_clip, state.offset / (state.content - state.viewport), state.viewport / state.content);
    }
  }
  round_clip_ = outer_round;
}

void Engine::add_scrollbar(const Node &node, float alpha, IRect clip, float position, float fraction) {
  if (fraction >= 1) return;
  const float y0 = std::round(node.y * dpr), x1 = std::round((node.x + node.w) * dpr);
  const float track = std::round((node.y + node.h) * dpr) - y0;
  const float thumb = std::max(24 * dpr, track * fraction);
  const float thumb_top = std::round((track - thumb) * position);
  const bool hot = node.id == ui.scrollbar;
  const float bar_w = std::round((hot ? 7.f : 4.f) * dpr);
  uint32_t color = resolve(hot || node.id == ui.hovered ? ScrollbarHot : Scrollbar);
  color = (color & 0xffffff00u) | std::min(static_cast<uint32_t>(static_cast<float>(color & 0xffu) * alpha + 0.5f), 255u);
  add_rect(x1 - bar_w - std::round(2 * dpr), y0 + thumb_top, bar_w, std::round(thumb), color, bar_w / 2, 0, clip);

  HitEntry hit;
  hit.id = node.id;
  hit.x = node.x + node.w - kScrollbarZone;
  hit.y = node.y;
  hit.w = kScrollbarZone;
  hit.h = node.h;
  hit.cx0 = static_cast<float>(clip.x) / dpr;
  hit.cy0 = static_cast<float>(clip.y) / dpr;
  hit.cx1 = static_cast<float>(clip.x + clip.w) / dpr;
  hit.cy1 = static_cast<float>(clip.y + clip.h) / dpr;
  hit.scroll = hit.scrollbar = true;
  hit.thumb_y = node.y + thumb_top / dpr;
  hit.thumb_h = thumb / dpr;
  hits.push_back(hit);
}

void Engine::paint_children(const Node &node, float opacity, IRect clip) {
  bool layered = false;
  for (int32_t c = node.first_child; c >= 0; c = nodes[static_cast<size_t>(c)].next_sibling) layered |= nodes[static_cast<size_t>(c)].z != 0;
  if (!layered) {
    for (int32_t c = node.first_child; c >= 0; c = nodes[static_cast<size_t>(c)].next_sibling) paint_node(c, opacity, clip);
    return;
  }
  std::vector<int32_t> order;
  for (int32_t c = node.first_child; c >= 0; c = nodes[static_cast<size_t>(c)].next_sibling) order.push_back(c);
  std::stable_sort(order.begin(), order.end(), [&](int32_t a, int32_t b) { return nodes[static_cast<size_t>(a)].z < nodes[static_cast<size_t>(b)].z; });
  for (const int32_t c : order) paint_node(c, opacity, clip);
}

void Engine::paint() {
  scroll_parent_ = hit_parent_ = -1;
  round_clip_ = {};
  frame.instances.clear();
  frame.batches.clear();
  frame.shaders.clear();
  hits.clear();
  frame.width = static_cast<int>(std::round(nodes[0].w * dpr));
  frame.height = static_cast<int>(std::round(nodes[0].h * dpr));
  frame.clear = kThemes[theme][TokenBackground];
  // The system bars sit outside the root's padding, so they show the clear color.
  if (insets[0] > 0 || insets[1] > 0 || insets[2] > 0 || insets[3] > 0) {
    const int32_t child = nodes[0].first_child;
    if (child >= 0 && nodes[static_cast<size_t>(child)].bg && nodes[static_cast<size_t>(child)].w_mode == SizeFill && nodes[static_cast<size_t>(child)].h_mode == SizeFill)
      frame.clear = resolve(nodes[static_cast<size_t>(child)].bg);
  }
  paint_node(0, 1, IRect{0, 0, frame.width, frame.height});
}

IRect Engine::swap_and_damage() {
  const IRect full{0, 0, frame.width, frame.height};
  IRect damage{};
  if (frame.width != previous.width || frame.height != previous.height || frame.clear != previous.clear ||
      frame.instances.size() != previous.instances.size() || frame.batches.size() != previous.batches.size() ||
      frame.shaders.size() != previous.shaders.size()) {
    damage = full;
  } else {
    for (size_t i = 0; i < frame.instances.size(); ++i) {
      const Instance &a = frame.instances[i], &b = previous.instances[i];
      if (std::memcmp(&a, &b, sizeof(Instance)) == 0) continue;
      const auto rect = [](const Instance &v) {
        return IRect{static_cast<int>(std::floor(v.x)), static_cast<int>(std::floor(v.y)), static_cast<int>(std::ceil(v.w)) + 1, static_cast<int>(std::ceil(v.h)) + 1};
      };
      damage = unite(damage, unite(rect(a), rect(b)));
    }
    for (size_t i = 0; i < frame.batches.size(); ++i) {
      const IRect &a = frame.batches[i].clip, &b = previous.batches[i].clip;
      if (a.x != b.x || a.y != b.y || a.w != b.w || a.h != b.h || frame.batches[i].count != previous.batches[i].count || !(frame.batches[i].round == previous.batches[i].round)) {
        damage = full;
        break;
      }
    }
    for (size_t i = 0; i < frame.shaders.size(); ++i) {
      const ShaderDraw &a = frame.shaders[i], &b = previous.shaders[i];
      if (a.hash != b.hash) {
        damage = full;
      } else if (a.uniforms != b.uniforms || a.opacity != b.opacity || a.rect.x != b.rect.x || a.rect.y != b.rect.y || a.rect.w != b.rect.w || a.rect.h != b.rect.h) {
        damage = unite(damage, unite(a.rect, b.rect));
      }
    }
    damage = intersect(damage, full);
  }
  std::swap(frame, previous);
  return damage;
}

std::string Engine::tree_json() const {
  std::string out = "[";
  bool first = true;
  const auto escape = [](std::string_view text) {
    std::string escaped;
    for (const char c : text) {
      if (c == '"' || c == '\\') {
        escaped.push_back('\\');
        escaped.push_back(c);
      } else if (static_cast<unsigned char>(c) < 0x20) {
        char buffer[8];
        std::snprintf(buffer, sizeof(buffer), "\\u%04x", c);
        escaped += buffer;
      } else {
        escaped.push_back(c);
      }
    }
    return escaped;
  };
  static const char *const kinds[] = {"box", "text", "button", "list", "shader", "input"};
  for (size_t i = 1; i < nodes.size(); ++i) {
    const Node &n = nodes[i];
    char buffer[256];
    std::snprintf(buffer, sizeof(buffer), "%s{\"id\":\"%016llx\",\"parent\":%d,\"kind\":\"%s\",\"x\":%.1f,\"y\":%.1f,\"w\":%.1f,\"h\":%.1f",
                  first ? "" : ",", static_cast<unsigned long long>(n.id), n.parent, kinds[n.kind < 6 ? n.kind : 0], n.x, n.y, n.w, n.h);
    out += buffer;
    if (n.placeholder_shown) out += ",\"placeholder\":\"" + escape(n.text) + "\"";
    else if (!n.text.empty()) out += ",\"text\":\"" + escape(n.text) + "\"";
    if (n.hit) out += ",\"hit\":true";
    if (n.gestures) out += ",\"gestures\":" + std::to_string(n.gestures);
    if (n.id == ui.focused) out += ",\"focused\":true";
    if (n.kind == KindInput) {
      if (const auto input = inputs.find(n.id); input != inputs.end()) {
        std::snprintf(buffer, sizeof(buffer), ",\"caret\":%zu,\"anchor\":%zu", input->second.caret, input->second.anchor);
        out += buffer;
      }
    }
    out += "}";
    first = false;
  }
  out += "]";
  return out;
}

}  // namespace qui
