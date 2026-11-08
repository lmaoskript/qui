package main

import (
	"fmt"
	"strings"

	"github.com/torisetxd/qui"
)

type window struct {
	id, title, short string
	x, y, w, h       float32
	tx, ty, tw, th   float32
	gliding          bool
	open             bool
	shown, focus     float32
	z                int32
	delay            float32
	minW, minH       float32
	dot              qui.Color
	body             func()
}

func (w *window) moving() bool {
	return w.gliding || (w.open && w.shown < 0.995) || (!w.open && w.shown > 0.005)
}

func (a *app) setup() {
	add := func(id, title, short string, open bool, dot qui.Color, minW, minH float32, body func()) {
		a.windows = append(a.windows, &window{id: id, title: title, short: short, open: open, dot: dot, minW: minW, minH: minH, body: body})
	}
	add("lab", "Shader Lab", "Lab", true, qui.RGB(120, 190, 255), 420, 280, a.labWindow)
	add("rows", "One Million Rows", "Rows", true, qui.RGB(120, 225, 170), 360, 220, a.rowsWindow)
	add("monitor", "Monitor", "Stats", true, qui.RGB(250, 200, 110), 260, 200, a.monitorWindow)
	add("swarm", "Swarm", "Swarm", true, qui.RGB(240, 130, 200), 260, 200, a.swarmWindow)
	add("chat", "Chat", "Chat", true, qui.RGB(170, 150, 255), 280, 260, a.chatWindow)
	add("widgets", "Widgets", "UI", false, qui.RGB(150, 225, 235), 320, 300, a.widgetsWindow)
	add("capture", "Capture", "Cap", false, qui.RGB(255, 140, 120), 340, 300, a.captureWindow)
	for i, w := range a.windows {
		a.topZ++
		w.z = a.topZ
		w.delay = float32(i) * 0.07
		if a.skipIntro && w.open {
			w.shown = 1
		}
	}
	a.current = a.windows[0]
	a.lab.setup()
	a.rows.setup()
	a.chat.setup(a.ui.Wake)
	a.swarm.setup()
	a.widgets.setup()
	a.palette.commands = a.commands()
}

func (a *app) window(id string) *window {
	for _, w := range a.windows {
		if w.id == id {
			return w
		}
	}
	return nil
}

func (a *app) raise(w *window) {
	if w.z != a.topZ {
		a.topZ++
		w.z = a.topZ
	}
	a.current = w
}

func (a *app) openWindow(w *window) {
	w.open = true
	a.raise(w)
}

func (a *app) toggle(w *window) {
	if a.narrow {
		w.open = true
		a.current = w
		return
	}
	if w.open && a.current != w {
		a.raise(w)
		return
	}
	w.open = !w.open
	if w.open {
		a.raise(w)
	}
}

func (a *app) desktopBounds() (x, y, w, h float32) {
	return 0, topBarH, a.w, a.h - topBarH - dockH
}

func (a *app) glide(w *window, x, y, width, height float32, animate bool) {
	w.tx, w.ty, w.tw, w.th = x, y, width, height
	w.gliding = animate
	if !animate {
		w.x, w.y, w.w, w.h = x, y, width, height
	}
}

func (a *app) tile(animate bool) {
	_, top, width, height := a.desktopBounds()
	const margin, gap = 30, 14
	innerW := width - margin*2 - gap*2
	innerH := height - margin*2
	leftW := innerW * 0.46
	midW := innerW * 0.27
	rightW := innerW - leftW - midW
	midX := margin + leftW + gap
	rightX := midX + midW + gap
	labH := (innerH - gap) * 0.62
	monH := (innerH - gap) * 0.38
	y := top + margin

	a.glide(a.window("lab"), margin, y, leftW, labH, animate)
	a.glide(a.window("rows"), margin, y+labH+gap, leftW, innerH-labH-gap, animate)
	a.glide(a.window("monitor"), midX, y, midW, monH, animate)
	a.glide(a.window("swarm"), midX, y+monH+gap, midW, innerH-monH-gap, animate)
	a.glide(a.window("chat"), rightX, y, rightW, innerH, animate)
	a.glide(a.window("widgets"), midX-60, y+40, 440, innerH-60, animate)
	a.glide(a.window("capture"), margin+120, y+70, 620, innerH-110, animate)
}

