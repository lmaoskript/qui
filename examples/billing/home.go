package main

import (
	"fmt"

	"github.com/torisetxd/qui"
)

var ranges = []struct {
	name       string
	points, by int
}{{"Last 7 days", 7, 1}, {"Last 4 weeks", 28, 1}, {"Last 3 months", 13, 7}, {"Last 12 months", 13, 28}}

type metric struct {
	name   string
	bucket func(d *dataset, from, to int) float64
	format func(float64) string
	axis   func(float64) string
	upGood bool
	latest bool
}

func sum(d *dataset, from, to int, pick func(*dayStats) float64) float64 {
	total := 0.0
	for i := from; i <= to && i < len(d.days); i++ {
		total += pick(&d.days[i])
	}
	return total
}

func gross(s *dayStats) float64 { return s.gross }
func net(s *dayStats) float64   { return s.gross - s.refunds - s.fees }

func mrrAt(d *dataset, day int) float64 {
	at := float64(len(d.months)-1) - float64(day)/30.4
	if at < 0 {
		at = 0
	}
	low := int(at)
	high := min(low+1, len(d.months)-1)
	return d.months[low].total + (d.months[high].total-d.months[low].total)*(at-float64(low))
}

var metrics = []metric{
	{"Gross volume", func(d *dataset, f, t int) float64 { return sum(d, f, t, gross) }, money, compactMoney, true, false},
	{"Net volume", func(d *dataset, f, t int) float64 { return sum(d, f, t, net) }, money, compactMoney, true, false},
	{"MRR", func(d *dataset, f, t int) float64 { return mrrAt(d, f) }, money, compactMoney, true, true},
	{"Successful payments", func(d *dataset, f, t int) float64 {
		return sum(d, f, t, func(s *dayStats) float64 { return s.succeeded })
	}, count, count, true, false},
	{"New customers", func(d *dataset, f, t int) float64 {
		return sum(d, f, t, func(s *dayStats) float64 { return s.signups })
	}, count, count, true, false},
	{"Failed payment rate", func(d *dataset, f, t int) float64 {
		failed := sum(d, f, t, func(s *dayStats) float64 { return s.failed })
		done := sum(d, f, t, func(s *dayStats) float64 { return s.succeeded })
		if failed+done == 0 {
			return 0
		}
		return failed / (failed + done)
	}, percent, percent, false, false},
}

func (a *app) series(m *metric) chartData {
	r := ranges[a.rangeIndex]
	d := a.data
	cur := make([]float64, r.points)
	prev := make([]float64, r.points)
	labels := make([]string, r.points)
	shift := r.points * r.by
	for i := 0; i < r.points; i++ {
		from := (r.points - 1 - i) * r.by
		to := from + r.by - 1
		cur[i] = m.bucket(d, from, to)
		prev[i] = m.bucket(d, from+shift, to+shift)
		date := d.asOf.AddDate(0, 0, -from)
		labels[i] = date.Format("Jan 2")
	}
	out := chartData{current: cur, labels: labels, format: m.format}
	if a.compare {
		out.previous = prev
	}
	if m.latest {
		low, high := cur[0], cur[0]
		for _, v := range append(append([]float64{}, cur...), out.previous...) {
			low = min(low, v)
			high = max(high, v)
		}
		out.floor = low - (high-low)*0.08
	}
	return out
}

func (a *app) legendKey(c qui.Color, name string) {
	ui := a.ui
	ui.Row(qui.Style{Gap: 6, Align: qui.Center}, func() {
		ui.Column(qui.Style{W: qui.Px(12), H: qui.Px(2), Bg: c}, func() {})
		ui.Text(name, qui.TextStyle{Size: 12, Color: muted})
	})
}

func (a *app) home() {
	ui := a.ui
	a.title("Today", nil)
	if a.narrow {
		ui.Column(qui.Style{W: qui.Fill(), Gap: 14}, func() { a.todayChart(qui.Fill()); a.balanceSummary(qui.Fill()) })
	} else {
		ui.Row(qui.Style{W: qui.Fill(), Gap: 16}, func() { a.todayChart(qui.FillN(7)); a.balanceSummary(qui.FillN(3)) })
	}
	ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Gap: 12, Pad: qui.Edges{Top: 8}}, func() {
		ui.Text("Reports", qui.TextStyle{Size: 18, Bold: true, Color: ink, Role: qui.RoleHeading})
		a.spacer()
		if !a.narrow {
			compare := "Compare: off"
			if a.compare {
				compare = "Compare: previous period"
			}
			if ui.Button(compare, qui.Style{Key: "cmp", Fg: muted, Bg: surface, Border: line, Radius: 7, Pad: qui.XY(11, 6)}).Clicked {
				a.compare = !a.compare
			}
		}
	})
	names := make([]string, len(ranges))
	for i, r := range ranges {
		names[i] = r.name
		if a.narrow {
			names[i] = []string{"7d", "4w", "3m", "12m"}[i]
		}
	}
	a.rangeIndex = a.segmented("range", names, a.rangeIndex)
	columns := 3
	if a.narrow {
		columns = 1
	} else if a.w < 1180 {
		columns = 2
	}
	ui.Column(qui.Style{W: qui.Fill(), Gap: 16}, func() {
		for row := 0; row*columns < len(metrics); row++ {
			ui.Row(qui.Style{Key: fmt.Sprint("r", row), W: qui.Fill(), Gap: 16}, func() {
				for i := row * columns; i < min((row+1)*columns, len(metrics)); i++ {
					a.metricCard(&metrics[i])
				}
			})
		}
	})
}

