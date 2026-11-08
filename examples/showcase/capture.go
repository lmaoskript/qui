package main

import (
	"github.com/torisetxd/qui"
)

const (
	mosaicCols = 64
	mosaicRows = 36
	bandCount  = 32
)

func (a *app) captureWindow() {
	ui := a.ui
	c := a.capture
	if !captureSupported {
		ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.All(24), Gap: 8, Justify: qui.Center, Align: qui.Center, Key: "unsupported"}, func() {
			ui.Text("Native only", qui.H2)
			ui.Text("Screen and microphone capture go through the desktop portal, so run the native build to try them.", qui.TextStyle{W: qui.Fill(), Align: qui.Center, Color: qui.Muted})
		})
		return
	}
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.All(10), Gap: 8, Key: "capture"}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Gap: 6, Align: qui.Center, Key: "actions"}, func() {
			if ui.Button(c.screenLabel(), qui.Style{H: qui.Px(28), Pad: qui.XY(10, 0), Bg: qui.Accent, Fg: qui.Strong, Key: "screen"}).Clicked {
				c.toggleScreen()
			}
			if ui.Button(c.micLabel(), qui.Style{H: qui.Px(28), Pad: qui.XY(10, 0), Key: "mic"}).Clicked {
				c.toggleMic()
			}
			ui.Row(qui.Style{W: qui.Fill()}, noop)
			if ui.Button(c.menuLabel(), qui.Style{H: qui.Px(28), Pad: qui.XY(10, 0), Bg: ghost, Key: "menu"}).Clicked {
				c.toggleMenu()
			}
		})
		ui.Text(c.status(), qui.TextStyle{Size: 12, Color: qui.Muted, W: qui.Fill()})
		a.mosaic()
		ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(58), Align: qui.End, Gap: 2, Key: "spectrum", Role: qui.RoleImage, Label: "Microphone spectrum"}, func() {
			bands := c.bands()
			for _, band := range bands {
				ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(clamp(band*58, 2, 58)), Bg: qui.Accent2, Radius: 1}, noop)
			}
		})
	})
}

func (a *app) mosaic() {
	ui := a.ui
	c := a.capture
	area := ui.Stack(qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: qui.RGBA(0, 0, 0, 90), Radius: 8, Clip: true, Hit: true, Key: "mosaic", Role: qui.RoleImage, Label: "Screen capture rendered as boxes"}, func() {
		cells := c.mosaic()
		if cells == nil {
			ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Align: qui.Center, Justify: qui.Center, Gap: 4}, func() {
				ui.Text("Your screen, as layout nodes", qui.TextStyle{Size: 15, Bold: true, Color: qui.Ink})
				ui.Text("Each cell is a real box in the tree, repainted at the capture rate.", qui.TextStyle{Size: 12, Color: qui.Muted})
			})
			return
		}
		w := a.mosaicW
		h := a.mosaicH
		cell := min(w/mosaicCols, h/mosaicRows)
		offsetX := (w - cell*mosaicCols) / 2
		offsetY := (h - cell*mosaicRows) / 2
		for row := 0; row < mosaicRows; row++ {
			for col := 0; col < mosaicCols; col++ {
				ui.Column(qui.Style{Absolute: true, X: offsetX + float32(col)*cell, Y: offsetY + float32(row)*cell, W: qui.Px(cell + 0.5), H: qui.Px(cell + 0.5), Bg: cells[row*mosaicCols+col]}, noop)
			}
		}
	})
	a.mosaicW, a.mosaicH = area.W, area.H
}
