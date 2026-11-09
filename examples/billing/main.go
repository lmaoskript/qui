// A billing dashboard for Sable, a made-up company. All data is generated.
//
//	go run ./examples/billing
//	go run ./examples/billing/serve
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/torisetxd/qui"
)

var (
	page       = qui.RGB(12, 16, 23)
	surface    = qui.RGB(19, 24, 33)
	raised     = qui.RGB(30, 36, 48)
	line       = qui.RGBA(255, 255, 255, 22)
	lineHot    = qui.RGBA(255, 255, 255, 60)
	gridLine   = qui.RGB(36, 42, 54)
	ink        = qui.RGB(231, 234, 240)
	muted      = qui.RGB(146, 155, 170)
	subtle     = qui.RGB(104, 113, 128)
	white      = qui.RGB(255, 255, 255)
	accent     = qui.RGB(112, 118, 245)
	accentHot  = qui.RGB(136, 142, 255)
	seriesOne  = accent
	seriesGrey = qui.RGB(107, 117, 135)
	good       = qui.RGB(12, 163, 12)
	warning    = qui.RGB(250, 178, 25)
	serious    = qui.RGB(236, 131, 90)
	critical   = qui.RGB(208, 59, 59)
	neutral    = qui.RGB(146, 155, 170)
)

type view int

const (
	viewHome view = iota
	viewBalances
	viewPayments
	viewCustomers
	viewSubscriptions
	viewInvoices
)

var views = []struct {
	name, short string
}{{"Home", "Home"}, {"Balances", "Balance"}, {"Payments", "Pay"}, {"Customers", "People"}, {"Subscriptions", "Subs"}, {"Invoices", "Bills"}}

type drawerKind int

const (
	drawerNone drawerKind = iota
	drawerPayment
	drawerInvoice
	drawerCustomer
)

type app struct {
	ui            *qui.Ui
	data          *dataset
	start         time.Time
	dt, w, h, dpr float32
	narrow        bool
	view          view
	viewFade      float32
	hover         map[string]bool
	charts        map[string]*chartState
	chartRects    map[string]qui.Response
	rangeIndex    int
	compare       bool
	search        string
	payFilter     int
	paySignature  string
	payView       []int32
	refunded      map[int]bool
	custSignature string
	custView      []int32
	invFilter     int
	invSignature  string
	invView       []int32
	invoiceStates map[int]invoiceStatus
	moveHover     int
	moveTable     bool
	plansHover    int
	drawer        drawerKind
	drawerIndex   int
	drawerT       float32
	drawerShown   drawerKind
	nav           qui.Drawer
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }

func easeOut(t float32) float32 {
	t = 1 - clamp(t, 0, 1)
	return 1 - t*t*t
}

func alpha(c qui.Color, a float32) qui.Color {
	base := uint32(c)
	return qui.Color(base&^0xff | uint32(float32(base&0xff)*clamp(a, 0, 1)+0.5))
}

func money(v float64) string {
	negative := v < 0
	if negative {
		v = -v
	}
	cents := int64(v*100 + 0.5)
	whole := fmt.Sprint(cents / 100)
	var b strings.Builder
	for i, ch := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s$%s.%02d", sign, b.String(), cents%100)
}

func cents(v int32) string { return money(float64(v) / 100) }

func compactMoney(v float64) string {
	switch {
	case v >= 1e6:
		return fmt.Sprintf("$%.1fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("$%.0fK", v/1e3)
	}
	return fmt.Sprintf("$%.0f", v)
}

func count(v float64) string {
	whole := fmt.Sprintf("%.0f", v)
	var b strings.Builder
	for i, ch := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	return b.String()
}

func percent(v float64) string { return fmt.Sprintf("%.2f%%", v*100) }

func (a *app) spacer() { a.ui.Column(qui.Style{W: qui.Fill()}, func() {}) }

func (a *app) hairline() { a.ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(1), Bg: line}, func() {}) }

func (a *app) box(x, y, w, h, radius float32, color qui.Color) {
	a.ui.Column(qui.Style{Absolute: true, X: x, Y: y, W: qui.Px(w), H: qui.Px(h), Radius: radius, Bg: color}, func() {})
}

func (a *app) outline(x, y, w, h, radius float32, color qui.Color) {
	a.ui.Column(qui.Style{Absolute: true, X: x, Y: y, W: qui.Px(w), H: qui.Px(h), Radius: radius, Border: color, BorderW: 1.4}, func() {})
}

