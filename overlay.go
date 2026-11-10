package qui

// Overlays are boxes that float above the whole window. Call them after the
// content they cover, so they paint and hit-test on top. Popup and Tooltip are
// placed in window coordinates from anywhere; Modal centers in its parent, so
// call it from a container that covers the area it should dim.

const (
	overlayZ    = 1 << 20
	modalMargin = 16
	sheetSlide  = 48
	sheetTop    = 48
	// Below this width a Menu opened by touch shows as a Sheet.
	sheetBelow = 600
)

// Modal shows body in a centered panel over a dimming scrim. It returns true
// when the user clicked the scrim or pressed Escape, which is the cue to close
// it. Panel is the style of the centered box; give it a W and a Bg.
func (u *Ui) Modal(key string, panel Style, body func()) bool {
	dismissed := false
	u.Stack(Style{Key: key, Absolute: true, W: Fill(), H: Fill(), Z: overlayZ, Align: Center, Justify: Center}, func() {
		pop := u.TweenFrom(key+"/pop", 0.01, 1, 160)
		scrim := u.Column(Style{Key: key + "/scrim", Absolute: true, W: Fill(), H: Fill(), Bg: Scrim, Opacity: pop, Hit: true}, func() {})
		if scrim.Clicked || scrim.DragEnd {
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
		panel.Select, panel.Opacity = true, pop
		u.Column(panel, body)
	})
	return dismissed || u.KeyPressed(KeyEscape)
}

// Sheet shows body in a panel that slides up from the bottom edge over a
// dimming scrim, the phone form of Modal. It returns true when the user tapped
// the scrim, swiped the panel down or pressed Escape. Without a Class, unset
// panel fields get a full-width raised card.
func (u *Ui) Sheet(key string, panel Style, body func()) bool {
	dismissed := false
	u.Stack(Style{Key: key, Absolute: true, W: Fill(), H: Fill(), Z: overlayZ}, func() {
		pop := u.TweenFrom(key+"/pop", 0.01, 1, 200)
		scrim := u.Column(Style{Key: key + "/scrim", Absolute: true, W: Fill(), H: Fill(), Bg: Scrim, Opacity: pop, Hit: true}, func() {})
		if scrim.Clicked || scrim.DragEnd {
			dismissed = true
		}
		u.Column(Style{Absolute: true, Y: (1 - pop) * sheetSlide, W: Fill(), H: Fill(), Pad: All(8), Align: Center, Justify: End}, func() {
			if panel.Class == "" {
				sheetDefaults(&panel)
			}
			if h := u.Height() - sheetTop; panel.MaxH == 0 || panel.MaxH > h {
				panel.MaxH = h
			}
			panel.Hit, panel.Select, panel.Opacity = true, true, pop
			panel.Gestures |= GesturePanY
			if panel.Key == "" {
				panel.Key = key + "/panel"
			}
			if u.Column(panel, body).Swipe == SwipeDown {
				dismissed = true
			}
		})
	})
	return dismissed || u.KeyPressed(KeyEscape)
}

func sheetDefaults(s *Style) {
	if s.W == (Size{}) {
		s.W = Fill()
	}
	if s.MaxW == 0 {
		s.MaxW = 560
	}
	if s.Bg == 0 {
		s.Bg = Raised
	}
	if s.Border == 0 {
		s.Border = Line
	}
	if s.BorderW == 0 {
		s.BorderW = 1
	}
	if s.Radius == 0 {
		s.Radius = 16
	}
	if s.Pad == (Edges{}) {
		s.Pad = All(8)
	}
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
	u.Stack(Style{Key: key, Fixed: true, W: Fill(), H: Fill(), Z: overlayZ}, func() {
		catcher := u.Column(Style{Key: key + "/catcher", Absolute: true, W: Fill(), H: Fill(), Hit: true}, func() {})
		if catcher.Clicked || catcher.RightClicked || catcher.DragEnd {
			dismissed = true
		}
		panel.Absolute, panel.X, panel.Y, panel.Hit, panel.Select = true, x, y, true, true
		panel.Opacity = u.TweenFrom(key+"/pop", 0.01, 1, 110)
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
	const wrapW, wrapH = 280, 160
	wrap := Style{Fixed: true, X: u.PointerX() + 12, Y: u.PointerY() + 16, Z: overlayZ + 1, W: Px(wrapW), H: Px(wrapH), Key: "qui/tooltip/wrap"}
	if wrap.X+wrapW > u.Width() {
		wrap.X, wrap.Align = u.PointerX()-12-wrapW, End
	}
	if wrap.Y+wrapH > u.Height() {
		wrap.Y, wrap.Justify = u.PointerY()-16-wrapH, End
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
	u.Column(wrap, func() {
		u.Column(s, func() { u.Text(text, TextStyle{Size: 12, Color: Ink}) })
	})
}
