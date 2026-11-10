package qui

import "encoding/binary"

const (
	iconSize       = 20
	iconButtonSize = 32
)

// Svg is an SVG document drawn as a one-color icon. Make it once with NewSvg
// and keep it: the engine receives its text the first time Icon uses it, and
// later frames send only its hash. Paths and the basic shapes are drawn with
// their fills, strokes and transforms; colors are ignored, so anything the
// document paints takes the icon's color.
type Svg struct {
	id  uint64
	src string
}

func NewSvg(src string) Svg { return Svg{mixString(0, src), src} }

func (u *Ui) sendSvg(svg Svg) {
	if svg.id == 0 {
		return
	}
	if _, ok := u.sentSvgs[svg.id]; ok {
		return
	}
	if u.sentSvgs == nil {
		u.sentSvgs = map[uint64]struct{}{}
	}
	u.sentSvgs[svg.id] = struct{}{}
	u.pendingImgs = append(u.pendingImgs, opSvgData)
	u.pendingImgs = binary.LittleEndian.AppendUint64(u.pendingImgs, svg.id)
	u.pendingImgs = binary.LittleEndian.AppendUint32(u.pendingImgs, uint32(len(svg.src)))
	u.pendingImgs = append(u.pendingImgs, svg.src...)
}

// Icon draws an SVG fitted inside W by H, 20px square unless set, in Fg (Ink
// when unset). With a Label it is an image for assistive tech; without one it
// is decorative.
func (u *Ui) Icon(svg Svg, s Style) Response {
	u.sendSvg(svg)
	if s.Class != "" {
		s = u.resolveStyle(s)
	}
	if s.W.mode == sizeFit && s.H.mode == sizeFit {
		s.W, s.H = Px(iconSize), Px(iconSize)
	} else if s.H.mode == sizeFit && s.W.mode == sizePx {
		s.H = s.W
	}
	if s.Label != "" && s.Role == 0 {
		s.Role = RoleImage
	}
	id := u.open(kindIcon, s.Key)
	u.u8(opImg)
	u.u64(svg.id)
	u.emitStyle(&s)
	u.u8(opEnd)
	return u.lookup(id)
}

// IconButton draws a button showing only an icon: 32px square with the icon
// centered at 20px unless W and H say otherwise. Label names it for assistive
// tech and keys it unless Key is set. Fg colors the icon; left unset it is
// Muted, and Ink while hovered or focused.
func (u *Ui) IconButton(svg Svg, label string, s Style) Response {
	if s.Class != "" {
		s = u.resolveStyle(s)
	}
	if s.Key == "" {
		s.Key = label
	}
	if s.W.mode == sizeFit {
		s.W = Px(iconButtonSize)
	}
	if s.H.mode == sizeFit {
		s.H = Px(iconButtonSize)
		if s.W.mode == sizePx {
			s.H = s.W
		}
	}
	if s.Radius == 0 {
		s.Radius = 6
	}
	s.Align, s.Justify, s.Hit, s.Role, s.Label = Center, Center, true, RoleButton, label
	fg := s.Fg
	if fg == 0 {
		fg = Muted
		if prev := u.peek(s.Key); prev.Hovered || prev.Focused {
			fg = Ink
		}
	}
	s.Fg = 0
	size := float32(iconSize)
	if s.W.mode == sizePx && s.W.value < iconSize+8 {
		size = max(s.W.value-8, 8)
	}
	return u.Row(s, func() { u.Icon(svg, Style{W: Px(size), H: Px(size), Fg: fg}) })
}

// peek returns last frame's Response for the keyed node about to be opened in
// the current scope.
func (u *Ui) peek(key string) Response {
	return u.lookup(mixString(u.stack[len(u.stack)-1].id, key))
}
