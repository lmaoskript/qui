package qui

import (
	"encoding/binary"
	"sort"
	"strings"
)

// Token returns the theme color slot id. Ids 1 to 19 are the built-in tokens
// (Background .. Shadow); ids 20 to 255 belong to your app. Give them values
// with SetPalette and use them wherever a Color is expected, so they follow
// theme changes like the built-in ones.
func Token(id uint8) Color { return Color(uint32(id) << 24) }

type paletteEntry struct {
	id    uint8
	value Color
}

// SetPalette selects a built-in theme and overrides or adds tokens on top of
// it. The result is a complete custom theme: call it again with another map to
// switch themes, and every box and text that uses those tokens follows.
func (u *Ui) SetPalette(base Theme, colors map[Color]Color) {
	u.theme = base
	u.palette = u.palette[:0]
	for token, value := range colors {
		u.palette = append(u.palette, paletteEntry{uint8(token >> 24), value})
	}
	sort.Slice(u.palette, func(i, j int) bool { return u.palette[i].id < u.palette[j].id })
}

func (u *Ui) emitPalette() {
	if len(u.palette) == 0 {
		return
	}
	u.u8(opPalette)
	u.buf = append(u.buf, byte(len(u.palette)), byte(len(u.palette)>>8))
	for _, e := range u.palette {
		u.u8(e.id)
		u.u32(uint32(e.value))
	}
}

// Class registers a named style. Set Style.Class to a class name, or several
// separated by spaces, and the class fills every field you left at zero. Later
// classes override earlier ones, and the element's own fields win over both.
// A class may itself name other classes.
func (u *Ui) Class(name string, s Style) {
	if u.styleClasses == nil {
		u.styleClasses = map[string]Style{}
	}
	u.styleClasses[name] = s
}

// TextClass is Class for TextStyle.
func (u *Ui) TextClass(name string, t TextStyle) {
	if u.textClasses == nil {
		u.textClasses = map[string]TextStyle{}
	}
	u.textClasses[name] = t
}

func (u *Ui) resolveStyle(s Style) Style {
	names := strings.Fields(s.Class)
	s.Class = ""
	for i := len(names) - 1; i >= 0; i-- {
		base, ok := u.styleClasses[names[i]]
		if !ok {
			continue
		}
		if base.Class != "" {
			base = u.resolveStyle(base)
		}
		s = s.fill(base)
	}
	return s
}

func (u *Ui) resolveText(t TextStyle) TextStyle {
	names := strings.Fields(t.Class)
	t.Class = ""
	for i := len(names) - 1; i >= 0; i-- {
		base, ok := u.textClasses[names[i]]
		if !ok {
			continue
		}
		if base.Class != "" {
			base = u.resolveText(base)
		}
		t = t.fill(base)
	}
	return t
}

func (s Style) fill(b Style) Style {
	if s.W.mode == sizeFit {
		s.W = b.W
	}
	if s.H.mode == sizeFit {
		s.H = b.H
	}
	setF(&s.MinW, b.MinW)
	setF(&s.MaxW, b.MaxW)
	setF(&s.MinH, b.MinH)
	setF(&s.MaxH, b.MaxH)
	if s.Pad == (Edges{}) {
		s.Pad = b.Pad
	}
	setF(&s.Gap, b.Gap)
	if s.Align == 0 {
		s.Align = b.Align
	}
	if s.Justify == 0 {
		s.Justify = b.Justify
	}
	if !s.Absolute {
		s.Absolute, s.X, s.Y = b.Absolute, b.X, b.Y
	}
	if s.Z == 0 {
		s.Z = b.Z
	}
	if s.Bg == 0 {
		s.Bg = b.Bg
	}
	if s.Fg == 0 {
		s.Fg = b.Fg
	}
	if s.Border == 0 {
		s.Border, s.BorderW = b.Border, b.BorderW
	}
	setF(&s.Radius, b.Radius)
	setF(&s.Opacity, b.Opacity)
	if s.Gradient == (Gradient{}) {
		s.Gradient = b.Gradient
	}
	if s.Shadow == (BoxShadow{}) {
		s.Shadow = b.Shadow
	}
	s.Clip = s.Clip || b.Clip
	s.Scroll = s.Scroll || b.Scroll
	s.Hit = s.Hit || b.Hit
	if s.Gestures == 0 {
		s.Gestures = b.Gestures
	}
	if s.Role == 0 {
		s.Role = b.Role
	}
	if s.Label == "" {
		s.Label = b.Label
	}
	return s
}

