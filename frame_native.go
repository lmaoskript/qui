//go:build cgo

package qui

/*
#cgo CXXFLAGS: -std=c++20
#cgo linux,!android pkg-config: sdl3 freetype2 harfbuzz fontconfig
#cgo linux,!android LDFLAGS: -lstdc++ -lm
#cgo windows pkg-config: --static sdl3 freetype2 harfbuzz
#cgo windows LDFLAGS: -static
#cgo android pkg-config: --static sdl3 freetype2 harfbuzz
#cgo android LDFLAGS: -static-libstdc++ -llog -landroid
#include <stdlib.h>
#include "qui_c.h"
*/
import "C"

import "unsafe"

type platform struct {
	app *C.qui_app
	n   C.size_t
}

// SetAppID sets the Wayland app id. Call it before Open. It must match the
// basename of the app's .desktop file.
func SetAppID(id string) {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	C.qui_set_app_id(cid)
}

// Open creates the app window. Call it from the main goroutine.
func Open(title string, width, height int) *Ui {
	ctitle := C.CString(title)
	defer C.free(unsafe.Pointer(ctitle))
	u := newUi()
	u.app = C.qui_open(ctitle, C.int(width), C.int(height))
	return u
}

func (u *Ui) Close() { C.qui_close(u.app) }

// Wake requests a frame. It is safe to call from any goroutine.
func (u *Ui) Wake() { C.qui_wake(u.app) }

func (u *Ui) submit() {
	C.qui_frame_end(u.app, (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(u.buf))), C.size_t(len(u.buf)))
}

func (u *Ui) wait() []byte {
	p := C.qui_frame_begin(u.app, &u.n)
	if p == nil {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(p)), int(u.n))
}
