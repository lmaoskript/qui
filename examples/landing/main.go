// A landing page for Sable, a made-up deployment platform.
//
//	go run ./examples/landing
//	go run ./examples/landing/serve
package main

import (
	"math"
	"strings"
	"time"

	"github.com/torisetxd/qui"
)

var (
	black   = qui.RGB(0, 0, 0)
	panel   = qui.RGB(9, 9, 11)
	raised  = qui.RGB(24, 24, 27)
	white   = qui.RGB(255, 255, 255)
	ink     = qui.RGB(237, 237, 237)
	muted   = qui.RGB(143, 143, 143)
	subtle  = qui.RGB(92, 92, 96)
	hair    = qui.RGBA(255, 255, 255, 28)
	hairHot = qui.RGBA(255, 255, 255, 70)
	orange  = qui.RGB(255, 122, 26)
	magenta = qui.RGB(255, 61, 129)
	violet  = qui.RGB(122, 92, 255)
	cyan    = qui.RGB(41, 211, 255)
	lime    = qui.RGB(190, 242, 100)
)

type tracked struct {
	y     float32
	found bool
}

type revealState struct {
	mark     tracked
	on       bool
	progress float32
}

type app struct {
	ui            *qui.Ui
	start         time.Time
	t, dt         float32
	w, h, dpr     float32
	inner, margin float32
	narrow        bool
	scroll        float32
	spy           string
	anchorOn      bool
	anchorFrom    float32
	anchorTo      float32
	anchorT       float32
	marks         map[string]*tracked
	reveals       map[string]*revealState
	hover         map[string]bool
	deployClock   float32
	bars          float32
	rollback      int
	rollbackFlash float32
	bentoHover    float32
	bentoRect     qui.Response
	coldRect      qui.Response
	globe         globeState
	annual        bool
	annualT       float32
	email         string
	joined        bool
}

func newApp(ui *qui.Ui) *app {
	return &app{
		ui: ui, start: time.Now(), spy: "", rollback: 5,
		marks: map[string]*tracked{}, reveals: map[string]*revealState{}, hover: map[string]bool{},
		globe: newGlobe(),
	}
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }

func easeOut(t float32) float32 {
	t = 1 - clamp(t, 0, 1)
	return 1 - t*t*t
}

func easeInOut(t float32) float32 {
	t = clamp(t, 0, 1)
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := 2*t - 2
	return 1 + u*u*u/2
}

func approach(current, target, rate, dt float32) float32 {
	return current + (target-current)*(1-float32(math.Exp(float64(-rate*dt))))
}

func alpha(c qui.Color, a float32) qui.Color {
	base := uint32(c)
	return qui.Color(base&^0xff | uint32(float32(base&0xff)*clamp(a, 0, 1)+0.5))
}

func (a *app) mark(key string) *tracked {
	m := a.marks[key]
	if m == nil {
		m = &tracked{y: 1e6}
		a.marks[key] = m
	}
	r := a.ui.Column(qui.Style{Key: key, W: qui.Fill(), Hit: true}, func() {})
	if r.Found {
		m.y, m.found = r.Y, true
	}
	return m
}

func (a *app) visible(key string, height float32) bool {
	m := a.marks[key]
	return m == nil || !m.found || (m.y < a.h && m.y+height > 0)
}

func (a *app) fps(key string, height float32) float32 {
	if a.visible(key, height) {
		return 60
	}
	return 0
}

func (a *app) reveal(key string, s qui.Style, body func()) {
	state := a.reveals[key]
	if state == nil {
		state = &revealState{}
		a.reveals[key] = state
	}
	if state.mark.found && state.mark.y < a.h*0.92 {
		state.on = true
	}
	if state.on {
		state.progress = min(1, state.progress+a.dt/0.75)
	}
	e := easeOut(state.progress)
	const travel = 22
	s.Key = key
	s.Opacity = max(0.001, e)
	s.Pad.Top += (1 - e) * travel
	s.Pad.Bottom += e * travel
	a.ui.Column(s, func() {
		r := a.ui.Column(qui.Style{Key: "at", Absolute: true, X: 0, Y: 0, W: qui.Px(1), H: qui.Px(1), Hit: true}, func() {})
		if r.Found {
			state.mark.y, state.mark.found = r.Y, true
		}
		body()
	})
}

func (a *app) revealed(key string) float32 {
	if s := a.reveals[key]; s != nil {
		return easeOut(s.progress)
	}
	return 0
}

func (a *app) label(text string, color ...qui.Color) {
	c := muted
	if len(color) > 0 {
		c = color[0]
	}
	a.ui.Text(strings.ToUpper(text), qui.TextStyle{Size: 11, Mono: true, Color: c, Tracking: 0.8})
}

func (a *app) statement(first, second string) {
	size := clamp(a.w*0.04, 30, 54)
	track := -size * 0.036
	a.ui.Text(first, qui.TextStyle{Size: size, Color: ink, Tracking: track, W: qui.Fill(), Role: qui.RoleHeading})
	if second != "" {
		a.ui.Text(second, qui.TextStyle{Size: size, Color: subtle, Tracking: track, W: qui.Fill()})
	}
}