func (a *app) handlePress() {
	down := a.ui.PointerDown()
	if down && !a.wasDown && !a.narrow {
		px, py := a.ui.PointerX(), a.ui.PointerY()
		var top *window
		for _, w := range a.windows {
			if w.open && px >= w.x && px < w.x+w.w && py >= w.y && py < w.y+w.h && (top == nil || w.z > top.z) {
				top = w
			}
		}
		if top != nil && py > topBarH && py < a.h-dockH {
			a.raise(top)
		}
	}
	a.wasDown = down
}

func (a *app) wallpaper() {
	px, py := float32(0.5), float32(0.5)
	if a.ui.PointerX() >= 0 && a.w > 0 && a.h > 0 {
		px, py = a.ui.PointerX()/a.w, a.ui.PointerY()/a.h
	}
	a.wallX = approach(a.wallX, px, 5, a.dt)
	a.wallY = approach(a.wallY, py, 5, a.dt)
	fps := float32(30)
	if a.calm {
		fps = 0
	}
	a.ui.Shader(qui.ShaderOptions{
		Src:   wallpaperShader,
		Style: qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: qui.Background, Key: "wallpaper"},
		Fps:   fps,
		Uniforms: []qui.Uniform{
			{Name: "u_c1", Values: a.wall[0:3]},
			{Name: "u_c2", Values: a.wall[3:6]},
			{Name: "u_c3", Values: a.wall[6:9]},
			{Name: "u_pointer", Values: []float32{a.wallX, a.wallY}},
			{Name: "u_energy", Values: []float32{a.energy}},
		},
	})
}

func (a *app) hero() {
	for _, w := range a.windows {
		if w.open {
			return
		}
	}
	ui := a.ui
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Align: qui.Center, Justify: qui.Center, Gap: 8, Key: "hero"}, func() {
		ui.Text("qui", qui.TextStyle{Size: 88, Bold: true, Color: qui.Strong, Role: qui.RoleHeading})
		ui.Text("One Go source. A native window or real DOM.", qui.TextStyle{Size: 17, Color: qui.Ink})
		ui.Text("Press Ctrl+K, or open something from the dock.", qui.TextStyle{Size: 13, Color: qui.Ink})
	})
}

func (a *app) drawWindows() {
	for _, w := range a.windows {
		target := float32(0)
		if w.open && a.clock > float64(w.delay) {
			target = 1
		}
		w.shown = approach(w.shown, target, 13, a.dt)
		if w.gliding {
			w.x = approach(w.x, w.tx, 12, a.dt)
			w.y = approach(w.y, w.ty, 12, a.dt)
			w.w = approach(w.w, w.tw, 12, a.dt)
			w.h = approach(w.h, w.th, 12, a.dt)
			if abs(w.x-w.tx)+abs(w.y-w.ty)+abs(w.w-w.tw)+abs(w.h-w.th) < 0.4 {
				w.x, w.y, w.w, w.h = w.tx, w.ty, w.tw, w.th
				w.gliding = false
			}
		}
		if !w.open && w.shown < 0.01 {
			continue
		}
		if a.narrow {
			if a.current == w {
				a.drawNarrow(w)
			}
			continue
		}
		a.drawFloating(w)
	}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func (a *app) drawNarrow(w *window) {
	ui := a.ui
	_, top, width, height := a.desktopBounds()
	ui.Column(qui.Style{Absolute: true, X: 0, Y: top, W: qui.Px(width), H: qui.Px(height), Bg: looks[a.look].glass, Clip: true, Hit: true, Key: w.id, Z: 10, Role: qui.RoleMain, Label: w.title}, func() {
		ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(36), Pad: qui.XY(14, 0), Align: qui.Center}, func() {
			ui.Text(w.title, qui.TextStyle{Size: 13, Bold: true, Color: qui.Strong, Role: qui.RoleHeading})
		})
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Clip: true, Key: "body"}, w.body)
	})
}

