package main

import (
	"fmt"

	"github.com/torisetxd/qui"
)

const deployCycle = 15

type logLine struct {
	at    float32
	text  string
	color qui.Color
}

var deployLog = []logLine{
	{0.0, "$ git push origin main", ink},
	{0.9, "remote: queued deployment dpl_7f3a91", muted},
	{1.3, "cloning storefront@8e21c4f", muted},
	{2.0, "restored 412 packages from cache", muted},
	{3.0, "building 184 routes, 62 static, 122 edge", muted},
	{4.7, "bundled edge functions, 48.2 kB gzip", muted},
	{6.2, "uploading 61 changed assets, 1,204 skipped", muted},
	{7.4, "propagating to 310 cities", orange},
	{9.0, "live in 8.9s at storefront.sable.app", lime},
	{9.5, "previous deployment kept warm for rollback", muted},
}

var deploySteps = []struct {
	name, note string
	from, to   float32
}{
	{"Clone", "shallow, 1 commit", 0.9, 2.0},
	{"Install", "lockfile cache", 2.0, 3.0},
	{"Build", "incremental", 3.0, 6.2},
	{"Upload", "content-addressed", 6.2, 7.4},
	{"Propagate", "310 cities", 7.4, 9.0},
}

func (a *app) deploy() {
	ui := a.ui
	a.section("platform", func() {
		a.header("deploy", "01", "Deploy", "Git push is the whole interface.", "Everything after it is our problem.")
		if a.visible("platform", 900) {
			a.deployClock += a.dt
		}
		for a.deployClock > deployCycle {
			a.deployClock -= deployCycle
		}
		pad := a.pad()
		left := func(s qui.Size) {
			a.reveal("deploy.steps", qui.Style{W: s, Pad: qui.Edges{Left: pad, Right: pad, Top: 8, Bottom: 44}, Gap: 0}, func() { a.steps() })
		}
		right := func(s qui.Size) {
			a.reveal("deploy.term", qui.Style{W: s, Pad: qui.Edges{Left: pad, Right: pad, Top: 8, Bottom: 44}, Gap: 14}, func() { a.terminal() })
		}
		if a.narrow {
			left(qui.Fill())
			right(qui.Fill())
			return
		}
		ui.Row(qui.Style{W: qui.Fill()}, func() {
			left(qui.FillN(4))
			right(qui.FillN(6))
		})
	})
}

func (a *app) steps() {
	ui := a.ui
	clock := a.deployClock
	for i, step := range deploySteps {
		ui.Column(qui.Style{Key: step.name, W: qui.Fill()}, func() {
			ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Gap: 14, Pad: qui.XY(0, 17)}, func() {
				ui.Text(fmt.Sprintf("0%d", i+1), qui.TextStyle{Size: 11, Mono: true, Color: subtle})
				switch {
				case clock >= step.to:
					ui.Column(qui.Style{W: qui.Px(8), H: qui.Px(8), Radius: 2, Bg: lime}, func() {})
				case clock >= step.from:
					ui.Column(qui.Style{W: qui.Px(8), H: qui.Px(8), Radius: 2, Bg: orange, Shadow: qui.BoxShadow{Color: alpha(orange, 0.9), Blur: 14}}, func() {})
				default:
					ui.Column(qui.Style{W: qui.Px(8), H: qui.Px(8), Radius: 2, Border: hairHot}, func() {})
				}
				color := subtle
				if clock >= step.from {
					color = ink
				}
				ui.Text(step.name, qui.TextStyle{Size: 16, Color: color, W: qui.Fill()})
				ui.Text(step.note, qui.TextStyle{Size: 12, Color: subtle, Mono: true})
			})
			ui.Stack(qui.Style{W: qui.Fill(), H: qui.Px(1), Bg: hair}, func() {
				p := clamp((clock-step.from)/(step.to-step.from), 0, 1)
				if p > 0 {
					ui.Column(qui.Style{H: qui.Fill(), W: qui.Pct(p * 100), Gradient: qui.Gradient{From: orange, To: magenta, Angle: 90}}, func() {})
				}
			})
		})
	}
}

func (a *app) terminal() {
	ui := a.ui
	clock := a.deployClock
	ui.Column(qui.Style{W: qui.Fill(), Bg: panel, Border: hair, Radius: 10, Clip: true}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Pad: qui.XY(16, 11), Gap: 8}, func() {
			a.label("deploy.log")
			a.spacer()
			if clock >= 9 {
				ui.Column(qui.Style{W: qui.Px(6), H: qui.Px(6), Radius: 3, Bg: lime}, func() {})
				a.label("live", lime)
			} else {
				ui.Column(qui.Style{W: qui.Px(6), H: qui.Px(6), Radius: 3, Bg: orange}, func() {})
				a.label("deploying", orange)
			}
		})
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(1), Bg: hair}, func() {})
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(238), Pad: qui.All(16), Gap: 3}, func() {
			last := -1
			for i, entry := range deployLog {
				if clock >= entry.at {
					last = i
				}
			}
			for i := 0; i <= last; i++ {
				entry := deployLog[i]
				text := entry.text
				if i == 0 {
					text = text[:min(len(text), int((clock+0.05)*30))]
				}
				ui.Row(qui.Style{Key: fmt.Sprint("l", i), W: qui.Fill(), Align: qui.Center, Gap: 2}, func() {
					ui.Text(text, qui.TextStyle{Size: 12.5, Mono: true, Color: entry.color, Ellipsis: true})
					if i == last && int(a.t*2.4)%2 == 0 {
						ui.Column(qui.Style{W: qui.Px(7), H: qui.Px(14), Bg: ink}, func() {})
					}
				})
			}
		})
	})
	p := clamp((clock-7.4)/1.6, 0, 1)
	live := float32(0)
	if clock >= 9 {
		live = 1
	}
	ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center}, func() {
		a.label("edge propagation")
		a.spacer()
		a.label(fmt.Sprintf("%d / 310 cities", int(310*p)), ink)
	})
	ui.Shader(qui.ShaderOptions{
		Src: waveShader, Style: qui.Style{W: qui.Fill(), H: qui.Px(120), Key: "wave"}, Fps: a.fps("platform", 900),
		Uniforms: []qui.Uniform{{Name: "u_dpr", Values: []float32{a.dpr}}, {Name: "u_p", Values: []float32{p}}, {Name: "u_live", Values: []float32{live}}},
	})
}
