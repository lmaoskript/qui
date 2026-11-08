//go:build cgo

#include "text_shaper.hpp"

#include <hb.h>

#include <algorithm>
#include <cmath>
#include <cstdio>
#include <cstring>

namespace qui {

namespace {

void append_number(std::string &key, uint64_t value) {
  key.append(reinterpret_cast<const char *>(&value), sizeof(value));
}

struct Utf8Character {
  uint32_t codepoint = 0xfffd;
  uint32_t end = 0;
};

Utf8Character decode_utf8(std::string_view text, uint32_t start,
                          uint32_t limit) {
  const unsigned char first = static_cast<unsigned char>(text[start]);
  uint32_t length = 1;
  uint32_t value = first;
  if ((first & 0xe0u) == 0xc0u) {
    length = 2;
    value = first & 0x1fu;
  } else if ((first & 0xf0u) == 0xe0u) {
    length = 3;
    value = first & 0x0fu;
  } else if ((first & 0xf8u) == 0xf0u) {
    length = 4;
    value = first & 0x07u;
  } else if (first >= 0x80u) {
    return {0xfffd, start + 1};
  }
  if (start + length > limit)
    return {0xfffd, start + 1};
  for (uint32_t index = 1; index < length; ++index) {
    const unsigned char continuation =
        static_cast<unsigned char>(text[start + index]);
    if ((continuation & 0xc0u) != 0x80u)
      return {0xfffd, start + 1};
    value = (value << 6) | (continuation & 0x3fu);
  }
  return {value, start + length};
}

enum class Bidi : uint8_t { Left, Right, Arabic, Number, ArabicNumber, Neutral, Mark };

Bidi bidi_class(uint32_t c) {
  if (c >= '0' && c <= '9')
    return Bidi::Number;
  if ((c >= 0x660 && c <= 0x669) || c == 0x66b || c == 0x66c)
    return Bidi::ArabicNumber;
  if (c >= 0x6f0 && c <= 0x6f9)
    return Bidi::Number;
  if (c == 0x60c)
    return Bidi::Neutral;
  if ((c >= 0x600 && c <= 0x7bf) || (c >= 0x860 && c <= 0x8ff) ||
      (c >= 0xfb50 && c <= 0xfdff) || (c >= 0xfe70 && c <= 0xfefc) ||
      (c >= 0x1ee00 && c <= 0x1eeff))
    return Bidi::Arabic;
  if ((c >= 0x590 && c <= 0x5ff) || (c >= 0x7c0 && c <= 0x85f) ||
      (c >= 0xfb1d && c <= 0xfb4f) || (c >= 0x10800 && c <= 0x10fff) ||
      (c >= 0x1e800 && c <= 0x1efff))
    return Bidi::Right;
  if ((c >= 0x300 && c <= 0x36f) || (c >= 0xfe00 && c <= 0xfe0f) ||
      (c >= 0x1f3fb && c <= 0x1f3ff))
    return Bidi::Mark;
  if (c < 0x80)
    return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ? Bidi::Left
                                                            : Bidi::Neutral;
  if ((c >= 0xa0 && c <= 0xbf) || c == 0xd7 || c == 0xf7 ||
      (c >= 0x2000 && c <= 0x2bff) || (c >= 0x3000 && c <= 0x303f) ||
      (c >= 0x1f000 && c <= 0x1faff))
    return Bidi::Neutral;
  return Bidi::Left;
}

bool joins_any_numbers(uint32_t c) {
  return c == ',' || c == '.' || c == ':' || c == '/' || c == 0x60c;
}

bool joins_european_numbers(uint32_t c) { return c == '+' || c == '-'; }

bool is_strong(Bidi kind) {
  return kind == Bidi::Left || kind == Bidi::Right || kind == Bidi::Arabic ||
         kind == Bidi::Number || kind == Bidi::ArabicNumber;
}

bool sticks_to_numbers(uint32_t c) {
  return c == '%' || c == '$' || c == '#' || c == 0x20ac || c == 0xa3;
}

// A compact form of the Unicode bidi algorithm for a left-to-right paragraph,
// which is what the browser uses for text without a dir attribute. Levels are
// 0 for left-to-right, 1 for right-to-left and 2 for numbers inside it.
bool resolve_levels(std::string_view text, std::vector<uint8_t> &levels) {
  if (std::none_of(text.begin(), text.end(), [](char c) {
        return static_cast<unsigned char>(c) >= 0xd6;
      }))
    return false;

  struct Character {
    uint32_t start, end, codepoint;
    Bidi kind;
    uint8_t level;
  };
  std::vector<Character> characters;
  bool any_right = false;
  const auto size = static_cast<uint32_t>(text.size());
  for (uint32_t at = 0; at < size;) {
    const Utf8Character c = decode_utf8(text, at, size);
    const Bidi kind = bidi_class(c.codepoint);
    any_right |= kind == Bidi::Right || kind == Bidi::Arabic;
    characters.push_back({at, c.end, c.codepoint, kind, 0});
    at = c.end;
  }
  if (!any_right)
    return false;

  const auto number = [](Bidi kind) {
    return kind == Bidi::Number || kind == Bidi::ArabicNumber;
  };
  const size_t count = characters.size();
  Bidi last_strong = Bidi::Left;
  for (Character &c : characters) {
    if (c.kind == Bidi::Left || c.kind == Bidi::Right || c.kind == Bidi::Arabic)
      last_strong = c.kind;
    else if (c.kind == Bidi::Number && last_strong == Bidi::Arabic)
      c.kind = Bidi::ArabicNumber;
  }
  for (size_t i = 0; i < count; ++i) {
    Character &c = characters[i];
    if (c.kind != Bidi::Neutral)
      continue;
    const Bidi before = i > 0 ? characters[i - 1].kind : Bidi::Neutral;
    const Bidi after = i + 1 < count ? characters[i + 1].kind : Bidi::Neutral;
    if (joins_any_numbers(c.codepoint) && number(before) && before == after)
      c.kind = before;
    else if (joins_european_numbers(c.codepoint) && before == Bidi::Number &&
             after == Bidi::Number)
      c.kind = Bidi::Number;
    else if (sticks_to_numbers(c.codepoint) &&
             (before == Bidi::Number || after == Bidi::Number))
      c.kind = Bidi::Number;
  }

  bool after_right = false;
  for (Character &c : characters) {
    if (c.kind == Bidi::Left) {
      after_right = false;
    } else if (c.kind == Bidi::Right || c.kind == Bidi::Arabic) {
      after_right = true;
      c.level = 1;
    } else if (c.kind == Bidi::ArabicNumber) {
      c.level = 2;
    } else if (c.kind == Bidi::Number) {
      c.level = after_right ? 2 : 0;
    }
  }

  std::vector<size_t> open_brackets;
  std::vector<std::pair<size_t, size_t>> bracket_pairs;
  for (size_t i = 0; i < count; ++i) {
    if (characters[i].kind != Bidi::Neutral)
      continue;
    const uint32_t c = characters[i].codepoint;
    if (c == '(' || c == '[' || c == '{') {
      open_brackets.push_back(i);
    } else if (c == ')' || c == ']' || c == '}') {
      const uint32_t opener = c == ')' ? '(' : c - 2;
      for (size_t depth = open_brackets.size(); depth-- > 0;) {
        if (characters[open_brackets[depth]].codepoint != opener)
          continue;
        bracket_pairs.emplace_back(open_brackets[depth], i);
        open_brackets.resize(depth);
        break;
      }
    }
  }
  std::sort(bracket_pairs.begin(), bracket_pairs.end());
  for (const auto &[open, close] : bracket_pairs) {
    bool inner_left = false, inner_right = false;
    for (size_t i = open + 1; i < close; ++i) {
      if (!is_strong(characters[i].kind))
        continue;
      (characters[i].level > 0 ? inner_right : inner_left) = true;
    }
    if (!inner_left && !inner_right)
      continue;
    bool right = false;
    for (size_t i = open; !inner_left && i-- > 0;) {
      if (!is_strong(characters[i].kind))
        continue;
      right = characters[i].level > 0;
      break;
    }
    for (const size_t i : {open, close}) {
      characters[i].kind = right ? Bidi::Right : Bidi::Left;
      characters[i].level = right ? 1 : 0;
    }
  }

  for (size_t i = 0; i < count;) {
    const Bidi kind = characters[i].kind;
    if (kind == Bidi::Mark && i > 0) {
      characters[i].level = characters[i - 1].level;
      ++i;
      continue;
    }
    if (kind != Bidi::Neutral && kind != Bidi::Mark) {
      ++i;
      continue;
    }
    size_t end = i;
    while (end < count && (characters[end].kind == Bidi::Neutral ||
                           characters[end].kind == Bidi::Mark))
      ++end;
    const uint8_t level =
        i > 0 && characters[i - 1].level > 0 && end < count &&
                characters[end].level > 0
            ? 1
            : 0;
    for (; i < end; ++i)
      characters[i].level = level;
  }

  levels.assign(text.size(), 0);
  for (const Character &c : characters)
    std::fill(levels.begin() + c.start, levels.begin() + c.end, c.level);
  return true;
}

bool joins_previous_font(uint32_t codepoint) {
  return codepoint == 0x200d || codepoint == 0xfe0e ||
         codepoint == 0xfe0f ||
         (codepoint >= 0x1f3fb && codepoint <= 0x1f3ff);
}

}  // namespace

void TextShaper::make_runs(std::vector<Run> &runs, uint32_t text_size,
                           std::span<const TextStyleSpan> styles,
                           ShapingOptions options) {
  runs.clear();
  runs.reserve(styles.size() + 1);

  const auto merge = [&](const TextStyleSpan *begin, const TextStyleSpan *end) {
    uint32_t cursor = 0;
    for (const TextStyleSpan *span = begin; span != end; ++span) {
      const uint32_t start = std::min(span->start_byte, text_size);
      const uint32_t finish =
          std::min(std::max(span->end_byte, start), text_size);
      if (start > cursor) {
        runs.push_back({cursor, start, options.default_color,
                        options.default_font, options.ligatures_enabled});
      }
      if (finish > start) {
        Run run{start, finish, span->packed_rgba,
                span->font ? span->font : options.default_font,
                span->ligatures_enabled && options.ligatures_enabled};
        if (!runs.empty() && runs.back().end == run.start &&
            runs.back().packed_rgba == run.packed_rgba &&
            runs.back().font == run.font &&
            runs.back().ligatures_enabled == run.ligatures_enabled) {
          runs.back().end = run.end;
        } else {
          runs.push_back(run);
        }
        cursor = std::max(cursor, finish);
      }
    }
    if (cursor < text_size)
      runs.push_back({cursor, text_size, options.default_color,
                      options.default_font, options.ligatures_enabled});
  };

  // Callers (syntax highlighting, labels) already emit spans ordered by start
  // byte. Skip the copy + sort on that common path and only fall back to
  // sorting when the vector is genuinely out of order.
  const bool sorted = std::is_sorted(
      styles.begin(), styles.end(), [](const TextStyleSpan &a, const TextStyleSpan &b) {
        return a.start_byte < b.start_byte;
      });
  if (sorted) {
    merge(styles.data(), styles.data() + styles.size());
  } else {
    std::vector<TextStyleSpan> ordered(styles.begin(), styles.end());
    std::sort(ordered.begin(), ordered.end(),
              [](const TextStyleSpan &a, const TextStyleSpan &b) {
                return a.start_byte < b.start_byte;
              });
    merge(ordered.data(), ordered.data() + ordered.size());
  }
  if (runs.empty() && text_size != 0)
    runs.push_back({0, text_size, options.default_color,
                    options.default_font, options.ligatures_enabled});
}

void TextShaper::make_cache_key(std::string &key, std::string_view text,
                                const std::vector<Run> &runs) {
  key.clear();
  key.reserve(text.size() + runs.size() * 24 + 16);
  key.append(text.data(), text.size());
  key.push_back('\0');
  for (const Run &run : runs) {
    append_number(key, run.start);
    append_number(key, run.end);
    append_number(key, run.packed_rgba);
    append_number(key, run.font);
    append_number(key, run.ligatures_enabled ? 1u : 0u);
  }
}

const ShapedLine &TextShaper::shape_line(
    std::string_view utf8, std::span<const TextStyleSpan> styles,
    ShapingOptions options) {
  make_runs(scratch_runs_, static_cast<uint32_t>(utf8.size()), styles, options);
  return shape_from_runs(utf8);
}

const ShapedLine &TextShaper::shape_line_single(std::string_view utf8,
                                                TextStyleSpan style,
                                                ShapingOptions options) {
  scratch_runs_.clear();
  const uint32_t size = static_cast<uint32_t>(utf8.size());
  const uint32_t start = std::min(style.start_byte, size);
  const uint32_t end = std::min(std::max(style.end_byte, start), size);
  if (start > 0)
    scratch_runs_.push_back({0, start, options.default_color,
                             options.default_font, options.ligatures_enabled});
  if (end > start)
    scratch_runs_.push_back(
        {start, end, style.packed_rgba,
         style.font ? style.font : options.default_font,
         style.ligatures_enabled && options.ligatures_enabled});
  if (end < size)
    scratch_runs_.push_back({end, size, options.default_color,
                             options.default_font, options.ligatures_enabled});
  if (scratch_runs_.empty() && size != 0)
    scratch_runs_.push_back({0, size, options.default_color,
                             options.default_font, options.ligatures_enabled});
  return shape_from_runs(utf8);
}

const ShapedLine &TextShaper::shape_from_runs(std::string_view utf8) {
  make_cache_key(scratch_key_, utf8, scratch_runs_);
  const auto existing = cache_.find(scratch_key_);
  if (existing != cache_.end()) {
    ++cache_hits_;
    return existing->second;
  }
  ++cache_misses_;
  ++shape_calls_;

  ShapedLine line;
  const bool bidi = resolve_levels(utf8, scratch_levels_);
  const auto level_at = [&](uint32_t byte) -> uint8_t {
    return bidi && byte < scratch_levels_.size() ? scratch_levels_[byte] : 0;
  };
  for (const Run &run : scratch_runs_) {
    if (run.start >= run.end)
      continue;

    auto shape_segment = [&](uint32_t segment_start, uint32_t segment_end,
                             FontInstanceId segment_font, uint8_t level) {
      if (segment_start >= segment_end)
        return;
      const FontMetrics *font_metrics = fonts_.metrics(segment_font);
      hb_font_t *font = fonts_.hb_font(segment_font);
      if (!font || !font_metrics)
        return;
      line.ascent = std::max(line.ascent, font_metrics->ascender);
      line.descent = std::min(line.descent, font_metrics->descender);
      line.line_height = std::max(line.line_height, font_metrics->line_height);
      hb_buffer_t *buffer = hb_buffer_create();
      if (!buffer)
        return;
      hb_buffer_set_cluster_level(buffer,
                                  HB_BUFFER_CLUSTER_LEVEL_MONOTONE_CHARACTERS);
      const bool right_to_left = (level & 1) != 0;
      hb_buffer_set_direction(buffer, right_to_left ? HB_DIRECTION_RTL
                                                    : HB_DIRECTION_LTR);
      hb_buffer_set_language(buffer, hb_language_from_string("en", -1));
      hb_buffer_add_utf8(buffer, utf8.data(), static_cast<int>(utf8.size()),
                         static_cast<unsigned int>(segment_start),
                         static_cast<int>(segment_end - segment_start));
      hb_buffer_guess_segment_properties(buffer);

      hb_feature_t features[4];
      const char *feature_text = run.ligatures_enabled
                                     ? "kern=1,liga=1,clig=1,calt=1"
                                     : "kern=1,liga=0,clig=0,calt=0";
      unsigned feature_count = 0;
      // Parse the comma-separated feature string explicitly so the feature
      // count stays correct on older HarfBuzz versions.
      const char *cursor = feature_text;
      while (*cursor && feature_count < 4) {
        const char *next = std::strchr(cursor, ',');
        const int length = next ? static_cast<int>(next - cursor) : -1;
        if (!hb_feature_from_string(cursor, length,
                                    &features[feature_count])) {
          feature_count = 0;
          break;
        }
        ++feature_count;
        if (!next)
          break;
        cursor = next + 1;
      }
      if (feature_count != 0) {
        hb_shape(font, buffer, features, feature_count);
        unsigned glyph_count = 0;
        const hb_glyph_info_t *infos =
            hb_buffer_get_glyph_infos(buffer, &glyph_count);
        const hb_glyph_position_t *positions =
            hb_buffer_get_glyph_positions(buffer, &glyph_count);
        // Glyphs are stored in logical order so wrapping stays simple; the
        // painter mirrors right-to-left spans per line.
        const auto logical = [&](unsigned n) {
          return right_to_left ? glyph_count - 1 - n : n;
        };
        for (unsigned n = 0; n < glyph_count; ++n) {
          const unsigned i = logical(n);
          uint32_t cluster_end = segment_end;
          for (unsigned m = n + 1; m < glyph_count; ++m) {
            const unsigned j = logical(m);
            if (infos[j].cluster != infos[i].cluster) {
              cluster_end = infos[j].cluster;
              break;
            }
          }
          ShapedGlyph glyph;
          glyph.font = segment_font;
          glyph.glyph_id = infos[i].codepoint;
          glyph.cluster_start_byte = infos[i].cluster;
          glyph.cluster_end_byte =
              std::max(glyph.cluster_start_byte, cluster_end);
          glyph.x = line.total_advance;
          const float scale = font_metrics->position_scale / 64.0f;
          glyph.x_advance = positions[i].x_advance * scale;
          glyph.y_advance = positions[i].y_advance * scale;
          glyph.x_offset = positions[i].x_offset * scale;
          glyph.y_offset = positions[i].y_offset * scale;
          glyph.packed_rgba = run.packed_rgba;
          glyph.level = level;
          line.has_rtl |= level != 0;
          line.glyphs.push_back(glyph);
          line.total_advance += glyph.x_advance;
        }
      }
      hb_buffer_destroy(buffer);
    };

    uint32_t segment_start = run.start;
    FontInstanceId segment_font = run.font;
    uint8_t segment_level = level_at(run.start);
    for (uint32_t position = run.start; position < run.end;) {
      const Utf8Character character = decode_utf8(utf8, position, run.end);
      if (character.codepoint == '\t') {
        shape_segment(segment_start, position, segment_font, segment_level);
        const FontMetrics *metrics = fonts_.metrics(run.font);
        const float cell =
            metrics && metrics->cell_advance > 0.0f
                ? metrics->cell_advance
                : std::max(1.0f, metrics ? metrics->line_height * 0.5f : 8.0f);
        const float tab_width = cell * 4.0f;
        line.total_advance =
            (std::floor(line.total_advance / tab_width) + 1.0f) * tab_width;
        segment_start = character.end;
        segment_font = run.font;
        segment_level = level_at(character.end);
        position = character.end;
        continue;
      }
      const FontInstanceId character_font =
          joins_previous_font(character.codepoint) && position > segment_start
              ? segment_font
              : fonts_.font_for_codepoint(run.font, character.codepoint);
      const uint8_t character_level = level_at(position);
      if (position > segment_start && (character_font != segment_font ||
                                       character_level != segment_level)) {
        shape_segment(segment_start, position, segment_font, segment_level);
        segment_start = position;
      }
      segment_font = character_font;
      segment_level = character_level;
      position = character.end;
    }
    shape_segment(segment_start, run.end, segment_font, segment_level);
  }

  if (line.line_height <= 0.0f)
    line.line_height = line.ascent - line.descent;
  const size_t estimated_bytes =
      scratch_key_.size() + line.glyphs.capacity() * sizeof(ShapedGlyph) +
      sizeof(ShapedLine);
  if (estimated_bytes > kMaximumCacheBytes) {
    uncached_line_ = std::move(line);
    return uncached_line_;
  }
  // The key is copied from the reusable scratch buffer on insertion; a cache
  // miss is rare enough that one copy per miss is irrelevant.
  auto [it, inserted] =
      cache_.emplace(scratch_key_, std::move(line));
  (void)inserted;
  cache_bytes_ += estimated_bytes;
  return it->second;
}


}  // namespace qui
