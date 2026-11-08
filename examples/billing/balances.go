package main

import (
	"fmt"

	"github.com/torisetxd/qui"
)

func (a *app) balances() {
	ui := a.ui
	d := a.data
	a.title("Balances", nil)
	available, pending := a.balance()
	tiles := func() {
		a.statTile("Available to pay out", money(available), "USD")
		a.statTile("Pending", money(pending), "Clears in about 2 days")
		a.statTile("Paid out, last 30 days", money(sum(d, 7, 36, net)), "To account ending 4417")
	}
	if a.narrow {
		ui.Column(qui.Style{W: qui.Fill(), Gap: 12}, tiles)
	} else {
		ui.Row(qui.Style{W: qui.Fill(), Gap: 16}, tiles)
	}
	a.card("weekly", qui.Style{W: qui.Fill(), Pad: qui.All(20), Gap: 14}, func() {
		ui.Text("Weekly payouts", qui.TextStyle{Size: 15, Bold: true, Color: ink, Role: qui.RoleHeading})
		a.weeklyBars()
	})
	a.card("payouts", qui.Style{W: qui.Fill(), Clip: true}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Pad: qui.Edges{Top: 16, Bottom: 8, Left: a.pagePad(), Right: a.pagePad()}}, func() {
			ui.Text("Recent payouts", qui.TextStyle{Size: 15, Bold: true, Color: ink, Role: qui.RoleHeading})
		})
		columns := []column{{"Amount", qui.Px(130), true}, {"Status", qui.Px(110), false}, {"Destination", qui.Fill(), false}, {"Arrival", qui.Px(120), true}}
		if a.narrow {
			columns = []column{{"Amount", qui.Px(112), true}, {"Status", qui.Fill(), false}}
		}
		a.tableHead(columns)
		for n := 0; n < 8; n++ {
			day := 3 + n*2
			value := sum(d, day, day+1, net)
			status, tone := "Paid", good
			if n == 0 {
				status, tone = "In transit", warning
			}
			a.tableRow(columns, 100+n, false, func(col int) {
				switch col {
				case 0:
					ui.Text(money(value), qui.TextStyle{Size: 13.5, Bold: true, Color: ink})
				case 1:
					a.pill(status, tone)
				case 2:
					ui.Text("Northkiln Bank, ending 4417", qui.TextStyle{Size: 13, Color: muted, Ellipsis: true, W: qui.Fill()})
				case 3:
					ui.Text(d.asOf.AddDate(0, 0, -day+2).Format("Jan 2"), qui.TextStyle{Size: 13, Color: muted})
				}
			})
		}
	})
}

func (a *app) weeklyBars() {
	ui := a.ui
	d := a.data
	const weeks = 20
	const height = 180
	values := make([]float64, weeks)
	limit := 0.0
	for i := range values {
		from := (weeks - 1 - i) * 7
		values[i] = sum(d, from, from+6, net)
		limit = max(limit, values[i])
	}
	limit = niceMax(limit * 1.04)
	hovered := -1
	for i := range values {
		if a.hover[fmt.Sprint("wk", i)] {
			hovered = i
		}
	}
	ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center}, func() {
		ui.Text(compactMoney(limit)+" max", qui.TextStyle{Size: 11, Color: subtle, W: qui.Fill()})
		if hovered >= 0 {
			ui.Text(fmt.Sprintf("Week of %s: %s", d.asOf.AddDate(0, 0, -(weeks-1-hovered)*7-6).Format("Jan 2"), money(values[hovered])), qui.TextStyle{Size: 12, Color: ink})
		}
	})
	ui.Stack(qui.Style{Key: "weekly-chart", W: qui.Fill(), H: qui.Px(height)}, func() {
		ui.Column(qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Fill(), H: qui.Px(1), Bg: gridLine}, func() {})
		ui.Column(qui.Style{Absolute: true, X: 0, Y: height / 2, W: qui.Fill(), H: qui.Px(1), Bg: gridLine}, func() {})
		ui.Column(qui.Style{Absolute: true, X: 0, Y: height - 1, W: qui.Fill(), H: qui.Px(1), Bg: lineHot}, func() {})
		ui.Row(qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Fill(), H: qui.Fill(), Align: qui.End}, func() {
			for i, v := range values {
				key := fmt.Sprint("wk", i)
				color := seriesOne
				if hovered == i {
					color = accentHot
				}
				r := ui.Column(qui.Style{Key: key, W: qui.Fill(), H: qui.Fill(), Hit: true, Align: qui.Center, Justify: qui.End}, func() {
					ui.Column(qui.Style{W: qui.Px(min(24, (a.moveWidth()/weeks)*0.7)), H: qui.Px(max(2, float32(v/limit)*(height-1))), Bg: color, Radius: 3}, func() {})
				})
				a.hover[key] = r.Hovered
			}
		})
	})
}
