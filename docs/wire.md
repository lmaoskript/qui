# Wire format

An app describes each frame as one flat byte stream. The native engine and the
browser DOM patcher both consume this stream, so a language only needs an
encoder to get every backend. Constants live in `spec.txt`.

All values are little-endian. `str` is a `u32` byte length followed by UTF-8.

## Frame stream

The stream starts with `Frame` and then holds nodes in pre-order. A node is
`Open`, its property ops, its children, then `End`.

| Op | Payload |
| --- | --- |
| `Frame` | `version:u8 theme:u8` |
| `Open` | `kind:u8 id:u64` |
| `End` | none |
| `Dir` | `u8` |
| `Width`, `Height` | `mode:u8 value:f32` |
| `MinW`, `MaxW`, `MinH`, `MaxH`, `Gap`, `Radius`, `Opacity`, `FontSize`, `Fps` | `f32` |
| `Pad` | `top:f32 right:f32 bottom:f32 left:f32` |
| `Align`, `Justify`, `Hit`, `Role`, `Weight` | `u8` |
| `Pos` | `x:f32 y:f32` |
| `Bg`, `Fg` | `color:u32` |
| `Border` | `color:u32 width:f32` |
| `Clip`, `Scroll`, `Mono`, `Ellipsis` | none |
| `Label`, `Text`, `Src` | `str` |
| `Virtual` | `count:u32 estimate:f32 bottom:u8` |
| `Item` | `index:u32` |
| `Uniform` | `name_len:u8 name n:u8 f32 * n` |

A color is `0xRRGGBBAA`. A theme token is `token << 24` with the low 24 bits
clear, which would otherwise be an invisible color.

## Results table

The backend answers each frame with a table the app reads while building the
next one, so widget calls never cross into the engine.

The header is 32 bytes: `magic:u32 capacity:u32 width:f32 height:f32 dpr:f32
dt:f32 pointer_x:f32 pointer_y:f32`. It is followed by `capacity` slots of 48
bytes, open-addressed with linear probing from `id & (capacity - 1)`. An empty
slot has `id == 0`.

| Offset | Field |
| --- | --- |
| 0 | `id:u64` |
| 8 | `flags:u32` |
| 12 | `first:u32` |
| 16 | `count:u32` |
| 20 | reserved `u32` |
| 24 | `x:f32 y:f32 w:f32 h:f32` |
| 40 | `dx:f32 dy:f32` |

`first` and `count` are the visible item range of a `List`.

## Layout

Layout is a strict subset of CSS flexbox so the DOM backend maps it directly.
Every box is `box-sizing: border-box` with `min-width: 0` and `min-height: 0`.

| Wire | CSS |
| --- | --- |
| `Column`, `Row` | `display: flex` with `flex-direction` |
| `Stack` | one-cell grid, every child in cell `1 / 1` |
| `Pos` | `position: absolute` with `left` and `top` |
| `Fit` on a row's width | `flex: 0 1 auto` |
| `Fit` elsewhere | `flex: 0 0 auto` |
| `Fill` on the main axis | `flex: <weight> 1 0` |
| `Fill` on the cross axis | `align-self: stretch` |
| `Px` | fixed size with `flex: 0 0 auto` |
| `Pct` | percentage size with `flex: 0 0 auto` |
