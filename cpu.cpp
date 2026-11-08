//go:build cgo

#include "engine.hpp"

#include <algorithm>
#include <cmath>
#include <cstring>

namespace qui {

namespace {

inline uint32_t blend_channel(uint32_t src, uint32_t dst, uint32_t inverse_alpha) {
  const uint32_t value = dst * inverse_alpha + 128;
  return src + ((value + (value >> 8)) >> 8);
}

inline uint32_t over(uint32_t src, uint32_t dst) {
  const uint32_t inverse = 255 - (src >> 24);
  const uint32_t r = blend_channel(src & 0xff, dst & 0xff, inverse);
  const uint32_t g = blend_channel((src >> 8) & 0xff, (dst >> 8) & 0xff, inverse);
  const uint32_t b = blend_channel((src >> 16) & 0xff, (dst >> 16) & 0xff, inverse);
  const uint32_t a = blend_channel(src >> 24, dst >> 24, inverse);
  return r | (g << 8) | (b << 16) | (a << 24);
}

inline uint32_t mul(uint32_t channel, uint32_t coverage) {
  const uint32_t value = channel * coverage + 128;
  return (value + (value >> 8)) >> 8;
}

inline uint32_t premultiply(uint32_t rgba, uint32_t coverage) {
  const uint32_t a = mul(rgba >> 24, coverage);
  return mul(rgba & 0xff, a) | (mul((rgba >> 8) & 0xff, a) << 8) | (mul((rgba >> 16) & 0xff, a) << 16) | (a << 24);
}

inline float smooth_coverage(float dist) {
  const float t = std::clamp(0.5f - dist, 0.f, 1.f);
  return t * t * (3 - 2 * t);
}

inline float rounded_distance(float px, float py, float half_w, float half_h, float r) {
  const float qx = std::abs(px - half_w) - half_w + r;
  const float qy = std::abs(py - half_h) - half_h + r;
  const float ox = std::max(qx, 0.f), oy = std::max(qy, 0.f);
  return std::sqrt(ox * ox + oy * oy) + std::min(std::max(qx, qy), 0.f) - r;
}

class Cpu final : public CpuBackend {
 public:
  void resize(int width, int height) override {
    if (width == width_ && height == height_) return;
    width_ = width;
    height_ = height;
    pixels_.assign(static_cast<size_t>(width) * static_cast<size_t>(height), 0xff000000u);
  }

  void upload(const AtlasUpload &upload) override {
    if (upload.atlas_page >= pages_.size()) pages_.resize(upload.atlas_page + 1);
    Page &page = pages_[upload.atlas_page];
    if (page.width != upload.page_width) {
      page.width = upload.page_width;
      page.height = upload.page_height;
      page.pixels.assign(static_cast<size_t>(page.width) * page.height * 4, 0);
    }
    for (uint32_t y = 0; y < upload.height; ++y) {
      std::memcpy(page.pixels.data() + (static_cast<size_t>(upload.destination_y + y) * page.width + upload.destination_x) * 4,
                  upload.pixels.data() + static_cast<size_t>(y) * upload.width * 4, upload.width * 4u);
    }
  }

  void draw(const FrameData &frame, IRect damage, double) override {
    if (damage.empty()) return;
    const uint32_t clear = __builtin_bswap32(frame.clear) | 0xff000000u;
    for (int y = damage.y; y < damage.y + damage.h; ++y)
      std::fill_n(pixels_.data() + static_cast<size_t>(y) * width_ + damage.x, damage.w, clear);

    for (const Batch &batch : frame.batches) {
      if (batch.shader >= 0) continue;
      const IRect clip = intersect(batch.clip, damage);
      if (clip.empty()) continue;
      saved_.clear();
      each_corner_pixel(batch.round, clip, [&](int, int, uint32_t &pixel) { saved_.push_back(pixel); });
      for (uint32_t i = batch.first; i < batch.first + batch.count; ++i) {
        const Instance &inst = frame.instances[i];
        if (inst.flags & InstSolid) {
          if (inst.flags & InstRounded)
            rounded(inst, clip);
          else
            solid(inst, clip);
        } else {
          glyph(inst, clip, batch.page);
        }
      }
      size_t next = 0;
      const float half_w = static_cast<float>(batch.round.w) / 2, half_h = static_cast<float>(batch.round.h) / 2;
      const float radius = std::min({batch.round.radius, half_w, half_h});
      each_corner_pixel(batch.round, clip, [&](int x, int y, uint32_t &pixel) {
        const float dist = rounded_distance(static_cast<float>(x - batch.round.x) + 0.5f, static_cast<float>(y - batch.round.y) + 0.5f, half_w, half_h, radius);
        const auto cover = static_cast<uint32_t>(smooth_coverage(dist) * 255 + 0.5f);
        const uint32_t before = saved_[next++];
        if (cover == 255) return;
        uint32_t mixed = 0;
        for (const uint32_t shift : {0u, 8u, 16u, 24u})
          mixed |= (mul((pixel >> shift) & 0xff, cover) + mul((before >> shift) & 0xff, 255 - cover)) << shift;
        pixel = mixed;
      });
    }
  }

  void read_pixels(std::vector<uint32_t> &out) override { out = pixels_; }

  const uint32_t *pixels() const override { return pixels_.data(); }

 private:
  struct Page {
    uint32_t width = 0, height = 0;
    std::vector<uint8_t> pixels;
  };

