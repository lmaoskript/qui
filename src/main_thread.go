//go:build cgo && !android

package native

import "runtime"

func init() { runtime.LockOSThread() }
