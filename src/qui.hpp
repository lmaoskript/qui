#pragma once

#include "qui_c.h"
#include "wire.gen.hpp"

#include <algorithm>
#include <cmath>
#include <cstdint>
#include <cstring>
#include <initializer_list>
#include <optional>
#include <string>
#include <string_view>
#include <unordered_map>
#include <unordered_set>
#include <vector>

namespace qui {

using Color = uint32_t;

constexpr Color rgba(uint8_t r, uint8_t g, uint8_t b, uint8_t a = 255) {
  return (uint32_t(r) << 24) | (uint32_t(g) << 16) | (uint32_t(b) << 8) | a;
}

struct Size {
  uint8_t mode = wire::SizeFit;
  float value = 0;
};
constexpr Size px(float v) { return {wire::SizePx, v}; }
constexpr Size fill(float weight = 1) { return {wire::SizeFill, weight}; }
constexpr Size pct(float percent) { return {wire::SizePct, percent}; }

struct Edges {
  float top = 0, right = 0, bottom = 0, left = 0;
  constexpr Edges() = default;
  constexpr Edges(float all) : top(all), right(all), bottom(all), left(all) {}
  constexpr Edges(float vertical, float horizontal) : top(vertical), right(horizontal), bottom(vertical), left(horizontal) {}
  constexpr Edges(float t, float r, float b, float l) : top(t), right(r), bottom(b), left(l) {}
};

struct Gradient {
  Color from = 0, to = 0;
  float angle = 0;
};

struct BoxShadow {
  Color color = 0;
  float blur = 0, y = 0;
};

struct Style {
  std::string_view klass{};
  Size w{}, h{};
  float min_w = 0, max_w = 0, min_h = 0, max_h = 0;
  Edges pad{};
  float gap = 0;
  uint8_t align = 0, justify = 0;
  bool absolute = false;
  float x = 0, y = 0;
  int32_t z = 0;
  Color bg = 0, fg = 0, border = 0;
  float border_w = 0, radius = 0, opacity = 0;
  Gradient gradient{};
  BoxShadow shadow{};
  bool clip = false, scroll = false, hit = false;
  std::optional<float> scroll_to{};
  uint8_t gestures = 0;
  uint8_t role = 0;
  std::string_view label{}, key{};
};

struct TextStyle {
  std::string_view klass{};
  float size = 0;
  bool bold = false, mono = false, ellipsis = false;
  Color color = 0;
  Size w{};
  uint8_t align = 0;
  uint8_t role = 0;
  Gradient gradient{};
  float tracking = 0;
};

inline constexpr TextStyle Body{};
inline constexpr TextStyle H1{.size = 24, .bold = true, .color = Strong, .role = RoleHeading};
inline constexpr TextStyle H2{.size = 18, .bold = true, .color = Strong, .role = RoleHeading};
inline constexpr TextStyle Small{.size = 12, .color = Muted};
inline constexpr TextStyle Code{.size = 13, .mono = true};

enum class Swipe : uint8_t { None, Left, Right, Up, Down };

struct Response {
  bool found = false, hovered = false, pressed = false, clicked = false, right_clicked = false;
  bool focused = false, drag_start = false, dragging = false, drag_end = false;
  bool changed = false, submitted = false;
  bool long_pressed = false, double_tapped = false, pan_start = false, panning = false, pan_end = false, pinching = false;
  float x = 0, y = 0, w = 0, h = 0, dx = 0, dy = 0;
  float vx = 0, vy = 0, scale = 1;
  Swipe swipe = Swipe::None;
  uint32_t first = 0, count = 0;
  std::string_view error{};
};

struct Stats {
  size_t nodes = 0, wire_bytes = 0;
  float engine_ms = 0;
};

struct ListOptions {
  Style style{};
  bool bottom = false;
};

struct Uniform {
  std::string_view name;
  std::initializer_list<float> values;
};

struct ShaderOptions {
  std::string_view src;
  Style style{};
  float fps = 0;
  std::initializer_list<Uniform> uniforms{};
};

struct InputOptions {
  Style style{};
  std::string_view placeholder{};
  float size = 0;
  bool mono = false, multiline = false, enter_submits = false, password = false, autofocus = false, focus = false;
};

enum class Side : uint8_t { Left, Right, Top, Bottom };

// State of a panel that slides over other content. Keep one per drawer and set
// open to show or hide it from code.
struct Drawer {
  bool open = false;
  float position = 0, speed = 0;
  bool held = false, started = false;
};

struct DrawerOptions {
  Side side = Side::Left;
  float size = 0;
  Style style{};
  std::string_view key{};
};

inline void set_app_id(const char *id) { qui_set_app_id(id); }

class Ui {
 public:
  Ui(const char *title, int width, int height) : app_(qui_open(title, width, height)) {
    buf_.reserve(1 << 16);
    stack_.reserve(64);
  }
  ~Ui() { qui_close(app_); }
  Ui(const Ui &) = delete;
  Ui &operator=(const Ui &) = delete;

