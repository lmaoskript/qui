package main

import (
	"fmt"
	"strings"

	"github.com/torisetxd/qui"
)

type column struct {
	title string
	w     qui.Size
	end   bool
}

func (a *app) tableHead(columns []column) {
	ui := a.ui
	ui.Row(qui.Style{W: qui.Fill(), Pad: qui.XY(a.pagePad(), 0), H: qui.Px(34), Align: qui.Center, Gap: 16}, func() {
		for _, c := range columns {
			justify := qui.Start
			if c.end {
				justify = qui.End
			}
			ui.Row(qui.Style{W: c.w, Justify: justify}, func() {
				ui.Text(strings.ToUpper(c.title), qui.TextStyle{Size: 11, Color: subtle, Tracking: 0.4})
			})
		}
	})
	a.hairline()
}

func (a *app) tableRow(columns []column, i int, selected bool, cell func(c int)) qui.Response {
	ui := a.ui
	key := fmt.Sprint("r", i)
	bg := qui.Color(0)
	if selected {
		bg = qui.RGBA(255, 255, 255, 16)
	} else if a.hover[key] {
		bg = qui.RGBA(255, 255, 255, 8)
	}
	var r qui.Response
	ui.Column(qui.Style{W: qui.Fill()}, func() {
		r = ui.Row(qui.Style{Key: "row", W: qui.Fill(), H: qui.Px(47), Hit: true, Align: qui.Center, Gap: 16, Pad: qui.XY(a.pagePad(), 0), Bg: bg}, func() {
			for c, col := range columns {
				justify := qui.Start
				if col.end {
					justify = qui.End
				}
				ui.Row(qui.Style{W: col.w, Justify: justify, Align: qui.Center}, func() { cell(c) })
			}
		})
		a.hairline()
	})
	a.hover[key] = r.Hovered
	return r
}

func (a *app) filterTabs(key string, names []string, counts []int, selected int) int {
	ui := a.ui
	result := selected
	pad := a.pagePad()
	ui.Row(qui.Style{Key: key, W: qui.Fill(), Gap: 8, Pad: qui.Edges{Left: pad, Right: pad, Top: 12, Bottom: 12}}, func() {
		for i, name := range names {
			if a.narrow && i >= 3 {
				break
			}
			fg, bg, border := muted, surface, line
			if i == selected {
				fg, bg, border = ink, alpha(accent, 0.18), accent
			}
			label := fmt.Sprintf("%s  %s", name, count(float64(counts[i])))
			if ui.Button(label, qui.Style{Key: name, Fg: fg, Bg: bg, Border: border, Radius: 7, Pad: qui.XY(11, 5)}).Clicked {
				result = i
			}
		}
	})
	return result
}

func (a *app) detailRow(name, value string) {
	ui := a.ui
	ui.Row(qui.Style{W: qui.Fill(), Gap: 10}, func() {
		ui.Text(name, qui.TextStyle{Size: 13, Color: muted, W: qui.Px(120)})
		ui.Text(value, qui.TextStyle{Size: 13, Color: ink, W: qui.Fill()})
	})
}

func (a *app) overlay() {
	ui := a.ui
	if a.drawer != drawerNone {
		a.drawerShown = a.drawer
		a.drawerT = min(1, a.drawerT+a.dt/0.18)
	} else {
		a.drawerT = max(0, a.drawerT-a.dt/0.14)
	}
	if a.drawerT <= 0 {
		return
	}
	e := easeOut(a.drawerT)
	width := min(440, a.w)
	scrim := ui.Column(qui.Style{Key: "scrim", Absolute: true, X: 0, Y: 0, W: qui.Fill(), H: qui.Fill(), Bg: qui.RGBA(0, 0, 0, uint8(120*e)), Hit: true, Z: 20}, func() {})
	if scrim.Clicked {
		a.drawer = drawerNone
	}
	ui.Column(qui.Style{Key: "drawer", Absolute: true, X: a.w - width*e, Y: 0, W: qui.Px(width), H: qui.Fill(), Z: 21, Bg: surface, Hit: true, Border: line, Shadow: qui.BoxShadow{Color: qui.RGBA(0, 0, 0, 170), Blur: 40}}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Pad: qui.XY(20, 12)}, func() {
			title := "Payment"
			switch a.drawerShown {
			case drawerInvoice:
				title = "Invoice"
			case drawerCustomer:
				title = "Customer"
			}
			ui.Text(title, qui.TextStyle{Size: 13, Color: muted, W: qui.Fill()})
			if ui.Button("Close", qui.Style{Key: "close", Fg: muted, Bg: qui.RGBA(0, 0, 0, 1), Radius: 6, Pad: qui.XY(8, 4)}).Clicked {
				a.drawer = drawerNone
			}
		})
		a.hairline()
		ui.Column(qui.Style{Key: "body", W: qui.Fill(), H: qui.Fill(), Scroll: true, Pad: qui.All(20), Gap: 16}, func() {
			switch a.drawerShown {
			case drawerPayment:
				a.paymentDetail(a.drawerIndex)
			case drawerInvoice:
				a.invoiceDetail(a.drawerIndex)
			case drawerCustomer:
				a.customerDetail(a.drawerIndex)
			}
		})
	})
}