func (a *app) drawFloating(w *window) {
	ui := a.ui
	l := looks[a.look]
	lift := (1 - w.shown) * 16
	opacity := clamp(w.shown, 0.01, 1)
	active := a.current == w
	focus := float32(0)
	if active {
		focus = 1
	}
	w.focus = approach(w.focus, focus, 14, a.dt)
	glass := qui.Color(uint32(l.glass)&0xffffff00 | uint32(190+58*w.focus))

	ui.Stack(qui.Style{Absolute: true, X: w.x, Y: w.y + lift, W: qui.Px(w.w), H: qui.Px(w.h), Z: w.z, Opacity: opacity, Key: w.id}, func() {
		for i := 0; i < 4; i++ {
			s := float32(4 + i*5)
			ui.Column(qui.Style{Absolute: true, X: -s, Y: -s + 6, W: qui.Px(w.w + s*2), H: qui.Px(w.h + s*2), Bg: qui.RGBA(0, 0, 0, 20), Radius: 12 + s}, noop)
		}
		border := qui.Color(qui.RGBA(255, 255, 255, 22))
		if active {
			border = qui.RGBA(255, 255, 255, 46)
		}
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: glass, Border: border, Radius: 11, Clip: true, Hit: true, Key: "frame", Label: w.title}, func() {
			bar := ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(34), Pad: qui.Edges{Left: 13, Right: 6}, Align: qui.Center, Gap: 9, Bg: l.title, Hit: true, Key: "bar"}, func() {
				ui.Column(qui.Style{W: qui.Px(8), H: qui.Px(8), Bg: w.dot, Radius: 4}, noop)
				ui.Text(w.title, qui.TextStyle{Size: 12.5, Bold: true, Color: qui.Strong, W: qui.Fill(), Ellipsis: true, Role: qui.RoleHeading})
				if ui.Button("✕", qui.Style{W: qui.Px(24), H: qui.Px(24), Pad: qui.All(0), Bg: ghost, Fg: qui.Muted, Key: "close", Label: "Close " + w.title}).Clicked {
					w.open = false
				}
			})
			if bar.Dragging || bar.DragEnd {
				w.gliding = false
				a.touched = true
				_, top, width, height := a.desktopBounds()
				w.x = clamp(w.x+bar.DX, 60-w.w, width-60)
				w.y = clamp(w.y+bar.DY, top, top+height-34)
			}
			ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Clip: true, Key: "body"}, w.body)
		})
		grip := ui.Column(qui.Style{Absolute: true, X: w.w - 16, Y: w.h - 16, W: qui.Px(16), H: qui.Px(16), Hit: true, Key: "grip", Align: qui.End, Justify: qui.End, Pad: qui.All(3)}, func() {
			ui.Column(qui.Style{W: qui.Px(6), H: qui.Px(6), Border: qui.Muted, BorderW: 1, Radius: 2}, noop)
		})
		if grip.Dragging || grip.DragEnd {
			w.gliding = false
			a.touched = true
			w.w = clamp(w.w+grip.DX, w.minW, a.w)
			w.h = clamp(w.h+grip.DY, w.minH, a.h)
		}
	})
}

