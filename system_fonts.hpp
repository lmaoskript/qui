#pragma once

#include <cstdint>
#include <string>
#include <vector>

namespace qui {

struct SystemFont {
  std::string path;
  int index = 0;
  // wght axis value for a variable font; 0 leaves the face at its default.
  float weight = 0;
};

void init_system_fonts();
SystemFont system_font(bool mono, bool bold);

// Fonts that may cover codepoint, best first. The caller still checks coverage.
std::vector<SystemFont> fallback_fonts(uint32_t codepoint, bool emoji);

}  // namespace qui
