package main

import (
	"fmt"

	"github.com/torisetxd/qui"
)

func invoiceColor(s invoiceStatus) qui.Color {
	switch s {
	case invoicePaid:
		return good
	case invoiceOpen:
		return warning
	case invoicePastDue:
		return critical
	}
	return neutral
}

func (a *app) invoiceStatus(i int) invoiceStatus {
	if s, ok := a.invoiceStates[i]; ok {
		return s
	}
	return a.data.invoices[i].status
}

func (a *app) invoiceRows() []int32 {
	signature := fmt.Sprint(a.invFilter, "|", len(a.invoiceStates))
	if signature == a.invSignature {
		return a.invView
	}
	a.invSignature = signature
	view := a.invView[:0]
	for i := range a.data.invoices {
		if a.invFilter == 0 || int(a.invoiceStatus(i)) == a.invFilter-1 {
			view = append(view, int32(i))
		}
	}
	a.invView = view
	return view
}

func (a *app) invoices() {
	ui := a.ui
	d := a.data
	pad := a.pagePad()
	columns := []column{{"Amount", qui.Px(118), true}, {"Status", qui.Px(112), false}, {"Number", qui.Px(120), false}, {"Customer", qui.Fill(), false}, {"Issued", qui.Px(116), true}}
	if a.narrow {
		columns = []column{{"Amount", qui.Px(96), false}, {"Status", qui.Px(106), false}, {"Customer", qui.Fill(), false}}
	}
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill()}, func() {
		ui.Column(qui.Style{W: qui.Fill(), Pad: qui.Edges{Top: 22, Left: pad, Right: pad}}, func() { a.title("Invoices", nil) })
		counts := []int{len(d.invoices), 0, 0, 0, 0}
		for i := range d.invoices {
			counts[int(a.invoiceStatus(i))+1]++
		}
		a.invFilter = a.filterTabs("inv-tabs", append([]string{"All"}, invoiceNames...), counts, a.invFilter)
		a.hairline()
		a.tableHead(columns)
		rows := a.invoiceRows()
		ui.List("inv-list", len(rows), 48, qui.ListOptions{Style: qui.Style{Key: "inv-list", W: qui.Fill(), H: qui.Fill()}}, func(n int) {
			i := int(rows[n])
			inv := &d.invoices[i]
			status := a.invoiceStatus(i)
			c := &d.customers[inv.customer]
			r := a.tableRow(columns, n, a.drawer == drawerInvoice && a.drawerIndex == i, func(col int) {
				if a.narrow && col == 2 {
					col = 3
				}
				switch col {
				case 0:
					ui.Text(cents(inv.amount), qui.TextStyle{Size: 13.5, Bold: true, Color: ink})
				case 1:
					a.pill(invoiceNames[status], invoiceColor(status))
				case 2:
					ui.Text(inv.number, qui.TextStyle{Size: 13, Color: muted})
				case 3:
					ui.Text(c.name, qui.TextStyle{Size: 13, Color: muted, Ellipsis: true, W: qui.Fill()})
				case 4:
					ui.Text(inv.issued.Format("Jan 2, 2006"), qui.TextStyle{Size: 13, Color: muted})
				}
			})
			if r.Clicked {
				a.drawer, a.drawerIndex = drawerInvoice, i
			}
		})
	})
}

func (a *app) invoiceDetail(i int) {
	ui := a.ui
	d := a.data
	inv := &d.invoices[i]
	status := a.invoiceStatus(i)
	c := &d.customers[inv.customer]
	ui.Column(qui.Style{W: qui.Fill(), Pad: qui.All(20), Gap: 14, Bg: page, Border: line, Radius: 10}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center}, func() {
			ui.Text("Sable Cloud, Inc.", qui.TextStyle{Size: 14, Bold: true, Color: ink, W: qui.Fill()})
			a.pill(invoiceNames[status], invoiceColor(status))
		})
		ui.Text(inv.number, qui.TextStyle{Size: 12, Color: subtle})
		ui.Text(cents(inv.amount), qui.TextStyle{Size: 30, Color: ink, Tracking: -0.9})
		ui.Text("Due "+inv.issued.AddDate(0, 0, 14).Format("January 2, 2006"), qui.TextStyle{Size: 13, Color: muted})
		a.hairline()
		a.detailRow("Billed to", c.name)
		a.detailRow("Email", c.email)
		a.hairline()
		line := func(name string, amount int32) {
			ui.Row(qui.Style{W: qui.Fill()}, func() {
				ui.Text(name, qui.TextStyle{Size: 13, Color: ink, W: qui.Fill()})
				ui.Text(cents(amount), qui.TextStyle{Size: 13, Color: ink})
			})
		}
		line(plans[inv.plan].name+" plan", plans[inv.plan].cents)
		if inv.usage > 0 {
			line("Metered usage", inv.usage)
		}
		a.hairline()
		ui.Row(qui.Style{W: qui.Fill()}, func() {
			ui.Text("Total", qui.TextStyle{Size: 14, Bold: true, Color: ink, W: qui.Fill()})
			ui.Text(cents(inv.amount), qui.TextStyle{Size: 14, Bold: true, Color: ink})
		})
	})
	if status == invoiceOpen || status == invoicePastDue {
		if ui.Button("Mark as paid", qui.Style{Key: "paid", W: qui.Fill(), Fg: ink, Bg: raised, Border: line, Radius: 7, Pad: qui.XY(10, 8)}).Clicked {
			a.invoiceStates[i] = invoicePaid
		}
	}
}
