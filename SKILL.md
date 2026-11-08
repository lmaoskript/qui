---
name: qui
description: Build, change or verify UIs with qui, the in-house immediate-mode framework (Go or C++; native through OpenGL ES or a CPU rasterizer, browser through real DOM elements and Go wasm), and its qui/sys module for Linux app install, background services and screen or microphone capture. Use when a project imports github.com/torisetxd/qui or includes qui.hpp, or when asked to add a screen, widget, list or shader to a qui app, or to install a native app or capture the screen or mic with it.
---

# qui

Immediate-mode API over retained internals. Rebuild the whole UI every frame from your own state: no widget objects, callbacks or bindings. `Frame()` blocks until something changes, so an idle app uses no CPU.

## Minimal app

Go. The same source builds for native and wasm.

```go
package main

import (
	"strconv"

	"github.com/torisetxd/qui"
)

func main() {
	qui.SetAppID("dev.example.counter")
	ui := qui.Open("Counter", 480, 320)
	defer ui.Close()
	count := 0
	for ui.Frame() {
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.All(16), Gap: 8}, func() {
			ui.Text("Counter", qui.H1)
			ui.Text(strconv.Itoa(count))
			if ui.Button("Add").Clicked {
				count++
			}
		})
	}
}
```

C++. Names are snake_case; everything else matches.

```cpp
#include "qui.hpp"
#include <string>
using namespace qui;

int main() {
  set_app_id("dev.example.counter");
  Ui ui("Counter", 480, 320);
  int count = 0;
  while (ui.frame()) {
    ui.column({.w = fill(), .h = fill(), .pad = 16, .gap = 8}, [&] {
      ui.text("Counter", H1);
      ui.text(std::to_string(count));
      if (ui.button("Add").clicked) ++count;
    });
  }
}
```

## Rules that bite

- **Responses come from the previous frame.** `Clicked` is true exactly once, on the frame after the input. The engine schedules that frame for you.
- **Never block the loop.** Do I/O on a goroutine or thread, guard the shared state yourself, then call `ui.Wake()` (safe from any thread) to request a frame.
- **Go: call everything from the main goroutine**, except `Wake`.
- **Identity is parent id + `Key`, or sibling index without one.** Buttons key on their label, so same-label siblings need `Key`. Give `Key` to scrollers, lists and `Hit` boxes whose earlier siblings come and go, or their state moves to another node.
- **Boxes report nothing without `Hit: true`.** Buttons and lists always report.
- **Zero means unset** in `Style` and `TextStyle`: `Opacity: 0` is opaque, `Bg: 0` paints nothing.
- **C++ designated initializers must follow declaration order** (the field list below).
- **Wrap the root in `W: Fill(), H: Fill()`**; the default is fit-to-content.

## Layout

A strict flexbox subset that maps directly to CSS.

- `Column`, `Row`, and `Stack` (children overlap). `Absolute: true, X, Y` positions relative to the parent.
- Sizes: fit by default, `Px(n)`, `Fill()`, `FillN(weight)`, `Pct(n)`.
- Padding: Go `All(n)`, `XY(x, y)`, `Edges{Top, Right, Bottom, Left}`; C++ `16`, `{v, h}`, `{t, r, b, l}`.
- `Align` is the cross axis (`Start Center End Stretch`), `Justify` the main axis (`Start Center End Between`).
- Long text inside a `Row` needs `W: Fill()` to wrap instead of overflow.
- `Scroll: true` scrolls vertically and needs a bounded height. Use `List` for long content.
- `Z` sets paint and hit order among siblings. Use it to raise overlapping panels: unlike reordering children, it keeps their scroll position and focus on the web. Give overlapping panels `Hit: true` so the pointer and wheel stop at them.

- `Gradient: qui.Gradient{From, To, Angle}` fills a box; on `TextStyle` it colors the letters across the text box (keep such text fit-width). Angles are CSS degrees. `Shadow: qui.BoxShadow{Color, Blur, Y}` paints outside the box, so a bright color with no `Y` is a glow. `TextStyle.Tracking` is letter spacing in px; large headings look best around `-0.04 * Size`.
- `ScrollTo: &offset` sets a `Scroll` box's offset for that frame. Animate it yourself for smooth anchors, and stop sending it when done so the user can scroll. A zero-height `Hit` box inside the scroller reports its viewport-relative `Y` even offscreen, which gives you scroll position, reveal-on-scroll and scroll-spy. Web scrolling does not wake the loop, so run a ticker calling `ui.Wake()` for those.

`Style` fields, in order: `W H MinW MaxW MinH MaxH Pad Gap Align Justify Absolute X Y Z Bg Fg Border BorderW Radius Opacity Gradient Shadow Clip Scroll Hit ScrollTo Role Label Key`

`TextStyle` fields, in order: `Size Bold Mono Ellipsis Color W Align Role Gradient Tracking`. Presets: `Body H1 H2 Small Code`.

