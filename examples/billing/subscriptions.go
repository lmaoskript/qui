package main

import (
	"fmt"

	"github.com/torisetxd/qui"
)

var (
	moveNew         = qui.RGB(57, 135, 229)
	moveExpansion   = qui.RGB(25, 158, 112)
	moveContraction = qui.RGB(144, 133, 233)
	moveChurn       = qui.RGB(217, 89, 38)
	planColors      = []qui.Color{qui.RGB(112, 118, 245), qui.RGB(217, 89, 38), qui.RGB(25, 158, 112), qui.RGB(201, 133, 0)}
)

func (a *app) statTile(name, value, note string) {
	ui := a.ui
	a.card(name, qui.Style{W: qui.Fill(), Pad: qui.All(18), Gap: 4}, func() {
		ui.Text(name, qui.TextStyle{Size: 13, Color: muted, Ellipsis: true, W: qui.Fill()})
		ui.Text(value, qui.TextStyle{Size: 26, Color: ink, Tracking: -0.7})
		ui.Text(note, qui.TextStyle{Size: 12, Color: subtle, Ellipsis: true, W: qui.Fill()})
	})
}

func (a *app) subscriptions() {
	ui := a.ui
	d := a.data
	a.title("Subscriptions", nil)
	last := d.months[len(d.months)-1]
	lost := last.lost / d.months[len(d.months)-2].total
	mrr := float64(d.active) / 100
	tiles := func() {
		a.statTile("MRR", money(mrr), fmt.Sprintf("%s ARR", compactMoney(mrr*12)))
		a.statTile("Active subscriptions", count(float64(len(d.subs))), "of "+count(float64(len(d.customers)))+" customers")
		a.statTile("Average revenue per account", money(mrr/float64(max(len(d.subs), 1))), "per month")
		a.statTile("Revenue churn", percent(lost), "last month")
	}
	if a.narrow {
		ui.Column(qui.Style{W: qui.Fill(), Gap: 12}, tiles)
	} else {
		ui.Row(qui.Style{W: qui.Fill(), Gap: 16}, tiles)
	}
	a.movements()
	a.planMix()
	a.subscriptionTable()
}

func (a *app) movements() {
	ui := a.ui
	d := a.data
	months := d.months[len(d.months)-12:]
	a.card("moves", qui.Style{W: qui.Fill(), Pad: qui.All(20), Gap: 14}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Gap: 12}, func() {
			ui.Text("MRR movements", qui.TextStyle{Size: 15, Bold: true, Color: ink, Role: qui.RoleHeading})
			a.spacer()
			choice := 0
			if a.moveTable {
				choice = 1
			}
			a.moveTable = a.segmented("mode", []string{"Chart", "Table"}, choice) == 1
		})
		ui.Row(qui.Style{Gap: 14}, func() {
			for _, item := range []struct {
				c qui.Color
				n string
			}{{moveNew, "New"}, {moveExpansion, "Expansion"}, {moveContraction, "Contraction"}, {moveChurn, "Churn"}} {
				ui.Row(qui.Style{Gap: 6, Align: qui.Center}, func() {
					ui.Column(qui.Style{W: qui.Px(9), H: qui.Px(9), Radius: 2, Bg: item.c}, func() {})
					ui.Text(item.n, qui.TextStyle{Size: 12, Color: muted})
				})
			}
		})
		if a.moveTable {
			a.movementTable(months)
			return
		}
		a.movementChart(months)
	})
}

