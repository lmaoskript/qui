//go:build cgo

package qui

/*
#cgo CXXFLAGS: -std=c++20
#cgo pkg-config: sdl3 freetype2 harfbuzz fontconfig
#cgo LDFLAGS: -lstdc++ -lm
#include <stdlib.h>
#include "qui_c.h"
*/
import "C"

import (
	"runtime"
	"unsafe"
)

func init() { runtime.LockOSThread() }

type platform struct {
	app *C.qui_app
	n   C.size_t
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