`Response` fields: `Found Hovered Pressed Clicked RightClicked Focused DragStart Dragging DragEnd X Y W H DX DY First Count Error`

Frame info: `Width Height Dpr Dt PointerX PointerY PointerDown`. `Stats()` gives `Nodes WireBytes EngineMs` for the last frame.

## Color and semantics

Prefer theme tokens, which track `ui.SetTheme(qui.Midnight)` (`Slate` default, `Midnight`, `Warm`, `Neutral`):

`Background Surface Raised Inset Ink Muted Strong Accent Accent2 Hover Active Item Line Danger Selection Scrollbar ScrollbarHot Scrim Shadow`

Literals: `qui.RGB(r, g, b)`, `qui.RGBA(r, g, b, a)`; C++ `rgba(r, g, b, a)`.

On the web, `Role` picks the element (`RoleHeading` h2, `RoleLink` a, `RoleMain` main, `RoleNav` nav, `RoleImage` role=img) and `Label` sets the accessible name.

## Text input and keys

```go
package main

import "github.com/torisetxd/qui"

func main() {
	ui := qui.Open("Chat", 480, 320)
	defer ui.Close()
	var draft, last string
	for ui.Frame() {
		if ui.KeyPressed('k', qui.ModCtrl) {
			draft = ""
		}
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.All(16), Gap: 8}, func() {
			ui.Text(last)
			field := ui.Input("draft", &draft, qui.InputOptions{
				Style:        qui.Style{W: qui.Fill(), MaxH: 120},
				Placeholder:  "Message",
				Multiline:    true,
				EnterSubmits: true,
				Autofocus:    true,
			})
			if field.Submitted {
				last, draft = draft, ""
			}
		})
	}
}
```

`Input` takes a stable key and a pointer to your string, which it updates when `Changed` is true. Assigning to the string yourself replaces the field and moves the caret to the end. Options: `Style Placeholder Size Mono Multiline EnterSubmits Password Autofocus Focus` (`Focus` takes focus on every frame it is true); C++ `input(key, value, {.style, .placeholder, .size, .mono, .multiline, .enter_submits, .password, .autofocus, .focus})`.

Enter submits a single-line field. In a multiline field Enter adds a newline and Ctrl+Enter submits, unless `EnterSubmits` is set, where Enter submits and Shift+Enter adds a newline. Multiline fields grow with their text up to `MaxH`, then scroll. Native fields handle caret movement, selection, clipboard, undo, IME composition and Linux primary selection; the web uses real `<input>` and `<textarea>`.

`ui.KeyPressed(key, mods...)` (C++ `ui.key_pressed(key, mods)`) needs the exact modifiers. Keys are `KeyEnter KeyEscape KeyBackspace KeyDelete KeySpace KeyLeft KeyRight KeyUp KeyDown KeyHome KeyEnd KeyPageUp KeyPageDown KeyF1..KeyF12` or a lowercase rune. Mods are `ModShift ModCtrl ModAlt ModMeta`. A focused field keeps the keys it uses, Tab moves focus, and Enter or Space clicks a focused button.

## Virtual list

```go
package main

import (
	"strconv"

	"github.com/torisetxd/qui"
)

func main() {
	ui := qui.Open("List", 480, 640)
	defer ui.Close()
	for ui.Frame() {
		ui.List("rows", 100000, 34, qui.ListOptions{Style: qui.Style{W: qui.Fill(), H: qui.Fill()}}, func(i int) {
			ui.Text("Row "+strconv.Itoa(i), qui.TextStyle{W: qui.Fill()})
		})
	}
}
```

Arguments are key, row count, estimated row height, options, row body. The body runs only for visible rows and heights may vary. The list needs a bounded height. `Bottom: true` starts at the end and stays pinned there, and a short list sits at the bottom, for chat logs.

## Shaders

```go
package main

import "github.com/torisetxd/qui"

const glow = `uniform float u_speed;
vec4 shade(vec2 uv) {
  return vec4(uv, 0.5 + 0.5 * sin(u_time * u_speed), 1.0);
}`

func main() {
	ui := qui.Open("Shader", 480, 320)
	defer ui.Close()
	for ui.Frame() {
		ui.Shader(qui.ShaderOptions{
			Src:      glow,
			Style:    qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: qui.Raised},
			Fps:      24,
			Uniforms: []qui.Uniform{{Name: "u_speed", Values: []float32{1}}},
		})
	}
}
```

`Src` is GLSL ES 3.00 without `#version` or `precision`, and must define `vec4 shade(vec2 uv)`, with uv in 0..1 from the top-left. `u_time` and `u_resolution` are already declared; declare your own uniforms. `Fps` repaints without running your loop. The CPU backend does not run shaders and paints `Bg` instead, so always set one. A shader that fails to compile also paints `Bg`, and `ui.Shader` returns the compiler log in `Response.Error`, with line numbers relative to `Src`.

