package main

import (
	"fmt"
	"math"
	"strings"
)

type city struct {
	name     string
	lat, lon float64
}

var cities = []city{
	{"San Francisco", 37.8, -122.4}, {"New York", 40.7, -74.0}, {"Toronto", 43.7, -79.4}, {"Mexico City", 19.4, -99.1},
	{"Sao Paulo", -23.5, -46.6}, {"London", 51.5, -0.1}, {"Frankfurt", 50.1, 8.7}, {"Stockholm", 59.3, 18.1},
	{"Madrid", 40.4, -3.7}, {"Lagos", 6.5, 3.4}, {"Johannesburg", -26.2, 28.0}, {"Dubai", 25.2, 55.3},
	{"Mumbai", 19.1, 72.9}, {"Singapore", 1.4, 103.8}, {"Hong Kong", 22.3, 114.2}, {"Seoul", 37.6, 127.0},
	{"Tokyo", 35.7, 139.7}, {"Sydney", -33.9, 151.2},
}

var routes = [][2]int{
	{0, 1}, {1, 5}, {5, 6}, {6, 11}, {11, 12}, {12, 13}, {13, 14}, {14, 16}, {15, 16}, {16, 0},
	{0, 3}, {3, 4}, {4, 5}, {7, 6}, {8, 9}, {9, 10}, {10, 11}, {13, 17}, {2, 7}, {1, 4},
}

func unit(c city) [3]float64 {
	lat, lon := c.lat*math.Pi/180, c.lon*math.Pi/180
	return [3]float64{math.Cos(lat) * math.Sin(lon), math.Sin(lat), math.Cos(lat) * math.Cos(lon)}
}

