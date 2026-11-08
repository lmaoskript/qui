package main

import (
	"fmt"
	"math"

	"github.com/torisetxd/qui"
)

var coldStarts = func() []float32 {
	bars := make([]float32, 44)
	for i := range bars {
		x := float64(i)
		bars[i] = float32(math.Exp(-math.Pow(x-5, 2)/9) + 0.16*math.Exp(-math.Pow(x-14, 2)/42) + 0.05*math.Exp(-x/60) + 0.012*math.Abs(math.Sin(x*2.3)))
	}
	return bars
}()

var branches = []struct {
	name, state, url string
	lane             int
}{
	{"main", "production", "storefront.sable.app", 0},
	{"feat/checkout-v2", "preview", "checkout-v2.storefront.sable.dev", 1},
	{"fix/cart-race", "building", "", 1},
	{"docs/edge-cache", "preview", "edge-cache.storefront.sable.dev", 2},
}

var releases = []struct{ sha, message, age string }{
	{"1ad90be", "Bump image optimizer", "3 days ago"},
	{"c47f2e1", "Add Swedish locale", "2 days ago"},
	{"90e3ab7", "Migrate sessions to KV", "yesterday"},
	{"5fb1c02", "Tune product page cache", "5 hours ago"},
	{"e22a4d8", "New checkout header", "47 minutes ago"},
	{"8e21c4f", "Fix cart race on retry", "12 minutes ago"},
}

var regions = []struct {
	name string
	ms   float32
}{
	{"North America", 11}, {"Europe", 14}, {"Asia Pacific", 23}, {"South America", 29}, {"Middle East", 31}, {"Africa", 38},
}

func (a *app) divider(vertical bool) {
	if vertical && !a.narrow {
		a.ui.Column(qui.Style{W: qui.Px(1), H: qui.Fill(), Bg: hair}, func() {})
		return
	}
	a.ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(1), Bg: hair}, func() {})
}

func (a *app) tile(key string, w qui.Size, label, title string, body func()) {
	pad := float32(32)
	if a.narrow {
		pad = 20
	}
	a.reveal(key, qui.Style{W: w, Pad: qui.Edges{Top: pad, Right: pad, Bottom: pad - 8, Left: pad}, Gap: 12}, func() {
		a.label(label)
		a.ui.Text(title, qui.TextStyle{Size: 21, Color: ink, Tracking: -0.5, W: qui.Fill()})
		a.ui.Column(qui.Style{H: qui.Px(6)}, func() {})
		body()
	})
}

func (a *app) tiles(left, right func(qui.Size)) {
	if a.narrow {
		left(qui.Fill())
		a.divider(false)
		right(qui.Fill())
		return
	}
	a.ui.Row(qui.Style{W: qui.Fill()}, func() {
		left(qui.FillN(7))
		a.divider(true)
		right(qui.FillN(5))
	})
}

func (a *app) bento() {
	ui := a.ui
	a.section("bento", func() {
		a.header("bento", "02", "Platform", "Compute, cache and shields.", "Already wired together.")
		a.divider(false)
		a.bentoRect = ui.Stack(qui.Style{Key: "bento.grid", W: qui.Fill(), Clip: true, Hit: true}, func() {
			ui.Column(qui.Style{W: qui.Fill()}, func() {
				a.tiles(a.coldStartTile, a.previewsTile)
				a.divider(false)
				a.attackTile()
				a.divider(false)
				if a.narrow {
					a.rollbackTile(qui.Fill())
					a.divider(false)
					a.latencyTile(qui.Fill())
					a.divider(false)
					a.cacheTile(qui.Fill())
				} else {
					ui.Row(qui.Style{W: qui.Fill()}, func() {
						a.rollbackTile(qui.FillN(5))
						a.divider(true)
						a.latencyTile(qui.FillN(4))
						a.divider(true)
						a.cacheTile(qui.FillN(3))
					})
				}
			})
			a.spotlight()
		})
	})
}

