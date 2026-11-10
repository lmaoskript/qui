#pragma once

#include <cstdint>
#include <string_view>
#include <vector>

struct FT_LibraryRec_;

namespace qui {

enum SvgOp : uint8_t { SvgMove, SvgLine, SvgQuad, SvgCubic, SvgClose };

// One filled and/or stroked element, already transformed into viewBox units.
struct SvgShape {
  std::vector<uint8_t> ops;
  std::vector<float> points;
  float fill = 1, stroke = 0;
  float stroke_width = 1, miter = 4;
  uint8_t cap = 0, join = 0;
  bool even_odd = false;
};

// The single-color geometry of an SVG document: paths and basic shapes with
// their fills, strokes and transforms. Colors, gradients, text and clipping
// are not drawn; anything painted counts as covered.
struct SvgIcon {
  float x = 0, y = 0, w = 24, h = 24;
  std::vector<SvgShape> shapes;
};

bool parse_svg(std::string_view text, SvgIcon &out);

// Renders the icon fitted and centered in width x height pixels as 8-bit coverage.
void rasterize_svg(FT_LibraryRec_ *library, const SvgIcon &icon, int width, int height, std::vector<uint8_t> &coverage);

}  // namespace qui