func (a *app) topBar() {
	ui := a.ui
	l := looks[a.look]
	stats := ui.Stats()
	ui.Row(qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Fill(), H: qui.Px(topBarH), Pad: qui.XY(16, 0), Align: qui.Center, Gap: 14, Bg: l.glass, Z: 9000, Hit: true, Key: "topbar", Role: qui.RoleNav}, func() {
		ui.Text("qui", qui.TextStyle{Size: 16, Bold: true, Color: qui.Strong})
		if !a.narrow {
			ui.Text(platformName(), qui.TextStyle{Size: 12, Color: qui.Muted})
		}
		ui.Row(qui.Style{W: qui.Fill()}, noop)
		if !a.narrow {
			ui.Text(fmt.Sprintf("%.0f fps", a.fps), qui.TextStyle{Size: 12, Mono: true, Color: qui.Ink})
			ui.Text(fmt.Sprintf("%.2f ms", a.buildMs+stats.EngineMs), qui.TextStyle{Size: 12, Mono: true, Color: qui.Ink})
			ui.Text(group(stats.Nodes)+" nodes", qui.TextStyle{Size: 12, Mono: true, Color: qui.Ink})
		}
		ui.Row(qui.Style{Gap: 7, Align: qui.Center}, func() {
			for i := range looks {
				a.swatch(i)
			}
		})
		label := "Calm"
		if a.calm {
			label = "Animate"
		}
		if ui.Button(label, qui.Style{H: qui.Px(26), Pad: qui.XY(10, 0), Bg: ghost, Fg: qui.Muted, Key: "calm"}).Clicked {
			a.calm = !a.calm
		}
		if ui.Button("Ctrl K", qui.Style{H: qui.Px(26), Pad: qui.XY(10, 0), Key: "search"}).Clicked {
			a.openPalette()
		}
	})
}

func (a *app) swatch(i int) {
	ui := a.ui
	border := qui.Color(0)
	if a.look == i {
		border = qui.Strong
	}
	r := ui.Column(qui.Style{W: qui.Px(16), H: qui.Px(16), Bg: looks[i].swatch, Radius: 8, Border: border, BorderW: 2, Hit: true, Key: "swatch" + looks[i].name, Label: looks[i].name + " theme"}, noop)
	if r.Clicked {
		a.applyLook(i)
	}
}

func (a *app) dock() {
	ui := a.ui
	l := looks[a.look]
	ui.Row(qui.Style{Absolute: true, X: 0, Y: a.h - dockH + 8, W: qui.Fill(), H: qui.Px(50), Justify: qui.Center, Z: 9000, Key: "dockrow"}, func() {
		ui.Row(qui.Style{H: qui.Px(50), Pad: qui.XY(8, 0), Gap: 4, Align: qui.Center, Bg: l.glass, Border: qui.RGBA(255, 255, 255, 22), Radius: 14, Hit: true, Key: "dock", Role: qui.RoleNav}, func() {
			for _, w := range a.windows {
				a.dockItem(w)
			}
		})
	})
}

func (a *app) dockItem(w *window) {
	ui := a.ui
	label := w.title
	if a.narrow || strings.Contains(label, "Million") {
		label = w.short
	}
	var clicked bool
	ui.Column(qui.Style{Align: qui.Center, Gap: 3, Key: "item" + w.id}, func() {
		bg := ghost
		fg := qui.Color(qui.Muted)
		if w.open {
			fg = qui.Ink
		}
		if a.current == w && w.open {
			bg = qui.Item
			fg = qui.Strong
		}
		pad := qui.XY(12, 0)
		if a.narrow {
			pad = qui.XY(7, 0)
		}
		clicked = ui.Button(label, qui.Style{H: qui.Px(32), Pad: pad, Bg: bg, Fg: fg, Radius: 8, Key: "button", Label: w.title}).Clicked
		dot := qui.Color(ghost)
		if w.open {
			dot = w.dot
		}
		ui.Column(qui.Style{W: qui.Px(14), H: qui.Px(2), Radius: 1, Bg: dot}, noop)
	})
	if clicked {
		a.toggle(w)
	}
}

