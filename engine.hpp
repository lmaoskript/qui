#pragma once

#include "font_manager.hpp"
#include "text_shaper.hpp"
#include "wire.gen.hpp"

#include <cmath>
#include <cstdint>
#include <memory>
#include <string>
#include <string_view>
#include <unordered_map>
#include <vector>

namespace qui {

inline constexpr float kDefaultFontSize = 14;
inline float line_height_for(float size) { return std::ceil(size * 1.4f); }

struct IRect {
  int x = 0, y = 0, w = 0, h = 0;
  bool empty() const { return w <= 0 || h <= 0; }
};

IRect intersect(IRect a, IRect b);
IRect unite(IRect a, IRect b);

struct TextLine {
  const ShapedLine *shaped = nullptr;
  uint32_t first = 0, last = 0;
  float width = 0;
  bool ellipsis = false;
  uint32_t para_byte = 0, start_byte = 0, end_byte = 0;
};

struct UniformRef {
  std::string_view name;
  float v[4] = {};
  uint8_t n = 0;
};

struct Node {
  uint64_t id = 0;
  int32_t parent = -1, first_child = -1, last_child = -1, next_sibling = -1;
  uint8_t kind = 0, dir = 0, align = 0, justify = 0, role = 0, hit = 0, weight = 0;
  uint8_t w_mode = 0, h_mode = 0;
  uint8_t multiline = 0, focus_mode = 0;
  bool mono = false, clip = false, scroll = false, ellipsis = false, has_pos = false, bottom = false;
  bool password = false, placeholder_shown = false;
  float w_val = 0, h_val = 0, min_w = 0, max_w = 0, min_h = 0, max_h = 0;
  float pad[4] = {};
  float gap = 0, pos_x = 0, pos_y = 0;
  int32_t z = 0;
  uint32_t bg = 0, fg = 0, border = 0;
  float border_w = 0, radius = 0, opacity = 1, font_size = 0;
  std::string_view text, label, src, placeholder;
  uint32_t count = 0;
  float est = 0;
  int64_t item = -1;
  float fps = 0;
  uint32_t uniform_first = 0, uniform_count = 0;

  float iw = 0, ih = 0, x = 0, y = 0, w = 0, h = 0;
  uint32_t line_first = 0, line_count = 0;
  FontInstanceId font = 0;
};

enum InstanceFlag : uint32_t {
  InstSolid = 1u << 0,
  InstGlyph = 1u << 1,
  InstColorGlyph = 1u << 2,
  InstRounded = 1u << 3,
};

struct Instance {
  float x, y, w, h;
  float u0, v0, u1, v1;
  uint32_t rgba;
  uint32_t flags;
};
static_assert(sizeof(Instance) == 40);

struct RoundClip {
  int x = 0, y = 0, w = 0, h = 0;
  float radius = 0;
  bool operator==(const RoundClip &) const = default;
};

struct Batch {
  uint32_t first = 0, count = 0;
  IRect clip;
  RoundClip round;
  int page = -1;
  int shader = -1;
};

struct ShaderUniform {
  std::string name;
  float v[4] = {};
  uint8_t n = 0;
};

struct ShaderDraw {
  uint64_t id = 0;
  uint64_t hash = 0;
  std::string src;
  IRect rect;
  float fps = 0;
  std::vector<ShaderUniform> uniforms;
};

struct FrameData {
  std::vector<Instance> instances;
  std::vector<Batch> batches;
  std::vector<ShaderDraw> shaders;
  uint32_t clear = 0;
  int width = 0, height = 0;
};

class Backend {
 public:
  virtual ~Backend() = default;
  virtual void resize(int width, int height) = 0;
  virtual void upload(const AtlasUpload &upload) = 0;
  virtual void draw(const FrameData &frame, IRect damage, double time) = 0;
  virtual void read_pixels(std::vector<uint32_t> &out) = 0;
  virtual std::string_view shader_error(uint64_t) const { return {}; }
  virtual bool take_compiled() { return false; }
};

class CpuBackend : public Backend {
 public:
  virtual const uint32_t *pixels() const = 0;
};

std::unique_ptr<CpuBackend> make_cpu_backend();
std::unique_ptr<Backend> make_gl_backend(void *(*get_proc)(const char *), bool offscreen, bool es);

struct HitEntry {
  uint64_t id = 0;
  float x = 0, y = 0, w = 0, h = 0;
  float cx0 = 0, cy0 = 0, cx1 = 0, cy1 = 0;
  bool press = false, scroll = false, focusable = false;
  int32_t scroll_parent = -1;
};

struct ScrollState {
  float offset = 0, content = 0, viewport = 0;
  uint64_t frame = 0;
};

struct ListState {
  uint32_t anchor = 0, count = 0, first = 0, visible = 0;
  float offset = 0, est = 32, gap = 0, viewport = 0;
  bool seen = false, bottom = false, stuck = true;
  uint64_t frame = 0;
  std::unordered_map<uint32_t, float> heights;

