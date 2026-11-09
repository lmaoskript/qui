//go:build cgo && android

package qui

import "C"

import (
	"runtime"
	_ "unsafe"
)

// SDL's Activity calls SDL_main on its own thread after the library loads, and
// a c-shared library never runs main.main by itself.
//
//go:linkname mainMain main.main
func mainMain()

//export SDL_main
func SDL_main(C.int, **C.char) C.int {
	runtime.LockOSThread()
	mainMain()
	return 0
}
