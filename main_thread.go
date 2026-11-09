//go:build cgo && !android

package qui

import "runtime"

func init() { runtime.LockOSThread() }