  bool frame() {
    if (started_) {
      stats_.nodes = nodes_;
      stats_.wire_bytes = buf_.size();
      flush_images();
      qui_frame_end(app_, buf_.data(), buf_.size());
    }
    size_t len = 0;
    res_ = qui_frame_begin(app_, &len);
    if (!res_) return false;
    started_ = true;
    nodes_ = 0;
    buf_.clear();
    stack_.clear();
    stack_.push_back({kSeed, 0});
    u8(wire::OpFrame);
    u8(wire::kVersion);
    u8(theme_);
    emit_palette();
    image_at_ = buf_.size();
    return true;
  }

  void wake() { qui_wake(app_); }
  void set_theme(uint8_t theme) { theme_ = theme; }

  constexpr static Color token(uint8_t id) { return Color(id) << 24; }

  void set_palette(uint8_t base, std::initializer_list<std::pair<Color, Color>> colors) {
    theme_ = base;
    palette_.clear();
    for (const auto &[token, value] : colors) set_token(token, value);
  }

  void set_token(Color token, Color value) {
    const uint8_t id = token >> 24;
    for (auto &entry : palette_) {
      if (entry.first == id) {
        entry.second = value;
        return;
      }
    }
    palette_.emplace_back(id, value);
    std::sort(palette_.begin(), palette_.end());
  }

  void define_class(std::string name, Style style) {
    style.key = style.label = {};
    style_classes_[std::move(name)] = std::move(style);
  }

  void define_text_class(std::string name, TextStyle style) { text_classes_[std::move(name)] = std::move(style); }
  float width() const { return header(8); }
  float height() const { return header(12); }
  float dpr() const { return header(16); }
  float dt() const { return header(20); }
  float pointer_x() const { return header(24); }
  float pointer_y() const { return header(28); }
  bool pointer_down() const { return header_u32(44) != 0; }
  Stats stats() const {
    Stats out = stats_;
    out.engine_ms = header(48);
    return out;
  }

  template <class F>
  Response column(const Style &style, F &&body) { return container(wire::DirColumn, style, body); }
  template <class F>
  Response row(const Style &style, F &&body) { return container(wire::DirRow, style, body); }
  template <class F>
  Response stack(const Style &style, F &&body) { return container(wire::DirStack, style, body); }

  void text(std::string_view value, TextStyle style = Body) {
    if (!style.klass.empty()) style = resolve_text(style);
    open(wire::KindText, {});
    str(wire::OpText, value);
    if (style.size) f32_op(wire::OpFontSize, style.size);
    if (style.bold) u8_op(wire::OpWeight, 1);
    if (style.mono) u8(wire::OpMono);
    if (style.ellipsis) u8(wire::OpEllipsis);
    if (style.color) u32_op(wire::OpFg, style.color);
    size_op(wire::OpWidth, style.w);
    if (style.align) u8_op(wire::OpJustify, style.align);
    if (style.role) u8_op(wire::OpRole, style.role);
    emit_gradient(style.gradient);
    if (style.tracking) f32_op(wire::OpTracking, style.tracking);
    u8(wire::OpEnd);
  }