func (a *app) todayChart(w qui.Size) {
	ui := a.ui
	d := a.data
	today, yesterday := d.today[:], d.yesterday[:]
	labels := make([]string, 24)
	for i := range labels {
		hour := i % 12
		if hour == 0 {
			hour = 12
		}
		suffix := "am"
		if i >= 12 {
			suffix = "pm"
		}
		labels[i] = fmt.Sprintf("%d:00 %s", hour, suffix)
	}
	const height = 190
	end := (float32(17) + 20.0/60) / 23
	value := today[17]
	a.card("today", qui.Style{W: w, Pad: qui.All(20), Gap: 12}, func() {
		figure := func() {
			ui.Column(qui.Style{Gap: 3}, func() {
				ui.Text("Gross volume", qui.TextStyle{Size: 13, Color: muted})
				ui.Text(money(value), qui.TextStyle{Size: 40, Color: ink, Tracking: -1.3})
			})
		}
		legend := func() {
			ui.Row(qui.Style{Gap: 14}, func() {
				a.legendKey(seriesOne, "Today")
				a.legendKey(seriesGrey, "Yesterday")
			})
		}
		if a.narrow {
			figure()
			legend()
		} else {
			ui.Row(qui.Style{W: qui.Fill(), Align: qui.End, Gap: 12}, func() {
				figure()
				a.spacer()
				legend()
			})
		}
		a.chart("today-line", height, chartData{current: today, previous: yesterday, labels: labels, format: money, end: end}, true)
		ui.Row(qui.Style{W: qui.Fill(), Justify: qui.Between}, func() {
			ui.Text("12:00 am", qui.TextStyle{Size: 11, Color: subtle})
			ui.Text("Now, 5:20 pm", qui.TextStyle{Size: 11, Color: subtle})
			ui.Text("11:00 pm", qui.TextStyle{Size: 11, Color: subtle})
		})
	})
}

func (a *app) balanceSummary(w qui.Size) {
	ui := a.ui
	available, pending := a.balance()
	a.card("balances", qui.Style{W: w, H: qui.Fill(), Pad: qui.All(20), Gap: 16}, func() {
		row := func(name, value string) {
			ui.Column(qui.Style{Gap: 2, W: qui.Fill()}, func() {
				ui.Text(name, qui.TextStyle{Size: 13, Color: muted})
				ui.Text(value, qui.TextStyle{Size: 22, Color: ink, Tracking: -0.5})
			})
		}
		row("USD balance", money(available))
		row("Pending", money(pending))
		a.hairline()
		ui.Column(qui.Style{Gap: 2, W: qui.Fill()}, func() {
			ui.Text("Next payout", qui.TextStyle{Size: 13, Color: muted})
			ui.Text(money(available*0.86), qui.TextStyle{Size: 22, Color: ink, Tracking: -0.5})
			ui.Text("Expected "+a.data.asOf.AddDate(0, 0, 2).Format("Mon, Jan 2"), qui.TextStyle{Size: 12, Color: subtle})
		})
		if ui.Button("View balances", qui.Style{Key: "vb", W: qui.Fill(), Fg: ink, Bg: raised, Radius: 7, Pad: qui.XY(10, 7)}).Clicked {
			a.open(viewBalances)
		}
	})
}

func (a *app) balance() (available, pending float64) {
	d := a.data
	return sum(d, 2, 6, net), sum(d, 0, 1, net)
}

func (a *app) metricCard(m *metric) {
	ui := a.ui
	series := a.series(m)
	r := ranges[a.rangeIndex]
	shift := r.points * r.by
	head := m.bucket(a.data, 0, shift-1)
	before := m.bucket(a.data, shift, shift*2-1)
	if m.latest {
		head = m.bucket(a.data, 0, 0)
		before = m.bucket(a.data, shift, shift)
	}
	a.card(m.name, qui.Style{W: qui.Fill(), Pad: qui.All(18), Gap: 10}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center}, func() {
			ui.Text(m.name, qui.TextStyle{Size: 13, Color: muted, W: qui.Fill(), Ellipsis: true})
			if before != 0 {
				change := (head - before) / before
				up := change >= 0
				tone := good
				if up != m.upGood {
					tone = critical
				}
				sign := "-"
				if up {
					sign = "+"
				}
				magnitude := change
				if magnitude < 0 {
					magnitude = -magnitude
				}
				a.pill(fmt.Sprintf("%s%.1f%%", sign, magnitude*100), tone)
			}
		})
		ui.Text(m.format(head), qui.TextStyle{Size: 24, Color: ink, Tracking: -0.6})
		sized := series
		sized.format = m.format
		a.chart(m.name, 96, sized, false)
		ui.Row(qui.Style{W: qui.Fill(), Justify: qui.Between}, func() {
			ui.Text(series.labels[0], qui.TextStyle{Size: 11, Color: subtle})
			ui.Text(series.labels[len(series.labels)-1], qui.TextStyle{Size: 11, Color: subtle})
		})
	})
}
