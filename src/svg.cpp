#include "svg.hpp"

#include <algorithm>
#include <cmath>
#include <type_traits>

#include <ft2build.h>
#include FT_FREETYPE_H
#include FT_OUTLINE_H
#include FT_STROKER_H

namespace qui {
namespace {

constexpr double kPi = 3.14159265358979323846;
constexpr float kKappa = 0.5522847498f;

bool is_space(char c) { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'; }
bool is_digit(char c) { return c >= '0' && c <= '9'; }
bool is_alpha(char c) { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'); }

std::string_view trim(std::string_view s) {
  while (!s.empty() && is_space(s.front())) s.remove_prefix(1);
  while (!s.empty() && is_space(s.back())) s.remove_suffix(1);
  return s;
}

struct Cursor {
  const char *p, *end;

  explicit Cursor(std::string_view s) : p(s.data()), end(s.data() + s.size()) {}

  void skip() {
    while (p < end && (is_space(*p) || *p == ',')) ++p;
  }

  bool number(float &out) {
    skip();
    const char *start = p;
    double sign = 1, value = 0;
    if (p < end && (*p == '+' || *p == '-')) sign = *p++ == '-' ? -1 : 1;
    bool digits = false;
    while (p < end && is_digit(*p)) {
      value = value * 10 + (*p++ - '0');
      digits = true;
    }
    if (p < end && *p == '.') {
      ++p;
      for (double scale = 0.1; p < end && is_digit(*p); scale *= 0.1) {
        value += (*p++ - '0') * scale;
        digits = true;
      }
    }
    if (!digits) {
      p = start;
      return false;
    }
    if (p < end && (*p == 'e' || *p == 'E')) {
      const char *e = p + 1;
      int exponent_sign = 1, exponent = 0;
      if (e < end && (*e == '+' || *e == '-')) exponent_sign = *e++ == '-' ? -1 : 1;
      if (e < end && is_digit(*e)) {
        while (e < end && is_digit(*e)) exponent = std::min(exponent * 10 + (*e++ - '0'), 400);
        value *= std::pow(10.0, exponent_sign * exponent);
        p = e;
      }
    }
    out = static_cast<float>(sign * value);
    return std::isfinite(out);
  }

  bool flag(bool &out) {
    skip();
    if (p >= end || (*p != '0' && *p != '1')) return false;
    out = *p++ == '1';
    return true;
  }
};

float number_or(std::string_view s, float fallback) {
  Cursor c(s);
  float v;
  return c.number(v) ? v : fallback;
}

struct Affine {
  float a = 1, b = 0, c = 0, d = 1, e = 0, f = 0;

  Affine then(const Affine &n) const {
    return {a * n.a + c * n.b, b * n.a + d * n.b, a * n.c + c * n.d, b * n.c + d * n.d, a * n.e + c * n.f + e, b * n.e + d * n.f + f};
  }
  void apply(float x, float y, float &ox, float &oy) const {
    ox = a * x + c * y + e;
    oy = b * x + d * y + f;
  }
  float scale() const { return std::sqrt(std::abs(a * d - b * c)); }
};

Affine parse_transform(std::string_view s) {
  Affine m;
  for (;;) {
    s = trim(s);
    while (!s.empty() && (s.front() == ',' || is_space(s.front()))) s.remove_prefix(1);
    const size_t open = s.find('('), close = s.find(')');
    if (open == std::string_view::npos || close == std::string_view::npos || close < open) return m;
    const std::string_view name = trim(s.substr(0, open));
    Cursor c(s.substr(open + 1, close - open - 1));
    float v[6];
    int n = 0;
    while (n < 6 && c.number(v[n])) ++n;
    Affine t;
    if (name == "matrix" && n == 6) {
      t = {v[0], v[1], v[2], v[3], v[4], v[5]};
    } else if (name == "translate" && n >= 1) {
      t.e = v[0];
      t.f = n > 1 ? v[1] : 0;
    } else if (name == "scale" && n >= 1) {
      t.a = v[0];
      t.d = n > 1 ? v[1] : v[0];
    } else if (name == "rotate" && n >= 1) {
      const float r = static_cast<float>(v[0] * kPi / 180), cs = std::cos(r), sn = std::sin(r);
      t = {cs, sn, -sn, cs, 0, 0};
      if (n >= 3) t = Affine{1, 0, 0, 1, v[1], v[2]}.then(t).then(Affine{1, 0, 0, 1, -v[1], -v[2]});
    } else if (name == "skewX" && n >= 1) {
      t.c = std::tan(static_cast<float>(v[0] * kPi / 180));
    } else if (name == "skewY" && n >= 1) {
      t.b = std::tan(static_cast<float>(v[0] * kPi / 180));
    }
    m = m.then(t);
    s.remove_prefix(close + 1);
  }
}

struct Paint {
  Affine m;
  float fill = 1, stroke = 0, fill_opacity = 1, stroke_opacity = 1, opacity = 1;
  float stroke_width = 1, miter = 4;
  uint8_t cap = 0, join = 0;
  bool even_odd = false, display_none = false, invisible = false;
};

bool paints(std::string_view value) {
  value = trim(value);
  return !value.empty() && value != "none" && value != "transparent";
}

void apply_property(Paint &p, std::string_view name, std::string_view value) {
  value = trim(value);
  if (name == "fill") p.fill = paints(value) ? 1.f : 0.f;
  else if (name == "stroke") p.stroke = paints(value) ? 1.f : 0.f;
  else if (name == "fill-rule") p.even_odd = value == "evenodd";
  else if (name == "fill-opacity") p.fill_opacity = std::clamp(number_or(value, 1), 0.f, 1.f);
  else if (name == "stroke-opacity") p.stroke_opacity = std::clamp(number_or(value, 1), 0.f, 1.f);
  else if (name == "opacity") p.opacity *= std::clamp(number_or(value, 1), 0.f, 1.f);
  else if (name == "stroke-width") p.stroke_width = std::max(0.f, number_or(value, 1));
  else if (name == "stroke-miterlimit") p.miter = std::max(1.f, number_or(value, 4));
  else if (name == "stroke-linecap") p.cap = value == "round" ? 1 : value == "square" ? 2 : 0;
  else if (name == "stroke-linejoin") p.join = value == "round" ? 1 : value == "bevel" ? 2 : 0;
  else if (name == "display") p.display_none = p.display_none || value == "none";
  else if (name == "visibility") p.invisible = value == "hidden" || value == "collapse";
}

template <class F>
void for_each_attribute(std::string_view s, F &&fn) {
  size_t i = 0;
  while (i < s.size()) {
    while (i < s.size() && is_space(s[i])) ++i;
    const size_t name_start = i;
    while (i < s.size() && s[i] != '=' && !is_space(s[i])) ++i;
    const std::string_view name = s.substr(name_start, i - name_start);
    while (i < s.size() && is_space(s[i])) ++i;
    if (i >= s.size() || s[i] != '=') {
      if (i == name_start) ++i;
      continue;
    }
    ++i;
    while (i < s.size() && is_space(s[i])) ++i;
    if (i >= s.size() || (s[i] != '"' && s[i] != '\'')) continue;
    const char quote = s[i++];
    const size_t value_end = s.find(quote, i);
    if (value_end == std::string_view::npos) return;
    fn(name, s.substr(i, value_end - i));
    i = value_end + 1;
  }
}

class Builder {
 public:
  Builder(SvgShape &shape, const Affine &m) : shape_(shape), m_(m) {}

  float x = 0, y = 0;

  void move(float px, float py) {
    push(SvgMove, px, py);
    start_x_ = x = px;
    start_y_ = y = py;
    open_ = true;
  }
  void line(float px, float py) {
    begin();
    push(SvgLine, px, py);
    x = px;
    y = py;
  }
  void quad(float cx, float cy, float px, float py) {
    begin();
    push(SvgQuad, cx, cy);
    push_point(px, py);
    x = px;
    y = py;
  }
  void cubic(float c1x, float c1y, float c2x, float c2y, float px, float py) {
    begin();
    push(SvgCubic, c1x, c1y);
    push_point(c2x, c2y);
    push_point(px, py);
    x = px;
    y = py;
  }
  void close() {
    if (open_) shape_.ops.push_back(SvgClose);
    open_ = false;
    x = start_x_;
    y = start_y_;
  }

  void arc(float rx, float ry, float rotation, bool large, bool sweep, float px, float py) {
    const double x0 = x, y0 = y;
    if (x0 == px && y0 == py) return;
    double a = std::abs(rx), b = std::abs(ry);
    if (a == 0 || b == 0) {
      line(px, py);
      return;
    }
    const double phi = rotation * kPi / 180, cs = std::cos(phi), sn = std::sin(phi);
    const double dx = (x0 - px) / 2, dy = (y0 - py) / 2;
    const double x1 = cs * dx + sn * dy, y1 = -sn * dx + cs * dy;
    const double lambda = x1 * x1 / (a * a) + y1 * y1 / (b * b);
    if (lambda > 1) {
      a *= std::sqrt(lambda);
      b *= std::sqrt(lambda);
    }
    const double num = a * a * b * b - a * a * y1 * y1 - b * b * x1 * x1, den = a * a * y1 * y1 + b * b * x1 * x1;
    double coef = den > 0 ? std::sqrt(std::max(0.0, num / den)) : 0;
    if (large == sweep) coef = -coef;
    const double cxp = coef * a * y1 / b, cyp = -coef * b * x1 / a;
    const double cx = cs * cxp - sn * cyp + (x0 + px) / 2, cy = sn * cxp + cs * cyp + (y0 + py) / 2;
    const auto angle = [](double ux, double uy, double vx, double vy) { return std::atan2(ux * vy - uy * vx, ux * vx + uy * vy); };
    const double ux = (x1 - cxp) / a, uy = (y1 - cyp) / b, vx = (-x1 - cxp) / a, vy = (-y1 - cyp) / b;
    const double theta = angle(1, 0, ux, uy);
    double delta = angle(ux, uy, vx, vy);
    if (!sweep && delta > 0) delta -= 2 * kPi;
    if (sweep && delta < 0) delta += 2 * kPi;
    const int segments = std::max(1, static_cast<int>(std::ceil(std::abs(delta) / (kPi / 2) - 1e-6)));
    const double step = delta / segments, k = 4.0 / 3.0 * std::tan(step / 4);
    const auto map = [&](double u, double v, float &ox, float &oy) {
      ox = static_cast<float>(cx + a * u * cs - b * v * sn);
      oy = static_cast<float>(cy + a * u * sn + b * v * cs);
    };
    for (int i = 0; i < segments; ++i) {
      const double t0 = theta + step * i, t1 = t0 + step;
      const double c0 = std::cos(t0), s0 = std::sin(t0), c1 = std::cos(t1), s1 = std::sin(t1);
      float p1x, p1y, p2x, p2y, ex = px, ey = py;
      map(c0 - k * s0, s0 + k * c0, p1x, p1y);
      map(c1 + k * s1, s1 - k * c1, p2x, p2y);
      if (i + 1 < segments) map(c1, s1, ex, ey);
      cubic(p1x, p1y, p2x, p2y, ex, ey);
    }
  }

  void ellipse(float cx, float cy, float rx, float ry) {
    if (rx <= 0 || ry <= 0) return;
    const float kx = rx * kKappa, ky = ry * kKappa;
    move(cx + rx, cy);
    cubic(cx + rx, cy + ky, cx + kx, cy + ry, cx, cy + ry);
    cubic(cx - kx, cy + ry, cx - rx, cy + ky, cx - rx, cy);
    cubic(cx - rx, cy - ky, cx - kx, cy - ry, cx, cy - ry);
    cubic(cx + kx, cy - ry, cx + rx, cy - ky, cx + rx, cy);
    close();
  }

  void rect(float rx0, float ry0, float w, float h, float rx, float ry) {
    if (w <= 0 || h <= 0) return;
    rx = std::min(rx, w / 2);
    ry = std::min(ry, h / 2);
    if (rx <= 0 || ry <= 0) {
      move(rx0, ry0);
      line(rx0 + w, ry0);
      line(rx0 + w, ry0 + h);
      line(rx0, ry0 + h);
      close();
      return;
    }
    const float kx = rx * (1 - kKappa), ky = ry * (1 - kKappa), x1 = rx0 + w, y1 = ry0 + h;
    move(rx0 + rx, ry0);
    line(x1 - rx, ry0);
    cubic(x1 - kx, ry0, x1, ry0 + ky, x1, ry0 + ry);
    line(x1, y1 - ry);
    cubic(x1, y1 - ky, x1 - kx, y1, x1 - rx, y1);
    line(rx0 + rx, y1);
    cubic(rx0 + kx, y1, rx0, y1 - ky, rx0, y1 - ry);
    line(rx0, ry0 + ry);
    cubic(rx0, ry0 + ky, rx0 + kx, ry0, rx0 + rx, ry0);
    close();
  }

 private:
  void begin() {
    if (!open_) move(x, y);
  }
  void push(uint8_t op, float px, float py) {
    shape_.ops.push_back(op);
    push_point(px, py);
  }
  void push_point(float px, float py) {
    float ox, oy;
    m_.apply(px, py, ox, oy);
    shape_.points.push_back(ox);
    shape_.points.push_back(oy);
  }

  SvgShape &shape_;
  const Affine &m_;
  float start_x_ = 0, start_y_ = 0;
  bool open_ = false;
};

void parse_path(std::string_view d, Builder &b) {
  Cursor c(d);
  char cmd = 0, last = 0;
  float ctrl_x = 0, ctrl_y = 0;
  for (;;) {
    c.skip();
    if (c.p >= c.end) return;
    if (is_alpha(*c.p)) cmd = *c.p++;
    else if (!cmd) return;
    const bool rel = cmd >= 'a';
    const char upper = rel ? static_cast<char>(cmd - 32) : cmd;
    const float ox = rel ? b.x : 0, oy = rel ? b.y : 0;
    float v[7];
    const auto read = [&](int n) {
      for (int i = 0; i < n; ++i)
        if (!c.number(v[i])) return false;
      return true;
    };
    switch (upper) {
      case 'M':
        if (!read(2)) return;
        b.move(ox + v[0], oy + v[1]);
        cmd = rel ? 'l' : 'L';
        break;
      case 'L':
        if (!read(2)) return;
        b.line(ox + v[0], oy + v[1]);
        break;
      case 'H':
        if (!read(1)) return;
        b.line(ox + v[0], b.y);
        break;
      case 'V':
        if (!read(1)) return;
        b.line(b.x, oy + v[0]);
        break;
      case 'C':
        if (!read(6)) return;
        ctrl_x = ox + v[2];
        ctrl_y = oy + v[3];
        b.cubic(ox + v[0], oy + v[1], ctrl_x, ctrl_y, ox + v[4], oy + v[5]);
        break;
      case 'S': {
        if (!read(4)) return;
        const bool smooth = last == 'C' || last == 'S';
        const float c1x = smooth ? 2 * b.x - ctrl_x : b.x, c1y = smooth ? 2 * b.y - ctrl_y : b.y;
        ctrl_x = ox + v[0];
        ctrl_y = oy + v[1];
        b.cubic(c1x, c1y, ctrl_x, ctrl_y, ox + v[2], oy + v[3]);
        break;
      }
      case 'Q':
        if (!read(4)) return;
        ctrl_x = ox + v[0];
        ctrl_y = oy + v[1];
        b.quad(ctrl_x, ctrl_y, ox + v[2], oy + v[3]);
        break;
      case 'T': {
        if (!read(2)) return;
        const bool smooth = last == 'Q' || last == 'T';
        ctrl_x = smooth ? 2 * b.x - ctrl_x : b.x;
        ctrl_y = smooth ? 2 * b.y - ctrl_y : b.y;
        b.quad(ctrl_x, ctrl_y, ox + v[0], oy + v[1]);
        break;
      }
      case 'A': {
        bool large, sweep;
        if (!c.number(v[0]) || !c.number(v[1]) || !c.number(v[2]) || !c.flag(large) || !c.flag(sweep) || !c.number(v[3]) || !c.number(v[4])) return;
        b.arc(v[0], v[1], v[2], large, sweep, ox + v[3], oy + v[4]);
        break;
      }
      case 'Z':
        b.close();
        cmd = 0;
        break;
      default:
        return;
    }
    last = upper;
  }
}

void parse_points(std::string_view s, Builder &b, bool closed) {
  Cursor c(s);
  float x, y;
  bool first = true;
  while (c.number(x) && c.number(y)) {
    if (first) b.move(x, y);
    else b.line(x, y);
    first = false;
  }
  if (closed && !first) b.close();
}

bool skipped_container(std::string_view name) {
  static constexpr std::string_view kSkipped[] = {"defs", "clipPath", "mask", "symbol", "title", "desc", "metadata", "style", "script",
                                                  "linearGradient", "radialGradient", "pattern", "filter", "marker", "text", "foreignObject"};
  return std::find(std::begin(kSkipped), std::end(kSkipped), name) != std::end(kSkipped);
}

size_t tag_end(std::string_view text, size_t at) {
  char quote = 0;
  for (size_t i = at; i < text.size(); ++i) {
    const char ch = text[i];
    if (quote) {
      if (ch == quote) quote = 0;
    } else if (ch == '"' || ch == '\'') {
      quote = ch;
    } else if (ch == '>') {
      return i;
    }
  }
  return std::string_view::npos;
}

void add_shape(SvgIcon &icon, const Paint &paint, std::string_view name, std::string_view attrs) {
  SvgShape shape;
  Builder b(shape, paint.m);
  float v[6] = {0, 0, 0, 0, 0, 0};
  bool has_rx = false, has_ry = false;
  std::string_view geometry;
  for_each_attribute(attrs, [&](std::string_view key, std::string_view value) {
    const auto set = [&](int i) { v[i] = number_or(value, 0); };
    if (name == "path" && key == "d") geometry = value;
    else if ((name == "polygon" || name == "polyline") && key == "points") geometry = value;
    else if (key == "cx" || key == "x" || key == "x1") set(0);
    else if (key == "cy" || key == "y" || key == "y1") set(1);
    else if (key == "r" || key == "width" || key == "x2") set(2);
    else if (key == "height" || key == "y2") set(3);
    else if (key == "rx") {
      set(4);
      has_rx = true;
    } else if (key == "ry") {
      set(5);
      has_ry = true;
    }
  });
  if (name == "path") {
    parse_path(geometry, b);
  } else if (name == "polygon" || name == "polyline") {
    parse_points(geometry, b, name == "polygon");
  } else if (name == "circle") {
    b.ellipse(v[0], v[1], v[2], v[2]);
  } else if (name == "ellipse") {
    b.ellipse(v[0], v[1], v[4], v[5]);
  } else if (name == "rect") {
    const float rx = has_rx ? v[4] : has_ry ? v[5] : 0, ry = has_ry ? v[5] : rx;
    b.rect(v[0], v[1], v[2], v[3], rx, ry);
  } else if (name == "line") {
    b.move(v[0], v[1]);
    b.line(v[2], v[3]);
  } else {
    return;
  }
  if (shape.ops.empty()) return;
  shape.fill = paint.fill * paint.fill_opacity * paint.opacity;
  shape.stroke = paint.stroke * paint.stroke_opacity * paint.opacity;
  shape.stroke_width = paint.stroke_width * paint.m.scale();
  shape.miter = paint.miter;
  shape.cap = paint.cap;
  shape.join = paint.join;
  shape.even_odd = paint.even_odd;
  if (shape.fill > 0 || (shape.stroke > 0 && shape.stroke_width > 0)) icon.shapes.push_back(std::move(shape));
}

}  // namespace

bool parse_svg(std::string_view text, SvgIcon &out) {
  out = {};
  std::vector<Paint> stack{Paint{}};
  int skip = 0;
  bool root = false;
  size_t at = 0;
  while ((at = text.find('<', at)) != std::string_view::npos) {
    const std::string_view rest = text.substr(at);
    if (rest.starts_with("<!--") || rest.starts_with("<![CDATA[")) {
      const std::string_view close = rest[2] == '-' ? "-->" : "]]>";
      const size_t end = text.find(close, at);
      if (end == std::string_view::npos) break;
      at = end + close.size();
      continue;
    }
    const size_t end = tag_end(text, at + 1);
    if (end == std::string_view::npos) break;
    std::string_view tag = text.substr(at + 1, end - at - 1);
    at = end + 1;
    if (tag.starts_with('?') || tag.starts_with('!')) continue;
    if (tag.starts_with('/')) {
      if (skip) --skip;
      else if (stack.size() > 1) stack.pop_back();
      continue;
    }
    const bool self_closing = tag.ends_with('/');
    if (self_closing) tag.remove_suffix(1);
    size_t name_end = 0;
    while (name_end < tag.size() && !is_space(tag[name_end])) ++name_end;
    std::string_view name = tag.substr(0, name_end);
    if (const size_t colon = name.find(':'); colon != std::string_view::npos) name.remove_prefix(colon + 1);
    const std::string_view attrs = tag.substr(name_end);
    if (skip || skipped_container(name)) {
      if (!self_closing) ++skip;
      continue;
    }

    Paint paint = stack.back();
    paint.opacity = 1;
    std::string_view style, transform;
    float view[4] = {0, 0, 0, 0};
    bool has_view = false;
    float width = 0, height = 0;
    for_each_attribute(attrs, [&](std::string_view key, std::string_view value) {
      if (key == "style") style = value;
      else if (key == "transform") transform = value;
      else if (key == "viewBox") {
        Cursor c(value);
        has_view = c.number(view[0]) && c.number(view[1]) && c.number(view[2]) && c.number(view[3]);
      } else if (key == "width") width = number_or(value, 0);
      else if (key == "height") height = number_or(value, 0);
      else apply_property(paint, key, value);
    });
    for (size_t i = 0; i < style.size();) {
      const size_t semi = std::min(style.find(';', i), style.size());
      const std::string_view decl = style.substr(i, semi - i);
      if (const size_t colon = decl.find(':'); colon != std::string_view::npos) apply_property(paint, trim(decl.substr(0, colon)), decl.substr(colon + 1));
      i = semi + 1;
    }
    paint.opacity *= stack.back().opacity;
    if (!transform.empty()) paint.m = paint.m.then(parse_transform(transform));

    if (name == "svg" && !root) {
      root = true;
      if (has_view && view[2] > 0 && view[3] > 0) {
        out.x = view[0];
        out.y = view[1];
        out.w = view[2];
        out.h = view[3];
      } else if (width > 0 && height > 0) {
        out.w = width;
        out.h = height;
      }
    } else if (!paint.display_none && !paint.invisible) {
      add_shape(out, paint, name, attrs);
    }
    if (!self_closing) stack.push_back(paint);
  }
  return root;
}

void rasterize_svg(FT_Library library, const SvgIcon &icon, int width, int height, std::vector<uint8_t> &coverage) {
  coverage.assign(static_cast<size_t>(std::max(0, width)) * static_cast<size_t>(std::max(0, height)), 0);
  if (!library || coverage.empty() || icon.w <= 0 || icon.h <= 0) return;
  const float scale = std::min(static_cast<float>(width) / icon.w, static_cast<float>(height) / icon.h);
  const float ox = (static_cast<float>(width) - icon.w * scale) / 2 - icon.x * scale;
  const float oy = (static_cast<float>(height) - icon.h * scale) / 2 - icon.y * scale;
  const auto to_ft = [&](const float *p) {
    return FT_Vector{static_cast<FT_Pos>(std::lround((p[0] * scale + ox) * 64)), static_cast<FT_Pos>(std::lround((static_cast<float>(height) - (p[1] * scale + oy)) * 64))};
  };

  std::vector<uint8_t> layer(coverage.size());
  const auto render = [&](FT_Outline &outline, bool even_odd, float alpha) {
    std::fill(layer.begin(), layer.end(), 0);
    FT_Bitmap bitmap{};
    bitmap.rows = static_cast<unsigned>(height);
    bitmap.width = static_cast<unsigned>(width);
    bitmap.pitch = width;
    bitmap.buffer = layer.data();
    bitmap.num_grays = 256;
    bitmap.pixel_mode = FT_PIXEL_MODE_GRAY;
    outline.flags = (even_odd ? FT_OUTLINE_EVEN_ODD_FILL : FT_OUTLINE_NONE) | FT_OUTLINE_HIGH_PRECISION;
    if (FT_Outline_Get_Bitmap(library, &outline, &bitmap)) return;
    const auto a = static_cast<uint32_t>(std::clamp(alpha, 0.f, 1.f) * 256);
    for (size_t i = 0; i < layer.size(); ++i) {
      if (!layer[i]) continue;
      const uint32_t t = (layer[i] * a) >> 8, c = coverage[i];
      coverage[i] = static_cast<uint8_t>(c + t - (c * t + 127) / 255);
    }
  };

  FT_Outline outline{};
  std::vector<FT_Vector> points;
  std::vector<std::remove_pointer_t<decltype(outline.tags)>> tags;
  std::vector<std::remove_pointer_t<decltype(outline.contours)>> contours;
  using PointCount = decltype(outline.n_points);
  using ContourCount = decltype(outline.n_contours);
  FT_Stroker stroker = nullptr;

  for (const SvgShape &shape : icon.shapes) {
    if (shape.fill > 0) {
      points.clear();
      tags.clear();
      contours.clear();
      size_t start = 0, at = 0;
      bool open = false;
      const auto finish = [&] {
        if (!open) return;
        if (points.size() - start >= 3) {
          contours.push_back(static_cast<std::remove_pointer_t<decltype(outline.contours)>>(points.size() - 1));
        } else {
          points.resize(start);
          tags.resize(start);
        }
        open = false;
      };
      const auto add = [&](int count, char tag) {
        for (int i = 0; i < count; ++i, at += 2) {
          points.push_back(to_ft(&shape.points[at]));
          tags.push_back(static_cast<std::remove_pointer_t<decltype(outline.tags)>>(i + 1 == count ? FT_CURVE_TAG_ON : tag));
        }
      };
      for (const uint8_t op : shape.ops) {
        if (op == SvgMove) {
          finish();
          start = points.size();
          open = true;
          add(1, FT_CURVE_TAG_ON);
        } else if (op == SvgLine) {
          add(1, FT_CURVE_TAG_ON);
        } else if (op == SvgQuad) {
          add(2, FT_CURVE_TAG_CONIC);
        } else if (op == SvgCubic) {
          add(3, FT_CURVE_TAG_CUBIC);
        } else {
          finish();
        }
      }
      finish();
      if (!contours.empty() && points.size() < 32767 && contours.size() < 32767) {
        outline.n_points = static_cast<PointCount>(points.size());
        outline.n_contours = static_cast<ContourCount>(contours.size());
        outline.points = points.data();
        outline.tags = tags.data();
        outline.contours = contours.data();
        render(outline, shape.even_odd, shape.fill);
      }
    }

    if (shape.stroke > 0 && shape.stroke_width > 0) {
      if (!stroker && FT_Stroker_New(library, &stroker)) continue;
      static constexpr FT_Stroker_LineCap kCaps[] = {FT_STROKER_LINECAP_BUTT, FT_STROKER_LINECAP_ROUND, FT_STROKER_LINECAP_SQUARE};
      static constexpr FT_Stroker_LineJoin kJoins[] = {FT_STROKER_LINEJOIN_MITER_FIXED, FT_STROKER_LINEJOIN_ROUND, FT_STROKER_LINEJOIN_BEVEL};
      FT_Stroker_Set(stroker, std::max<FT_Fixed>(1, std::lround(shape.stroke_width * scale * 32)), kCaps[shape.cap % 3], kJoins[shape.join % 3],
                     static_cast<FT_Fixed>(shape.miter * 65536));
      size_t at = 0;
      bool in_path = false, pending = false;
      FT_Vector start{};
      for (size_t i = 0; i < shape.ops.size(); ++i) {
        const uint8_t op = shape.ops[i];
        if (op == SvgMove || op == SvgClose) {
          if (in_path) FT_Stroker_EndSubPath(stroker);
          in_path = pending = false;
          if (op == SvgMove) {
            start = to_ft(&shape.points[at]);
            at += 2;
            pending = true;
          }
          continue;
        }
        if (pending) {
          bool closed = false;
          for (size_t j = i; j < shape.ops.size() && shape.ops[j] != SvgMove; ++j)
            if (shape.ops[j] == SvgClose) closed = true;
          FT_Stroker_BeginSubPath(stroker, &start, !closed);
          in_path = true;
          pending = false;
        }
        FT_Vector p[3];
        const int count = op == SvgLine ? 1 : op == SvgQuad ? 2 : 3;
        for (int k = 0; k < count; ++k, at += 2) p[k] = to_ft(&shape.points[at]);
        if (op == SvgLine) FT_Stroker_LineTo(stroker, &p[0]);
        else if (op == SvgQuad) FT_Stroker_ConicTo(stroker, &p[0], &p[1]);
        else FT_Stroker_CubicTo(stroker, &p[0], &p[1], &p[2]);
      }
      if (in_path) FT_Stroker_EndSubPath(stroker);
      FT_UInt point_count = 0, contour_count = 0;
      if (!FT_Stroker_GetCounts(stroker, &point_count, &contour_count) && point_count && point_count < 32767) {
        FT_Outline stroked{};
        if (!FT_Outline_New(library, point_count, static_cast<FT_Int>(contour_count), &stroked)) {
          stroked.n_points = 0;
          stroked.n_contours = 0;
          FT_Stroker_Export(stroker, &stroked);
          render(stroked, false, shape.stroke);
          FT_Outline_Done(library, &stroked);
        }
      }
      FT_Stroker_Rewind(stroker);
    }
  }
  if (stroker) FT_Stroker_Done(stroker);
}

}  // namespace qui