  // Hands the engine straight (not premultiplied) RGBA pixels under `key`. Do it
  // once; `image` then refers to it by key. Loading a key again replaces it.
  void load_image(std::string_view key, int width, int height, const uint8_t *rgba) {
    if (width <= 0 || height <= 0 || width > 65535 || height > 65535) return;
    const uint64_t id = mix(0, key.data(), key.size());
    loaded_images_.insert(id);
    const size_t bytes = static_cast<size_t>(width) * static_cast<size_t>(height) * 4;
    pending_images_.push_back(wire::OpImageData);
    pending_images_.insert(pending_images_.end(), reinterpret_cast<const uint8_t *>(&id), reinterpret_cast<const uint8_t *>(&id) + 8);
    const uint16_t w = static_cast<uint16_t>(width), h = static_cast<uint16_t>(height);
    pending_images_.insert(pending_images_.end(), reinterpret_cast<const uint8_t *>(&w), reinterpret_cast<const uint8_t *>(&w) + 2);
    pending_images_.insert(pending_images_.end(), reinterpret_cast<const uint8_t *>(&h), reinterpret_cast<const uint8_t *>(&h) + 2);
    pending_images_.insert(pending_images_.end(), rgba, rgba + bytes);
  }

  bool has_image(std::string_view key) const { return loaded_images_.count(mix(0, key.data(), key.size())) != 0; }

  // Draws a loaded picture. With no w and h it takes the image's pixel size; with
  // only a width it keeps the aspect ratio. `radius` clips it, so a square with
  // radius = size / 2 is a circle.
  Response image(std::string_view key, Style style = {}, uint8_t fit = wire::FitCover) {
    const uint64_t id = open(wire::KindImage, style.key);
    u8(wire::OpImg);
    u64(mix(0, key.data(), key.size()));
    if (fit != wire::FitCover) u8_op(wire::OpFit, fit);
    emit_style(style);
    u8(wire::OpEnd);
    return lookup(id);
  }

  Response button(std::string_view label, Style style = {}) {
    if (style.key.empty()) style.key = label;
    const uint64_t id = open(wire::KindButton, style.key);
    emit_style(style);
    str(wire::OpText, label);
    u8(wire::OpEnd);
    return lookup(id);
  }

  template <class F>
  Response list(std::string_view key, uint32_t count, float estimate, const ListOptions &options, F &&row_body) {
    const uint64_t id = open(wire::KindList, key);
    emit_style(options.style);
    u8(wire::OpVirtual);
    u32(count);
    f32(estimate);
    u8(options.bottom ? 1 : 0);
    const Response result = lookup(id);
    uint32_t first = result.first, visible = result.count;
    if (!result.found || visible == 0) {
      visible = std::min<uint32_t>(count, 32);
      first = options.bottom && count > visible ? count - visible : 0;
    }
    stack_.push_back({id, 0});
    for (uint32_t i = first; i < std::min(first + visible, count); ++i) {
      uint64_t item = hash_u32(id, i);
      ++nodes_;
      u8(wire::OpOpen);
      u8(wire::KindBox);
      u64(item);
      size_op(wire::OpWidth, fill());
      u8(wire::OpItem);
      u32(i);
      stack_.push_back({item, 0});
      row_body(i);
      stack_.pop_back();
      u8(wire::OpEnd);
    }
    stack_.pop_back();
    u8(wire::OpEnd);
    return result;
  }

  Response input(std::string_view key, std::string &value, const InputOptions &options = {}) {
    const uint64_t id = open(wire::KindInput, key);
    Response result = lookup(id);
    if (result.changed) value.assign(reinterpret_cast<const char *>(res_) + header_u32(40) + result.first, result.count);
    result.first = result.count = 0;
    emit_style(options.style);
    str(wire::OpText, value);
    if (!options.placeholder.empty()) str(wire::OpPlaceholder, options.placeholder);
    if (options.size) f32_op(wire::OpFontSize, options.size);
    if (options.mono) u8(wire::OpMono);
    if (options.multiline) u8_op(wire::OpMultiline, options.enter_submits ? 2 : 1);
    if (options.password) u8(wire::OpPassword);
    if (options.focus) u8_op(wire::OpFocus, 2);
    else if (options.autofocus) u8_op(wire::OpFocus, 1);
    u8(wire::OpEnd);
    return result;
  }

  bool key_pressed(uint32_t key, uint32_t mods = 0) {
    if (!res_) return false;
    const uint32_t at = header_u32(32), count = header_u32(36);
    for (uint32_t i = 0; i < count; ++i) {
      uint32_t event[2];
      std::memcpy(event, res_ + at + i * 8, 8);
      if (event[0] == key && event[1] == mods) {
        if (key == KeyEscape) qui_consume_back(app_);
        return true;
      }
    }
    return false;
  }