func (a *app) movementChart(months []movement) {
	ui := a.ui
	const upH, downH = 150, 70
	maxUp, maxDown := 1.0, 1.0
	for _, m := range months {
		maxUp = max(maxUp, m.added+m.expansion)
		maxDown = max(maxDown, m.contraction+m.lost)
	}
	limit := niceMax(max(maxUp, maxDown*upH/downH) * 1.04)
	scale := float32(upH) / float32(limit)
	hovered := -1
	ui.Stack(qui.Style{Key: "moves-chart", W: qui.Fill(), H: qui.Px(upH + downH + 1)}, func() {
		ui.Row(qui.Style{Absolute: true, X: 0, Y: 0}, func() {
			ui.Text(compactMoney(limit), qui.TextStyle{Size: 10.5, Color: subtle})
		})
		ui.Column(qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Fill(), H: qui.Px(1), Bg: gridLine}, func() {})
		ui.Column(qui.Style{Absolute: true, X: 0, Y: upH / 2, W: qui.Fill(), H: qui.Px(1), Bg: gridLine}, func() {})
		ui.Column(qui.Style{Absolute: true, X: 0, Y: upH, W: qui.Fill(), H: qui.Px(1), Bg: lineHot}, func() {})
		ui.Row(qui.Style{Absolute: true, X: 0, Y: 0, W: qui.Fill(), H: qui.Fill()}, func() {
			for i, m := range months {
				key := fmt.Sprint("m", i)
				bg := qui.Color(0)
				if a.hover[key] {
					bg = qui.RGBA(255, 255, 255, 12)
				}
				r := ui.Column(qui.Style{Key: key, W: qui.Fill(), H: qui.Fill(), Hit: true, Bg: bg, Align: qui.Center}, func() {
					ui.Column(qui.Style{H: qui.Px(upH), Justify: qui.End, Align: qui.Center, Gap: 2}, func() {
						ui.Column(qui.Style{W: qui.Px(22), H: qui.Px(max(1, float32(m.expansion)*scale)), Bg: moveExpansion, Radius: 2}, func() {})
						ui.Column(qui.Style{W: qui.Px(22), H: qui.Px(max(1, float32(m.added)*scale)), Bg: moveNew, Radius: 2}, func() {})
					})
					ui.Column(qui.Style{H: qui.Px(1)}, func() {})
					ui.Column(qui.Style{Align: qui.Center, Gap: 2}, func() {
						ui.Column(qui.Style{W: qui.Px(22), H: qui.Px(max(1, float32(m.contraction)*scale)), Bg: moveContraction, Radius: 2}, func() {})
						ui.Column(qui.Style{W: qui.Px(22), H: qui.Px(max(1, float32(m.lost)*scale)), Bg: moveChurn, Radius: 2}, func() {})
					})
				})
				a.hover[key] = r.Hovered
				if r.Hovered {
					hovered = i
				}
			}
		})
		if hovered >= 0 {
			a.moveTooltip(months[hovered], float32(hovered), float32(len(months)))
		}
	})
	ui.Row(qui.Style{W: qui.Fill()}, func() {
		for i, m := range months {
			text := m.label
			if a.narrow && i%3 != 0 {
				text = ""
			}
			ui.Row(qui.Style{Key: fmt.Sprint("x", i), W: qui.Fill(), Justify: qui.Center}, func() {
				ui.Text(text, qui.TextStyle{Size: 10.5, Color: subtle})
			})
		}
	})
}

func (a *app) moveTooltip(m movement, index, total float32) {
	ui := a.ui
	const width = 188
	slot := a.moveWidth() / total
	x := slot*index + slot + 6
	if x+width > a.moveWidth() {
		x = slot*index - width - 6
	}
	ui.Column(qui.Style{Absolute: true, X: x, Y: 4, W: qui.Px(width), Z: 5, Pad: qui.XY(11, 9), Gap: 4, Bg: raised, Border: line, Radius: 8, Shadow: qui.BoxShadow{Color: qui.RGBA(0, 0, 0, 150), Blur: 16, Y: 6}}, func() {
		ui.Text(m.label, qui.TextStyle{Size: 11, Color: muted})
		row := func(c qui.Color, name string, v float64) {
			ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Gap: 7}, func() {
				ui.Column(qui.Style{W: qui.Px(10), H: qui.Px(2), Bg: c}, func() {})
				ui.Text(money(v), qui.TextStyle{Size: 12.5, Bold: true, Color: ink})
				ui.Text(name, qui.TextStyle{Size: 12, Color: muted})
			})
		}
		row(moveNew, "New", m.added)
		row(moveExpansion, "Expansion", m.expansion)
		row(moveContraction, "Contraction", -m.contraction)
		row(moveChurn, "Churn", -m.lost)
	})
}

func (a *app) moveWidth() float32 {
	width := a.w - 2*a.pagePad() - 42
	if !a.narrow {
		width -= 233
	}
	return width
}

func (a *app) movementTable(months []movement) {
	ui := a.ui
	columns := []struct {
		name string
		pick func(*movement) float64
	}{
		{"New", func(m *movement) float64 { return m.added }}, {"Expansion", func(m *movement) float64 { return m.expansion }},
		{"Contraction", func(m *movement) float64 { return -m.contraction }}, {"Churn", func(m *movement) float64 { return -m.lost }},
		{"MRR", func(m *movement) float64 { return m.total }},
	}
	ui.Column(qui.Style{W: qui.Fill(), Gap: 7}, func() {
		ui.Row(qui.Style{W: qui.Fill()}, func() {
			ui.Text("Month", qui.TextStyle{Size: 12, Color: subtle, W: qui.Fill()})
			for _, c := range columns {
				ui.Row(qui.Style{W: qui.Fill(), Justify: qui.End}, func() { ui.Text(c.name, qui.TextStyle{Size: 12, Color: subtle}) })
			}
		})
		for i := len(months) - 1; i >= 0; i-- {
			m := &months[i]
			ui.Row(qui.Style{Key: m.label, W: qui.Fill()}, func() {
				ui.Text(m.label, qui.TextStyle{Size: 13, Color: muted, W: qui.Fill()})
				for _, c := range columns {
					ui.Row(qui.Style{W: qui.Fill(), Justify: qui.End}, func() { ui.Text(compactMoney2(c.pick(m)), qui.TextStyle{Size: 13, Color: ink}) })
				}
			})
		}
	})
}

