package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/torisetxd/qui"
)

const chartPoints = 64

type chartState struct {
	signature string
	source    string
	progress  float32
	from, to  [2][chartPoints]float32
}

type chartData struct {
	current, previous []float64
	labels            []string
	format            func(float64) string
	end               float32
	floor             float64
}

func niceMax(v float64) float64 {
	if v <= 0 {
		return 1
	}
	power := math.Pow(10, math.Floor(math.Log10(v)))
	for _, step := range []float64{1, 1.5, 2, 2.5, 3, 4, 5, 7.5, 10} {
		if v <= step*power {
			return step * power
		}
	}
	return 10 * power
}

func resample(values []float64, floor, limit float64) [chartPoints]float32 {
	var out [chartPoints]float32
	if len(values) == 0 {
		return out
	}
	for i := range out {
		at := float64(i) / (chartPoints - 1) * float64(len(values)-1)
		low := int(at)
		high := min(low+1, len(values)-1)
		out[i] = float32((values[low] + (values[high]-values[low])*(at-float64(low)) - floor) / (limit - floor))
	}
	return out
}

func glslArray(name string, values *[chartPoints]float32) string {
	items := make([]string, chartPoints)
	for i, v := range values {
		items[i] = fmt.Sprintf("%.4f", v)
	}
	return fmt.Sprintf("const float %s[%d]=float[%d](%s);\n", name, chartPoints, chartPoints, strings.Join(items, ","))
}

const chartBody = `
uniform float u_p;
uniform float u_dpr;
uniform float u_end;
uniform float u_prev;
uniform float u_wash;
uniform vec3 u_line;
const int N = 64;
float va(int i) { return mix(A0[i], A1[i], u_p); }
float vb(int i) { return mix(B0[i], B1[i], u_p); }
float seg(vec2 p, vec2 a, vec2 b) {
  vec2 ab = b - a;
  return length(p - a - ab * clamp(dot(p - a, ab) / max(dot(ab, ab), 1e-4), 0.0, 1.0));
}
vec4 shade(vec2 uv) {
  vec2 res = u_resolution;
  vec2 p = uv * res;
  float dx = res.x / float(N - 1);
  float margin = 3.0 * u_dpr;
  float H = res.y - margin * 2.0;
  int c = int(floor(p.x / dx));
  float da = 1e9, db = 1e9;
  for (int i = max(c - 2, 0); i <= min(c + 2, N - 2); i++) {
    vec2 a0 = vec2(float(i) * dx, margin + (1.0 - va(i)) * H);
    vec2 a1 = vec2(float(i + 1) * dx, margin + (1.0 - va(i + 1)) * H);
    da = min(da, seg(p, a0, a1));
    vec2 b0 = vec2(float(i) * dx, margin + (1.0 - vb(i)) * H);
    vec2 b1 = vec2(float(i + 1) * dx, margin + (1.0 - vb(i + 1)) * H);
    db = min(db, seg(p, b0, b1));
  }
  int k = clamp(c, 0, N - 2);
  float f = clamp(p.x / dx - float(k), 0.0, 1.0);
  float curve = margin + (1.0 - mix(va(k), va(k + 1), f)) * H;
  float inRange = step(p.x, u_end * res.x + 0.5 * u_dpr);
  float covA = smoothstep(u_dpr + 0.6, u_dpr - 0.6, da) * inRange;
  float covB = smoothstep(0.75 * u_dpr + 0.6, 0.75 * u_dpr - 0.6, db) * u_prev;
  float wash = p.y > curve ? 0.16 * u_wash * (1.0 - clamp((p.y - curve) / max(res.y - curve, 1.0), 0.0, 1.0)) * inRange : 0.0;
  vec3 grey = vec3(0.42, 0.46, 0.53);
  float alpha = covA + (1.0 - covA) * (covB * 0.85 + (1.0 - covB * 0.85) * wash);
  vec3 col = u_line * covA + (1.0 - covA) * (grey * covB * 0.85 + (1.0 - covB * 0.85) * u_line * wash);
  return vec4(col, alpha);
}
`

func colorVec(c qui.Color) []float32 {
	v := uint32(c)
	return []float32{float32(v>>24) / 255, float32(v>>16&0xff) / 255, float32(v>>8&0xff) / 255}
}

