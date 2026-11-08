package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/torisetxd/qui"
)

var payFilters = []struct {
	name   string
	status int
}{{"All", -1}, {"Succeeded", 0}, {"Refunded", 1}, {"Failed", 2}, {"Disputed", 3}}

func statusColor(status uint8) qui.Color {
	switch status {
	case statusSucceeded:
		return good
	case statusRefunded:
		return neutral
	case statusFailed:
		return critical
	case statusDisputed:
		return serious
	}
	return warning
}

func (a *app) statusOf(i int) uint8 {
	if a.refunded[i] {
		return statusRefunded
	}
	return a.data.status[i]
}

func (a *app) paymentRows() []int32 {
	query := strings.ToLower(strings.TrimSpace(a.search))
	signature := fmt.Sprint(a.payFilter, "|", query, "|", len(a.refunded))
	if signature == a.paySignature {
		return a.payView
	}
	a.paySignature = signature
	d := a.data
	want := payFilters[a.payFilter].status
	var match []bool
	if query != "" {
		match = make([]bool, len(d.customers))
		for i := range d.customers {
			match[i] = strings.Contains(d.customers[i].search, query)
		}
	}
	byID := strings.HasPrefix(query, "py_")
	view := a.payView[:0]
	for i := range d.status {
		if want >= 0 && int(a.statusOf(i)) != want {
			continue
		}
		if query != "" && !match[d.buyer[i]] && !(byID && strings.HasPrefix(paymentID(i), query)) {
			continue
		}
		view = append(view, int32(i))
	}
	a.payView = view
	return view
}

func (a *app) payCounts() []int {
	d := a.data
	counts := []int{len(d.status), d.counts[0], d.counts[1], d.counts[2], d.counts[3]}
	for i := range a.refunded {
		counts[1+int(d.status[i])]--
		counts[2]++
	}
	return counts
}

func (a *app) paymentColumns() []column {
	if a.narrow {
		return []column{{"Amount", qui.Px(92), false}, {"Status", qui.Px(112), false}, {"Customer", qui.Fill(), false}}
	}
	return []column{
		{"Amount", qui.Px(118), true}, {"Status", qui.Px(112), false}, {"Description", qui.FillN(3), false},
		{"Customer", qui.FillN(3), false}, {"Method", qui.Px(136), false}, {"Date", qui.Px(136), true},
	}
}

func (a *app) payments() {
	ui := a.ui
	d := a.data
	pad := a.pagePad()
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill()}, func() {
		ui.Column(qui.Style{W: qui.Fill(), Pad: qui.Edges{Top: 22, Left: pad, Right: pad}}, func() { a.title("Payments", nil) })
		names := make([]string, len(payFilters))
		for i, f := range payFilters {
			names[i] = f.name
		}
		a.payFilter = a.filterTabs("pay-tabs", names, a.payCounts(), a.payFilter)
		a.hairline()
		columns := a.paymentColumns()
		a.tableHead(columns)
		rows := a.paymentRows()
		ui.List("pay-list", len(rows), 48, qui.ListOptions{Style: qui.Style{Key: "pay-list", W: qui.Fill(), H: qui.Fill()}}, func(n int) {
			i := int(rows[n])
			status := a.statusOf(i)
			r := a.tableRow(columns, n, a.drawer == drawerPayment && a.drawerIndex == i, func(c int) {
				if a.narrow && c == 2 {
					c = 3
				}
				switch c {
				case 0:
					ui.Text(cents(d.amount[i]), qui.TextStyle{Size: 13.5, Bold: true, Color: ink})
				case 1:
					a.pill(statusNames[status], statusColor(status))
				case 2:
					ui.Text(plans[d.customers[d.buyer[i]].plan].name+" plan, usage", qui.TextStyle{Size: 13.5, Color: muted, Ellipsis: true, W: qui.Fill()})
				case 3:
					if a.narrow {
						ui.Text(d.customers[d.buyer[i]].name, qui.TextStyle{Size: 13, Color: muted, Ellipsis: true, W: qui.Fill()})
					} else {
						ui.Text(d.customers[d.buyer[i]].email, qui.TextStyle{Size: 13, Color: muted, Ellipsis: true, W: qui.Fill()})
					}
				case 4:
					ui.Text(fmt.Sprintf("%s %s", methods[d.method[i]], cardLast4(i)), qui.TextStyle{Size: 13, Color: muted, Ellipsis: true})
				case 5:
					ui.Text(time.Unix(d.when[i], 0).Format("Jan 2, 3:04 PM"), qui.TextStyle{Size: 13, Color: muted})
				}
			})
			if r.Clicked {
				a.drawer, a.drawerIndex = drawerPayment, i
			}
		})
		if len(rows) == 0 {
			ui.Column(qui.Style{Absolute: true, X: 0, Y: 220, W: qui.Fill(), Align: qui.Center}, func() {
				ui.Text("No payments match this search", qui.TextStyle{Size: 14, Color: muted})
			})
		}
	})
}