  float height(uint32_t index) const;
  void scroll_by(float delta);
  void compute_range();
};

enum class EditKind : uint8_t { None, Type, Erase, Other };

struct InputState {
  struct Snapshot {
    std::string text;
    size_t caret = 0, anchor = 0;
  };

  uint64_t id = 0, frame = 0;
  int node = -1;
  std::string text, seen, display, preedit, placeholder;
  size_t caret = 0, anchor = 0, preedit_cursor = 0, last_end = 0;
  float scroll_x = 0, scroll_y = 0, goal_x = -1;
  bool changed = false, submitted = false, multiline = false, submit_on_enter = false, password = false, reveal = true;
  EditKind last_edit = EditKind::None;
  std::vector<Snapshot> undo, redo;

  bool has_selection() const { return caret != anchor; }
  size_t selection_start() const { return std::min(caret, anchor); }
  size_t selection_end() const { return std::max(caret, anchor); }
  std::string_view selected() const;
  void snapshot();
  void replace_range(size_t from, size_t to, std::string_view with, EditKind kind);
  void restore(std::vector<Snapshot> &from, std::vector<Snapshot> &to);
  void assign(std::string_view value);
  void rebuild_display();
  size_t to_display(size_t byte) const;
  size_t from_display(size_t byte) const;
  size_t caret_display() const;
};

size_t next_boundary(std::string_view text, size_t at);
size_t previous_boundary(std::string_view text, size_t at);
size_t next_word(std::string_view text, size_t at);
size_t previous_word(std::string_view text, size_t at);
void word_range(std::string_view text, size_t at, size_t &from, size_t &to);
std::string clean_input(std::string_view text, bool multiline);

struct Interaction {
  uint64_t hovered = 0, pressed = 0, focused = 0;
  bool focus_ring = false, caret_on = true;
};

class Engine {
 public:
  Engine();

  void decode(const uint8_t *wire, size_t len);
  void layout(float width, float height, float dpr);
  void paint();

  void refresh_input(InputState &state);
  size_t input_byte_at(const InputState &state, float x, float y) const;
  void input_caret_rect(const InputState &state, float &x, float &y, float &h) const;
  size_t input_line_edge(const InputState &state, bool end) const;
  size_t input_vertical(InputState &state, int direction) const;

  std::vector<Node> nodes;
  std::vector<TextLine> lines;
  std::vector<UniformRef> uniforms;
  std::vector<HitEntry> hits;
  std::unordered_map<uint64_t, ScrollState> scrolls;
  std::unordered_map<uint64_t, ListState> lists;
  std::unordered_map<uint64_t, InputState> inputs;
  Interaction ui;
  uint64_t focus_request = 0;
  FontManager fonts;
  TextShaper shaper{fonts};
  FrameData frame, previous;
  uint8_t theme = 0;
  float dpr = 1;
  uint64_t frame_number = 0;
  bool wants_another_frame = false;

  IRect swap_and_damage();
  std::string tree_json() const;

 private:
  std::string sans_regular_, sans_bold_, mono_regular_, mono_bold_;
  std::unordered_map<uint64_t, FontInstanceId> font_cache_;
  int32_t scroll_parent_ = -1;
  RoundClip round_clip_;

  FontInstanceId font_for(const Node &node);
  uint32_t resolve(uint32_t color) const;
  void intrinsic_width(Node &node);
  void distribute_width(Node &node);
  void intrinsic_height(Node &node);
  void distribute_height(Node &node);
  void place_children(Node &node);
  void place_list(Node &node);
  void wrap(Node &node, float inner_width);
  void sync_input(Node &node, int index);
  void apply_input_view(Node &node, const InputState &state);
  float input_text_top(const Node &node, const InputState &state) const;
  void paint_node(int index, float opacity, IRect clip);
  void paint_children(const Node &node, float opacity, IRect clip);
  void paint_text(const Node &node, float opacity, IRect clip, float dx = 0, float dy = 0, uint32_t color_override = 0);
  void paint_input(const Node &node, float alpha, IRect clip);
  void add_rect(float x, float y, float w, float h, uint32_t color, float radius, float stroke, IRect clip);
  void add_instance(const Instance &instance, int page, IRect clip);
  void prune();
};

}  // namespace qui