func (a *app) spotlight() {
	ui := a.ui
	r := a.bentoRect
	px, py := ui.PointerX(), ui.PointerY()
	target := float32(0)
	if r.Found && px >= r.X && py >= r.Y && px <= r.X+r.W && py <= r.Y+r.H && py > 64 {
		target = 1
	}
	a.bentoHover = approach(a.bentoHover, target, 10, a.dt)
	if a.bentoHover < 0.01 {
		return
	}
	const size = 620
	ui.Shader(qui.ShaderOptions{
		Src: spotShader, Style: qui.Style{Absolute: true, X: px - r.X - size/2, Y: py - r.Y - size/2, W: qui.Px(size), H: qui.Px(size), Key: "spot"}, Fps: 60,
		Uniforms: []qui.Uniform{{Name: "u_a", Values: []float32{a.bentoHover}}},
	})
}

func (a *app) coldStartTile(w qui.Size) {
	ui := a.ui
	a.tile("t.cold", w, "Edge functions", "Cold starts you can't feel.", func() {
		p := a.revealed("t.cold")
		const chart = 148
		index := -1
		r := a.coldRect
		if r.Hovered && r.W > 0 {
			index = int(clamp((ui.PointerX()-r.X)/r.W, 0, 0.999) * float32(len(coldStarts)))
		}
		ui.Row(qui.Style{W: qui.Fill(), Align: qui.End}, func() {
			if index >= 0 {
				ui.Text(fmt.Sprintf("%d ms", index+1), qui.TextStyle{Size: 34, Color: ink, Tracking: -1.2})
				ui.Text(fmt.Sprintf("   %s of invocations", percent(coldStarts[index])), qui.TextStyle{Size: 13, Color: muted, Mono: true})
			} else {
				ui.Text("4 ms", qui.TextStyle{Size: 34, Color: ink, Tracking: -1.2})
				ui.Text("   median, last 24 hours", qui.TextStyle{Size: 13, Color: muted, Mono: true})
			}
		})
		a.coldRect = ui.Row(qui.Style{Key: "bars", W: qui.Fill(), H: qui.Px(chart), Align: qui.End, Gap: 2, Hit: true}, func() {
			for i, v := range coldStarts {
				color := qui.RGBA(255, 255, 255, 56)
				if i == 3 {
					color = orange
				}
				if i == index {
					color = white
				}
				ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(max(2, v*(chart-2)*p)), Radius: 2, Bg: color}, func() {})
			}
		})
		ui.Row(qui.Style{W: qui.Fill(), Justify: qui.Between}, func() {
			for _, tick := range []string{"1 ms", "11", "22", "33", "44 ms"} {
				ui.Text(tick, qui.TextStyle{Size: 11, Mono: true, Color: subtle})
			}
		})
	})
}

func percent(v float32) string {
	var total float32
	for _, x := range coldStarts {
		total += x
	}
	return fmt.Sprintf("%.1f%%", v/total*100)
}

func (a *app) previewsTile(w qui.Size) {
	ui := a.ui
	a.tile("t.preview", w, "Previews", "Every branch gets a URL.", func() {
		for i, b := range branches {
			r := ui.Row(qui.Style{Key: b.name, W: qui.Fill(), Hit: true, Align: qui.Center, Gap: 12, Pad: qui.XY(0, 8)}, func() {
				ui.Stack(qui.Style{W: qui.Px(46), H: qui.Px(38)}, func() {
					ui.Column(qui.Style{Absolute: true, X: 4, Y: -10, W: qui.Px(1), H: qui.Px(58), Bg: hairHot}, func() {})
					if b.lane > 0 {
						x := float32(b.lane) * 16
						ui.Column(qui.Style{Absolute: true, X: 4, Y: 18, W: qui.Px(x), H: qui.Px(1), Bg: hairHot}, func() {})
					}
					dot := white
					switch b.state {
					case "preview":
						dot = cyan
					case "building":
						dot = alpha(orange, 0.55+0.45*float32(math.Sin(float64(a.t*4))))
					}
					ui.Column(qui.Style{Absolute: true, X: float32(b.lane)*16 + 1, Y: 15, W: qui.Px(7), H: qui.Px(7), Radius: 4, Bg: dot, Shadow: qui.BoxShadow{Color: alpha(dot, 0.7), Blur: 10}}, func() {})
				})
				ui.Column(qui.Style{W: qui.Fill(), Gap: 1}, func() {
					ui.Text(b.name, qui.TextStyle{Size: 13.5, Mono: true, Color: ink, Ellipsis: true, W: qui.Fill()})
					url := b.url
					if url == "" {
						url = "building..."
					}
					color := subtle
					if a.hover[b.name] {
						color = muted
					}
					ui.Text(url, qui.TextStyle{Size: 12, Color: color, Ellipsis: true, W: qui.Fill()})
				})
				if !a.narrow || i == 0 {
					a.label(b.state)
				}
			})
			a.hover[b.name] = r.Hovered
		}
	})
}

