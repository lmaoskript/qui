---
name: qui
description: Build, change or verify UIs with qui, the in-house immediate-mode framework (Go or C++; native through OpenGL ES or a CPU rasterizer, browser through real DOM elements and Go wasm). Use when a project imports github.com/torisetxd/qui or includes qui.hpp, or when asked to add a screen, widget, list or shader to a qui app.
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

`Style` fields, in order: `W H MinW MaxW MinH MaxH Pad Gap Align Justify Absolute X Y Bg Fg Border BorderW Radius Opacity Clip Scroll Hit Role Label Key`

`TextStyle` fields, in order: `Size Bold Mono Ellipsis Color W Align Role`. Presets: `Body H1 H2 Small Code`.

`Response` fields: `Found Hovered Pressed Clicked RightClicked Focused DragStart Dragging DragEnd X Y W H DX DY First Count`

Frame info: `Width Height Dpr Dt PointerX PointerY`.

## Color and semantics

Prefer theme tokens, which track `ui.SetTheme(qui.Midnight)` (`Slate` default, `Midnight`, `Warm`, `Neutral`):

`Background Surface Raised Inset Ink Muted Strong Accent Accent2 Hover Active Item Line Danger Selection Scrollbar ScrollbarHot Scrim Shadow`

Literals: `qui.RGB(r, g, b)`, `qui.RGBA(r, g, b, a)`; C++ `rgba(r, g, b, a)`.

On the web, `Role` picks the element (`RoleHeading` h2, `RoleLink` a, `RoleMain` main, `RoleNav` nav, `RoleImage` role=img) and `Label` sets the accessible name.

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

Arguments are key, row count, estimated row height, options, row body. The body runs only for visible rows and heights may vary. The list needs a bounded height. `Bottom: true` starts at the end and stays pinned there, for chat logs.

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

`Src` is GLSL ES 3.00 without `#version` or `precision`, and must define `vec4 shade(vec2 uv)`, with uv in 0..1 from the top-left. `u_time` and `u_resolution` are already declared; declare your own uniforms. `Fps` repaints without running your loop. The CPU backend does not run shaders and paints `Bg` instead, so always set one.

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
  await start({ wasm: "app.wasm", root: document.body });
</script>
```

On Linux, `SetAppID` must equal the app's `.desktop` basename or the compositor shows the wrong icon and name.

## Verify without a window

Any of the first three variables runs the app offscreen for a few frames and exits. Inspect the PNG or JSON rather than assuming the UI is right.

| Variable | Effect |
| --- | --- |
| `QUI_SHOT=a.png` | Write a screenshot |
| `QUI_TREE=a.json` | Dump every node: `id parent kind x y w h text hit` |
| `QUI_WIRE=a.bin` | Dump the frame's wire bytes |
| `QUI_SIZE=1100x700` | Viewport size |
| `QUI_DPR=2` | Pixel ratio |
| `QUI_BACKEND=cpu` or `gl` | Force a renderer |
| `QUI_FRAMES=n` | Minimum frames before exiting |
| `QUI_DRIVE='click:58,231;wheel:600,400,-40'` | Scripted input in logical pixels: `click:x,y`, `rclick:x,y`, `move:x,y`, `wheel:x,y,amount` (negative scrolls down) |

## Not available yet

Do not assume these exist: text input and keyboard events, images, touch gestures, Android builds, Windows. Native focus comes only from clicks. On the web, `Response` has no drag flags or `X Y W H`, and list spacers use the estimate, so the scrollbar can shift when real heights differ.

To add a node kind, op or token, edit `spec.txt` and run `go run ./tools/gen`; `docs/wire.md` documents the format.