  Response shader(const ShaderOptions &options) {
    const uint64_t id = open(wire::KindShader, options.style.key);
    Response result = lookup(id);
    result.first = result.count = 0;
    emit_style(options.style);
    str(wire::OpSrc, options.src);
    if (options.fps) f32_op(wire::OpFps, options.fps);
    for (const Uniform &uniform : options.uniforms) {
      u8(wire::OpUniform);
      u8(static_cast<uint8_t>(uniform.name.size()));
      raw(uniform.name.data(), uniform.name.size());
      u8(static_cast<uint8_t>(uniform.values.size()));
      for (const float v : uniform.values) f32(v);
    }
    u8(wire::OpEnd);
    return result;
  }

  // Fills its parent with content and slides panel over it. A touch swipe on
  // content opens it; a swipe, a tap on the scrim or Escape closes it.
  template <class C, class P>
  Response drawer(Drawer &d, const DrawerOptions &options, C &&content, P &&panel) {
    const float size = options.size > 0 ? options.size : 280;
    const bool horizontal = options.side == Side::Left || options.side == Side::Right;
    Style root{};
    root.w = root.h = fill();
    root.clip = true;
    root.key = options.key;
    root.gestures = (horizontal ? GesturePanX : GesturePanY) | GestureTouchOnly;
    const uint64_t id = open(wire::KindBox, options.key);
    u8_op(wire::OpDir, wire::DirStack);
    emit_style(root);
    const Response result = lookup(id);
    update_drawer(d, options, result, size, horizontal);

    stack_.push_back({id, 0});
    column({.w = fill(), .h = fill(), .key = "content"}, content);
    if (d.position > 0) {
      if (column({.w = fill(), .h = fill(), .absolute = true, .bg = Scrim, .opacity = d.position, .hit = true, .key = "scrim"}, [] {}).clicked) d.open = false;
      const float parent_w = result.found ? result.w : width(), parent_h = result.found ? result.h : height();
      const float travel = size * (d.position - 1);
      Style s = options.style;
      s.key = "panel";
      s.absolute = true;
      s.hit = true;
      s.w = horizontal ? px(size) : fill();
      s.h = horizontal ? fill() : px(size);
      s.x = options.side == Side::Left ? travel : options.side == Side::Right ? parent_w - size - travel : 0;
      s.y = options.side == Side::Top ? travel : options.side == Side::Bottom ? parent_h - size - travel : 0;
      column(s, panel);
    }
    stack_.pop_back();
    u8(wire::OpEnd);
    return result;
  }

 private:
  void update_drawer(Drawer &d, const DrawerOptions &options, const Response &r, float size, bool horizontal) {
    constexpr float spring = 20, fling_speed = 1.5f;
    const auto clamp01 = [](float v) { return std::min(std::max(v, 0.f), 1.f); };
    if (!d.started) {
      d.started = true;
      if (d.open) d.position = 1;
    }
    const float direction = options.side == Side::Right || options.side == Side::Bottom ? -1.f : 1.f;
    const float delta = horizontal ? r.dx : r.dy, velocity = horizontal ? r.vx : r.vy;
    if (r.panning || r.pan_end) d.position = clamp01(d.position + direction * delta / size);
    d.held = r.panning;
    if (r.pan_end) {
      const float fling = direction * velocity / size;
      d.open = fling > fling_speed ? true : fling < -fling_speed ? false : d.position > 0.5f;
      d.speed = fling;
    }
    if (d.open && key_pressed(KeyEscape)) d.open = false;
    if (d.held) {
      d.speed = 0;
      return;
    }
    const float target = d.open ? 1.f : 0.f;
    if (d.position == target && d.speed == 0) return;
    const float step = std::min(dt(), 1.f / 30);
    d.speed += (spring * spring * (target - d.position) - 2 * spring * d.speed) * step;
    d.position = clamp01(d.position + d.speed * step);
    if (std::abs(target - d.position) < 0.001f && std::abs(d.speed) < 0.02f) {
      d.position = target;
      d.speed = 0;
      return;
    }
    wake();
  }

