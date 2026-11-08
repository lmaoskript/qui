package main

import (
	"fmt"

	"github.com/torisetxd/qui"
)

var logos = []string{"TESSEL", "Oakline", "mirador", "VANTABLUE", "Kestrel/Co", "paperweight", "DRIFTWOOD", "Halcyon"}

func (a *app) intro(i int, gap float32, body func()) {
	e := easeOut((a.t - 0.15 - float32(i)*0.11) / 0.9)
	a.ui.Column(qui.Style{Align: qui.Center, Opacity: max(0.001, e), Pad: qui.Edges{Top: (1 - e) * 16, Bottom: gap + e*16}}, body)
}

func (a *app) hero() {
	ui := a.ui
	height := clamp(a.h, 700, 1040)
	size := min(clamp(a.w*0.067, 40, 96), (a.w-40)/8.6)
	track := -size * 0.042
	line := size * 1.12
	yaw := float32(0.1) + a.t*0.03
	top := 64 + clamp(height*0.09, 28, 110)
	ui.Stack(qui.Style{Key: "hero", W: qui.Fill(), H: qui.Px(height), Clip: true}, func() {
		ui.Shader(qui.ShaderOptions{
			Src: heroShader, Style: qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: black, Key: "sky"}, Fps: a.fps("top", height),
			Uniforms: []qui.Uniform{{Name: "u_dpr", Values: []float32{a.dpr}}, {Name: "u_yaw", Values: []float32{yaw}}},
		})
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Align: qui.Center, Pad: qui.Edges{Top: top}}, func() {
			a.intro(0, 12, func() {
				ui.Row(qui.Style{Gap: 9, Align: qui.Center, Pad: qui.XY(6, 5), Radius: 999, Border: hairHot, Bg: qui.RGBA(255, 255, 255, 10)}, func() {
					ui.Row(qui.Style{Pad: qui.XY(8, 2), Radius: 999, Gradient: qui.Gradient{From: orange, To: magenta, Angle: 90}}, func() {
						ui.Text("NEW", qui.TextStyle{Size: 10, Mono: true, Bold: true, Color: black, Tracking: 0.5})
					})
					ui.Text("Edge runtime now boots in 4 ms", qui.TextStyle{Size: 13, Color: ink})
					ui.Text("→", qui.TextStyle{Size: 13, Color: muted})
					ui.Column(qui.Style{W: qui.Px(4)}, func() {})
				})
			})
			a.intro(1, 0, func() {
				ui.Stack(qui.Style{H: qui.Px(line)}, func() {
					ui.Text("Push once.", qui.TextStyle{Size: size, Bold: true, Color: ink, Tracking: track, Role: qui.RoleHeading})
				})
			})
			a.intro(2, 16, func() {
				ui.Row(qui.Style{H: qui.Px(line)}, func() {
					ui.Text("Live ", qui.TextStyle{Size: size, Bold: true, Color: ink, Tracking: track})
					ui.Text("everywhere.", qui.TextStyle{Size: size, Bold: true, Tracking: track, Gradient: qui.Gradient{From: qui.RGB(255, 176, 64), To: qui.RGB(255, 52, 130), Angle: 90}})
				})
			})
			a.intro(3, 14, func() {
				ui.Text("Sable turns a git push into a global deployment: rendered at the edge, protected at the perimeter, reversible in one click.",
					qui.TextStyle{Size: 18, Color: muted, W: qui.Px(min(640, a.w-48)), Align: qui.Center})
			})
			a.intro(4, 0, func() {
				ui.Row(qui.Style{Gap: 10, Align: qui.Center}, func() {
					if a.primary("Start deploying").Clicked {
						a.goTo("top-cta")
					}
					if a.secondary("See the network").Clicked {
						a.goTo("network")
					}
					if !a.narrow {
						ui.Row(qui.Style{Pad: qui.XY(14, 10), Radius: 8, Border: hair, Bg: qui.RGBA(0, 0, 0, 120), Gap: 8}, func() {
							ui.Text("$", qui.TextStyle{Size: 13, Mono: true, Color: subtle})
							ui.Text("npx sable init", qui.TextStyle{Size: 13, Mono: true, Color: ink})
						})
					}
				})
			})
		})
		a.marquee(height)
	})
}

func (a *app) marquee(height float32) {
	ui := a.ui
	const each = 176
	span := float32(len(logos)) * each
	offset := float32(0)
	if span > 0 {
		offset = a.t * 22
		for offset >= span {
			offset -= span
		}
	}
	ui.Stack(qui.Style{Absolute: true, X: 0, Y: height - 70, W: qui.Fill(), H: qui.Px(44), Clip: true}, func() {
		for copy := 0; copy < 3; copy++ {
			for i, name := range logos {
				x := float32(copy)*span + float32(i)*each - offset
				edge := min(x+each/2, a.w-x-each/2)
				shade := clamp(edge/200, 0, 1)
				if shade <= 0 {
					continue
				}
				ui.Row(qui.Style{Key: fmt.Sprint(copy, ":", i), Absolute: true, X: x, Y: 10, W: qui.Px(each), Justify: qui.Center, Opacity: shade}, func() {
					ui.Text(name, qui.TextStyle{Size: 17, Bold: i%2 == 0, Color: qui.RGBA(255, 255, 255, 150), Tracking: -0.3})
				})
			}
		}
	})
}

func (a *app) stats() {
	ui := a.ui
	type stat struct {
		value func(p float32) string
		text  string
	}
	items := []stat{
		{func(p float32) string { return fmt.Sprintf("%d+", int(310*p)) }, "cities on the network"},
		{func(p float32) string { return fmt.Sprintf("%.0f ms", 40-36*p) }, "median cold start"},
		{func(p float32) string { return fmt.Sprintf("%d Tbps", int(42*p)) }, "mitigation capacity"},
		{func(p float32) string { return fmt.Sprintf("%.3f%%", 90+9.999*p) }, "uptime, trailing year"},
	}
	cell := func(i int) {
		a.reveal(fmt.Sprint("stat", i), qui.Style{W: qui.Fill(), Pad: qui.XY(a.pad(), 16), Gap: 6}, func() {
			ui.Text(items[i].value(a.revealed(fmt.Sprint("stat", i))), qui.TextStyle{Size: clamp(a.w*0.032, 26, 40), Color: ink, Tracking: -1.2})
			a.label(items[i].text)
		})
	}
	divider := func() { ui.Column(qui.Style{W: qui.Px(1), H: qui.Fill(), Bg: hair}, func() {}) }
	a.section("stats", func() {
		if a.narrow {
			ui.Row(qui.Style{W: qui.Fill()}, func() { cell(0); divider(); cell(1) })
			ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(1), Bg: hair}, func() {})
			ui.Row(qui.Style{W: qui.Fill()}, func() { cell(2); divider(); cell(3) })
			return
		}
		ui.Row(qui.Style{W: qui.Fill()}, func() {
			cell(0)
			divider()
			cell(1)
			divider()
			cell(2)
			divider()
			cell(3)
		})
	})
}

func (a *app) pad() float32 {
	if a.narrow {
		return 20
	}
	return 40
}