func (a *app) paymentDetail(i int) {
	ui := a.ui
	d := a.data
	c := &d.customers[d.buyer[i]]
	status := a.statusOf(i)
	when := time.Unix(d.when[i], 0)
	value := float64(d.amount[i]) / 100
	fee := value*0.029 + 0.30
	ui.Row(qui.Style{Gap: 10, Align: qui.Center}, func() {
		ui.Text(cents(d.amount[i]), qui.TextStyle{Size: 32, Color: ink, Tracking: -1})
		ui.Text("USD", qui.TextStyle{Size: 14, Color: subtle})
	})
	a.pill(statusNames[status], statusColor(status))
	a.hairline()
	ui.Text("Timeline", qui.TextStyle{Size: 14, Bold: true, Color: ink, Role: qui.RoleHeading})
	events := []struct {
		name string
		at   time.Time
	}{{"Payment created", when}, {"Authorized by " + methods[d.method[i]], when.Add(900 * time.Millisecond)}}
	switch status {
	case statusFailed:
		events = append(events, struct {
			name string
			at   time.Time
		}{"Declined: insufficient funds", when.Add(2 * time.Second)})
	case statusRefunded:
		events = append(events, struct {
			name string
			at   time.Time
		}{"Captured", when.Add(2 * time.Second)}, struct {
			name string
			at   time.Time
		}{"Refunded to customer", when.Add(52 * time.Hour)})
	default:
		events = append(events, struct {
			name string
			at   time.Time
		}{"Captured", when.Add(2 * time.Second)})
	}
	ui.Column(qui.Style{W: qui.Fill(), Gap: 10}, func() {
		for _, e := range events {
			ui.Row(qui.Style{Gap: 10, W: qui.Fill(), Align: qui.Center}, func() {
				ui.Column(qui.Style{W: qui.Px(7), H: qui.Px(7), Radius: 4, Bg: accentHot}, func() {})
				ui.Text(e.name, qui.TextStyle{Size: 13, Color: ink, W: qui.Fill()})
				ui.Text(e.at.Format("Jan 2, 3:04:05 PM"), qui.TextStyle{Size: 12, Color: subtle})
			})
		}
	})
	a.hairline()
	a.detailRow("Payment ID", paymentID(i))
	a.detailRow("Customer", c.name)
	a.detailRow("Email", c.email)
	a.detailRow("Method", fmt.Sprintf("%s ending %s", methods[d.method[i]], cardLast4(i)))
	a.detailRow("Fee", money(fee))
	a.detailRow("Net", money(value-fee))
	a.hairline()
	if status == statusSucceeded {
		if ui.Button("Refund payment", qui.Style{Key: "refund", W: qui.Fill(), Fg: ink, Bg: raised, Border: line, Radius: 7, Pad: qui.XY(10, 8)}).Clicked {
			a.refunded[i] = true
		}
	}
}