  struct Scope {
    uint64_t id;
    uint32_t index;
  };

  static constexpr uint64_t kSeed = 14695981039346656037ull;

  static uint64_t mix(uint64_t h, const void *data, size_t len) {
    const auto *p = static_cast<const unsigned char *>(data);
    for (size_t i = 0; i < len; ++i) {
      h ^= p[i];
      h *= 1099511628211ull;
    }
    return h ? h : 1;
  }
  static uint64_t hash_u32(uint64_t seed, uint32_t v) { return mix(seed, &v, 4); }

  float header(size_t offset) const {
    float value = 0;
    if (res_) std::memcpy(&value, res_ + offset, 4);
    return value;
  }

  uint32_t header_u32(size_t offset) const {
    uint32_t value = 0;
    if (res_) std::memcpy(&value, res_ + offset, 4);
    return value;
  }

  void raw(const void *data, size_t len) {
    const size_t at = buf_.size();
    buf_.resize(at + len);
    std::memcpy(buf_.data() + at, data, len);
  }
  void u8(uint8_t v) { buf_.push_back(v); }
  void u32(uint32_t v) { raw(&v, 4); }
  void u64(uint64_t v) { raw(&v, 8); }
  void f32(float v) { raw(&v, 4); }
  void u8_op(uint8_t op, uint8_t v) { u8(op); u8(v); }
  void u32_op(uint8_t op, uint32_t v) { u8(op); u32(v); }
  void f32_op(uint8_t op, float v) { u8(op); f32(v); }
  void str(uint8_t op, std::string_view v) {
    u8(op);
    u32(static_cast<uint32_t>(v.size()));
    raw(v.data(), v.size());
  }
  void size_op(uint8_t op, Size s) {
    if (s.mode == wire::SizeFit) return;
    u8(op);
    u8(s.mode);
    f32(s.value);
  }

  uint64_t open(uint8_t kind, std::string_view key) {
    Scope &scope = stack_.back();
    const uint64_t id = key.empty() ? hash_u32(scope.id, scope.index) : mix(scope.id, key.data(), key.size());
    ++scope.index;
    ++nodes_;
    u8(wire::OpOpen);
    u8(kind);
    u64(id);
    return id;
  }

  void emit_gradient(const Gradient &g) {
    if (!g.from && !g.to) return;
    u8(wire::OpGradient);
    u32(g.from);
    u32(g.to);
    f32(g.angle);
  }

  void flush_images() {
    if (pending_images_.empty()) return;
    buf_.insert(buf_.begin() + static_cast<std::ptrdiff_t>(image_at_), pending_images_.begin(), pending_images_.end());
    pending_images_.clear();
  }

  void emit_palette() {
    if (palette_.empty()) return;
    u8(wire::OpPalette);
    const uint16_t count = static_cast<uint16_t>(palette_.size());
    u8(count & 255);
    u8(count >> 8);
    for (const auto &[id, value] : palette_) {
      u8(id);
      u32(value);
    }
  }

  template <class Map>
  static std::vector<const typename Map::mapped_type *> class_chain(const Map &map, std::string_view names) {
    std::vector<const typename Map::mapped_type *> out;
    size_t i = 0;
    while (i < names.size()) {
      while (i < names.size() && names[i] == ' ') ++i;
      size_t j = i;
      while (j < names.size() && names[j] != ' ') ++j;
      if (j > i)
        if (const auto it = map.find(std::string(names.substr(i, j - i))); it != map.end()) out.push_back(&it->second);
      i = j;
    }
    return out;
  }

  Style resolve_style(Style s) const {
    const auto chain = class_chain(style_classes_, s.klass);
    s.klass = {};
    for (auto it = chain.rbegin(); it != chain.rend(); ++it) {
      Style base = **it;
      if (!base.klass.empty()) base = resolve_style(base);
      merge_style(s, base);
    }
    return s;
  }

