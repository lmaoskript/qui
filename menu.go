package qui

const (
	menuRowHeight  = 32
	sheetRowHeight = 48
	menuWidth      = 224
	menuMargin     = 8
)

// MenuItem is one row of a Menu.
type MenuItem struct {
	Label string
	// Confirm, when set, replaces Label after the first press, and the item is
	// chosen only by the second.
	Confirm string
	Danger  bool
}

// Menu is the state of a context menu. Keep one per menu in your app state and
// open it with Show.
type Menu struct {
	Open bool
	X, Y float32
	// Row positions are index+1, so 0 means none.
	active, hovered, confirm int
	// Latched on the first frame: 1 for a popup, 2 for a sheet.
	mode uint8
}

// Show opens the menu at x, y, usually PointerX and PointerY.
func (m *Menu) Show(x, y float32) {
	*m = Menu{Open: true, X: x, Y: y}
}

// Menu draws an open menu and returns the index of the chosen item, or -1. It
// closes itself on a choice, a press outside it and Escape (Back on Android).
// The arrow keys, Home and End move through the items, Enter and Space choose,
// and focus returns to where it was when the menu closes. Opened by touch on a
// narrow window, it shows as a Sheet with taller rows. Panel restyles the
// menu; without a Class, unset fields get the defaults.
func (u *Ui) Menu(m *Menu, items []MenuItem, panel ...Style) int {
	if !m.Open || len(items) == 0 {
		return -1
	}
	if m.mode == 0 {
		m.mode = 1
		if u.PointerTouch() && u.Width() < sheetBelow {
			m.mode = 2
		}
	}
	sheet := m.mode == 2
	var s Style
	if len(panel) > 0 {
		s = panel[0]
	}
	if sheet {
		s.W = Fill()
		if s.Class == "" {
			sheetDefaults(&s)
		}
	} else if s.Class == "" {
		menuDefaults(&s)
	}
	s.Role = RoleMenu
	rowHeight := float32(menuRowHeight)
	limit := u.Height() - 2*menuMargin
	if sheet {
		rowHeight, limit = sheetRowHeight, u.Height()-sheetTop
	}

	count := len(items)
	before := m.active
	switch {
	case u.KeyPressed(KeyDown):
		m.active = m.active%count + 1
	case u.KeyPressed(KeyUp):
		m.active--
		if m.active < 1 {
			m.active = count
		}
	case u.KeyPressed(KeyHome):
		m.active = 1
	case u.KeyPressed(KeyEnd):
		m.active = count
	}

	height := float32(count)*rowHeight + float32(count-1)*s.Gap + s.Pad.Top + s.Pad.Bottom
	if height > limit {
		height, s.MaxH, s.Scroll = limit, limit, true
		if m.active != before {
			target := max(0, float32(m.active-1)*(rowHeight+s.Gap)-(height-rowHeight)/2)
			s.ScrollTo = &target
		}
	}

	chosen, hovered := -1, 0
	rows := func() {
		if m.active == 0 {
			u.u8Op(opFocus, 2)
		}
		for i, item := range items {
			position := i + 1
			row := Style{W: Fill(), H: Px(rowHeight), Pad: XY(8, 0), Align: Center, Radius: 6, Hit: true, Role: RoleMenuItem}
			if sheet {
				row.Pad, row.Radius = XY(12, 0), 10
			}
			if position == m.active {
				row.Bg = Hover
			}
			label, color := item.Label, Ink
			if position == m.confirm {
				label = item.Confirm
			}
			if item.Danger {
				color = Danger
			}
			r := u.Row(row, func() {
				if position == m.active {
					u.u8Op(opFocus, 2)
				}
				text := TextStyle{Color: color, Ellipsis: true, W: Fill()}
				if sheet {
					text.Size = 16
				}
				u.Text(label, text)
			})
			if r.Hovered {
				hovered = position
			}
			if r.Clicked {
				if item.Confirm != "" && m.confirm != position {
					m.confirm = position
					u.Wake()
				} else {
					chosen = i
				}
			}
		}
	}
	var dismissed bool
	if sheet {
		dismissed = u.Sheet("qui/menu", s, rows)
	} else {
		dismissed = u.Popup("qui/menu", m.X, max(menuMargin, min(m.Y, u.Height()-height-menuMargin)), s, rows)
	}
	if hovered != m.hovered {
		if hovered != 0 {
			m.active = hovered
		} else if m.active == m.hovered {
			m.active = 0
		}
		m.hovered = hovered
		u.Wake()
	}
	if chosen >= 0 || dismissed {
		m.Open = false
		u.Wake()
	}
	return chosen
}

func menuDefaults(s *Style) {
	if s.W == (Size{}) {
		s.W = Px(menuWidth)
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
		s.Radius = 8
	}
	if s.Pad == (Edges{}) {
		s.Pad = All(6)
	}
	if s.Shadow == (BoxShadow{}) {
		s.Shadow = BoxShadow{Color: Shadow, Blur: 16, Y: 4}
	}
}