func (a *app) attackTile() {
	ui := a.ui
	pad := float32(32)
	if a.narrow {
		pad = 20
	}
	chart := func(height float32, w qui.Size) {
		ui.Shader(qui.ShaderOptions{
			Src: attackShader, Style: qui.Style{W: w, H: qui.Px(height), Key: "attack"}, Fps: a.fps("bento", 1500),
			Uniforms: []qui.Uniform{{Name: "u_dpr", Values: []float32{a.dpr}}, {Name: "u_ready", Values: []float32{a.revealed("t.attack")}}},
		})
	}
	legend := func(color qui.Color, name string) {
		ui.Row(qui.Style{Gap: 7, Align: qui.Center}, func() {
			ui.Column(qui.Style{W: qui.Px(14), H: qui.Px(2), Bg: color}, func() {})
			a.label(name)
		})
	}
	info := func() {
		a.ui.Text("Attacks end at the perimeter.", qui.TextStyle{Size: 21, Color: ink, Tracking: -0.5, W: qui.Fill()})
		ui.Text("Every city scrubs traffic locally, so a 7.4 Tbps flood never reaches your origin.", qui.TextStyle{Size: 14, Color: muted, W: qui.Fill()})
		ui.Column(qui.Style{H: qui.Px(10)}, func() {})
		ui.Row(qui.Style{Gap: 30}, func() {
			for _, s := range [][2]string{{"7.4 Tbps", "peak mitigated"}, {"0", "packets at origin"}} {
				ui.Column(qui.Style{Gap: 3}, func() {
					ui.Text(s[0], qui.TextStyle{Size: 28, Color: ink, Tracking: -0.9})
					a.label(s[1])
				})
			}
		})
	}
	a.reveal("t.attack", qui.Style{W: qui.Fill(), Pad: qui.Edges{Top: pad, Right: pad, Bottom: pad - 8, Left: pad}, Gap: 16}, func() {
		a.label("Security")
		if a.narrow {
			info()
			ui.Row(qui.Style{Gap: 16}, func() { legend(orange, "attack"); legend(qui.RGB(209, 224, 250), "clean") })
			chart(190, qui.Fill())
			return
		}
		ui.Row(qui.Style{W: qui.Fill(), Gap: 44, Align: qui.Stretch}, func() {
			ui.Column(qui.Style{W: qui.FillN(4), Gap: 8}, info)
			ui.Column(qui.Style{W: qui.FillN(7), Gap: 8}, func() {
				ui.Row(qui.Style{Gap: 16, W: qui.Fill(), Justify: qui.End}, func() { legend(orange, "attack traffic"); legend(qui.RGB(209, 224, 250), "clean traffic") })
				chart(230, qui.Fill())
			})
		})
	})
}

