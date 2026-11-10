package qui

type tween struct {
	from, to, elapsed, duration float32
	seen                        uint64
}

func (t *tween) value() float32 {
	k := float32(1)
	if t.duration > 0 {
		k = min(t.elapsed/t.duration, 1)
	}
	return t.from + (t.to-t.from)*(1-(1-k)*(1-k)*(1-k))
}

// Tween eases toward target over ms milliseconds and returns this frame's
// value, for what Ease does not animate, such as a size or a position. Call it
// once per frame per key. The first call, and a call after a frame without
// one, returns target.
func (u *Ui) Tween(key string, target, ms float32) float32 {
	return u.TweenFrom(key, target, target, ms)
}

// TweenFrom is Tween that starts at from the first time the key is seen, or
// after a frame without a call, so a box can animate in when it appears.
func (u *Ui) TweenFrom(key string, from, target, ms float32) float32 {
	id := mixString(u.stack[len(u.stack)-1].id, key)
	t := u.tweens[id]
	switch {
	case t == nil || t.seen+1 < u.frame || u.reduceMotion || ms <= 0:
		if u.tweens == nil {
			u.tweens = map[uint64]*tween{}
		}
		t = &tween{from: target, to: target}
		if from != target && !u.reduceMotion && ms > 0 {
			t.from, t.duration = from, ms/1000
		}
		u.tweens[id] = t
	case t.to != target:
		t.from, t.to, t.elapsed, t.duration = t.value(), target, 0, ms/1000
	default:
		t.elapsed += min(u.Dt(), 1.0/30)
	}
	t.seen = u.frame
	if t.elapsed < t.duration {
		u.Wake()
	}
	return t.value()
}
