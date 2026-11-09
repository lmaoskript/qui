package qui

const (
	sliderThumb = 14
	sliderStep  = 0.05
)

// Switch draws an on/off switch and reports whether it was toggled this frame,
// by a click or by Enter or Space while it has focus. Label names it for
// assistive tech.
func (u *Ui) Switch(key string, on bool, label string) bool {
	var value, knob float32 = 0, 3
	track := Active
	if on {
		value, knob, track = 1, 19, Accent
	}
	style := Style{W: Px(38), H: Px(22), Bg: track, Radius: 11, Hit: true, Role: RoleSwitch, Label: label, Key: key}
	return u.Stack(style, func() {
		u.f32Op(opValue, value)
		u.Column(Style{W: Px(16), H: Px(16), Absolute: true, X: knob, Y: 3, Bg: Strong, Radius: 8}, func() {})
	}).Clicked
}

// Slider draws a horizontal slider for a value from 0 to 1 and returns the new
// value. Pressing or dragging sets it; with focus, the arrow keys step it by 5%
// and Home and End jump to the ends.
func (u *Ui) Slider(key string, value float32, w Size, label string) float32 {
	value = clamp01(value)
	style := Style{W: w, H: Px(20), Align: Center, Hit: true, Role: RoleSlider, Label: label, Key: key, Gestures: GesturePanX | GestureTouchOnly}
	r := u.Row(style, func() {
		u.f32Op(opValue, value)
		u.Column(Style{W: FillN(value), H: Px(6), Bg: Accent, Radius: 3}, func() {})
		u.Column(Style{W: Px(sliderThumb), H: Px(sliderThumb), Bg: Strong, Radius: sliderThumb / 2}, func() {})
		u.Column(Style{W: FillN(1 - value), H: Px(6), Bg: Active, Radius: 3}, func() {})
	})
	if r.Pressed || r.Panning || r.Clicked {
		if span := r.W - sliderThumb; span > 0 {
			value = clamp01((u.PointerX() - r.X - sliderThumb/2) / span)
		}
	}
	if r.Focused {
		switch {
		case u.KeyPressed(KeyLeft), u.KeyPressed(KeyDown):
			value = clamp01(value - sliderStep)
		case u.KeyPressed(KeyRight), u.KeyPressed(KeyUp):
			value = clamp01(value + sliderStep)
		case u.KeyPressed(KeyHome):
			value = 0
		case u.KeyPressed(KeyEnd):
			value = 1
		}
	}
	return value
}

func clamp01(v float32) float32 {
	return min(max(v, 0), 1)
}
