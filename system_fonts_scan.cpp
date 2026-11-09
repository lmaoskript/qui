//go:build cgo && (windows || android)

#include "system_fonts.hpp"

#include <ft2build.h>
#include FT_FREETYPE_H

#include <algorithm>
#include <cctype>
#include <cstdlib>
#include <filesystem>
#include <string>
#include <string_view>
#include <utility>

namespace qui {

namespace {

struct Face {
  const char *file;
  float weight = 0;
};

#if defined(_WIN32)
constexpr Face kSans[] = {{"segoeui.ttf"}, {"arial.ttf"}, {"tahoma.ttf"}};
constexpr Face kSansBold[] = {{"seguisb.ttf"}, {"segoeuib.ttf"}, {"arialbd.ttf"}, {"tahomabd.ttf"}};
constexpr Face kMono[] = {{"consola.ttf"}, {"cour.ttf"}, {"lucon.ttf"}};
constexpr Face kMonoBold[] = {{"consolab.ttf"}, {"courbd.ttf"}, {"lucon.ttf"}};
constexpr const char *kEmoji[] = {"seguiemj.ttf"};
constexpr const char *kPreferred[] = {"segoeui.ttf", "seguisym.ttf", "msyh.ttc", "YuGothM.ttc", "malgun.ttf", "arial.ttf", "tahoma.ttf"};
#else
constexpr Face kSans[] = {{"Roboto-Regular.ttf", 400}, {"NotoSans-Regular.ttf"}, {"DroidSans.ttf"}};
constexpr Face kSansBold[] = {{"Roboto-Regular.ttf", 600}, {"Roboto-Medium.ttf"}, {"NotoSans-Bold.ttf"}, {"DroidSans-Bold.ttf"}};
constexpr Face kMono[] = {{"DroidSansMono.ttf"}, {"CutiveMono.ttf"}};
constexpr Face kMonoBold[] = {{"DroidSansMono.ttf"}, {"CutiveMono.ttf"}};
constexpr const char *kEmoji[] = {"NotoColorEmoji.ttf", "NotoColorEmojiFlags.ttf"};
constexpr const char *kPreferred[] = {"Roboto-Regular.ttf", "NotoSansCJK-Regular.ttc", "NotoSansSymbols-Regular-Subsetted.ttf"};
#endif

std::vector<std::filesystem::path> font_dirs() {
#if defined(_WIN32)
  const char *windows = std::getenv("WINDIR");
  std::vector<std::filesystem::path> dirs{std::filesystem::path(windows ? windows : "C:\\Windows") / "Fonts"};
  if (const char *local = std::getenv("LOCALAPPDATA")) dirs.push_back(std::filesystem::path(local) / "Microsoft" / "Windows" / "Fonts");
  dirs.emplace_back("Z:\\usr\\share\\wine\\fonts");
  return dirs;
#else
  return {"/system/fonts"};
#endif
}

using Ranges = std::vector<std::pair<uint32_t, uint32_t>>;

struct Installed {
  std::string path;
  bool emoji = false, scanned = false;
  Ranges ranges;
};

FT_Library library() {
  static const FT_Library handle = [] {
    FT_Library out = nullptr;
    FT_Init_FreeType(&out);
    return out;
  }();
  return handle;
}

bool has_font_extension(const std::filesystem::path &path) {
  std::string extension = path.extension().string();
  std::transform(extension.begin(), extension.end(), extension.begin(), [](unsigned char c) { return static_cast<char>(std::tolower(c)); });
  return extension == ".ttf" || extension == ".ttc" || extension == ".otf";
}

bool same_name(std::string_view a, std::string_view b) {
  return a.size() == b.size() && std::equal(a.begin(), a.end(), b.begin(), [](unsigned char x, unsigned char y) { return std::tolower(x) == std::tolower(y); });
}

// Fallback should find upright sans faces before their bold, italic or serif siblings.
int style_rank(std::string name) {
  std::transform(name.begin(), name.end(), name.begin(), [](unsigned char c) { return static_cast<char>(std::tolower(c)); });
  int rank = 0;
  for (const char *word : {"bold", "italic", "serif", "bd.", "bi."})
    if (name.find(word) != std::string::npos) ++rank;
  return rank;
}

std::vector<Installed> &installed() {
  static std::vector<Installed> list = [] {
    std::vector<std::filesystem::path> files;
    for (const std::filesystem::path &dir : font_dirs()) {
      std::error_code error;
      for (const auto &entry : std::filesystem::directory_iterator(dir, error))
        if (has_font_extension(entry.path())) files.push_back(entry.path());
    }
    std::stable_sort(files.begin(), files.end(), [](const auto &a, const auto &b) {
      const int rank_a = style_rank(a.filename().string()), rank_b = style_rank(b.filename().string());
      return rank_a != rank_b ? rank_a < rank_b : a.filename() < b.filename();
    });
    std::vector<Installed> out;
    const auto take = [&](std::string_view name, bool emoji) {
      for (auto it = files.begin(); it != files.end(); ++it) {
        if (!same_name(it->filename().string(), name)) continue;
        out.push_back({it->string(), emoji});
        files.erase(it);
        return;
      }
    };
    for (const char *name : kEmoji) take(name, true);
    for (const char *name : kPreferred) take(name, false);
    for (const auto &file : files) out.push_back({file.string()});
    return out;
  }();
  return list;
}

void scan(Installed &font) {
  font.scanned = true;
  FT_Face face = nullptr;
  if (!library() || FT_New_Face(library(), font.path.c_str(), 0, &face) != 0) return;
  FT_UInt glyph = 0;
  for (FT_ULong code = FT_Get_First_Char(face, &glyph); glyph != 0; code = FT_Get_Next_Char(face, code, &glyph)) {
    const auto point = static_cast<uint32_t>(code);
    if (!font.ranges.empty() && font.ranges.back().second + 1 == point) font.ranges.back().second = point;
    else font.ranges.emplace_back(point, point);
  }
  FT_Done_Face(face);
}

bool covers(const Ranges &ranges, uint32_t codepoint) {
  const auto it = std::upper_bound(ranges.begin(), ranges.end(), codepoint, [](uint32_t value, const auto &range) { return value < range.first; });
  return it != ranges.begin() && std::prev(it)->second >= codepoint;
}

template <size_t N>
SystemFont first_present(const Face (&faces)[N]) {
  for (const Face &face : faces)
    for (const Installed &entry : installed())
      if (same_name(std::filesystem::path(entry.path).filename().string(), face.file)) return {entry.path, 0, face.weight};
  return {};
}

}  // namespace

void init_system_fonts() {}

SystemFont system_font(bool mono, bool bold) {
  SystemFont font = mono ? (bold ? first_present(kMonoBold) : first_present(kMono)) : (bold ? first_present(kSansBold) : first_present(kSans));
  if (!font.path.empty()) return font;
  if (!installed().empty()) return {installed().front().path};
  return {};
}

std::vector<SystemFont> fallback_fonts(uint32_t codepoint, bool emoji) {
  std::vector<SystemFont> out;
  for (Installed &font : installed()) {
    if (font.emoji && !emoji) continue;
    if (!font.scanned) scan(font);
    if (!covers(font.ranges, codepoint)) continue;
    out.push_back({font.path});
    if (out.size() == 3) break;
  }
  return out;
}

}  // namespace qui