  TextStyle resolve_text(TextStyle t) const {
    const auto chain = class_chain(text_classes_, t.klass);
    t.klass = {};
    for (auto it = chain.rbegin(); it != chain.rend(); ++it) {
      TextStyle base = **it;
      if (!base.klass.empty()) base = resolve_text(base);
      if (!t.size) t.size = base.size;
      t.bold |= base.bold;
      t.mono |= base.mono;
      t.ellipsis |= base.ellipsis;
      if (!t.color) t.color = base.color;
      if (t.w.mode == wire::SizeFit) t.w = base.w;
      if (!t.align) t.align = base.align;
      if (!t.role) t.role = base.role;
      if (!t.gradient.from && !t.gradient.to) t.gradient = base.gradient;
      if (!t.tracking) t.tracking = base.tracking;
    }
    return t;
  }

  static void merge_style(Style &s, const Style &b) {
    if (s.w.mode == wire::SizeFit) s.w = b.w;
    if (s.h.mode == wire::SizeFit) s.h = b.h;
    if (!s.min_w) s.min_w = b.min_w;
    if (!s.max_w) s.max_w = b.max_w;
    if (!s.min_h) s.min_h = b.min_h;
    if (!s.max_h) s.max_h = b.max_h;
    if (!s.pad.top && !s.pad.right && !s.pad.bottom && !s.pad.left) s.pad = b.pad;
    if (!s.gap) s.gap = b.gap;
    if (!s.align) s.align = b.align;
    if (!s.justify) s.justify = b.justify;
    if (!s.absolute) {
      s.absolute = b.absolute;
      s.x = b.x;
      s.y = b.y;
    }
    if (!s.z) s.z = b.z;
    if (!s.bg) s.bg = b.bg;
    if (!s.fg) s.fg = b.fg;
    if (!s.border) {
      s.border = b.border;
      s.border_w = b.border_w;
    }
    if (!s.radius) s.radius = b.radius;
    if (!s.opacity) s.opacity = b.opacity;
    if (!s.gradient.from && !s.gradient.to) s.gradient = b.gradient;
    if (!s.shadow.color) s.shadow = b.shadow;
    s.clip |= b.clip;
    s.scroll |= b.scroll;
    s.hit |= b.hit;
    if (!s.gestures) s.gestures = b.gestures;
    if (!s.role) s.role = b.role;
    if (s.label.empty()) s.label = b.label;
  }

  void emit_style(const Style &given) {
    Style resolved;
    if (!given.klass.empty()) resolved = resolve_style(given);
    const Style &s = given.klass.empty() ? given : resolved;
    size_op(wire::OpWidth, s.w);
    size_op(wire::OpHeight, s.h);
    if (s.min_w) f32_op(wire::OpMinW, s.min_w);
    if (s.max_w) f32_op(wire::OpMaxW, s.max_w);
    if (s.min_h) f32_op(wire::OpMinH, s.min_h);
    if (s.max_h) f32_op(wire::OpMaxH, s.max_h);
    if (s.pad.top || s.pad.right || s.pad.bottom || s.pad.left) {
      u8(wire::OpPad);
      f32(s.pad.top);
      f32(s.pad.right);
      f32(s.pad.bottom);
      f32(s.pad.left);
    }
    if (s.gap) f32_op(wire::OpGap, s.gap);
    if (s.align) u8_op(wire::OpAlign, s.align);
    if (s.justify) u8_op(wire::OpJustify, s.justify);
    if (s.absolute) {
      u8(wire::OpPos);
      f32(s.x);
      f32(s.y);
    }
    if (s.z) u32_op(wire::OpZ, static_cast<uint32_t>(s.z));
    if (s.bg) u32_op(wire::OpBg, s.bg);
    if (s.fg) u32_op(wire::OpFg, s.fg);
    if (s.border) {
      u8(wire::OpBorder);
      u32(s.border);
      f32(s.border_w ? s.border_w : 1);
    }
    if (s.radius) f32_op(wire::OpRadius, s.radius);
    if (s.opacity) f32_op(wire::OpOpacity, s.opacity);
    emit_gradient(s.gradient);
    if (s.shadow.color) {
      u8(wire::OpShadow);
      u32(s.shadow.color);
      f32(s.shadow.blur);
      f32(s.shadow.y);
    }
    if (s.clip) u8(wire::OpClip);
    if (s.scroll) u8(wire::OpScroll);
    if (s.scroll_to) f32_op(wire::OpScrollTo, *s.scroll_to);
    if (s.hit) u8_op(wire::OpHit, 1);
    if (s.gestures) u8_op(wire::OpGestures, s.gestures);
    if (s.role) u8_op(wire::OpRole, s.role);
    if (!s.label.empty()) str(wire::OpLabel, s.label);
  }