func (a *app) icon(v view, c qui.Color) {
	a.ui.Stack(qui.Style{W: qui.Px(16), H: qui.Px(16)}, func() {
		switch v {
		case viewHome:
			a.outline(1, 1, 6, 6, 1.5, c)
			a.outline(9, 1, 6, 6, 1.5, c)
			a.outline(1, 9, 6, 6, 1.5, c)
			a.outline(9, 9, 6, 6, 1.5, c)
		case viewBalances:
			a.box(1, 2, 14, 2, 1, c)
			a.box(1, 7, 10, 2, 1, c)
			a.box(1, 12, 6, 2, 1, c)
		case viewPayments:
			a.outline(0.5, 2.5, 15, 11, 2.5, c)
			a.box(0.5, 5.5, 15, 2, 0, c)
		case viewCustomers:
			a.outline(5, 1, 6, 6, 3, c)
			a.outline(2, 9, 12, 6, 3, c)
		case viewSubscriptions:
			a.outline(1.5, 1.5, 13, 13, 6.5, c)
			a.box(7, 4, 2, 5, 1, c)
			a.box(7, 8, 4, 2, 1, c)
		case viewInvoices:
			a.outline(2.5, 0.5, 11, 15, 2, c)
			a.box(5, 4, 6, 1.5, 0.5, c)
			a.box(5, 7, 6, 1.5, 0.5, c)
			a.box(5, 10, 4, 1.5, 0.5, c)
		}
	})
}

func (a *app) open(v view) {
	a.nav.Open = false
	if a.view != v {
		a.view = v
		a.viewFade = 0
		a.drawer = drawerNone
	}
}

func (a *app) navigation() {
	ui := a.ui
	ui.Row(qui.Style{W: qui.Fill(), Gap: 10, Align: qui.Center, Pad: qui.Edges{Left: 8, Right: 8, Bottom: 16}}, func() {
		ui.Stack(qui.Style{W: qui.Px(24), H: qui.Px(24), Radius: 6, Clip: true, Gradient: qui.Gradient{From: accentHot, To: qui.RGB(70, 60, 200), Angle: 145}}, func() {
			ui.Column(qui.Style{Absolute: true, X: 11, Y: 11, W: qui.Px(24), H: qui.Px(24), Radius: 6, Bg: surface}, func() {})
		})
		ui.Text("Sable Cloud, Inc.", qui.TextStyle{Size: 13.5, Bold: true, Color: ink, Ellipsis: true, W: qui.Fill()})
	})
	for i, item := range views {
		v := view(i)
		on := a.view == v
		fg, ic, bg := muted, subtle, qui.RGBA(0, 0, 0, 1)
		if on {
			fg, ic, bg = ink, accentHot, qui.RGBA(255, 255, 255, 16)
		} else if a.hover[item.name] {
			fg, ic, bg = ink, muted, qui.RGBA(255, 255, 255, 8)
		}
		ui.Stack(qui.Style{W: qui.Fill(), Key: item.name}, func() {
			r := ui.Button(item.name, qui.Style{Key: "b", W: qui.Fill(), Radius: 7, Bg: bg, Fg: fg, Justify: qui.Between, Pad: qui.Edges{Top: 8, Bottom: 8, Left: 36, Right: 8}})
			a.hover[item.name] = r.Hovered
			if r.Clicked {
				a.open(v)
			}
			ui.Row(qui.Style{Absolute: true, X: 10, Y: 9}, func() { a.icon(v, ic) })
		})
	}
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill()}, func() {})
	ui.Text("Sable is fictional. Every number here is generated.", qui.TextStyle{Size: 11.5, Color: subtle, W: qui.Fill()})
}

func (a *app) sidebar() {
	ui := a.ui
	ui.Column(qui.Style{W: qui.Px(232), H: qui.Fill(), Pad: qui.XY(12, 16), Gap: 2, Role: qui.RoleNav}, a.navigation)
	ui.Column(qui.Style{W: qui.Px(1), H: qui.Fill(), Bg: line}, func() {})
}

func (a *app) tabbar() {
	ui := a.ui
	a.hairline()
	ui.Row(qui.Style{W: qui.Fill(), Bg: surface, Role: qui.RoleNav}, func() {
		for i, item := range views {
			v := view(i)
			fg := muted
			if a.view == v {
				fg = ink
			}
			if ui.Button(item.short, qui.Style{Key: item.name, W: qui.Fill(), H: qui.Px(50), Fg: fg, Bg: qui.RGBA(0, 0, 0, 1), Pad: qui.XY(2, 6)}).Clicked {
				a.open(v)
			}
		}
	})
}

func (a *app) topbar() {
	ui := a.ui
	ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(56), Align: qui.Center, Gap: 12, Pad: qui.XY(a.pagePad(), 0)}, func() {
		if a.narrow && ui.Button("☰", qui.Style{Key: "menu", W: qui.Px(36), H: qui.Px(36), Pad: qui.All(0), Bg: qui.RGBA(0, 0, 0, 1), Fg: ink, Label: "Open navigation"}).Clicked {
			a.nav.Open = true
		}
		width := min(400, a.w-160)
		if a.narrow {
			width = max(120, width-48)
		}
		r := ui.Input("search", &a.search, qui.InputOptions{Placeholder: "Search payments and customers", Size: 13.5, Style: qui.Style{W: qui.Px(width), Bg: surface, Border: line, Radius: 7, Pad: qui.XY(12, 7), Fg: ink}})
		if r.Submitted && a.view != viewPayments && a.view != viewCustomers {
			a.open(viewPayments)
		}
		a.spacer()
		if !a.narrow {
			ui.Row(qui.Style{Pad: qui.XY(9, 4), Radius: 5, Bg: alpha(warning, 0.13), Gap: 6, Align: qui.Center}, func() {
				ui.Column(qui.Style{W: qui.Px(6), H: qui.Px(6), Radius: 3, Bg: warning}, func() {})
				ui.Text("Demo data", qui.TextStyle{Size: 12, Color: ink})
			})
		}
		ui.Row(qui.Style{W: qui.Px(30), H: qui.Px(30), Radius: 15, Bg: raised, Align: qui.Center, Justify: qui.Center}, func() {
			ui.Text("HO", qui.TextStyle{Size: 11, Bold: true, Color: ink})
		})
	})
	a.hairline()
}

