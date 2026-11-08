#pragma once

#include "font_manager.hpp"
#include "text_shaper.hpp"
#include "wire.gen.hpp"

#include <cstdint>
#include <memory>
#include <string>
#include <string_view>
#include <unordered_map>
#include <vector>

namespace qui {

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
  bool mono = false, clip = false, scroll = false, ellipsis = false, has_pos = false, bottom = false;
  float w_val = 0, h_val = 0, min_w = 0, max_w = 0, min_h = 0, max_h = 0;
  float pad[4] = {};
  float gap = 0, pos_x = 0, pos_y = 0;
  uint32_t bg = 0, fg = 0, border = 0;
  float border_w = 0, radius = 0, opacity = 1, font_size = 0;
  std::string_view text, label, src;
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

struct Batch {
  uint32_t first = 0, count = 0;
  IRect clip;
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
  bool press = false, scroll = false;
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

class Engine {
 public:
  Engine();

  void decode(const uint8_t *wire, size_t len);
  void layout(float width, float height, float dpr);
  void paint(uint64_t hovered, uint64_t pressed);

  std::vector<Node> nodes;
  std::vector<TextLine> lines;
  std::vector<UniformRef> uniforms;
  std::vector<HitEntry> hits;
  std::unordered_map<uint64_t, ScrollState> scrolls;
  std::unordered_map<uint64_t, ListState> lists;
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

  FontInstanceId font_for(const Node &node);
  uint32_t resolve(uint32_t color) const;
  void intrinsic_width(Node &node);
  void distribute_width(Node &node);
  void intrinsic_height(Node &node);
  void distribute_height(Node &node);
  void place_children(Node &node);
  void place_list(Node &node);
  void wrap(Node &node, float inner_width);
  void paint_node(int index, float opacity, IRect clip, uint64_t hovered, uint64_t pressed);
  void paint_text(const Node &node, float opacity, IRect clip);
  void add_rect(float x, float y, float w, float h, uint32_t color, float radius, float stroke, IRect clip);
  void add_instance(const Instance &instance, int page, IRect clip);
  void prune();
};

}  // namespace qui