func (a *app) handleKeys() {
	if a.ui.KeyPressed('k', qui.ModCtrl) {
		if a.palette.open {
			a.palette.open = false
		} else {
			a.openPalette()
		}
	}
}

type command struct {
	label string
	run   func()
}

type paletteState struct {
	open     bool
	query    string
	selected int
	commands []command
	matches  []int
	lastX    float32
	lastY    float32
}

func (a *app) commands() []command {
	var out []command
	for _, w := range a.windows {
		w := w
		out = append(out, command{"Open " + w.title, func() { a.openWindow(w) }})
	}
	for i := range looks {
		i := i
		out = append(out, command{"Theme: " + looks[i].name, func() { a.applyLook(i) }})
	}
	out = append(out,
		command{"Tile windows", func() {
			for _, w := range a.windows[:5] {
				w.open = true
			}
			a.tile(true)
		}},
		command{"Close all windows", func() {
			for _, w := range a.windows {
				w.open = false
			}
		}},
		command{"Toggle calm mode", func() { a.calm = !a.calm }},
		command{"Clear chat", a.chat.clear},
	)
	return out
}

func (a *app) openPalette() {
	a.palette.open = true
	a.palette.query = ""
	a.palette.selected = 0
}

func (a *app) drawPalette() {
	p := &a.palette
	if !p.open {
		return
	}
	ui := a.ui
	l := looks[a.look]
	if ui.KeyPressed(qui.KeyEscape) {
		p.open = false
		return
	}
	query := strings.ToLower(p.query)
	p.matches = p.matches[:0]
	for i, c := range p.commands {
		if strings.Contains(strings.ToLower(c.label), query) {
			p.matches = append(p.matches, i)
		}
	}
	if ui.KeyPressed(qui.KeyDown) {
		p.selected++
	}
	if ui.KeyPressed(qui.KeyUp) {
		p.selected--
	}
	p.selected = max(0, min(p.selected, len(p.matches)-1))

	moved := ui.PointerX() != p.lastX || ui.PointerY() != p.lastY
	p.lastX, p.lastY = ui.PointerX(), ui.PointerY()
	width := min(a.w-32, 540)
	scrim := ui.Stack(qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Fill(), H: qui.Fill(), Bg: qui.Scrim, Hit: true, Z: 9500, Key: "scrim"}, func() {
		ui.Column(qui.Style{Absolute: true, X: (a.w - width) / 2, Y: a.h * 0.14, W: qui.Px(width), MaxH: 420, Pad: qui.All(8), Gap: 6, Bg: l.glass, Border: qui.RGBA(255, 255, 255, 40), Radius: 13, Clip: true, Hit: true, Key: "panel", Label: "Command palette"}, func() {
			field := ui.Input("query", &p.query, qui.InputOptions{Style: qui.Style{W: qui.Fill(), Bg: ghost, Border: ghost}, Placeholder: "Type a command", Size: 16, Focus: true})
			ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(1), Bg: qui.Line}, noop)
			run := -1
			if field.Submitted && len(p.matches) > 0 {
				run = p.matches[p.selected]
			}
			shown := min(len(p.matches), 9)
			for i := 0; i < shown; i++ {
				command := p.commands[p.matches[i]]
				bg := ghost
				if i == p.selected {
					bg = qui.Item
				}
				row := ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(32), Pad: qui.XY(10, 0), Align: qui.Center, Radius: 7, Bg: bg, Hit: true, Key: command.label}, func() {
					ui.Text(command.label, qui.TextStyle{W: qui.Fill(), Color: qui.Ink})
				})
				if row.Hovered && moved {
					p.selected = i
				}
				if row.Clicked {
					run = p.matches[i]
				}
			}
			if len(p.matches) == 0 {
				ui.Text("Nothing matches.", qui.TextStyle{Size: 13, Color: qui.Muted})
			}
			if run >= 0 {
				p.open = false
				p.commands[run].run()
			}
		})
	})
	if scrim.Clicked {
		p.open = false
	}
}
