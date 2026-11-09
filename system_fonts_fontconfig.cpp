//go:build cgo && linux && !android

#include "system_fonts.hpp"

#include <fontconfig/fontconfig.h>

#include <cstdio>

namespace qui {

namespace {

SystemFont match(const char *family, int weight, FcCharSet *charset) {
  SystemFont font;
  FcPattern *pattern = FcPatternCreate();
  if (!pattern) return font;
  FcPatternAddString(pattern, FC_FAMILY, reinterpret_cast<const FcChar8 *>(family));
  if (weight >= 0) FcPatternAddInteger(pattern, FC_WEIGHT, weight);
  if (charset) FcPatternAddCharSet(pattern, FC_CHARSET, charset);
  FcConfigSubstitute(nullptr, pattern, FcMatchPattern);
  FcDefaultSubstitute(pattern);
  FcResult result = FcResultNoMatch;
  if (FcPattern *found = FcFontMatch(nullptr, pattern, &result)) {
    FcChar8 *file = nullptr;
    if (FcPatternGetString(found, FC_FILE, 0, &file) == FcResultMatch) {
      font.path = reinterpret_cast<const char *>(file);
      if (charset) FcPatternGetInteger(found, FC_INDEX, 0, &font.index);
    }
    FcPatternDestroy(found);
  }
  FcPatternDestroy(pattern);
  return font;
}

}  // namespace

void init_system_fonts() {
  if (!FcInit()) std::fprintf(stderr, "font: Fontconfig initialization failed\n");
}

SystemFont system_font(bool mono, bool bold) {
  return match(mono ? "monospace" : "sans-serif", bold ? FC_WEIGHT_SEMIBOLD : FC_WEIGHT_REGULAR, nullptr);
}

std::vector<SystemFont> fallback_fonts(uint32_t codepoint, bool emoji) {
  std::vector<SystemFont> out;
  FcCharSet *charset = FcCharSetCreate();
  if (!charset) return out;
  if (FcCharSetAddChar(charset, static_cast<FcChar32>(codepoint))) {
    // Some text-presentation symbols live inside broad Unicode emoji blocks.
    // Fontconfig can honor the requested "emoji" family even when that face
    // does not contain the requested character (for example fish's U+276F
    // prompt arrow), so the normal text chain is always tried as well.
    if (emoji) out.push_back(match("emoji", -1, charset));
    out.push_back(match("sans-serif", -1, charset));
  }
  FcCharSetDestroy(charset);
  return out;
}

}  // namespace qui
