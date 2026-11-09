package qui

// Side is the edge a Drawer is attached to.
type Side uint8

const (
	SideLeft Side = iota
	SideRight
	SideTop
	SideBottom
)

// Drawer is the state of a panel that slides over other content. Keep one per
// drawer in your app state and set Open to show or hide it from code.
type Drawer struct {
	Open     bool
	position float32
	speed    float32
	held     bool
	started  bool
}

// DrawerOptions configures Ui.Drawer. Style applies to the panel; the drawer
// manages its position and size.
type DrawerOptions struct {
	Side  Side
	Size  float32
	Style Style
	Key   string
}

const (
	drawerDefaultSize = 280
	drawerSpring      = 20
	drawerFling       = 1.5
)

// Drawer fills its parent with content and slides panel over it. A touch swipe
// anywhere on content opens it; a swipe, a tap on the scrim, or Escape (Back on
// Android) closes it. The returned Response is the area covering both.
func (u *Ui) Drawer(d *Drawer, o DrawerOptions, content, panel func()) Response {
	size := o.Size
	if size <= 0 {
		size = drawerDefaultSize
	}
	horizontal := o.Side == SideLeft || o.Side == SideRight
	style := Style{W: Fill(), H: Fill(), Clip: true, Key: o.Key, Gestures: GesturePanY | GestureTouchOnly}
	if horizontal {
		style.Gestures = GesturePanX | GestureTouchOnly
	}
	id := u.open(kindBox, o.Key)
	u.u8Op(opDir, dirStack)
	u.emitStyle(&style)
	result := u.lookup(id)
	u.updateDrawer(d, &o, &result, size, horizontal)

	u.stack = append(u.stack, scope{id, 0})
	u.Column(Style{W: Fill(), H: Fill(), Key: "content"}, content)
	if d.position > 0 {
		if u.Column(Style{Key: "scrim", Absolute: true, W: Fill(), H: Fill(), Bg: Scrim, Opacity: d.position, Hit: true}, func() {}).Clicked {
			d.Open = false
		}
		width, height := result.W, result.H
		if !result.Found {
			width, height = u.Width(), u.Height()
		}
		s := o.Style
		s.Key = "panel"
		s.Absolute = true
		s.Hit = true
		s.W, s.H = Px(size), Fill()
		if !horizontal {
			s.W, s.H = Fill(), Px(size)
		}
		s.X, s.Y = 0, 0
		travel := size * (d.position - 1)
		switch o.Side {
		case SideLeft:
			s.X = travel
		case SideRight:
			s.X = width - size - travel
		case SideTop:
			s.Y = travel
		case SideBottom:
			s.Y = height - size - travel
		}
		u.Column(s, panel)
	}
	u.stack = u.stack[:len(u.stack)-1]
	u.u8(opEnd)
	return result
}

func (u *Ui) updateDrawer(d *Drawer, o *DrawerOptions, r *Response, size float32, horizontal bool) {
	if !d.started {
		d.started = true
		if d.Open {
			d.position = 1
		}
	}
	direction := float32(1)
	if o.Side == SideRight || o.Side == SideBottom {
		direction = -1
	}
	delta, velocity := r.DY, r.VY
	if horizontal {
		delta, velocity = r.DX, r.VX
	}
	if r.Panning || r.PanEnd {
		d.position = min(max(d.position+direction*delta/size, 0), 1)
	}
	d.held = r.Panning
	if r.PanEnd {
		fling := direction * velocity / size
		switch {
		case fling > drawerFling:
			d.Open = true
		case fling < -drawerFling:
			d.Open = false
		default:
			d.Open = d.position > 0.5
		}
		d.speed = fling
	}
	if d.Open && u.KeyPressed(KeyEscape) {
		d.Open = false
	}
	if d.held {
		d.speed = 0
		return
	}
	target := float32(0)
	if d.Open {
		target = 1
	}
	if d.position == target && d.speed == 0 {
		return
	}
	dt := min(u.Dt(), 1.0/30)
	d.speed += (drawerSpring*drawerSpring*(target-d.position) - 2*drawerSpring*d.speed) * dt
	d.position = min(max(d.position+d.speed*dt, 0), 1)
	if max(target-d.position, d.position-target) < 0.001 && max(d.speed, -d.speed) < 0.02 {
		d.position, d.speed = target, 0
		return
	}
	u.Wake()
}