func (a *app) chart(key string, height float32, d chartData, showAxis bool) {
	ui := a.ui
	state := a.charts[key]
	if state == nil {
		state = &chartState{progress: 1}
		a.charts[key] = state
	}
	limit := 0.0
	for _, v := range d.current {
		limit = math.Max(limit, v)
	}
	for _, v := range d.previous {
		limit = math.Max(limit, v)
	}
	if d.floor > 0 {
		limit *= 1.0005
	} else {
		limit = niceMax(limit * 1.04)
	}
	signature := fmt.Sprint(limit, d.floor, d.current, d.previous)
	if signature != state.signature {
		var shown [2][chartPoints]float32
		for s := range shown {
			for i := range shown[s] {
				shown[s][i] = state.from[s][i] + (state.to[s][i]-state.from[s][i])*state.progress
			}
		}
		state.from = shown
		state.to[0] = resample(d.current, d.floor, limit)
		state.to[1] = resample(d.previous, d.floor, limit)
		state.signature = signature
		state.progress = 0
		state.source = glslArray("A0", &state.from[0]) + glslArray("A1", &state.to[0]) + glslArray("B0", &state.from[1]) + glslArray("B1", &state.to[1]) + chartBody
	}
	state.progress = min(1, state.progress+a.dt/0.45)
	end := d.end
	if end == 0 {
		end = 1
	}
	prev := float32(0)
	if len(d.previous) > 0 {
		prev = 1
	}
	wash := float32(1)
	if d.floor > 0 {
		wash = 0
	}
	r := a.chartRects[key]
	rect := ui.Stack(qui.Style{Key: key, W: qui.Fill(), H: qui.Px(height), Hit: true, Role: qui.RoleImage, Label: "Line chart"}, func() {
		if showAxis {
			for _, f := range []float32{0, 0.5, 1} {
				y := 3 + (height-6)*f
				ui.Column(qui.Style{Absolute: true, X: 0, Y: y, W: qui.Fill(), H: qui.Px(1), Bg: gridLine}, func() {})
				if f < 1 {
					ui.Row(qui.Style{Absolute: true, X: 0, Y: y + 2}, func() {
						ui.Text(d.format(float64(1-f)*limit), qui.TextStyle{Size: 10.5, Color: subtle})
					})
				}
			}
		} else {
			ui.Column(qui.Style{Absolute: true, X: 0, Y: height - 3, W: qui.Fill(), H: qui.Px(1), Bg: gridLine}, func() {})
		}
		ui.Shader(qui.ShaderOptions{
			Src: state.source, Style: qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Fill(), H: qui.Fill(), Key: "line"}, Fps: 60,
			Uniforms: []qui.Uniform{
				{Name: "u_p", Values: []float32{easeOut(state.progress)}}, {Name: "u_dpr", Values: []float32{a.dpr}},
				{Name: "u_end", Values: []float32{end}}, {Name: "u_prev", Values: []float32{prev}}, {Name: "u_wash", Values: []float32{wash}}, {Name: "u_line", Values: colorVec(seriesOne)},
			},
		})
		if end < 1 && r.W > 0 && len(d.current) > 1 {
			at := float64(end) * float64(len(d.current)-1)
			low := min(int(at), len(d.current)-2)
			value := d.current[low] + (d.current[low+1]-d.current[low])*(at-float64(low))
			ui.Column(qui.Style{Absolute: true, X: r.W*end - 5, Y: 3 + (height-6)*(1-float32(value/limit)) - 5, W: qui.Px(10), H: qui.Px(10), Radius: 5, Bg: seriesOne, Border: surface, BorderW: 2}, func() {})
		}
		a.crosshair(key, r, height, limit, d, end)
	})
	a.chartRects[key] = rect
}

func (a *app) crosshair(key string, r qui.Response, height float32, limit float64, d chartData, end float32) {
	ui := a.ui
	if !r.Hovered || r.W <= 0 || len(d.current) == 0 {
		return
	}
	span := len(d.current) - 1
	frac := clamp((ui.PointerX()-r.X)/r.W, 0, 1)
	if end < 1 {
		frac = min(frac, end)
	}
	index := int(float32(span)*frac + 0.5)
	x := r.W * float32(index) / float32(max(span, 1))
	y := func(v float64) float32 { return 3 + (height-6)*(1-float32((v-d.floor)/(limit-d.floor))) }
	ui.Column(qui.Style{Absolute: true, X: x, Y: 0, W: qui.Px(1), H: qui.Fill(), Bg: lineHot}, func() {})
	marker := func(v float64, c qui.Color) {
		ui.Column(qui.Style{Absolute: true, X: x - 5, Y: y(v) - 5, W: qui.Px(10), H: qui.Px(10), Radius: 5, Bg: c, Border: surface, BorderW: 2}, func() {})
	}
	if len(d.previous) > index {
		marker(d.previous[index], seriesGrey)
	}
	marker(d.current[index], seriesOne)
	label := ""
	if index < len(d.labels) {
		label = d.labels[index]
	}
	const tipW = 158
	tipX := x + 12
	if tipX+tipW > r.W {
		tipX = x - tipW - 12
	}
	ui.Column(qui.Style{Absolute: true, X: tipX, Y: 4, W: qui.Px(tipW), Z: 5, Pad: qui.XY(10, 8), Gap: 4, Bg: raised, Border: line, Radius: 8, Shadow: qui.BoxShadow{Color: qui.RGBA(0, 0, 0, 150), Blur: 16, Y: 6}}, func() {
		ui.Text(label, qui.TextStyle{Size: 11, Color: muted})
		ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Gap: 7}, func() {
			ui.Column(qui.Style{W: qui.Px(10), H: qui.Px(2), Bg: seriesOne}, func() {})
			ui.Text(d.format(d.current[index]), qui.TextStyle{Size: 13, Bold: true, Color: ink})
		})
		if len(d.previous) > index {
			ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Gap: 7}, func() {
				ui.Column(qui.Style{W: qui.Px(10), H: qui.Px(2), Bg: seriesGrey}, func() {})
				ui.Text(d.format(d.previous[index]), qui.TextStyle{Size: 12, Color: muted})
			})
		}
	})
}
