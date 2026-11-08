package main

import (
	"fmt"
	"math"

	"github.com/torisetxd/qui"
)

type widgetState struct {
	name, secret string
	notify, sync bool
	knobs        map[string]float32
	density      int
	volume       float32
	clicks       int
}

func (s *widgetState) setup() {
	s.name = "Ada Lovelace"
	s.notify = true
	s.knobs = map[string]float32{}
	s.density = 1
	s.volume = 0.65
}

func (a *app) section(title string, body func()) {
	ui := a.ui
	ui.Column(qui.Style{W: qui.Fill(), Pad: qui.All(12), Gap: 9, Bg: qui.RGBA(255, 255, 255, 7), Radius: 9, Key: title}, func() {
		ui.Text(title, qui.TextStyle{Size: 11.5, Bold: true, Color: qui.Muted})
		body()
	})
}

func (a *app) widgetsWindow() {
	ui := a.ui
	s := &a.widgets
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.All(12), Gap: 10, Scroll: true, Key: "widgets"}, func() {
		a.section("TYPE", func() {
			ui.Text("Heading one", qui.H1)
			ui.Text("Heading two", qui.H2)
			ui.Text("Body text wraps at the width of its container, shaped by HarfBuzz natively and by the browser on the web.", qui.TextStyle{W: qui.Fill()})
			ui.Text("A single line that is too long for the panel ends with an ellipsis instead of pushing the layout around", qui.TextStyle{W: qui.Fill(), Ellipsis: true, Color: qui.Muted})
			ui.Text("func main() { ui.Frame() }", qui.TextStyle{Size: 13, Mono: true, Color: qui.Accent2})
		})
		a.section("SCRIPTS", func() {
			ui.Text("日本語のテキスト · 中文排版 · 한국어 문장", qui.TextStyle{W: qui.Fill()})
			ui.Text("العربية تُكتب من اليمين · हिन्दी में लेख", qui.TextStyle{W: qui.Fill()})
			ui.Text("Emoji 🎨 👩‍💻 🇯🇵 🧑🏽‍🚀 · ligatures fi fl · café naïve", qui.TextStyle{W: qui.Fill()})
		})
		a.section("CONTROLS", func() {
			ui.Input("name", &s.name, qui.InputOptions{Style: qui.Style{W: qui.Fill()}, Placeholder: "Your name"})
			ui.Input("secret", &s.secret, qui.InputOptions{Style: qui.Style{W: qui.Fill()}, Placeholder: "Password", Password: true})
			a.switchRow("notify", "Send notifications", &s.notify)
			a.switchRow("sync", "Sync across devices", &s.sync)
			a.segmented("density", []string{"Compact", "Cozy", "Roomy"}, &s.density)
			a.slider("volume", "Volume", &s.volume, 0, 1)
			ui.Row(qui.Style{W: qui.Fill(), Gap: 6}, func() {
				if ui.Button("Primary", qui.Style{Bg: qui.Accent, Fg: qui.Strong}).Clicked {
					s.clicks++
				}
				if ui.Button("Default").Clicked {
					s.clicks++
				}
				if ui.Button("Danger", qui.Style{Bg: qui.Danger, Fg: qui.Strong}).Clicked {
					s.clicks++
				}
				ui.Text(fmt.Sprintf("%d clicks", s.clicks), qui.TextStyle{Size: 12, Mono: true, Color: qui.Muted})
			})
		})
		a.section("MOTION", func() {
			for i := 0; i < 3; i++ {
				phase := float32(0.5 + 0.5*math.Sin(a.clock*(0.9+float64(i)*0.4)+float64(i)))
				ui.Stack(qui.Style{W: qui.Fill(), H: qui.Px(6), Bg: qui.Line, Radius: 3, Key: fmt.Sprint("bar", i)}, func() {
					ui.Column(qui.Style{W: qui.Pct(phase * 100), H: qui.Fill(), Bg: qui.Accent2, Radius: 3}, noop)
				})
			}
		})
	})
}

func (a *app) switchRow(key, label string, value *bool) {
	ui := a.ui
	s := &a.widgets
	target := float32(0)
	if *value {
		target = 1
	}
	s.knobs[key] = approach(s.knobs[key], target, 18, a.dt)
	t := s.knobs[key]
	row := ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(24), Align: qui.Center, Gap: 10, Hit: true, Key: key, Label: label}, func() {
		ui.Text(label, qui.TextStyle{W: qui.Fill()})
		bg := qui.Color(qui.Line)
		if *value {
			bg = qui.Accent
		}
		ui.Row(qui.Style{W: qui.Px(34), H: qui.Px(20), Pad: qui.All(2), Bg: bg, Radius: 10}, func() {
			ui.Column(qui.Style{W: qui.FillN(t + 0.001), H: qui.Px(1)}, noop)
			ui.Column(qui.Style{W: qui.Px(16), H: qui.Px(16), Bg: qui.Strong, Radius: 8}, noop)
			ui.Column(qui.Style{W: qui.FillN(1 - t + 0.001), H: qui.Px(1)}, noop)
		})
	})
	if row.Clicked {
		*value = !*value
	}
}

func (a *app) segmented(key string, options []string, selected *int) {
	ui := a.ui
	ui.Row(qui.Style{W: qui.Fill(), Pad: qui.All(3), Gap: 3, Bg: qui.Inset, Radius: 8, Key: key}, func() {
		for i, option := range options {
			bg := qui.Color(ghost)
			if *selected == i {
				bg = qui.Item
			}
			if ui.Button(option, qui.Style{W: qui.Fill(), H: qui.Px(26), Pad: qui.All(0), Bg: bg, Key: option}).Clicked {
				*selected = i
			}
		}
	})
}
