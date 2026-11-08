// A small desktop built entirely from qui widgets. The same source runs as a
// native window and, compiled to wasm, as real DOM elements.
package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/torisetxd/qui"
)

const appID = "dev.qui.showcase"

const (
	topBarH = 40
	dockH   = 66
)

var (
	ghost = qui.RGBA(0, 0, 0, 1)
	noop  = func() {}
)

type look struct {
	name             string
	theme            qui.Theme
	glass, title     qui.Color
	swatch           qui.Color
	wall1, wall2, c3 [3]float32
	warm, cool       qui.Color
}

var looks = []look{
	{"Slate", qui.Slate, qui.RGBA(14, 16, 21, 196), qui.RGBA(255, 255, 255, 9), qui.RGB(82, 148, 245),
		[3]float32{0.035, 0.04, 0.06}, [3]float32{0.06, 0.2, 0.42}, [3]float32{0.42, 0.72, 0.98}, qui.RGB(225, 240, 255), qui.RGB(60, 100, 220)},
	{"Midnight", qui.Midnight, qui.RGBA(8, 8, 18, 196), qui.RGBA(255, 255, 255, 9), qui.RGB(150, 110, 245),
		[3]float32{0.025, 0.02, 0.06}, [3]float32{0.2, 0.08, 0.42}, [3]float32{0.82, 0.42, 0.95}, qui.RGB(255, 190, 250), qui.RGB(110, 70, 240)},
	{"Warm", qui.Warm, qui.RGBA(24, 17, 13, 196), qui.RGBA(255, 255, 255, 9), qui.RGB(236, 140, 72),
		[3]float32{0.07, 0.04, 0.03}, [3]float32{0.42, 0.16, 0.06}, [3]float32{0.98, 0.68, 0.32}, qui.RGB(255, 236, 180), qui.RGB(225, 80, 50)},
	{"Neutral", qui.Neutral, qui.RGBA(16, 16, 16, 196), qui.RGBA(255, 255, 255, 9), qui.RGB(180, 180, 180),
		[3]float32{0.04, 0.04, 0.04}, [3]float32{0.16, 0.16, 0.17}, [3]float32{0.7, 0.72, 0.76}, qui.RGB(255, 255, 255), qui.RGB(95, 95, 100)},
}

type app struct {
	ui      *qui.Ui
	windows []*window
	topZ    int32
	look    int
	wall    [9]float32
	wallX   float32
	wallY   float32
	energy  float32
	mosaicW float32
	mosaicH float32
	touched bool
	tiledW  float32
	tiledH  float32

	w, h, dt   float32
	clock      float64
	wasDown    bool
	narrow     bool
	current    *window
	calm       bool
	rate       atomic.Int32
	buildMs    float32
	skipIntro  bool
	fps        float32
	frameCount int
	fpsClock   float64

	palette paletteState
	lab     labState
	rows    rowsState
	chat    chatState
	swarm   swarmState
	monitor monitorState
	widgets widgetState
	capture *capture
}

func clamp(v, low, high float32) float32 {
	return float32(math.Min(float64(high), math.Max(float64(low), float64(v))))
}

func approach(current, target, rate, dt float32) float32 {
	return current + (target-current)*(1-float32(math.Exp(float64(-rate*dt))))
}

func group(n int) string {
	s := fmt.Sprint(n)
	out := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out += ","
		}
		out += string(c)
	}
	return out
}

func (a *app) applyLook(i int) {
	a.look = i
	a.ui.SetTheme(looks[i].theme)
}

func (a *app) stepWall() {
	l := looks[a.look]
	for i := 0; i < 3; i++ {
		a.wall[i] = approach(a.wall[i], l.wall1[i], 7, a.dt)
		a.wall[3+i] = approach(a.wall[3+i], l.wall2[i], 7, a.dt)
		a.wall[6+i] = approach(a.wall[6+i], l.c3[i], 7, a.dt)
	}
}

func (a *app) measure() {
	a.frameCount++
	if a.clock-a.fpsClock >= 0.5 {
		a.fps = float32(float64(a.frameCount) / (a.clock - a.fpsClock))
		a.frameCount, a.fpsClock = 0, a.clock
	}
}

func (a *app) chooseRate() {
	rate := int32(30)
	if a.needsFullRate() {
		rate = 60
	}
	if a.calm && !a.anyMotion() {
		rate = 0
	}
	a.rate.Store(rate)
}

func (a *app) anyMotion() bool {
	for _, w := range a.windows {
		if w.moving() {
			return true
		}
	}
	return a.chat.streaming.Load() || a.capture.live() || a.lab.busy()
}

func (a *app) needsFullRate() bool {
	if a.clock < 1.6 || a.capture.live() {
		return true
	}
	for _, w := range a.windows {
		if w.moving() || (w.id == "swarm" && w.open) {
			return true
		}
	}
	return false
}

func (a *app) ticker() {
	t := time.NewTicker(time.Second / 60)
	defer t.Stop()
	for n := 0; ; n++ {
		<-t.C
		rate := int(a.rate.Load())
		if rate > 0 && n%(60/rate) == 0 {
			a.ui.Wake()
		}
	}
}

func platformName() string {
	if runtime.GOOS == "js" {
		return "web · real DOM"
	}
	return "native · " + runtime.GOOS
}

func main() {
	qui.SetAppID(appID)
	ui := qui.Open("qui showcase", 1440, 900)
	defer ui.Close()

	a := &app{ui: ui, skipIntro: os.Getenv("QUI_SHOT") != ""}
	a.capture = newCapture(ui.Wake)
	a.setup()
	a.applyLook(0)
	for i := 0; i < 3; i++ {
		a.wall[i], a.wall[3+i], a.wall[6+i] = looks[0].wall1[i], looks[0].wall2[i], looks[0].c3[i]
	}
	a.rate.Store(60)
	go a.ticker()

	for ui.Frame() {
		began := time.Now()
		a.w, a.h = ui.Width(), ui.Height()
		a.dt = clamp(ui.Dt(), 0.001, 0.1)
		if a.skipIntro {
			a.dt = 1.0 / 60
		}
		a.clock += float64(a.dt)
		a.narrow = a.w < 860
		a.measure()
		a.stepWall()
		a.energy = approach(a.energy, a.capture.level()*2.4, 14, a.dt)

		if a.w > 0 && !a.touched && (a.w != a.tiledW || a.h != a.tiledH) {
			a.tiledW, a.tiledH = a.w, a.h
			a.tile(false)
		}
		a.handleKeys()
		a.handlePress()

		ui.Stack(qui.Style{W: qui.Fill(), H: qui.Fill(), Key: "root", Role: qui.RoleMain}, func() {
			a.wallpaper()
			a.hero()
			a.drawWindows()
			a.topBar()
			a.dock()
			a.drawPalette()
		})
		a.chooseRate()
		a.buildMs = float32(time.Since(began).Seconds() * 1000)
		a.monitor.record(a.buildMs + ui.Stats().EngineMs)
	}
}