func (t TextStyle) fill(b TextStyle) TextStyle {
	setF(&t.Size, b.Size)
	t.Bold = t.Bold || b.Bold
	t.Mono = t.Mono || b.Mono
	t.Ellipsis = t.Ellipsis || b.Ellipsis
	if t.Color == 0 {
		t.Color = b.Color
	}
	if t.W.mode == sizeFit {
		t.W = b.W
	}
	if t.Align == 0 {
		t.Align = b.Align
	}
	if t.Role == 0 {
		t.Role = b.Role
	}
	if t.Gradient == (Gradient{}) {
		t.Gradient = b.Gradient
	}
	setF(&t.Tracking, b.Tracking)
	return t
}

func setF(dst *float32, src float32) {
	if *dst == 0 {
		*dst = src
	}
}

// ImageFit chooses how an Image fills its box: FitCover crops to fill it,
// FitContain shows the whole image, FitStretch distorts to fit.
type ImageFit uint8

// LoadImage hands the engine straight (not premultiplied) RGBA pixels under a
// key of your choosing, once. Later frames refer to it by key with Image and
// send no pixels. Loading the same key again replaces the picture. Images are
// at most 65535 pixels on a side; send them near the size you show them,
// because the engine keeps its own copy.
func (u *Ui) LoadImage(key string, w, h int, rgba []byte) {
	if w <= 0 || h <= 0 || w > 65535 || h > 65535 || len(rgba) != w*h*4 {
		return
	}
	id := mixString(0, key)
	if u.sentImages == nil {
		u.sentImages = map[uint64]struct{}{}
	}
	u.sentImages[id] = struct{}{}
	u.pendingImgs = append(u.pendingImgs, opImageData)
	u.pendingImgs = binary.LittleEndian.AppendUint64(u.pendingImgs, id)
	u.pendingImgs = binary.LittleEndian.AppendUint16(u.pendingImgs, uint16(w))
	u.pendingImgs = binary.LittleEndian.AppendUint16(u.pendingImgs, uint16(h))
	u.pendingImgs = append(u.pendingImgs, rgba...)
}

// HasImage reports whether LoadImage was called for the key.
func (u *Ui) HasImage(key string) bool {
	_, ok := u.sentImages[mixString(0, key)]
	return ok
}

// Image draws a loaded picture. With no W and H it takes the image's pixel size;
// with only a width it keeps the aspect ratio. Radius clips it, so W == H and
// Radius == W/2 gives a circle. A key that was never loaded draws nothing but
// still takes its space once loaded.
func (u *Ui) Image(key string, s Style, fit ...ImageFit) Response {
	id := u.open(kindImage, s.Key)
	u.u8(opImg)
	u.u64(mixString(0, key))
	if len(fit) > 0 && fit[0] != FitCover {
		u.u8Op(opFit, uint8(fit[0]))
	}
	u.emitStyle(&s)
	u.u8(opEnd)
	return u.lookup(id)
}

func (u *Ui) flushImages() {
	if len(u.pendingImgs) == 0 {
		return
	}
	tail := append([]byte(nil), u.buf[u.imageAt:]...)
	u.buf = append(append(u.buf[:u.imageAt], u.pendingImgs...), tail...)
	u.pendingImgs = u.pendingImgs[:0]
}
