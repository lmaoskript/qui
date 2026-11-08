package main

const wallpaperShader = `uniform vec3 u_c1, u_c2, u_c3;
uniform vec2 u_pointer;
uniform float u_energy;

float rnd(vec2 p) { return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453); }

float noise(vec2 p) {
  vec2 i = floor(p), f = fract(p), u = f * f * (3.0 - 2.0 * f);
  return mix(mix(rnd(i), rnd(i + vec2(1, 0)), u.x), mix(rnd(i + vec2(0, 1)), rnd(i + vec2(1, 1)), u.x), u.y);
}

float fbm(vec2 p) {
  float value = 0.0, amplitude = 0.5;
  for (int i = 0; i < 4; i++) {
    value += amplitude * noise(p);
    p = p * 2.03 + 7.1;
    amplitude *= 0.5;
  }
  return value;
}

float bayer(ivec2 p) {
  int x = p.x & 7, y = p.y & 7, m = x ^ y;
  int v = ((m & 1) << 5) | ((y & 1) << 4) | ((m & 2) << 2) | ((y & 2) << 1) | ((m & 4) >> 1) | ((y & 4) >> 2);
  return (float(v) + 0.5) / 64.0;
}

vec4 shade(vec2 uv) {
  float aspect = u_resolution.x / max(u_resolution.y, 1.0);
  vec2 st = vec2(uv.x * aspect, uv.y);
  vec2 away = st - vec2(u_pointer.x * aspect, u_pointer.y);
  float pull = exp(-dot(away, away) * 9.0);
  vec2 q = st * 1.55 + away * pull * 0.9;
  float t = u_time * 0.055;
  vec2 warp = vec2(fbm(q + t), fbm(q + vec2(5.2, 1.3) - t * 0.7));
  float n = fbm(q + warp * (1.25 + u_energy * 1.6));
  vec3 col = mix(u_c1, u_c2, smoothstep(0.34, 0.56, n));
  col = mix(col, u_c3, smoothstep(0.56, 0.80, n + pull * 0.05 + u_energy * 0.12));
  col *= 1.0 - 0.32 * dot(uv - 0.5, uv - 0.5) * 2.0;
  vec3 scaled = col * 40.0;
  col = (floor(scaled) + step(vec3(bayer(ivec2(gl_FragCoord.xy))), fract(scaled))) / 40.0;
  return vec4(col, 1.0);
}
`

type preset struct{ name, src string }

var presets = []preset{
	{"Aurora", `uniform float u_a, u_b, u_c;

vec4 shade(vec2 uv) {
  vec2 p = uv * 2.0 - 1.0;
  p.x *= u_resolution.x / u_resolution.y;
  vec3 col = vec3(0.02, 0.03, 0.08);
  for (int i = 0; i < 6; i++) {
    float f = float(i);
    float wave = sin(p.x * (1.2 + f * 0.55) * (0.5 + u_b) + u_time * (0.4 + f * 0.13) * u_a * 2.0);
    float y = wave * 0.26 + 0.5 - f * 0.2;
    float band = exp(-abs(p.y - y) * (5.0 + 16.0 * u_c));
    col += band * (0.5 + 0.5 * cos(vec3(0.0, 2.1, 4.2) + f * 0.7 + u_time * 0.2));
  }
  return vec4(col * 0.8, 1.0);
}
`},
	{"Tunnel", `uniform float u_a, u_b, u_c;

vec4 shade(vec2 uv) {
  vec2 p = (uv - 0.5) * vec2(u_resolution.x / u_resolution.y, 1.0) * 2.0;
  float r = length(p), a = atan(p.y, p.x);
  float z = 0.35 / max(r, 0.02) + u_time * u_a * 1.6;
  float rings = smoothstep(0.32, 0.5, abs(fract(z * (1.0 + u_b * 2.0)) - 0.5));
  float count = 6.0 + floor(u_c * 12.0);
  float spokes = smoothstep(0.4, 0.5, abs(fract(a / 6.2832 * count) - 0.5));
  vec3 tint = 0.55 + 0.45 * cos(vec3(0.2, 1.4, 3.0) + z * 0.6);
  return vec4(tint * (rings + spokes) * smoothstep(0.0, 0.8, r), 1.0);
}
`},
	{"Blobs", `uniform float u_a, u_b, u_c;

vec4 shade(vec2 uv) {
  vec2 p = (uv - 0.5) * vec2(u_resolution.x / u_resolution.y, 1.0) * 2.0;
  float field = 0.0;
  for (int i = 0; i < 8; i++) {
    float f = float(i) * 1.7;
    vec2 c = 0.72 * vec2(sin(u_time * u_a * 1.2 + f), cos(u_time * u_a * 1.5 + f * 1.3));
    field += (0.035 + 0.09 * u_b) / dot(p - c, p - c);
  }
  float body = smoothstep(0.9, 1.1, field);
  float rim = smoothstep(1.0, 1.3, field) - smoothstep(1.3, 2.2, field);
  vec3 col = mix(vec3(0.03, 0.02, 0.07), vec3(0.16, 0.42, 0.95), body);
  col += rim * vec3(1.0, 0.45, 0.8) * (0.4 + u_c);
  return vec4(col, 1.0);
}
`},
	{"Kaleido", `uniform float u_a, u_b, u_c;

vec4 shade(vec2 uv) {
  vec2 p = (uv - 0.5) * vec2(u_resolution.x / u_resolution.y, 1.0) * 2.4;
  float sides = 3.0 + floor(u_c * 9.0);
  float a = atan(p.y, p.x) + u_time * u_a * 0.15;
  float r = length(p);
  a = abs(mod(a, 6.2832 / sides) - 3.1416 / sides);
  p = r * vec2(cos(a), sin(a));
  vec3 col = vec3(0.0);
  for (int i = 0; i < 7; i++) {
    p = abs(p) / max(dot(p, p), 0.02) - (0.62 + 0.3 * u_b + 0.05 * sin(u_time * u_a));
    float glow = exp(-length(p) * 1.1);
    col += glow * (0.5 + 0.5 * cos(vec3(0.0, 2.0, 4.0) + float(i) * 0.55 + u_time * u_a * 0.6));
  }
  return vec4(pow(col * 0.9, vec3(1.3)), 1.0);
}
`},
}