func (a *app) button(label string, s qui.Style, hot qui.Color) qui.Response {
	key := s.Key
	if key == "" {
		key = label
		s.Key = label
	}
	if a.hover[key] {
		s.Bg = hot
	}
	r := a.ui.Button(label, s)
	a.hover[key] = r.Hovered
	return r
}

func (a *app) primary(label string) qui.Response {
	s := qui.Style{Bg: white, Fg: black, Radius: 8, Pad: qui.XY(18, 10)}
	if a.hover[label] {
		s.Shadow = qui.BoxShadow{Color: qui.RGBA(255, 180, 120, 120), Blur: 26}
	}
	return a.button(label, s, qui.RGB(232, 232, 232))
}

func (a *app) secondary(label string) qui.Response {
	return a.button(label, qui.Style{Bg: raised, Fg: ink, Radius: 8, Pad: qui.XY(18, 10), Border: hair}, qui.RGB(38, 38, 42))
}

func (a *app) spacer() { a.ui.Column(qui.Style{W: qui.Fill()}, func() {}) }

func (a *app) rule() {
	ui := a.ui
	ui.Stack(qui.Style{W: qui.Fill(), H: qui.Px(1), Bg: hair, Align: qui.Center}, func() {
		ui.Column(qui.Style{W: qui.Px(a.inner), H: qui.Px(1)}, func() {
			for _, x := range []float32{0, a.inner - 1} {
				ui.Column(qui.Style{Absolute: true, X: x - 5, Y: 0, W: qui.Px(11), H: qui.Px(1), Bg: hairHot}, func() {})
				ui.Column(qui.Style{Absolute: true, X: x, Y: -5, W: qui.Px(1), H: qui.Px(11), Bg: hairHot}, func() {})
			}
		})
	})
}

func (a *app) section(key string, body func()) {
	ui := a.ui
	a.mark(key)
	a.rule()
	ui.Row(qui.Style{Key: key + ".frame", W: qui.Fill(), Justify: qui.Center}, func() {
		ui.Column(qui.Style{W: qui.Px(a.inner)}, func() {
			ui.Column(qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Px(1), H: qui.Fill(), Bg: hair}, func() {})
			ui.Column(qui.Style{Absolute: true, X: a.inner - 1, Y: 0, W: qui.Px(1), H: qui.Fill(), Bg: hair}, func() {})
			body()
		})
	})
}

func (a *app) header(key, index, name, first, second string) {
	padX := float32(40)
	if a.narrow {
		padX = 20
	}
	a.reveal(key+".head", qui.Style{W: qui.Fill(), Pad: qui.Edges{Top: 72, Left: padX, Right: padX, Bottom: 34}, Gap: 18}, func() {
		a.ui.Row(qui.Style{Gap: 10, Align: qui.Center}, func() {
			a.ui.Row(qui.Style{Pad: qui.XY(5, 1), Bg: white}, func() {
				a.ui.Text(index, qui.TextStyle{Size: 11, Mono: true, Color: black})
			})
			a.label(name)
		})
		a.statement(first, second)
	})
}

func (a *app) goTo(key string) {
	m := a.marks[key]
	if m == nil {
		return
	}
	a.anchorOn = true
	a.anchorFrom = a.scroll
	a.anchorTo = max(0, a.scroll+m.y-63)
	a.anchorT = 0
}

func (a *app) scrollStyle() *float32 {
	if !a.anchorOn {
		return nil
	}
	a.anchorT += a.dt / 0.8
	if a.anchorT >= 1 {
		a.anchorOn = false
	}
	v := a.anchorFrom + (a.anchorTo-a.anchorFrom)*easeInOut(a.anchorT)
	return &v
}

func (a *app) frame() {
	ui := a.ui
	a.t = float32(time.Since(a.start).Seconds())
	a.dt = clamp(ui.Dt(), 0.001, 0.05)
	a.w, a.h, a.dpr = ui.Width(), ui.Height(), ui.Dpr()
	a.narrow = a.w < 860
	a.margin = 32
	if a.narrow {
		a.margin = 12
	}
	a.inner = min(a.w-a.margin*2, 1200)
	for _, name := range []string{"platform", "network", "pricing"} {
		if m := a.marks[name]; m != nil && m.y < 140 {
			a.spy = name
		}
	}
	if m := a.marks["platform"]; m == nil || m.y >= 140 {
		a.spy = ""
	}
	ui.Stack(qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: black}, func() {
		ui.Column(qui.Style{Key: "page", W: qui.Fill(), H: qui.Fill(), Scroll: true, ScrollTo: a.scrollStyle(), Role: qui.RoleMain}, func() {
			top := ui.Column(qui.Style{Key: "top", W: qui.Fill(), Hit: true}, func() {})
			if top.Found {
				a.scroll = -top.Y
			}
			a.hero()
			a.stats()
			a.deploy()
			a.bento()
			a.network()
			a.pricing()
			a.finalCta()
			a.footer()
		})
		a.nav()
	})
}

func main() {
	ui := qui.Open("Sable", 1440, 900)
	ui.SetTheme(qui.Neutral)
	go func() {
		t := time.NewTicker(time.Second / 60)
		defer t.Stop()
		for range t.C {
			ui.Wake()
		}
	}()
	a := newApp(ui)
	for ui.Frame() {
		a.frame()
	}
}
