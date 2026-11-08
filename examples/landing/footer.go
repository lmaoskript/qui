package main

import (
	"fmt"
	"math"

	"github.com/torisetxd/qui"
)

func (a *app) finalCta() {
	ui := a.ui
	a.mark("top-cta")
	height := float32(600)
	ui.Stack(qui.Style{Key: "cta", W: qui.Fill(), H: qui.Px(height), Clip: true}, func() {
		ui.Shader(qui.ShaderOptions{Src: horizonShader, Style: qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: black, Key: "cta-sky"}, Fps: a.fps("top-cta", height)})
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Align: qui.Center, Pad: qui.Edges{Top: 110, Left: 20, Right: 20}}, func() {
			a.reveal("cta.body", qui.Style{Align: qui.Center, Gap: 18, W: qui.Fill()}, func() {
				size := clamp(a.w*0.052, 36, 72)
				ui.Text("Your next deploy", qui.TextStyle{Size: size, Color: ink, Tracking: -size * 0.04, Role: qui.RoleHeading})
				ui.Text("could already be live.", qui.TextStyle{Size: size, Color: subtle, Tracking: -size * 0.04})
				ui.Column(qui.Style{H: qui.Px(8)}, func() {})
				if a.joined {
					ui.Text("You're on the list. Well, you would be, if Sable existed.", qui.TextStyle{Size: 16, Color: ink, W: qui.Fill(), Align: qui.Center})
					return
				}
				ui.Row(qui.Style{Gap: 10, Align: qui.Center}, func() {
					width := min(300, a.w-170)
					r := ui.Input("email", &a.email, qui.InputOptions{Placeholder: "you@company.dev", Size: 14, Style: qui.Style{W: qui.Px(width), Bg: panel, Border: hairHot, Radius: 8, Pad: qui.XY(14, 10), Fg: ink}})
					if a.primary("Get access").Clicked || r.Submitted {
						if a.email != "" {
							a.joined = true
						}
					}
				})
			})
		})
	})
}

var footerLinks = []struct {
	title string
	items []string
}{
	{"Product", []string{"Edge functions", "Previews", "Observability", "Firewall"}},
	{"Developers", []string{"Documentation", "CLI reference", "Templates", "Changelog"}},
	{"Company", []string{"About", "Customers", "Careers", "Press kit"}},
	{"Legal", []string{"Terms", "Privacy", "Security", "Sub-processors"}},
}

func (a *app) footer() {
	ui := a.ui
	a.rule()
	ui.Row(qui.Style{W: qui.Fill(), Justify: qui.Center}, func() {
		ui.Column(qui.Style{W: qui.Px(a.inner), Pad: qui.XY(a.pad(), 52), Gap: 40, Clip: true}, func() {
			columns := func() {
				for _, group := range footerLinks {
					ui.Column(qui.Style{W: qui.Fill(), Gap: 10, Key: group.title}, func() {
						a.label(group.title)
						for _, item := range group.items {
							ui.Text(item, qui.TextStyle{Size: 14, Color: muted})
						}
					})
				}
			}
			if a.narrow {
				ui.Column(qui.Style{W: qui.Fill(), Gap: 28}, columns)
			} else {
				ui.Row(qui.Style{W: qui.Fill(), Gap: 24}, columns)
			}
			ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Gap: 10}, func() {
				pulse := 0.6 + 0.4*float32(math.Sin(float64(a.t*2.2)))
				ui.Column(qui.Style{W: qui.Px(7), H: qui.Px(7), Radius: 4, Bg: lime, Opacity: pulse, Shadow: qui.BoxShadow{Color: alpha(lime, 0.5), Blur: 10}}, func() {})
				ui.Text("All systems normal", qui.TextStyle{Size: 13, Color: muted, W: qui.Fill()})
				if !a.narrow {
					ui.Text(fmt.Sprintf("%d frames rendered by qui", int(a.t*60)), qui.TextStyle{Size: 12, Mono: true, Color: subtle})
				}
			})
			size := min(a.inner*0.22, 230)
			ui.Stack(qui.Style{H: qui.Px(size * 1.12)}, func() {
				ui.Text("SABLE", qui.TextStyle{Size: size, Bold: true, Tracking: -size * 0.04, Gradient: qui.Gradient{From: qui.RGBA(255, 255, 255, 70), To: qui.RGBA(255, 255, 255, 4), Angle: 180}})
			})
			ui.Text("Sable is a fictional company. This page is a demo of the qui UI framework, and every number on it is made up.", qui.TextStyle{Size: 12, Color: subtle, W: qui.Fill()})
		})
	})
}
