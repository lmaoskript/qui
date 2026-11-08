//go:build cgo

#include "font_manager.hpp"

#include <ft2build.h>
#include FT_FREETYPE_H
#include FT_GLYPH_H
#include <hb-ft.h>
#include <fontconfig/fontconfig.h>

#include <algorithm>
#include <cmath>
#include <cstdio>
#include <limits>

namespace qui {

namespace {

// HarfBuzz and FreeType must use the same grid-fitting mode. Otherwise a
// fixed-width face can advance by 9.59375px while its bitmap occupies a 10px
// cell, producing alternating gaps and crowded-looking letter pairs.
constexpr FT_Int32 kTextLoadFlags =
    FT_LOAD_DEFAULT | FT_LOAD_TARGET_NORMAL;

}  // namespace

struct FontManager::FontInstance {
  FontInstanceId id = 0;
  std::string path;
  FT_Face face = nullptr;
  hb_font_t *hb = nullptr;
  float pixel_size = 0.0f;
  float raster_scale = 1.0f;
  int face_index = 0;
  bool ligatures_enabled = true;
  FontMetrics metrics;
  std::unordered_map<uint32_t, CachedGlyph> glyphs;

  ~FontInstance() {
    if (hb)
      hb_font_destroy(hb);
    if (face)
      FT_Done_Face(face);
  }
};

FontManager::FontManager() {
  if (!FcInit())
    std::fprintf(stderr, "font: Fontconfig initialization failed\n");
  if (FT_Init_FreeType(&library_) != 0) {
    std::fprintf(stderr, "font: FT_Init_FreeType failed\n");
    library_ = nullptr;
  }
  // Page zero is also used by solid UI instances. Its first texel is reserved.
  pages_.emplace_back();
}

FontManager::~FontManager() {
  fonts_.clear();
  if (library_)
    FT_Done_FreeType(library_);
}

FontInstanceId FontManager::load_font(const std::string &path, float pixel_size,
                                      bool ligatures_enabled) {
  return load_font_face(path, 0, pixel_size, ligatures_enabled);
}

FontInstanceId FontManager::load_font_face(const std::string &path,
                                           int face_index, float pixel_size,
                                           bool ligatures_enabled) {
  if (!library_ || pixel_size <= 0.0f)
    return 0;

  for (const auto &font : fonts_) {
    if (font && font->path == path && font->face_index == face_index &&
        font->pixel_size == pixel_size)
      return font->id;
  }

  auto instance = std::make_unique<FontInstance>();
  if (FT_New_Face(library_, path.c_str(), face_index, &instance->face) != 0) {
    std::fprintf(stderr, "font: failed to load '%s'\n", path.c_str());
    return 0;
  }
  if (FT_Set_Pixel_Sizes(instance->face, 0,
                         static_cast<FT_UInt>(pixel_size + 0.5f)) != 0) {
    int best_strike = -1;
    float best_distance = std::numeric_limits<float>::max();
    for (int index = 0; index < instance->face->num_fixed_sizes; ++index) {
      const float strike_size =
          instance->face->available_sizes[index].y_ppem / 64.0f;
      const float distance = std::abs(strike_size - pixel_size);
      if (distance < best_distance) {
        best_distance = distance;
        best_strike = index;
      }
    }
    if (best_strike < 0 || FT_Select_Size(instance->face, best_strike) != 0) {
      std::fprintf(stderr, "font: failed to set %.1fpx size for '%s'\n",
                   pixel_size, path.c_str());
      return 0;
    }
    const float strike_size =
        instance->face->available_sizes[best_strike].y_ppem / 64.0f;
    instance->raster_scale =
        strike_size > 0.0f ? pixel_size / strike_size : 1.0f;
  }

  instance->hb = hb_ft_font_create_referenced(instance->face);
  if (!instance->hb) {
    std::fprintf(stderr, "font: failed to create HarfBuzz font for '%s'\n",
                 path.c_str());
    return 0;
  }
  hb_ft_font_set_load_flags(instance->hb, kTextLoadFlags);
  // Leave HarfBuzz at the selected bitmap strike. Overriding its scale made
  // hb-ft rescale advances a second time, collapsing a 14px emoji's advance
  // to about 2px; the shaper applies position_scale instead.

  instance->id = static_cast<FontInstanceId>(fonts_.size() + 1);
  instance->path = path;
  instance->face_index = face_index;
  instance->pixel_size = pixel_size;
  instance->ligatures_enabled = ligatures_enabled;
  instance->metrics.ascender =
      instance->face->size->metrics.ascender / 64.0f * instance->raster_scale;
  instance->metrics.descender =
      instance->face->size->metrics.descender / 64.0f * instance->raster_scale;
  instance->metrics.line_height =
      instance->face->size->metrics.height / 64.0f * instance->raster_scale;
  // The terminal grid is based on the advance of a normal printable glyph.
  // max_advance also includes unusually wide symbols and makes every terminal
  // column far too wide for otherwise-monospace fonts.
  if (FT_Load_Char(instance->face, 'M', FT_LOAD_DEFAULT) == 0)
    instance->metrics.cell_advance =
        instance->face->glyph->advance.x / 64.0f * instance->raster_scale;
  else
    instance->metrics.cell_advance =
        instance->face->size->metrics.max_advance / 64.0f *
        instance->raster_scale;
  instance->metrics.position_scale = instance->raster_scale;

  fonts_.push_back(std::move(instance));
  return fonts_.back()->id;
}

bool FontManager::valid_font(FontInstanceId id) const {
  return id != 0 && id <= fonts_.size() && fonts_[id - 1] != nullptr;
}

hb_font_t *FontManager::hb_font(FontInstanceId id) const {
  return valid_font(id) ? fonts_[id - 1]->hb : nullptr;
}

FT_Face FontManager::ft_face(FontInstanceId id) const {
  return valid_font(id) ? fonts_[id - 1]->face : nullptr;
}

const FontMetrics *FontManager::metrics(FontInstanceId id) const {
  return valid_font(id) ? &fonts_[id - 1]->metrics : nullptr;
}

const std::string *FontManager::font_path(FontInstanceId id) const {
  return valid_font(id) ? &fonts_[id - 1]->path : nullptr;
}

bool FontManager::has_codepoint(FontInstanceId font,
                                uint32_t codepoint) const {
  return valid_font(font) &&
         FT_Get_Char_Index(fonts_[font - 1]->face, codepoint) != 0;
}

namespace {

bool emoji_presentation(uint32_t codepoint) {
  return (codepoint >= 0x1f000 && codepoint <= 0x1faff) ||
         (codepoint >= 0x2600 && codepoint <= 0x27ff) ||
         (codepoint >= 0x2300 && codepoint <= 0x23ff);
}

}  // namespace

FontInstanceId FontManager::font_for_codepoint(FontInstanceId preferred,
                                                uint32_t codepoint) {
  if (!valid_font(preferred) || codepoint > 0x10ffff)
    return preferred;
  if (has_codepoint(preferred, codepoint))
    return preferred;

  const uint64_t key = (static_cast<uint64_t>(preferred) << 32) | codepoint;
  const auto cached = fallback_fonts_.find(key);
  if (cached != fallback_fonts_.end())
    return cached->second;

  const float pixel_size = fonts_[preferred - 1]->pixel_size;
  const bool ligatures_enabled = fonts_[preferred - 1]->ligatures_enabled;
  auto match_family = [&](const char *family) -> FontInstanceId {
    FontInstanceId match_font = 0;
    FcPattern *pattern = FcPatternCreate();
    FcCharSet *charset = FcCharSetCreate();
    if (!pattern || !charset ||
        !FcCharSetAddChar(charset, static_cast<FcChar32>(codepoint))) {
      if (charset)
        FcCharSetDestroy(charset);
      if (pattern)
        FcPatternDestroy(pattern);
      return 0;
    }
    FcPatternAddString(pattern, FC_FAMILY,
                       reinterpret_cast<const FcChar8 *>(family));
    FcPatternAddCharSet(pattern, FC_CHARSET, charset);
    FcConfigSubstitute(nullptr, pattern, FcMatchPattern);
    FcDefaultSubstitute(pattern);
    FcResult match_result = FcResultNoMatch;
    FcPattern *match = FcFontMatch(nullptr, pattern, &match_result);
    if (match) {
      FcChar8 *file = nullptr;
      int face_index = 0;
      if (FcPatternGetString(match, FC_FILE, 0, &file) == FcResultMatch) {
        FcPatternGetInteger(match, FC_INDEX, 0, &face_index);
        const FontInstanceId candidate =
            load_font_face(reinterpret_cast<const char *>(file), face_index,
                           pixel_size, ligatures_enabled);
        if (candidate && has_codepoint(candidate, codepoint))
          match_font = candidate;
      }
      FcPatternDestroy(match);
    }
    FcCharSetDestroy(charset);
    FcPatternDestroy(pattern);
    return match_font;
  };

  // Some text-presentation symbols live inside broad Unicode emoji blocks.
  // Fontconfig can honor the requested "emoji" family even when that face does
  // not contain the requested character (for example fish's U+276F prompt
  // arrow). Try the normal text fallback chain when the emoji face has no
  // glyph instead of returning the primary font's .notdef square.
  FontInstanceId result =
      emoji_presentation(codepoint) ? match_family("emoji") : 0;
  if (!result)
    result = match_family("sans-serif");
  if (!result)
    result = preferred;
  fallback_fonts_[key] = result;
  return result;
}

bool FontManager::allocate_atlas_rect(uint32_t width, uint32_t height,
                                      uint32_t &page, uint32_t &x,
                                      uint32_t &y) {
  if (width + kAtlasPadding * 2 > kMaxAtlasSize ||
      height + kAtlasPadding * 2 > kMaxAtlasSize)
    return false;

  const uint32_t required_width = width + kAtlasPadding * 2;
  const uint32_t required_height = height + kAtlasPadding * 2;
  const auto page_size_for = [](uint32_t rw, uint32_t rh) {
    uint32_t size = kDefaultAtlasSize;
    while ((size < rw || size < rh) && size < kMaxAtlasSize)
      size *= 2;
    return size;
  };
  for (;;) {
    AtlasPage &current = pages_.back();
    if (required_width > current.width || required_height > current.height) {
      const uint32_t size = page_size_for(required_width, required_height);
      pages_.push_back({1, 1, 0, size, size});
      continue;
    }
    if (current.cursor_x + required_width > current.width) {
      current.cursor_x = 1;
      current.cursor_y += current.shelf_height;
      current.shelf_height = 0;
    }
    if (current.cursor_y + required_height <= current.height) {
      page = static_cast<uint32_t>(pages_.size() - 1);
      x = current.cursor_x + kAtlasPadding;
      y = current.cursor_y + kAtlasPadding;
      current.cursor_x += required_width;
      current.shelf_height = std::max(current.shelf_height, required_height);
      return true;
    }
    const uint32_t size = page_size_for(required_width, required_height);
    pages_.push_back({1, 1, 0, size, size});
  }
}

bool FontManager::upload_rgba(uint32_t width, uint32_t height,
                              const std::vector<uint8_t> &pixels,
                              AtlasRegion &region) {
  const size_t expected = static_cast<size_t>(width) * height * 4;
  if (width == 0 || height == 0 || pixels.size() != expected)
    return false;

  // Terminal image clients commonly delete and retransmit frames under the
  // same Kitty image id. The logical image disappears, but atlas allocations
  // are immutable, so allocating on every animation tick eventually exhausts
  // device memory. Cache by image content so a finite animation uploads each
  // distinct frame once and then reuses its atlas region.
  RgbaUploadKey key;
  key.width = width;
  key.height = height;
  key.hash_a = 1469598103934665603ull;
  key.hash_b = 0x9e3779b97f4a7c15ull;
  for (const uint8_t byte : pixels) {
    key.hash_a = (key.hash_a ^ byte) * 1099511628211ull;
    key.hash_b ^= static_cast<uint64_t>(byte) + 0x9e3779b97f4a7c15ull +
                  (key.hash_b << 6) + (key.hash_b >> 2);
  }
  if (const auto cached = rgba_upload_cache_.find(key);
      cached != rgba_upload_cache_.end()) {
    region = cached->second;
    return true;
  }

  uint32_t page = 0;
  uint32_t x = 0;
  uint32_t y = 0;
  if (!allocate_atlas_rect(width, height, page, x, y))
    return false;

  AtlasUpload upload;
  upload.atlas_page = page;
  upload.destination_x = x;
  upload.destination_y = y;
  upload.width = width;
  upload.height = height;
  upload.page_width = pages_[page].width;
  upload.page_height = pages_[page].height;
  upload.pixels = pixels;
  // The color-bitmap shader expects premultiplied atlas samples.
  for (size_t at = 0; at < upload.pixels.size(); at += 4) {
    const uint32_t alpha = upload.pixels[at + 3];
    upload.pixels[at] =
        static_cast<uint8_t>((upload.pixels[at] * alpha + 127) / 255);
    upload.pixels[at + 1] =
        static_cast<uint8_t>((upload.pixels[at + 1] * alpha + 127) / 255);
    upload.pixels[at + 2] =
        static_cast<uint8_t>((upload.pixels[at + 2] * alpha + 127) / 255);
  }
  pending_uploads_.push_back(std::move(upload));
  region.page = page;
  const float page_w = static_cast<float>(pages_[page].width);
  const float page_h = static_cast<float>(pages_[page].height);
  region.u0 = static_cast<float>(x) / page_w;
  region.v0 = static_cast<float>(y) / page_h;
  region.u1 = static_cast<float>(x + width) / page_w;
  region.v1 = static_cast<float>(y + height) / page_h;
  rgba_upload_cache_.emplace(key, region);
  return true;
}

const CachedGlyph *FontManager::get_glyph(FontInstanceId font,
                                          uint32_t glyph_id) {
  if (!valid_font(font))
    return nullptr;
  FontInstance &instance = *fonts_[font - 1];
  const auto existing = instance.glyphs.find(glyph_id);
  if (existing != instance.glyphs.end()) {
    ++glyph_cache_hits_;
    return &existing->second;
  }
  ++glyph_cache_misses_;

  CachedGlyph glyph;
  glyph.font = font;
  glyph.glyph_id = glyph_id;
  FT_Face face = instance.face;
  const FT_Int32 load_flags = kTextLoadFlags | FT_LOAD_COLOR;
  if (FT_Load_Glyph(face, glyph_id, load_flags) != 0) {
    std::fprintf(stderr, "font: FT_Load_Glyph failed for glyph %u\n",
                 static_cast<unsigned>(glyph_id));
    return nullptr;
  }
  glyph.advance_x =
      face->glyph->advance.x / 64.0f * instance.raster_scale;

  if (FT_Render_Glyph(face->glyph, FT_RENDER_MODE_NORMAL) != 0) {
    std::fprintf(stderr, "font: FT_Render_Glyph failed for glyph %u\n",
                 static_cast<unsigned>(glyph_id));
    return nullptr;
  }
  const FT_Bitmap &bitmap = face->glyph->bitmap;
  if (bitmap.pixel_mode != FT_PIXEL_MODE_GRAY &&
      bitmap.pixel_mode != FT_PIXEL_MODE_BGRA &&
      bitmap.pixel_mode != FT_PIXEL_MODE_MONO) {
    std::fprintf(stderr, "font: unsupported bitmap mode %u for glyph %u\n",
                 static_cast<unsigned>(bitmap.pixel_mode),
                 static_cast<unsigned>(glyph_id));
    return nullptr;
  }

  glyph.bitmap_width = static_cast<uint32_t>(
      std::lround(bitmap.width * instance.raster_scale));
  glyph.bitmap_height = static_cast<uint32_t>(
      std::lround(bitmap.rows * instance.raster_scale));
  glyph.bitmap_left = static_cast<int32_t>(
      std::lround(face->glyph->bitmap_left * instance.raster_scale));
  glyph.bitmap_top = static_cast<int32_t>(
      std::lround(face->glyph->bitmap_top * instance.raster_scale));
  glyph.has_bitmap = bitmap.width != 0 && bitmap.rows != 0;
  glyph.color_bitmap = bitmap.pixel_mode == FT_PIXEL_MODE_BGRA;

  if (glyph.has_bitmap) {
    uint32_t page = 0;
    uint32_t x = 0;
    uint32_t y = 0;
    if (!allocate_atlas_rect(glyph.bitmap_width, glyph.bitmap_height, page, x,
                             y)) {
      std::fprintf(stderr, "font: glyph %u is too large for a %ux%u atlas\n",
                   static_cast<unsigned>(glyph_id), kMaxAtlasSize, kMaxAtlasSize);
      return nullptr;
    }
    glyph.atlas_page = page;
    glyph.atlas_x = x;
    glyph.atlas_y = y;
    const float page_w = static_cast<float>(pages_[page].width);
    const float page_h = static_cast<float>(pages_[page].height);
    glyph.u0 = static_cast<float>(x) / page_w;
    glyph.v0 = static_cast<float>(y) / page_h;
    glyph.u1 = static_cast<float>(x + glyph.bitmap_width) / page_w;
    glyph.v1 = static_cast<float>(y + glyph.bitmap_height) / page_h;

    AtlasUpload upload;
    upload.atlas_page = page;
    upload.destination_x = x;
    upload.destination_y = y;
    upload.width = glyph.bitmap_width;
    upload.height = glyph.bitmap_height;
    upload.page_width = pages_[page].width;
    upload.page_height = pages_[page].height;
    upload.pixels.resize(static_cast<size_t>(upload.width) * upload.height * 4);

    const int pitch = bitmap.pitch;
    const uint8_t *first_row = bitmap.buffer;
    if (pitch < 0)
      first_row += static_cast<size_t>(bitmap.rows - 1) *
                   static_cast<size_t>(-pitch);
    auto source_pixel = [&](unsigned source_x, unsigned source_y,
                            unsigned channel) -> uint8_t {
      const uint8_t *source =
          first_row + static_cast<ptrdiff_t>(source_y) * pitch;
      if (bitmap.pixel_mode == FT_PIXEL_MODE_BGRA) {
        constexpr unsigned rgba_to_bgra[] = {2, 1, 0, 3};
        return source[source_x * 4 + rgba_to_bgra[channel]];
      }
      if (channel < 3)
        return 255;
      if (bitmap.pixel_mode == FT_PIXEL_MODE_MONO)
        return (source[source_x >> 3] & (0x80u >> (source_x & 7u))) ? 255
                                                                    : 0;
      return source[source_x];
    };
    for (uint32_t row = 0; row < upload.height; ++row) {
      uint8_t *destination =
          upload.pixels.data() + static_cast<size_t>(row) * upload.width * 4;
      for (uint32_t column = 0; column < upload.width; ++column) {
        const unsigned source_x0 = std::min<unsigned>(
            bitmap.width - 1,
            static_cast<unsigned>(column / instance.raster_scale));
        const unsigned source_y0 = std::min<unsigned>(
            bitmap.rows - 1,
            static_cast<unsigned>(row / instance.raster_scale));
        const unsigned source_x1 = std::min<unsigned>(
            bitmap.width,
            std::max(source_x0 + 1,
                     static_cast<unsigned>(
                         std::ceil((column + 1) / instance.raster_scale))));
        const unsigned source_y1 = std::min<unsigned>(
            bitmap.rows,
            std::max(source_y0 + 1,
                     static_cast<unsigned>(
                         std::ceil((row + 1) / instance.raster_scale))));
        const uint64_t sample_count =
            static_cast<uint64_t>(source_x1 - source_x0) *
            (source_y1 - source_y0);
        for (unsigned channel = 0; channel < 4; ++channel) {
          uint64_t sum = 0;
          for (unsigned source_y = source_y0; source_y < source_y1;
               ++source_y) {
            for (unsigned source_x = source_x0; source_x < source_x1;
                 ++source_x)
              sum += source_pixel(source_x, source_y, channel);
          }
          destination[column * 4 + channel] =
              static_cast<uint8_t>((sum + sample_count / 2) / sample_count);
        }
      }
    }
    pending_uploads_.push_back(std::move(upload));
    ++glyph_rasterizations_;
  }

  auto [it, inserted] = instance.glyphs.emplace(glyph_id, glyph);
  if (!inserted)
    return &it->second;
  return &it->second;
}

}  // namespace qui
