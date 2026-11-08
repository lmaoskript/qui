package main

import "github.com/torisetxd/qui"

func (a *app) logo(size float32) {
	ui := a.ui
	ui.Stack(qui.Style{W: qui.Px(size), H: qui.Px(size), Radius: size * 0.26, Clip: true, Gradient: qui.Gradient{From: orange, To: violet, Angle: 135}}, func() {
		ui.Column(qui.Style{Absolute: true, X: size * 0.42, Y: size * 0.42, W: qui.Px(size), H: qui.Px(size), Radius: size * 0.26, Bg: black}, func() {})
	})
}

func (a *app) navLink(label, target string) {
	color := muted
	if a.spy == target || a.hover[label] {
		color = ink
	}
	if a.button(label, qui.Style{Fg: color, Radius: 6, Pad: qui.XY(10, 6), Bg: qui.RGBA(0, 0, 0, 1)}, qui.RGBA(0, 0, 0, 1)).Clicked {
		a.goTo(target)
	}
}

func (a *app) nav() {
	ui := a.ui
	solid := clamp((a.scroll-12)/80, 0, 1)
	ui.Column(qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Fill(), Z: 10, Bg: alpha(black, solid*0.82), Role: qui.RoleNav}, func() {
		ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(64), Justify: qui.Center, Align: qui.Center}, func() {
			ui.Row(qui.Style{W: qui.Px(a.inner), Align: qui.Center, Gap: 8}, func() {
				ui.Row(qui.Style{Gap: 10, Align: qui.Center, Pad: qui.Edges{Right: 20}}, func() {
					a.logo(18)
					ui.Text("Sable", qui.TextStyle{Size: 16, Bold: true, Color: ink, Tracking: -0.4})
				})
				if !a.narrow {
					a.navLink("Platform", "platform")
					a.navLink("Network", "network")
					a.navLink("Pricing", "pricing")
				}
				a.spacer()
				if !a.narrow {
					a.navLink("Sign in", "top-cta")
				}
				if a.button("Deploy", qui.Style{Bg: white, Fg: black, Radius: 7, Pad: qui.XY(14, 7)}, qui.RGB(232, 232, 232)).Clicked {
					a.goTo("top-cta")
				}
			})
		})
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(1), Bg: alpha(hair, solid)}, func() {})
	})
}
