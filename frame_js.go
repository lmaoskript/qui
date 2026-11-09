//go:build js

package qui

import "syscall/js"

type platform struct {
	bridge  js.Value
	out     js.Value
	outCap  int
	results []byte
	wake    chan struct{}
	onWake  js.Func
}

// SetAppID has no effect in the browser.
func SetAppID(string) {}

// Open connects to the DOM patcher started by web/qui.js.
func Open(title string, width, height int) *Ui {
	bridge := js.Global().Get("__qui")
	if bridge.IsUndefined() {
		panic("qui: start the wasm module through start() from web/qui.js")
	}
	u := newUi()
	u.bridge = bridge
	u.wake = make(chan struct{}, 1)
	u.onWake = js.FuncOf(func(js.Value, []js.Value) any {
		select {
		case u.wake <- struct{}{}:
		default:
		}
		return nil
	})
	bridge.Call("connect", title, u.onWake)
	return u
}

func (u *Ui) Close() { u.onWake.Release() }

// Wake requests a frame. It is safe to call from any goroutine.
func (u *Ui) Wake() { u.bridge.Call("invalidate") }

func (u *Ui) consumeBack() {}

// PickFiles does nothing in the browser yet.
func (u *Ui) PickFiles(bool) {}

// PickedFiles is always empty in the browser.
func (u *Ui) PickedFiles() []string { return nil }

// SetClipboard puts text on the system clipboard. Browsers only allow it while
// handling a user gesture.
func (u *Ui) SetClipboard(text string) {
	if clipboard := js.Global().Get("navigator").Get("clipboard"); !clipboard.IsUndefined() {
		clipboard.Call("writeText", text)
	}
}

// SetSmoothScroll does nothing in the browser, which smooths the wheel itself.
func (u *Ui) SetSmoothScroll(bool) {}

func (u *Ui) submit() {
	if len(u.buf) > u.outCap {
		u.outCap = len(u.buf) * 2
		u.out = js.Global().Get("Uint8Array").New(u.outCap)
	}
	js.CopyBytesToJS(u.out, u.buf)
	u.bridge.Call("apply", u.out, len(u.buf))
}

func (u *Ui) wait() []byte {
	<-u.wake
	table := u.bridge.Call("takeResults")
	n := table.Length()
	if cap(u.results) < n {
		u.results = make([]byte, n*2)
	}
	u.results = u.results[:n]
	js.CopyBytesToGo(u.results, table)
	return u.results
}