func dot3(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func normalize3(a [3]float64) [3]float64 {
	l := math.Sqrt(dot3(a, a))
	return [3]float64{a[0] / l, a[1] / l, a[2] / l}
}

func cross3(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func vec3Array(name string, values [][3]float64) string {
	items := make([]string, len(values))
	for i, v := range values {
		items[i] = fmt.Sprintf("vec3(%.5f,%.5f,%.5f)", v[0], v[1], v[2])
	}
	return fmt.Sprintf("const vec3 %s[%d]=vec3[%d](%s);\n", name, len(values), len(values), strings.Join(items, ","))
}

func geometry() string {
	us, vs, ns := make([][3]float64, len(routes)), make([][3]float64, len(routes)), make([][3]float64, len(routes))
	angles := make([]string, len(routes))
	for i, r := range routes {
		a, b := unit(cities[r[0]]), unit(cities[r[1]])
		cosine := dot3(a, b)
		us[i] = a
		vs[i] = normalize3([3]float64{b[0] - a[0]*cosine, b[1] - a[1]*cosine, b[2] - a[2]*cosine})
		ns[i] = cross3(us[i], vs[i])
		angles[i] = fmt.Sprintf("%.5f", math.Acos(cosine))
	}
	points := make([][3]float64, len(cities))
	for i, c := range cities {
		points[i] = unit(c)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "const int ARCS=%d;\nconst int CITIES=%d;\n", len(routes), len(cities))
	b.WriteString(vec3Array("ARC_U", us))
	b.WriteString(vec3Array("ARC_V", vs))
	b.WriteString(vec3Array("ARC_N", ns))
	fmt.Fprintf(&b, "const float ARC_T[%d]=float[%d](%s);\n", len(routes), len(routes), strings.Join(angles, ","))
	b.WriteString(vec3Array("CITY", points))
	return b.String()
}

const common = `
const float PI = 3.14159265;
const float TAU = 6.28318531;
float hash(vec2 p) {
  vec3 q = fract(vec3(p.xyx) * 0.1031);
  q += dot(q, q.yzx + 33.33);
  return fract((q.x + q.y) * q.z);
}
float noise1(float x) {
  float i = floor(x), f = fract(x);
  f = f * f * (3.0 - 2.0 * f);
  return mix(hash(vec2(i, 0.0)), hash(vec2(i + 1.0, 0.0)), f);
}
vec3 prism(float k) {
  vec3 a = vec3(1.0, 0.48, 0.10), b = vec3(1.0, 0.24, 0.50), c = vec3(0.48, 0.36, 1.0), d = vec3(0.16, 0.83, 1.0);
  k = clamp(k, 0.0, 1.0) * 3.0;
  if (k < 1.0) return mix(a, b, k);
  if (k < 2.0) return mix(b, c, k - 1.0);
  return mix(c, d, k - 2.0);
}
vec3 dither(vec3 c) { return c + (hash(gl_FragCoord.xy) + hash(gl_FragCoord.yx + 17.0) - 1.0) * 2.0 / 255.0; }
`

const globeCode = `
mat3 rotY(float a) { float c = cos(a), s = sin(a); return mat3(c, 0.0, -s, 0.0, 1.0, 0.0, s, 0.0, c); }
mat3 rotX(float a) { float c = cos(a), s = sin(a); return mat3(1.0, 0.0, 0.0, 0.0, c, s, 0.0, -s, c); }
bool isLand(float lat, float lon) {
  int x = int(fract(lon / TAU + 0.5) * 256.0);
  int y = int(clamp(0.5 - lat / PI, 0.0, 0.9999) * 128.0);
  return ((LAND[y * 8 + (x >> 5)] >> uint(x & 31)) & 1u) != 0u;
}
float landDots(vec3 n, float aa) {
  const float rows = 190.0;
  float lat = asin(clamp(n.y, -1.0, 1.0));
  float lon = atan(n.x, n.z);
  float row = floor((lat / PI + 0.5) * rows);
  float rowLat = (row + 0.5) / rows * PI - PI * 0.5;
  float cols = max(1.0, floor(cos(rowLat) * rows * 2.0));
  float col = floor((lon / TAU + 0.5) * cols);
  float colLon = (col + 0.5) / cols * TAU - PI;
  if (!isLand(rowLat, colLon)) return 0.0;
  float d = length(vec2(lat - rowLat, (lon - colLon) * cos(rowLat)));
  float radius = 0.34 * PI / rows;
  return smoothstep(radius + aa, max(radius - aa, 0.0), d);
}
vec4 planet(vec2 q, float R, mat3 rot, float spread, float narrow) {
  float r = length(q);
  float limb = (r - 1.0) * R;
  float angle = atan(q.x, q.y);
  vec3 rim = prism(0.5 + 0.55 * sin(angle) + 0.06 * sin(u_time * 0.2));
  float focus = narrow > 0.0 ? exp(-angle * angle * narrow) : 1.0;
  float inside = smoothstep(0.75, -0.75, limb);
  vec3 col = vec3(0.0);
  if (inside > 0.0) {
    float z = sqrt(max(1.0 - r * r, 0.0));
    vec3 n = rot * vec3(q, z);
    float edge = pow(1.0 - z, 2.4);
    vec3 body = mix(vec3(0.006, 0.006, 0.010), rim * 0.16 * focus, edge * edge);
    float d = landDots(n, 0.85 / (R * max(z, 0.12)));
    vec3 tint = mix(vec3(0.62, 0.64, 0.72), rim * 1.15, pow(1.0 - z, 1.5));
    float light = (0.34 + 0.9 * pow(1.0 - z, 1.6)) * smoothstep(0.0, 0.12, z);
    col = (body + d * tint * light + rim * focus * exp(limb / (R * 0.010)) * 0.75) * inside;
  }
  float above = max(limb, 0.0);
  col += rim * focus * (exp(-above / (R * 0.004)) * 1.0 + exp(-above / (R * spread)) * 0.26) * (1.0 - inside);
  return vec4(col, inside);
}
vec3 arcs(vec2 q, float R, mat3 rot, float dpr) {
  bool overDisc = dot(q, q) < 1.0;
  vec3 ex = rot[0], ey = rot[1], rd = rot[2];
  vec3 ro = ex * q.x + ey * q.y;
  vec3 sum = vec3(0.0);
  for (int i = 0; i < ARCS; i++) {
    vec3 N = ARC_N[i], U = ARC_U[i], V = ARC_V[i];
    float T = ARC_T[i];
    float dn = dot(rd, N);
    if (abs(dn) < 0.004) continue;
    float t = -dot(ro, N) / dn;
    if (overDisc && t < 0.0) continue;
    vec3 x = ro + rd * t;
    float a = dot(x, V), b = dot(x, U);
    float s = atan(a, b) / T;
    if (s < 0.0 || s > 1.0) continue;
    float h = 0.05 + 0.20 * T / PI;
    float len = length(x);
    float f = len - 1.0 - h * sin(PI * s);
    vec3 grad = x / len - (h * PI * cos(PI * s) / T) * (b * V - a * U) / max(a * a + b * b, 1e-5);
    vec2 screen = vec2(dot(grad, ex - rd * dot(ex, N) / dn), dot(grad, ey - rd * dot(ey, N) / dn));
    float w = abs(f) * R / max(length(screen), 1e-4) / dpr;
    if (w > 48.0) continue;
    float head = fract(u_time * (0.09 + 0.02 * float(i % 3)) + float(i) * 0.381) * 1.25 - 0.12;
    float behind = head - s;
    float trail = behind > 0.0 ? exp(-behind * 6.0) : 0.0;
    float spark = exp(-pow((s - head) * 26.0, 2.0));
    vec3 hue = prism(fract(float(i) * 0.37));
    sum += hue * smoothstep(1.4, 0.3, w) * (0.16 + trail * 1.1);
    sum += hue * exp(-w * 0.3) * (trail * 0.22 + spark * 0.8);
    sum += vec3(1.0) * smoothstep(3.2, 0.4, w) * spark;
  }
  return sum;
}
vec3 markers(vec2 q, float R, mat3 rot, float dpr) {
  vec3 sum = vec3(0.0);
  for (int i = 0; i < CITIES; i++) {
    vec3 v = CITY[i] * rot;
    if (v.z < 0.06) continue;
    float d = length(q - v.xy) * R / dpr;
    float pulse = fract(u_time * 0.45 + float(i) * 0.131);
    sum += vec3(1.0, 0.96, 0.92) * smoothstep(2.6, 1.4, d);
    sum += prism(fract(float(i) * 0.23)) * smoothstep(1.4, 0.2, abs(d - pulse * 18.0)) * (1.0 - pulse) * 0.8;
  }
  return sum;
}
`

func globeBase() string {
	return "const uint LAND[1024]=uint[1024](" + landMask + ");\n" + geometry() + common + globeCode
}

var heroShader = "uniform float u_dpr;\nuniform float u_yaw;\n" + globeBase() + `
vec4 shade(vec2 uv) {
  vec2 res = u_resolution;
  vec2 px = vec2((uv.x - 0.5) * res.x, (1.0 - uv.y) * res.y);
  float R = max(res.x * 0.72, res.y * 0.82);
  vec2 q = (px - vec2(0.0, -R + res.y * 0.34)) / R;
  mat3 rot = rotY(u_yaw) * rotX(0.42);
  vec2 cell = floor(px / (3.0 * u_dpr));
  float twinkle = 0.45 + 0.55 * sin(u_time * (0.5 + hash(cell + 7.0) * 2.0) + hash(cell + 3.0) * 40.0);
  vec3 col = vec3(step(0.9988, hash(cell)) * twinkle * 0.4);
  vec4 body = planet(q, R, rot, 0.085, 2.1);
  col = col * (1.0 - body.a) + body.rgb;
  float angle = atan(q.x, q.y);
  float above = max((length(q) - 1.0) * R, 0.0);
  float breathe = 0.85 + 0.15 * sin(u_time * 0.35);
  col += prism(0.5 + 0.55 * sin(angle)) * exp(-above / (R * 0.20)) * 0.20 * breathe * exp(-angle * angle * 7.0) * (1.0 - body.a);
  col += arcs(q, R, rot, u_dpr);
  col += markers(q, R, rot, u_dpr);
  return vec4(dither(col), 1.0);
}
`

var globeShader = "uniform float u_dpr;\nuniform float u_yaw;\nuniform float u_tilt;\nuniform vec3 u_focus;\n" + globeBase() + `
vec4 shade(vec2 uv) {
  vec2 res = u_resolution;
  vec2 px = (vec2(uv.x, 1.0 - uv.y) - 0.5) * res;
  float R = 0.40 * min(res.x, res.y);
  vec2 q = px / R;
  mat3 rot = rotY(u_yaw) * rotX(u_tilt);
  vec4 body = planet(q, R, rot, 0.10, 0.0);
  vec3 col = body.rgb + arcs(q, R, rot, u_dpr) + markers(q, R, rot, u_dpr);
  vec3 focus = u_focus * rot;
  if (focus.z > 0.06) {
    float d = length(q - focus.xy) * R / u_dpr;
    float pulse = fract(u_time * 0.8);
    col += vec3(1.0) * smoothstep(5.5, 4.0, d);
    col += vec3(1.0, 0.65, 0.3) * smoothstep(1.6, 0.2, abs(d - 10.0 - pulse * 26.0)) * (1.0 - pulse);
  }
  return vec4(dither(col), 1.0);
}
`

const waveShader = common + `
uniform float u_dpr;
uniform float u_p;
uniform float u_live;
vec4 shade(vec2 uv) {
  float cell = 13.0 * u_dpr;
  vec2 px = uv * u_resolution;
  vec2 id = floor(px / cell);
  vec2 f = fract(px / cell) - 0.5;
  float box = smoothstep(0.08, 0.02, length(max(abs(f) - 0.22, 0.0)));
  vec2 grid = u_resolution / cell;
  vec2 from = (id + 0.5 - grid * 0.5) / max(grid.x * 0.5, grid.y * 0.5);
  float dist = length(from * vec2(1.0, 1.0));
  float wave = u_p * 1.25;
  float lit = smoothstep(wave, wave - 0.16, dist);
  float front = u_p > 0.001 && u_p < 0.999 ? exp(-pow((dist - wave) * 12.0, 2.0)) : 0.0;
  float tw = hash(id);
  float sparkle = u_live > 0.5 ? 0.55 + 0.45 * sin(u_time * (1.0 + 3.0 * tw) + tw * 40.0) : 1.0;
  vec3 on = prism(clamp(dist * 0.9 + 0.1 * tw, 0.0, 1.0));
  vec3 col = mix(vec3(0.075 + 0.05 * tw), on * (0.45 + 0.55 * sparkle), lit) + on * front * 1.2;
  return vec4(dither(col * box), box);
}
`

const attackShader = common + `
uniform float u_dpr;
uniform float u_ready;
float attack(float x, float t) {
  float base = 0.13 + 0.05 * noise1(x * 16.0 + t * 0.35) + 0.03 * noise1(x * 61.0 + t * 1.1);
  float big = exp(-pow((x - 0.63) * 6.5, 2.0)) * (0.60 + 0.12 * noise1(x * 40.0 + t));
  float small = exp(-pow((x - 0.27) * 16.0, 2.0)) * 0.24;
  return base + big + small;
}
vec4 shade(vec2 uv) {
  float x = uv.x, y = 1.0 - uv.y;
  float t = u_time * 0.55;
  float a = attack(x, t) * u_ready;
  float clean = (0.085 + 0.022 * noise1(x * 22.0 + t * 0.8)) * u_ready;
  float eps = 1.0 / u_resolution.y;
  float grid = 0.0;
  for (int i = 1; i < 4; i++) grid += smoothstep(eps * 1.2, 0.0, abs(y - float(i) * 0.25));
  vec3 col = vec3(0.075) * grid;
  float aw = eps * 1.5 * u_dpr * sqrt(1.0 + pow(dFdx(a) * u_resolution.y, 2.0));
  vec3 hot = mix(vec3(1.0, 0.48, 0.10), vec3(1.0, 0.24, 0.50), clamp(a / 0.8, 0.0, 1.0));
  if (y < a && y > clean) col += hot * (0.05 + 0.34 * pow(y / max(a, 0.001), 2.2));
  col += hot * smoothstep(aw, aw * 0.25, abs(y - a)) * 1.2;
  col += hot * exp(-abs(y - a) * 60.0) * 0.20;
  if (y < clean) col = mix(col, vec3(0.62, 0.70, 0.82) * 0.26, 0.85);
  float cw = eps * 1.5 * u_dpr;
  col += vec3(0.82, 0.88, 0.98) * smoothstep(cw, cw * 0.25, abs(y - clean));
  return vec4(dither(col), 1.0);
}
`

const spotShader = common + `
uniform float u_a;
vec4 shade(vec2 uv) {
  float d = length(uv - 0.5) * 2.0;
  float g = smoothstep(1.0, 0.0, d);
  float a = max(g * g * 0.13 * u_a + (hash(gl_FragCoord.xy) - 0.5) * 2.0 / 255.0 * smoothstep(0.0, 0.1, g), 0.0);
  return vec4(vec3(1.0, 0.62, 0.36) * a, a);
}
`

const borderShader = common + `
vec4 shade(vec2 uv) {
  vec2 p = (uv - 0.5) * u_resolution;
  float a = fract(atan(p.x, -p.y) / TAU + 0.5 - u_time * 0.11);
  float arc = smoothstep(0.0, 0.32, a) * smoothstep(0.62, 0.34, a);
  return vec4(prism(a / 0.62) * arc + vec3(0.13) * (1.0 - arc), 1.0);
}
`

const horizonShader = common + `
vec4 shade(vec2 uv) {
  vec2 p = vec2((uv.x - 0.5) * u_resolution.x / u_resolution.y, 1.0 - uv.y);
  const float R = 2.6;
  float limb = length(p - vec2(0.0, -R + 0.18)) - R;
  float angle = atan(p.x, p.y + R - 0.18);
  float above = max(limb, 0.0);
  vec3 rim = prism(0.5 + angle * 1.1 + 0.05 * sin(u_time * 0.25));
  float focus = exp(-angle * angle * 5.0);
  vec3 col = rim * focus * (exp(-above / 0.005) * 1.0 + exp(-above / 0.22) * 0.30);
  if (limb < 0.0) col = rim * focus * exp(limb / 0.016) * 0.6;
  return vec4(dither(col), 1.0);
}
`