func (a *app) pagePad() float32 {
	if a.narrow {
		return 16
	}
	return 32
}

func (a *app) title(text string, trailing func()) {
	ui := a.ui
	ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Gap: 12}, func() {
		ui.Text(text, qui.TextStyle{Size: 24, Bold: true, Color: ink, Tracking: -0.5, Role: qui.RoleHeading})
		a.spacer()
		if trailing != nil {
			trailing()
		}
	})
}

func (a *app) card(key string, s qui.Style, body func()) {
	s.Key = key
	s.Bg = surface
	s.Border = line
	s.Radius = 10
	a.ui.Column(s, body)
}

func (a *app) segmented(key string, options []string, selected int) int {
	ui := a.ui
	result := selected
	ui.Row(qui.Style{Key: key, Pad: qui.All(2), Gap: 2, Radius: 8, Bg: surface, Border: line}, func() {
		for i, option := range options {
			fg, bg := muted, qui.RGBA(0, 0, 0, 1)
			if i == selected {
				fg, bg = ink, raised
			}
			if ui.Button(option, qui.Style{Key: option, Fg: fg, Bg: bg, Radius: 6, Pad: qui.XY(11, 5)}).Clicked {
				result = i
			}
		}
	})
	return result
}

func (a *app) pill(text string, c qui.Color) {
	ui := a.ui
	ui.Row(qui.Style{Gap: 6, Align: qui.Center, Pad: qui.XY(8, 2), Radius: 5, Bg: alpha(c, 0.16)}, func() {
		ui.Column(qui.Style{W: qui.Px(6), H: qui.Px(6), Radius: 3, Bg: c}, func() {})
		ui.Text(text, qui.TextStyle{Size: 12, Color: ink})
	})
}

func (a *app) frame() {
	ui := a.ui
	a.dt = clamp(ui.Dt(), 0.001, 0.05)
	a.w, a.h, a.dpr = ui.Width(), ui.Height(), ui.Dpr()
	a.narrow = a.w < 860
	a.viewFade = min(1, a.viewFade+a.dt/0.16)
	if a.drawer != drawerNone && ui.KeyPressed(qui.KeyEscape) {
		a.drawer = drawerNone
	}
	ui.Stack(qui.Style{W: qui.Fill(), H: qui.Fill(), Bg: page}, func() {
		if a.narrow {
			ui.Drawer(&a.nav, qui.DrawerOptions{Size: min(280, a.w-56), Style: qui.Style{Bg: surface, Pad: qui.XY(12, 16), Gap: 2, Role: qui.RoleNav}}, a.mainArea, a.navigation)
		} else {
			a.nav = qui.Drawer{}
			ui.Row(qui.Style{W: qui.Fill(), H: qui.Fill()}, func() {
				a.sidebar()
				a.mainArea()
			})
		}
		a.overlay()
	})
}

func (a *app) mainArea() {
	ui := a.ui
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Role: qui.RoleMain}, func() {
		a.topbar()
		ui.Column(qui.Style{Key: views[a.view].name, W: qui.Fill(), H: qui.Fill(), Opacity: max(0.001, easeOut(a.viewFade))}, func() {
			switch a.view {
			case viewHome:
				a.scrollPage(a.home)
			case viewBalances:
				a.scrollPage(a.balances)
			case viewPayments:
				a.payments()
			case viewCustomers:
				a.customers()
			case viewSubscriptions:
				a.scrollPage(a.subscriptions)
			case viewInvoices:
				a.invoices()
			}
		})
		if a.narrow {
			a.tabbar()
		}
	})
}

func (a *app) scrollPage(body func()) {
	pad := a.pagePad()
	a.ui.Column(qui.Style{Key: "scroll", W: qui.Fill(), H: qui.Fill(), Scroll: true, Pad: qui.Edges{Top: 24, Left: pad, Right: pad, Bottom: 40}, Gap: 22}, body)
}

func main() {
	ui := qui.Open("Sable billing", 1440, 900)
	ui.SetTheme(qui.Midnight)
	a := &app{
		ui: ui, data: buildData(), start: time.Now(), compare: true, rangeIndex: 1, moveHover: -1, plansHover: -1,
		hover: map[string]bool{}, charts: map[string]*chartState{}, chartRects: map[string]qui.Response{},
		refunded: map[int]bool{}, invoiceStates: map[int]invoiceStatus{},
	}
	go func() {
		t := time.NewTicker(time.Second / 30)
		defer t.Stop()
		for range t.C {
			ui.Wake()
		}
	}()
	for ui.Frame() {
		a.frame()
	}
}
