package main

import (
	"fmt"
	"math"
	"math/rand"
	"runtime"

	"github.com/torisetxd/qui"
)

const maxSwarm = 6000

type particle struct{ x, y, vx, vy float32 }

type swarmState struct {
	particles []particle
	count     float32
	w, h      float32
	ramp      [32]qui.Color
	rampLook  int
}

func (s *swarmState) setup() {
	s.count = 1600
	if runtime.GOOS == "js" {
		s.count = 500
	}
	s.rampLook = -1
	s.particles = make([]particle, maxSwarm)
	for i := range s.particles {
		s.particles[i] = particle{rand.Float32(), rand.Float32(), rand.Float32() - 0.5, rand.Float32() - 0.5}
	}
}

func channel(c qui.Color, shift uint) float32 { return float32(uint32(c) >> shift & 255) }

func (s *swarmState) buildRamp(l look, index int) {
	if s.rampLook == index {
		return
	}
	s.rampLook = index
	for i := range s.ramp {
		t := float32(i) / float32(len(s.ramp)-1)
		mix := func(shift uint) uint8 { return uint8(channel(l.cool, shift)*(1-t) + channel(l.warm, shift)*t) }
		s.ramp[i] = qui.RGB(mix(24), mix(16), mix(8))
	}
}

func (s *swarmState) step(dt, targetX, targetY float32, repel bool) {
	count := int(s.count)
	for i := range s.particles[:count] {
		p := &s.particles[i]
		dx, dy := targetX-p.x, targetY-p.y
		distance := float32(math.Sqrt(float64(dx*dx+dy*dy))) + 14
		weight := 0.55 + float32(i%17)*0.05
		pull := 26000 * weight / distance
		if repel {
			pull = -90000 / distance
		}
		p.vx += (dx/distance*pull - dy/distance*230*weight) * dt
		p.vy += (dy/distance*pull + dx/distance*230*weight) * dt
		damping := 1 - 1.4*dt
		p.vx *= damping
		p.vy *= damping
		p.x += p.vx * dt
		p.y += p.vy * dt
		if p.x < 0 {
			p.x, p.vx = 0, -p.vx*0.6
		}
		if p.y < 0 {
			p.y, p.vy = 0, -p.vy*0.6
		}
		if p.x > s.w {
			p.x, p.vx = s.w, -p.vx*0.6
		}
		if p.y > s.h {
			p.y, p.vy = s.h, -p.vy*0.6
		}
	}
}

func (a *app) swarmWindow() {
	ui := a.ui
	s := &a.swarm
	s.buildRamp(looks[a.look], a.look)
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Key: "swarm"}, func() {
		area := ui.Stack(qui.Style{W: qui.Fill(), H: qui.Fill(), Clip: true, Hit: true, Key: "space", Role: qui.RoleImage, Label: "Particle swarm"}, func() {
			for i := range s.particles[:int(s.count)] {
				p := &s.particles[i]
				speed := float32(math.Sqrt(float64(p.vx*p.vx+p.vy*p.vy))) / 8
				shade := s.ramp[min(int(speed), len(s.ramp)-1)]
				ui.Column(qui.Style{Absolute: true, X: p.x, Y: p.y, W: qui.Px(3), H: qui.Px(3), Bg: shade}, noop)
			}
		})
		if area.W > 0 && (area.W != s.w || area.H != s.h) {
			if s.w == 0 {
				for i := range s.particles {
					s.particles[i].x *= area.W
					s.particles[i].y *= area.H
				}
			}
			s.w, s.h = area.W, area.H
		}
		targetX, targetY := s.w/2, s.h/2
		if area.Hovered {
			targetX, targetY = ui.PointerX()-area.X, ui.PointerY()-area.Y
		}
		for i := 0; i < 2 && s.w > 0; i++ {
			s.step(a.dt/2, targetX, targetY, area.Pressed)
		}
		ui.Row(qui.Style{W: qui.Fill(), Pad: qui.XY(10, 6), Gap: 10, Align: qui.Center, Key: "foot"}, func() {
			a.slider("n", "Nodes", &s.count, 100, maxSwarm)
			ui.Text(fmt.Sprintf("%5s", group(int(s.count))), qui.TextStyle{Size: 11.5, Mono: true, Color: qui.Ink})
		})
	})
}