  template <class F>
  Response container(uint8_t dir, const Style &style, F &&body) {
    const uint64_t id = open(wire::KindBox, style.key);
    if (dir != wire::DirColumn) u8_op(wire::OpDir, dir);
    emit_style(style);
    stack_.push_back({id, 0});
    body();
    stack_.pop_back();
    u8(wire::OpEnd);
    return lookup(id);
  }

  static Swipe swipe_of(const Response &r) {
    constexpr float speed = 300;
    if (!r.pan_end) return Swipe::None;
    const float x = r.vx < 0 ? -r.vx : r.vx, y = r.vy < 0 ? -r.vy : r.vy;
    if (x >= y && x >= speed) return r.vx < 0 ? Swipe::Left : Swipe::Right;
    if (y > x && y >= speed) return r.vy < 0 ? Swipe::Up : Swipe::Down;
    return Swipe::None;
  }

  Response lookup(uint64_t id) const {
    Response out;
    if (!res_) return out;
    uint32_t capacity = 0;
    std::memcpy(&capacity, res_ + 4, 4);
    if (!capacity) return out;
    for (uint32_t i = static_cast<uint32_t>(id) & (capacity - 1);; i = (i + 1) & (capacity - 1)) {
      const uint8_t *slot = res_ + 64 + static_cast<size_t>(i) * 56;
      uint64_t key = 0;
      std::memcpy(&key, slot, 8);
      if (key == 0) return out;
      if (key != id) continue;
      uint32_t flags = 0;
      std::memcpy(&flags, slot + 8, 4);
      std::memcpy(&out.first, slot + 12, 4);
      std::memcpy(&out.count, slot + 16, 4);
      std::memcpy(&out.x, slot + 24, 4);
      std::memcpy(&out.y, slot + 28, 4);
      std::memcpy(&out.w, slot + 32, 4);
      std::memcpy(&out.h, slot + 36, 4);
      std::memcpy(&out.dx, slot + 40, 4);
      std::memcpy(&out.dy, slot + 44, 4);
      std::memcpy(&out.vx, slot + 48, 4);
      std::memcpy(&out.vy, slot + 52, 4);
      out.long_pressed = flags & wire::FlagLongPressed;
      out.double_tapped = flags & wire::FlagDoubleTapped;
      out.pan_start = flags & wire::FlagPanStart;
      out.panning = flags & wire::FlagPanning;
      out.pan_end = flags & wire::FlagPanEnd;
      out.pinching = flags & wire::FlagPinching;
      if (out.pinching) std::memcpy(&out.scale, slot + 20, 4);
      out.swipe = swipe_of(out);
      out.found = true;
      out.hovered = flags & wire::FlagHovered;
      out.pressed = flags & wire::FlagPressed;
      out.clicked = flags & wire::FlagClicked;
      out.right_clicked = flags & wire::FlagRightClicked;
      out.focused = flags & wire::FlagFocused;
      out.drag_start = flags & wire::FlagDragStart;
      out.dragging = flags & wire::FlagDragging;
      out.drag_end = flags & wire::FlagDragEnd;
      out.changed = flags & wire::FlagChanged;
      out.submitted = flags & wire::FlagSubmitted;
      if (flags & wire::FlagShaderError) out.error = std::string_view(reinterpret_cast<const char *>(res_) + header_u32(40) + out.first, out.count);
      return out;
    }
  }

  qui_app *app_;
  const uint8_t *res_ = nullptr;
  bool started_ = false;
  uint8_t theme_ = 0;
  size_t image_at_ = 0;
  std::vector<uint8_t> pending_images_;
  std::unordered_set<uint64_t> loaded_images_;
  std::vector<std::pair<uint8_t, Color>> palette_;
  std::unordered_map<std::string, Style> style_classes_;
  std::unordered_map<std::string, TextStyle> text_classes_;
  size_t nodes_ = 0;
  Stats stats_{};
  std::vector<uint8_t> buf_;
  std::vector<Scope> stack_;
};

}  // namespace qui
