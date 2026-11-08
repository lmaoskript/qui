package main

import (
	"strings"

	"github.com/torisetxd/qui"
)

const labFps = 30

type labState struct {
	source, applied, good string
	failure               string
	editedAt              float64
	pending               int
	preset                int
	speed, shape, detail  float32
}

func (s *labState) setup() {
	s.speed, s.shape, s.detail = 0.35, 0.5, 0.45
	s.load(0)
}

func (s *labState) load(i int) {
	s.preset = i
	s.source, s.applied, s.good = presets[i].src, presets[i].src, presets[i].src
	s.failure = ""
}

func (s *labState) busy() bool { return s.source != s.applied || s.applied != s.good }

func shortLog(log string) string {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	return strings.Join(lines[:min(len(lines), 3)], "\n")
}

func (a *app) labWindow() {
	ui := a.ui
	s := &a.lab
	if s.source != s.applied && a.clock-s.editedAt > 0.18 {
		s.applied = s.source
		s.pending = 0
	}

	layout := ui.Row
	if a.narrow {
		layout = ui.Column
	}
	layout(qui.Style{W: qui.Fill(), H: qui.Fill(), Key: "split"}, func() {
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.All(10), Gap: 8, Key: "view"}, func() {
			a.labPreview()
			ui.Row(qui.Style{W: qui.Fill(), Gap: 5, Key: "presets"}, func() {
				for i, p := range presets {
					bg := qui.Color(ghost)
					if i == s.preset {
						bg = qui.Item
					}
					if ui.Button(p.name, qui.Style{W: qui.Fill(), H: qui.Px(26), Pad: qui.XY(4, 0), Bg: bg, Key: p.name}).Clicked {
						s.load(i)
					}
				}
			})
			a.slider("speed", "Speed", &s.speed, 0, 1)
			a.slider("shape", "Shape", &s.shape, 0, 1)
			a.slider("detail", "Detail", &s.detail, 0, 1)
		})
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.Edges{Top: 10, Right: 10, Bottom: 10}, Gap: 6, Key: "code"}, func() {
			ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Gap: 8}, func() {
				ui.Text("GLSL, compiled as you type", qui.TextStyle{Size: 11.5, Color: qui.Muted, W: qui.Fill()})
				switch {
				case s.failure != "":
					ui.Text("error", qui.TextStyle{Size: 11.5, Mono: true, Color: qui.Danger})
				case s.busy():
					ui.Text("compiling", qui.TextStyle{Size: 11.5, Mono: true, Color: qui.Muted})
				default:
					ui.Text("live", qui.TextStyle{Size: 11.5, Mono: true, Color: qui.Accent2})
				}
			})
			edit := ui.Input("source", &s.source, qui.InputOptions{Style: qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: qui.RGBA(0, 0, 0, 90)}, Mono: true, Multiline: true, Size: 11.5})
			if edit.Changed {
				s.editedAt = a.clock
			}
			if s.failure != "" {
				ui.Text(s.failure, qui.TextStyle{Size: 11, Mono: true, Color: qui.Danger, W: qui.Fill()})
			}
		})
	})
}

func (a *app) labPreview() {
	ui := a.ui
	s := &a.lab
	uniforms := []qui.Uniform{
		{Name: "u_a", Values: []float32{s.speed}},
		{Name: "u_b", Values: []float32{s.shape}},
		{Name: "u_c", Values: []float32{s.detail}},
	}
	fps := float32(labFps)
	if a.calm {
		fps = 0
	}
	ui.Stack(qui.Style{W: qui.Fill(), H: qui.Fill(), Radius: 8, Clip: true, Key: "preview"}, func() {
		ui.Shader(qui.ShaderOptions{Src: s.good, Style: qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: qui.Inset, Key: "live"}, Fps: fps, Uniforms: uniforms})
		if s.applied == s.good {
			return
		}
		probe := ui.Shader(qui.ShaderOptions{Src: s.applied, Style: qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Px(2), H: qui.Px(2), Bg: ghost, Z: -1, Key: "probe"}, Uniforms: uniforms})
		if s.pending > 0 {
			if probe.Error != "" {
				s.failure = shortLog(probe.Error)
			} else {
				s.good = s.applied
				s.failure = ""
			}
		}
		s.pending++
	})
}

func (a *app) slider(key, label string, value *float32, low, high float32) {
	ui := a.ui
	ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(18), Gap: 10, Align: qui.Center, Key: key}, func() {
		ui.Text(label, qui.TextStyle{Size: 11.5, Color: qui.Muted, W: qui.Px(48)})
		fraction := clamp((*value-low)/(high-low), 0, 1)
		track := ui.Stack(qui.Style{W: qui.Fill(), H: qui.Px(18), Justify: qui.Center, Hit: true, Key: "track", Label: label}, func() {
			ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(3), Bg: qui.Line, Radius: 2}, noop)
			ui.Column(qui.Style{W: qui.Pct(fraction * 100), H: qui.Px(3), Bg: qui.Accent, Radius: 2}, noop)
			ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(12), Key: "thumb"}, func() {
				ui.Column(qui.Style{W: qui.FillN(fraction + 0.001), H: qui.Px(1)}, noop)
				ui.Column(qui.Style{W: qui.Px(12), H: qui.Px(12), Bg: qui.Strong, Radius: 6}, noop)
				ui.Column(qui.Style{W: qui.FillN(1 - fraction + 0.001), H: qui.Px(1)}, noop)
			})
		})
		if (track.Pressed || track.Clicked) && track.W > 12 {
			*value = low + clamp((ui.PointerX()-track.X-6)/(track.W-12), 0, 1)*(high-low)
		}
	})
}
