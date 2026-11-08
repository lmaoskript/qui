package main

import (
	"fmt"
	"math"

	"github.com/torisetxd/qui"
)

type globeState struct {
	yaw, tilt, vx, vy     float32
	focus                 int
	aiming                bool
	targetYaw, targetTilt float32
}

func newGlobe() globeState { return globeState{yaw: -1.1, tilt: -0.5, focus: 5} }

var pops = []struct {
	city int
	ms   string
	note string
}{
	{0, "9 ms", "142 Tbit"}, {1, "7 ms", "210 Tbit"}, {5, "6 ms", "260 Tbit"}, {6, "8 ms", "240 Tbit"},
	{11, "14 ms", "88 Tbit"}, {13, "12 ms", "124 Tbit"}, {16, "9 ms", "172 Tbit"}, {4, "19 ms", "76 Tbit"},
}

func radians(degrees float64) float32 { return float32(degrees * math.Pi / 180) }

func (a *app) network() {
	ui := a.ui
	a.section("network", func() {
		a.header("net", "03", "Network", "One network, 310 cities.", "Drag the planet. Requests land on the nearest one.")
		a.divider(false)
		const wideHeight = 620
		height := float32(wideHeight)
		if a.narrow {
			height = 380
		}
		if a.narrow {
			a.globeView(height, qui.Fill())
			a.divider(false)
			a.regionList(qui.Fill())
			return
		}
		ui.Row(qui.Style{W: qui.Fill()}, func() {
			a.globeView(height, qui.FillN(7))
			a.divider(true)
			a.regionList(qui.FillN(4))
		})
	})
}

func (a *app) globeView(height float32, w qui.Size) {
	ui := a.ui
	g := &a.globe
	r := ui.Stack(qui.Style{Key: "globe", W: w, H: qui.Px(height), Hit: true, Role: qui.RoleImage, Label: "Interactive globe of the Sable network"}, func() {
		c := unit(cities[pops[g.focus].city])
		ui.Shader(qui.ShaderOptions{
			Src: globeShader, Style: qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: black, Key: "planet"}, Fps: a.fps("network", height+300),
			Uniforms: []qui.Uniform{
				{Name: "u_dpr", Values: []float32{a.dpr}}, {Name: "u_yaw", Values: []float32{g.yaw}}, {Name: "u_tilt", Values: []float32{g.tilt}},
				{Name: "u_focus", Values: []float32{float32(c[0]), float32(c[1]), float32(c[2])}},
			},
		})
		ui.Row(qui.Style{Absolute: true, X: 20, Y: 18, Gap: 8, Align: qui.Center}, func() {
			ui.Column(qui.Style{W: qui.Px(6), H: qui.Px(6), Radius: 3, Bg: lime}, func() {})
			a.label("all regions operational")
		})
	})
	radius := 0.40 * min(r.W, r.H)
	if radius < 40 {
		radius = 200
	}
	dt := a.dt
	switch {
	case r.Dragging:
		g.yaw -= r.DX / radius
		g.tilt = clamp(g.tilt-r.DY/radius, -1.3, 1.3)
		g.vx, g.vy = -r.DX/radius/dt, -r.DY/radius/dt
		g.aiming = false
	case g.aiming:
		for g.targetYaw-g.yaw > math.Pi {
			g.targetYaw -= 2 * math.Pi
		}
		for g.targetYaw-g.yaw < -math.Pi {
			g.targetYaw += 2 * math.Pi
		}
		g.yaw = approach(g.yaw, g.targetYaw, 5, dt)
		g.tilt = approach(g.tilt, g.targetTilt, 5, dt)
		if abs32(g.targetYaw-g.yaw)+abs32(g.targetTilt-g.tilt) < 0.002 {
			g.aiming = false
		}
	default:
		g.yaw += g.vx*dt + 0.09*dt
		g.tilt = clamp(g.tilt+g.vy*dt, -1.3, 1.3)
		g.vx = approach(g.vx, 0, 2.6, dt)
		g.vy = approach(g.vy, 0, 2.6, dt)
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func (a *app) regionList(w qui.Size) {
	ui := a.ui
	g := &a.globe
	ui.Column(qui.Style{W: w, Pad: qui.XY(0, 8)}, func() {
		for i, pop := range pops {
			c := cities[pop.city]
			key := fmt.Sprint("pop", i)
			bg := qui.Color(0)
			if i == g.focus {
				bg = qui.RGBA(255, 255, 255, 14)
			} else if a.hover[key] {
				bg = qui.RGBA(255, 255, 255, 8)
			}
			r := ui.Row(qui.Style{Key: key, W: qui.Fill(), Hit: true, Align: qui.Center, Gap: 12, Pad: qui.XY(a.pad()-14, 14), Bg: bg}, func() {
				dot := hairHot
				if i == g.focus {
					dot = orange
				}
				ui.Column(qui.Style{W: qui.Px(6), H: qui.Px(6), Radius: 3, Bg: dot}, func() {})
				color := muted
				if i == g.focus {
					color = ink
				}
				ui.Text(c.name, qui.TextStyle{Size: 15, Color: color, W: qui.Fill()})
				ui.Text(pop.ms, qui.TextStyle{Size: 12.5, Mono: true, Color: muted})
			})
			a.hover[key] = r.Hovered
			if r.Clicked {
				g.focus = i
				g.aiming = true
				g.targetYaw = radians(c.lon)
				g.targetTilt = -radians(c.lat)
				g.vx, g.vy = 0, 0
			}
			ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(1), Bg: alpha(hair, 0.55)}, func() {})
		}
	})
}