func (a *app) rollbackTile(w qui.Size) {
	ui := a.ui
	a.tile("t.rollback", w, "Rollback", "Undo the last ten minutes.", func() {
		if a.rollbackFlash > 0 {
			a.rollbackFlash -= a.dt
		}
		hovered := -1
		ui.Stack(qui.Style{W: qui.Fill(), H: qui.Px(34)}, func() {
			ui.Column(qui.Style{Absolute: true, X: 0, Y: 16, W: qui.Fill(), H: qui.Px(1), Bg: hairHot}, func() {})
			ui.Row(qui.Style{W: qui.Fill(), H: qui.Fill(), Justify: qui.Between, Align: qui.Center}, func() {
				for i := range releases {
					key := fmt.Sprint("rel", i)
					r := ui.Stack(qui.Style{Key: key, W: qui.Px(28), H: qui.Px(28), Hit: true, Align: qui.Center, Justify: qui.Center}, func() {
						switch {
						case i == a.rollback:
							ui.Column(qui.Style{W: qui.Px(14), H: qui.Px(14), Radius: 7, Gradient: qui.Gradient{From: orange, To: magenta, Angle: 135}, Shadow: qui.BoxShadow{Color: alpha(magenta, 0.75), Blur: 16}}, func() {})
						case a.hover[key]:
							ui.Column(qui.Style{W: qui.Px(11), H: qui.Px(11), Radius: 6, Bg: white}, func() {})
						default:
							ui.Column(qui.Style{W: qui.Px(9), H: qui.Px(9), Radius: 5, Bg: black, Border: hairHot}, func() {})
						}
					})
					a.hover[key] = r.Hovered
					if r.Hovered {
						hovered = i
					}
					if r.Clicked && i != a.rollback {
						a.rollback = i
						a.rollbackFlash = 2.4
					}
				}
			})
		})
		shown := a.rollback
		if hovered >= 0 {
			shown = hovered
		}
		release := releases[shown]
		ui.Column(qui.Style{W: qui.Fill(), Gap: 3}, func() {
			ui.Text(release.message, qui.TextStyle{Size: 15, Color: ink, W: qui.Fill(), Ellipsis: true})
			ui.Text(release.sha+"  "+release.age, qui.TextStyle{Size: 12, Mono: true, Color: subtle, W: qui.Fill(), Ellipsis: true})
		})
		switch {
		case a.rollbackFlash > 0:
			a.label("switched in 180 ms", lime)
		case shown == a.rollback:
			a.label("in production")
		default:
			a.label("click to roll back", orange)
		}
	})
}

func (a *app) latencyTile(w qui.Size) {
	ui := a.ui
	a.tile("t.latency", w, "Latency", "Close to everyone.", func() {
		for i, region := range regions {
			ms := region.ms + 1.4*float32(math.Sin(float64(a.t*0.9+float32(i)*1.7)))
			ui.Column(qui.Style{Key: region.name, W: qui.Fill(), Gap: 4}, func() {
				ui.Row(qui.Style{W: qui.Fill()}, func() {
					ui.Text(region.name, qui.TextStyle{Size: 12.5, Color: muted, W: qui.Fill()})
					ui.Text(fmt.Sprintf("%.0f ms", ms), qui.TextStyle{Size: 12.5, Mono: true, Color: ink})
				})
				ui.Stack(qui.Style{W: qui.Fill(), H: qui.Px(2), Bg: qui.RGBA(255, 255, 255, 18)}, func() {
					ui.Column(qui.Style{H: qui.Fill(), W: qui.Pct(clamp(ms/50*100*a.revealed("t.latency"), 0, 100)), Bg: qui.RGBA(255, 255, 255, 150)}, func() {})
				})
			})
		}
	})
}

func (a *app) cacheTile(w qui.Size) {
	ui := a.ui
	a.tile("t.cache", w, "Cache", "96.4% never hit origin.", func() {
		tick := int(a.t * 1.6)
		ui.Column(qui.Style{Gap: 3}, func() {
			for y := 0; y < 10; y++ {
				ui.Row(qui.Style{Key: fmt.Sprint("w", y), Gap: 3}, func() {
					for x := 0; x < 10; x++ {
						miss := (x*7+y*13+tick*29)%100 < 4
						color := qui.RGBA(255, 255, 255, 170)
						if miss {
							color = orange
						}
						ui.Column(qui.Style{W: qui.Px(13), H: qui.Px(13), Radius: 2, Bg: color}, func() {})
					}
				})
			}
		})
		ui.Row(qui.Style{Gap: 14}, func() {
			ui.Row(qui.Style{Gap: 6, Align: qui.Center}, func() {
				ui.Column(qui.Style{W: qui.Px(7), H: qui.Px(7), Radius: 1, Bg: qui.RGBA(255, 255, 255, 170)}, func() {})
				a.label("hit")
			})
			ui.Row(qui.Style{Gap: 6, Align: qui.Center}, func() {
				ui.Column(qui.Style{W: qui.Px(7), H: qui.Px(7), Radius: 1, Bg: orange}, func() {})
				a.label("miss")
			})
		})
	})
}