## Build

Native needs `sdl3 freetype2 harfbuzz fontconfig` through pkg-config.

- Go: `go build .`, with `replace github.com/torisetxd/qui => ../qui` in `go.mod`.
- C++: `add_subdirectory(qui)` and `target_link_libraries(app PRIVATE qui)`.
- Web: `GOOS=js GOARCH=wasm go build -o app.wasm .`, then serve it next to `web/qui.js`, `web/wire.gen.js` and `$(go env GOROOT)/lib/wasm/wasm_exec.js`:

```html
<!doctype html>
<style>html, body { height: 100%; margin: 0 }</style>
<script src="wasm_exec.js"></script>
<script type="module">
  import { start } from "./qui.js";
  await start({ wasm: "app.wasm", root: document.body, preventKeys: ["ctrl+k"] });
</script>
```

On Linux, `SetAppID` must equal the app's `.desktop` basename or the compositor shows the wrong icon and name. `sys.Install` writes that file.

## Linux install, services and capture

`github.com/torisetxd/qui/sys` does not link the UI, so daemons can use it. It needs `gio-2.0 gio-unix-2.0 libpipewire-0.3`. C++ uses `sys/qui_sys.h` and `sys/qui_sys.hpp`, and links the `qui_sys` target.

```go
package main

import (
	"errors"
	"log"
	"time"

	"github.com/torisetxd/qui/sys"
)

func main() {
	const id = "dev.example.recorder"
	app := sys.App{ID: id, Name: "Recorder", Service: &sys.Service{Args: "--background", NeedsPipeWire: true}}
	if err := sys.Install(app); err != nil {
		log.Fatal(err)
	}
	screen, err := sys.OpenScreen(sys.ScreenOptions{AppID: id, FPS: 30, ShowCursor: true})
	if err != nil {
		log.Fatal(err)
	}
	defer screen.Close()
	for {
		frame, err := screen.Next(0, time.Second)
		if errors.Is(err, sys.ErrTimeout) {
			continue
		}
		if err != nil {
			return
		}
		log.Println(frame.Width, frame.Height, frame.Sequence)
	}
}
```

- **Use one id everywhere**: `qui.SetAppID`, `App.ID` and `ScreenOptions.AppID`. It becomes the `.desktop` name, portal identity, unit `app-<id>.service` and `$XDG_STATE_HOME/<id>`.
- **`Install` before the first `OpenScreen`.** It is safe on every launch and leaves a service the user disabled alone. `Exec` defaults to the running binary and must be absolute; `InstallAs` copies the binary to `~/.local/bin`.
- `OpenScreen` shows the portal dialog once and saves the permission; later runs restore it. Use `Slot` to hold several. `Multiple` and `Windows` widen the picker, `NoPersist` keeps nothing.
- `Next(stream, timeout)` returns the newest BGRx frame, valid until the next `Next` on that stream. Use one goroutine per stream. Retry on `ErrTimeout`; `ErrClosed` means sharing stopped.
- `sys.OpenAudio(sys.AudioOptions{})` captures the microphone, `System: true` the default output, `Target` a PipeWire node. `Read(buf, timeout)` fills interleaved `float32` and returns frames and the monotonic time of the first.
- Also: `Uninstall StartService StopService RestartService EnableService DisableService ServiceActive Register`.

## Verify without a window

Any of the first three variables runs the app offscreen for a few frames and exits. Inspect the PNG or JSON rather than assuming the UI is right.

| Variable | Effect |
| --- | --- |
| `QUI_SHOT=a.png` | Write a screenshot |
| `QUI_TREE=a.json` | Dump every node: `id parent kind x y w h text placeholder hit focused caret anchor` |
| `QUI_WIRE=a.bin` | Dump the frame's wire bytes |
| `QUI_SIZE=1100x700` | Viewport size |
| `QUI_DPR=2` | Pixel ratio |
| `QUI_BACKEND=cpu` or `gl` | Force a renderer |
| `QUI_FRAMES=n` | Minimum frames before exiting |
| `QUI_DRIVE='click:58,231;type:hello;key:ctrl+a'` | Scripted input, `;` separated, in logical pixels: `click:x,y`, `dblclick:x,y`, `rclick:x,y`, `drag:x0,y0,x1,y1`, `down:x,y`, `up:x,y`, `move:x,y`, `wheel:x,y,amount` (negative scrolls down), `type:text`, `key:shift+left`, `ime:text` |

## Not available yet

Do not assume these exist: images, touch gestures, Android builds, Windows, right-to-left paragraphs (mixed text orders correctly, but the base direction is always left-to-right), global shortcuts. On the web, list spacers use the estimate, so the scrollbar can shift when real heights differ.

To add a node kind, op or token, edit `spec.txt` and run `go run ./tools/gen`; `docs/wire.md` documents the format.