  template <typename Visit>
  void each_corner_pixel(const RoundClip &round, IRect clip, Visit &&visit) {
    if (round.radius <= 0) return;
    const int size = std::min(static_cast<int>(std::ceil(round.radius)) + 1, (std::min(round.w, round.h) + 1) / 2);
    const IRect corners[4] = {{round.x, round.y, size, size},
                              {round.x + round.w - size, round.y, size, size},
                              {round.x, round.y + round.h - size, size, size},
                              {round.x + round.w - size, round.y + round.h - size, size, size}};
    for (const IRect &corner : corners) {
      const IRect r = intersect(corner, clip);
      if (r.empty()) continue;
      for (int y = r.y; y < r.y + r.h; ++y) {
        uint32_t *row = pixels_.data() + static_cast<size_t>(y) * width_;
        for (int x = r.x; x < r.x + r.w; ++x) visit(x, y, row[x]);
      }
    }
  }

  void solid(const Instance &inst, IRect clip) {
    const IRect r = intersect({static_cast<int>(inst.x), static_cast<int>(inst.y), static_cast<int>(inst.w), static_cast<int>(inst.h)}, clip);
    if (r.empty()) return;
    const uint32_t src = premultiply(inst.rgba, 255);
    if ((src >> 24) == 255) {
      for (int y = r.y; y < r.y + r.h; ++y) std::fill_n(pixels_.data() + static_cast<size_t>(y) * width_ + r.x, r.w, src);
      return;
    }
    for (int y = r.y; y < r.y + r.h; ++y) {
      uint32_t *row = pixels_.data() + static_cast<size_t>(y) * width_ + r.x;
      for (int x = 0; x < r.w; ++x) row[x] = over(src, row[x]);
    }
  }

  void rounded(const Instance &inst, IRect clip) {
    const int x0 = static_cast<int>(inst.x), y0 = static_cast<int>(inst.y);
    const int w = static_cast<int>(inst.w), h = static_cast<int>(inst.h);
    const IRect r = intersect({x0, y0, w, h}, clip);
    if (r.empty()) return;
    const float half_w = inst.w / 2, half_h = inst.h / 2;
    const float radius = std::min({inst.u0, half_w, half_h});
    const float stroke = inst.v0;
    const int corner = static_cast<int>(std::ceil(radius)) + 1;
    const int band = stroke > 0 ? static_cast<int>(std::ceil(stroke)) + 1 : 0;
    const int inset = std::max(corner, band);
    const uint32_t solid_src = premultiply(inst.rgba, 255);

    const auto shade = [&](int px, int py) {
      const float dist = rounded_distance(static_cast<float>(px) + 0.5f, static_cast<float>(py) + 0.5f, half_w, half_h, radius);
      const float cover = stroke > 0 ? smooth_coverage(std::abs(dist + stroke / 2) - stroke / 2) : smooth_coverage(dist);
      return static_cast<uint32_t>(cover * 255 + 0.5f);
    };

    for (int y = r.y; y < r.y + r.h; ++y) {
      const int py = y - y0;
      const bool edge_row = py < inset || py >= h - inset;
      uint32_t *row = pixels_.data() + static_cast<size_t>(y) * width_;
      for (int x = r.x; x < r.x + r.w; ++x) {
        const int px = x - x0;
        if (!edge_row && px >= inset && px < w - inset) {
          if (stroke > 0) {
            x = std::min(r.x + r.w, x0 + w - inset) - 1;
            continue;
          }
          const int stop = std::min(r.x + r.w, x0 + w - inset);
          if ((solid_src >> 24) == 255) {
            std::fill(row + x, row + stop, solid_src);
          } else {
            for (int fill = x; fill < stop; ++fill) row[fill] = over(solid_src, row[fill]);
          }
          x = stop - 1;
          continue;
        }
        const uint32_t cover = shade(px, py);
        if (cover == 0) continue;
        row[x] = over(premultiply(inst.rgba, cover), row[x]);
      }
    }
  }

  void glyph(const Instance &inst, IRect clip, int page_index) {
    if (page_index < 0 || static_cast<size_t>(page_index) >= pages_.size()) return;
    const Page &page = pages_[static_cast<size_t>(page_index)];
    if (page.pixels.empty()) return;
    const int x0 = static_cast<int>(inst.x), y0 = static_cast<int>(inst.y);
    const IRect r = intersect({x0, y0, static_cast<int>(inst.w), static_cast<int>(inst.h)}, clip);
    if (r.empty()) return;
    const int ax = static_cast<int>(inst.u0), ay = static_cast<int>(inst.v0);
    const bool color = (inst.flags & InstColorGlyph) != 0;
    const uint32_t tint_alpha = inst.rgba >> 24;

    for (int y = r.y; y < r.y + r.h; ++y) {
      const uint8_t *src = page.pixels.data() + (static_cast<size_t>(ay + y - y0) * page.width + static_cast<size_t>(ax + r.x - x0)) * 4;
      uint32_t *dst = pixels_.data() + static_cast<size_t>(y) * width_ + r.x;
      for (int x = 0; x < r.w; ++x, src += 4) {
        const uint32_t texel_alpha = src[3];
        if (texel_alpha == 0) continue;
        uint32_t pixel;
        if (color) {
          pixel = mul(src[0], tint_alpha) | (mul(src[1], tint_alpha) << 8) | (mul(src[2], tint_alpha) << 16) | (mul(texel_alpha, tint_alpha) << 24);
        } else {
          pixel = premultiply(inst.rgba, texel_alpha);
        }
        dst[x] = over(pixel, dst[x]);
      }
    }
  }

  int width_ = 0, height_ = 0;
  std::vector<uint32_t> pixels_, saved_;
  std::vector<Page> pages_;
};

}  // namespace

std::unique_ptr<CpuBackend> make_cpu_backend() { return std::make_unique<Cpu>(); }

}  // namespace qui
