package qui

// Overlays are boxes that float above the whole window. Call them from the
// root container (a Stack or any container the size of the window), after the
// content they cover, so they paint and hit-test on top.

const (
	overlayZ    = 1 << 20
	modalMargin = 16
)

// Modal shows body in a centered panel over a dimming scrim. It returns true
// when the user clicked the scrim or pressed Escape, which is the cue to close
// it. Panel is the style of the centered box; give it a W and a Bg.
func (u *Ui) Modal(key string, panel Style, body func()) bool {
	dismissed := false
	u.Stack(Style{Key: key, Absolute: true, W: Fill(), H: Fill(), Z: overlayZ, Align: Center, Justify: Center}, func() {
		scrim := u.Column(Style{Key: key + "/scrim", Absolute: true, W: Fill(), H: Fill(), Bg: Scrim, Hit: true}, func() {})
		if scrim.Clicked {
			dismissed = true
		}
		panel.Hit = true
		if panel.Key == "" {
			panel.Key = key + "/panel"
		}
		if w := u.Width() - 2*modalMargin; w > 0 && (panel.MaxW == 0 || panel.MaxW > w) {
			panel.MaxW = w
		}
		if h := u.Height() - 2*modalMargin; h > 0 && (panel.MaxH == 0 || panel.MaxH > h) {
			panel.MaxH = h
		}
		u.Column(panel, body)
	})
	return dismissed || u.KeyPressed(KeyEscape)
}

// Popup places body at (x, y) in window coordinates, nudged to stay on screen,
// with an invisible catcher behind it. It returns true when the user clicked
// outside or pressed Escape. Use it for context menus and dropdowns; take x
// and y from a Response (X, Y, H) or from PointerX and PointerY. Give the style
// a W so the panel can be kept on screen.
func (u *Ui) Popup(key string, x, y float32, panel Style, body func()) bool {
	dismissed := false
	if panel.W.mode == sizePx {
		if over := x + panel.W.value - u.Width(); over > 0 {
			x -= over + 8
		}
	}
	if x < 0 {
		x = 0
	}
	u.Stack(Style{Key: key, Absolute: true, W: Fill(), H: Fill(), Z: overlayZ}, func() {
		catcher := u.Column(Style{Key: key + "/catcher", Absolute: true, W: Fill(), H: Fill(), Hit: true}, func() {})
		if catcher.Clicked || catcher.RightClicked {
			dismissed = true
		}
		panel.Absolute, panel.X, panel.Y, panel.Hit = true, x, y, true
		if panel.Key == "" {
			panel.Key = key + "/panel"
		}
		u.Column(panel, body)
	})
	return dismissed || u.KeyPressed(KeyEscape)
}

// Tooltip shows text beside the pointer while show is true, usually a
// Response.Hovered. It never takes the pointer.
func (u *Ui) Tooltip(show bool, text string, style ...Style) {
	if !show || text == "" {
		return
	}
	var s Style
	if len(style) > 0 {
		s = style[0]
	}
	s.Absolute, s.X, s.Y, s.Z = true, u.PointerX()+12, u.PointerY()+16, overlayZ+1
	if s.X+200 > u.Width() {
		s.X = u.PointerX() - 212
	}
	if s.Bg == 0 {
		s.Bg = Raised
	}
	if s.Border == 0 {
		s.Border = Line
	}
	if s.Radius == 0 {
		s.Radius = 6
	}
	if s.Pad == (Edges{}) {
		s.Pad = XY(8, 4)
	}
	s.MaxW = 280
	s.Key = "qui/tooltip"
	u.Column(s, func() { u.Text(text, TextStyle{Size: 12, Color: Ink}) })
}
