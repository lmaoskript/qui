package main

import (
	"fmt"
	"strings"

	"github.com/torisetxd/qui"
)

func (a *app) customerRows() []int32 {
	query := strings.ToLower(strings.TrimSpace(a.search))
	if query == a.custSignature && a.custView != nil {
		return a.custView
	}
	a.custSignature = query
	view := a.custView[:0]
	if view == nil {
		view = make([]int32, 0, len(a.data.customers))
	}
	for i := range a.data.customers {
		if query == "" || strings.Contains(a.data.customers[i].search, query) {
			view = append(view, int32(i))
		}
	}
	a.custView = view
	return view
}

func (a *app) customers() {
	ui := a.ui
	d := a.data
	pad := a.pagePad()
	columns := []column{{"Name", qui.FillN(3), false}, {"Email", qui.FillN(4), false}, {"Plan", qui.Px(92), false}, {"Spent", qui.Px(112), true}, {"Payments", qui.Px(80), true}, {"Created", qui.Px(104), true}}
	if a.narrow {
		columns = []column{{"Name", qui.Fill(), false}, {"Spent", qui.Px(100), true}}
	}
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill()}, func() {
		rows := a.customerRows()
		ui.Column(qui.Style{W: qui.Fill(), Pad: qui.Edges{Top: 22, Left: pad, Right: pad, Bottom: 14}}, func() {
			a.title("Customers", func() { ui.Text(count(float64(len(rows)))+" total", qui.TextStyle{Size: 13, Color: muted}) })
		})
		a.hairline()
		a.tableHead(columns)
		ui.List("cust-list", len(rows), 48, qui.ListOptions{Style: qui.Style{Key: "cust-list", W: qui.Fill(), H: qui.Fill()}}, func(n int) {
			i := int(rows[n])
			c := &d.customers[i]
			r := a.tableRow(columns, n, a.drawer == drawerCustomer && a.drawerIndex == i, func(col int) {
				if a.narrow && col == 1 {
					col = 3
				}
				switch col {
				case 0:
					ui.Text(c.name, qui.TextStyle{Size: 13.5, Bold: true, Color: ink, Ellipsis: true, W: qui.Fill()})
				case 1:
					ui.Text(c.email, qui.TextStyle{Size: 13, Color: muted, Ellipsis: true, W: qui.Fill()})
				case 2:
					ui.Text(plans[c.plan].name, qui.TextStyle{Size: 13, Color: muted})
				case 3:
					ui.Text(money(float64(c.spent)/100), qui.TextStyle{Size: 13, Color: ink})
				case 4:
					ui.Text(fmt.Sprint(c.payments), qui.TextStyle{Size: 13, Color: muted})
				case 5:
					ui.Text(d.asOf.AddDate(0, 0, -c.created).Format("Jan 2, 2006"), qui.TextStyle{Size: 13, Color: muted})
				}
			})
			if r.Clicked {
				a.drawer, a.drawerIndex = drawerCustomer, i
			}
		})
	})
}

func (a *app) customerDetail(i int) {
	ui := a.ui
	d := a.data
	c := &d.customers[i]
	ui.Text(c.name, qui.TextStyle{Size: 24, Color: ink, Tracking: -0.5, W: qui.Fill()})
	ui.Text(c.email, qui.TextStyle{Size: 13, Color: muted, W: qui.Fill()})
	if c.active {
		a.pill("Active subscription", good)
	} else {
		a.pill("Canceled", neutral)
	}
	a.hairline()
	a.detailRow("Plan", plans[c.plan].name)
	a.detailRow("MRR", cents(c.mrr))
	a.detailRow("Lifetime spend", money(float64(c.spent)/100))
	a.detailRow("Payments", fmt.Sprint(c.payments))
	a.detailRow("Customer since", d.asOf.AddDate(0, 0, -c.created).Format("January 2, 2006"))
}
