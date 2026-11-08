//go:build !(linux && !android && cgo)

package main

import "github.com/torisetxd/qui"

const captureSupported = false

type capture struct{}

func newCapture(func()) *capture           { return &capture{} }
func (*capture) live() bool                { return false }
func (*capture) level() float32            { return 0 }
func (*capture) screenLabel() string       { return "" }
func (*capture) micLabel() string          { return "" }
func (*capture) menuLabel() string         { return "" }
func (*capture) status() string            { return "" }
func (*capture) toggleScreen()             {}
func (*capture) toggleMic()                {}
func (*capture) toggleMenu()               {}
func (*capture) mosaic() []qui.Color       { return nil }
func (*capture) bands() [bandCount]float32 { return [bandCount]float32{} }
