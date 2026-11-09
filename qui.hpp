#pragma once

#include "qui_c.h"
#include "wire.gen.hpp"

#include <algorithm>
#include <cstdint>
#include <cstring>
#include <initializer_list>
#include <optional>
#include <string>
#include <string_view>
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
  uint8_t role = 0;
  std::string_view label{}, key{};
};

struct TextStyle {
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

struct Response {
  bool found = false, hovered = false, pressed = false, clicked = false, right_clicked = false;
  bool focused = false, drag_start = false, dragging = false, drag_end = false;
  bool changed = false, submitted = false;
  float x = 0, y = 0, w = 0, h = 0, dx = 0, dy = 0;
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
    return true;
  }

  void wake() { qui_wake(app_); }
  void set_theme(uint8_t theme) { theme_ = theme; }
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

  void text(std::string_view value, const TextStyle &style = Body) {
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

 private:
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

  void emit_style(const Style &s) {
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

  Response lookup(uint64_t id) const {
    Response out;
    if (!res_) return out;
    uint32_t capacity = 0;
    std::memcpy(&capacity, res_ + 4, 4);
    if (!capacity) return out;
    for (uint32_t i = static_cast<uint32_t>(id) & (capacity - 1);; i = (i + 1) & (capacity - 1)) {
      const uint8_t *slot = res_ + 64 + static_cast<size_t>(i) * 48;
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
  size_t nodes_ = 0;
  Stats stats_{};
  std::vector<uint8_t> buf_;
  std::vector<Scope> stack_;
};

}  // namespace qui