func compactMoney2(v float64) string {
	if v < 0 {
		return "-" + compactMoney(-v)
	}
	return compactMoney(v)
}

func (a *app) planMix() {
	ui := a.ui
	d := a.data
	totals := make([]float64, len(plans))
	whole := 0.0
	for _, i := range d.subs {
		c := &d.customers[i]
		totals[c.plan] += float64(c.mrr) / 100
		whole += float64(c.mrr) / 100
	}
	a.card("plans", qui.Style{W: qui.Fill(), Pad: qui.All(20), Gap: 14}, func() {
		ui.Text("MRR by plan", qui.TextStyle{Size: 15, Bold: true, Color: ink, Role: qui.RoleHeading})
		hovered := -1
		ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(28), Gap: 2}, func() {
			for i, total := range totals {
				share := total / whole
				key := fmt.Sprint("plan", i)
				color := planColors[i]
				if a.hover[key] {
					color = alpha(color, 0.82)
				}
				r := ui.Row(qui.Style{Key: key, W: qui.FillN(max(float32(share), 0.001)), H: qui.Fill(), Bg: color, Radius: 4, Hit: true, Align: qui.Center, Pad: qui.XY(8, 0)}, func() {
					if share > 0.08 {
						text := white
						if i == 3 {
							text = qui.RGB(11, 11, 11)
						}
						ui.Text(fmt.Sprintf("%.0f%%", share*100), qui.TextStyle{Size: 12, Color: text})
					}
				})
				a.hover[key] = r.Hovered
				if r.Hovered {
					hovered = i
				}
			}
		})
		ui.Row(qui.Style{W: qui.Fill(), Gap: 16}, func() {
			for i, p := range plans {
				ui.Row(qui.Style{Gap: 6, Align: qui.Center}, func() {
					ui.Column(qui.Style{W: qui.Px(9), H: qui.Px(9), Radius: 2, Bg: planColors[i]}, func() {})
					ui.Text(p.name, qui.TextStyle{Size: 12, Color: muted})
				})
			}
			a.spacer()
			if hovered >= 0 && !a.narrow {
				ui.Text(fmt.Sprintf("%s: %s, %.1f%%", plans[hovered].name, money(totals[hovered]), totals[hovered]/whole*100), qui.TextStyle{Size: 12, Color: ink})
			}
		})
	})
}

func (a *app) subscriptionTable() {
	ui := a.ui
	d := a.data
	columns := []column{{"Customer", qui.FillN(3), false}, {"Plan", qui.FillN(2), false}, {"MRR", qui.Px(120), true}, {"Status", qui.Px(120), false}}
	if a.narrow {
		columns = []column{{"Customer", qui.Fill(), false}, {"MRR", qui.Px(100), true}}
	}
	a.card("subs", qui.Style{W: qui.Fill(), Clip: true, Gap: 0}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Pad: qui.Edges{Top: 16, Bottom: 8, Left: a.pagePad(), Right: a.pagePad()}}, func() {
			ui.Text("Recently updated", qui.TextStyle{Size: 15, Bold: true, Color: ink, Role: qui.RoleHeading})
		})
		a.tableHead(columns)
		for n := 0; n < min(8, len(d.subs)); n++ {
			c := &d.customers[d.subs[n]]
			a.tableRow(columns, n, false, func(col int) {
				if a.narrow && col == 1 {
					col = 2
				}
				switch col {
				case 0:
					ui.Text(c.name, qui.TextStyle{Size: 13.5, Bold: true, Color: ink, Ellipsis: true, W: qui.Fill()})
				case 1:
					ui.Text(plans[c.plan].name, qui.TextStyle{Size: 13, Color: muted})
				case 2:
					ui.Text(cents(c.mrr), qui.TextStyle{Size: 13, Color: ink})
				case 3:
					a.pill("Active", good)
				}
			})
		}
	})
}
