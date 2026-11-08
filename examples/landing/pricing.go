package main

import (
	"fmt"

	"github.com/torisetxd/qui"
)

var tiers = []struct {
	name, blurb     string
	monthly, annual float32
	custom, cta     string
	features        []string
}{
	{"Hobby", "For side projects and experiments.", 0, 0, "", "Start free", []string{"100 GB bandwidth", "Unlimited previews", "Automatic HTTPS", "Community support"}},
	{"Pro", "For teams shipping to production.", 24, 19, "", "Start a 14-day trial", []string{"1 TB bandwidth", "Edge functions in every city", "Instant rollbacks", "L7 attack protection", "Per-branch environments"}},
	{"Enterprise", "For regulated, global workloads.", 0, 0, "Custom", "Talk to us", []string{"Committed bandwidth", "Regional data residency", "L3-L7 mitigation with an SLA", "SSO and audit logs", "A named engineer"}},
}

func (a *app) pricing() {
	ui := a.ui
	target := float32(0)
	if a.annual {
		target = 1
	}
	a.annualT = approach(a.annualT, target, 12, a.dt)
	a.section("pricing", func() {
		a.header("price", "04", "Pricing", "Pay for what you ship.", "Not for what you might.")
		pad := a.pad()
		a.reveal("price.toggle", qui.Style{W: qui.Fill(), Pad: qui.Edges{Left: pad, Right: pad}}, func() {
			const each = 128
			ui.Stack(qui.Style{W: qui.Px(each*2 + 8), H: qui.Px(40), Radius: 8, Border: hair, Bg: panel}, func() {
				ui.Column(qui.Style{Absolute: true, X: 3 + a.annualT*each, Y: 3, W: qui.Px(each), H: qui.Px(32), Radius: 6, Bg: raised, Border: hair}, func() {})
				ui.Row(qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.All(3)}, func() {
					labels := []string{"Monthly", "Annual -20%"}
					for i, label := range labels {
						on := a.annual == (i == 1)
						color := muted
						if on {
							color = ink
						}
						if ui.Button(label, qui.Style{W: qui.Px(each), H: qui.Fill(), Fg: color, Bg: qui.RGBA(0, 0, 0, 1), Radius: 6, Key: label}).Clicked {
							a.annual = i == 1
						}
					}
				})
			})
		})
		ui.Column(qui.Style{H: qui.Px(22)}, func() {})
		body := func() {
			for i := range tiers {
				a.card(i)
			}
		}
		if a.narrow {
			ui.Column(qui.Style{W: qui.Fill(), Pad: qui.Edges{Left: pad, Right: pad, Bottom: 52}, Gap: 16}, body)
		} else {
			ui.Row(qui.Style{W: qui.Fill(), Pad: qui.Edges{Left: pad, Right: pad, Bottom: 76}, Gap: 16}, body)
		}
	})
}

func (a *app) card(i int) {
	ui := a.ui
	tier := tiers[i]
	featured := i == 1
	key := fmt.Sprint("card", i)
	content := func() {
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: panel, Radius: 11, Pad: qui.All(26), Gap: 16}, func() {
			ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center}, func() {
				ui.Text(tier.name, qui.TextStyle{Size: 18, Color: ink, W: qui.Fill(), Role: qui.RoleHeading})
				if featured {
					a.label("most teams", orange)
				}
			})
			ui.Text(tier.blurb, qui.TextStyle{Size: 14, Color: muted, W: qui.Fill()})
			ui.Row(qui.Style{Align: qui.End, Gap: 6}, func() {
				if tier.custom != "" {
					ui.Text(tier.custom, qui.TextStyle{Size: 44, Color: ink, Tracking: -1.8})
					return
				}
				price := tier.monthly + (tier.annual-tier.monthly)*a.annualT
				ui.Text(fmt.Sprintf("$%.0f", price), qui.TextStyle{Size: 44, Color: ink, Tracking: -1.8})
				ui.Text("/ user / mo", qui.TextStyle{Size: 13, Color: subtle, Mono: true})
			})
			var clicked bool
			if featured {
				clicked = a.primary(tier.cta).Clicked
			} else {
				clicked = a.secondary(tier.cta).Clicked
			}
			if clicked {
				a.goTo("top-cta")
			}
			ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(1), Bg: hair}, func() {})
			ui.Column(qui.Style{W: qui.Fill(), Gap: 11}, func() {
				for _, feature := range tier.features {
					ui.Row(qui.Style{W: qui.Fill(), Gap: 11, Align: qui.Center}, func() {
						color := qui.RGBA(255, 255, 255, 120)
						if featured {
							color = orange
						}
						ui.Column(qui.Style{W: qui.Px(5), H: qui.Px(5), Bg: color}, func() {})
						ui.Text(feature, qui.TextStyle{Size: 14, Color: ink, W: qui.Fill()})
					})
				}
			})
		})
	}
	w := qui.Fill()
	if featured {
		a.reveal(key, qui.Style{W: w, H: qui.Fill()}, func() {
			ui.Stack(qui.Style{W: qui.Fill(), H: qui.Fill(), Radius: 12, Clip: true, Shadow: qui.BoxShadow{Color: qui.RGBA(255, 100, 90, 56), Blur: 54}}, func() {
				ui.Shader(qui.ShaderOptions{Src: borderShader, Style: qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Fill(), H: qui.Fill(), Key: "ring", Bg: raised}, Fps: a.fps("pricing", 800)})
				ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.All(1.5)}, content)
			})
		})
		return
	}
	a.reveal(key, qui.Style{W: w, H: qui.Fill()}, func() {
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Radius: 12, Border: hair}, content)
	})
}
