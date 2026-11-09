package qui

type (
	Color uint32
	Align uint8
	Role  uint8
	Theme uint8
	Key   uint32
	Mods  uint32
	// Gestures selects what a box recognizes. Combine GesturePanX, GesturePanY
	// and GesturePinch, and add GestureTouchOnly to ignore the mouse.
	Gestures uint8
)

// Swipe is the direction of a pan released at speed.
type Swipe uint8

const (
	SwipeNone Swipe = iota
	SwipeLeft
	SwipeRight
	SwipeUp
	SwipeDown
)
