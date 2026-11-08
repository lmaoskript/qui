package main

import (
	"fmt"
	"runtime"

	"github.com/torisetxd/qui"
)

const monitorBars = 84

type monitorState struct {
	history    [monitorBars]float32
	at         int
	heapMB     float32
	gcRuns     uint32
	goroutines int
	nextRead   float64
}

func (m *monitorState) record(ms float32) {
	m.history[m.at] = ms
	m.at = (m.at + 1) % monitorBars
}

func (a *app) monitorWindow() {
	ui := a.ui
	m := &a.monitor
	if a.clock >= m.nextRead {
		m.nextRead = a.clock + 0.5
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		m.heapMB = float32(stats.HeapAlloc) / (1 << 20)
		m.gcRuns = stats.NumGC
		m.goroutines = runtime.NumGoroutine()
	}
	stats := ui.Stats()
	peak := float32(2)
	for _, ms := range m.history {
		peak = max(peak, ms)
	}

	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.All(12), Gap: 8, Scroll: true, Key: "monitor"}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Align: qui.End, Gap: 8}, func() {
			ui.Text(fmt.Sprintf("%.0f", a.fps), qui.TextStyle{Size: 34, Bold: true, Mono: true, Color: qui.Strong})
			ui.Text("fps", qui.TextStyle{Size: 13, Color: qui.Muted})
			ui.Row(qui.Style{W: qui.Fill()}, noop)
			ui.Column(qui.Style{Align: qui.End}, func() {
				ui.Text(fmt.Sprintf("%.2f ms", a.buildMs+stats.EngineMs), qui.TextStyle{Size: 15, Mono: true, Bold: true, Color: qui.Ink})
				ui.Text("CPU per frame", qui.TextStyle{Size: 11, Color: qui.Muted})
			})
		})
		ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(52), Align: qui.End, Gap: 1, Key: "graph", Role: qui.RoleImage, Label: "CPU time per frame"}, func() {
			for i := 0; i < monitorBars; i++ {
				ms := m.history[(m.at+i)%monitorBars]
				color := qui.Color(qui.Accent)
				if ms > 8 {
					color = qui.Danger
				}
				ui.Column(qui.Style{W: qui.Fill(), H: qui.Px(clamp(ms/peak*52, 1, 52)), Bg: color, Radius: 1}, noop)
			}
		})
		a.statRow("Go build", fmt.Sprintf("%.2f ms", a.buildMs), "Backend", fmt.Sprintf("%.2f ms", stats.EngineMs))
		a.statRow("Nodes", group(stats.Nodes), "Wire", fmt.Sprintf("%.1f KB", float32(stats.WireBytes)/1024))
		a.statRow("Heap", fmt.Sprintf("%.1f MB", m.heapMB), "GC runs", fmt.Sprint(m.gcRuns))
		a.statRow("Goroutines", fmt.Sprint(m.goroutines), "Runtime", runtime.GOARCH)
	})
}

func (a *app) statRow(k1, v1, k2, v2 string) {
	ui := a.ui
	pair := func(k, v string) {
		ui.Row(qui.Style{W: qui.Fill(), Align: qui.Center, Gap: 6}, func() {
			ui.Text(k, qui.TextStyle{Size: 11.5, Color: qui.Muted, W: qui.Fill()})
			ui.Text(v, qui.TextStyle{Size: 11.5, Mono: true, Color: qui.Ink})
		})
	}
	ui.Row(qui.Style{W: qui.Fill(), Gap: 16}, func() {
		pair(k1, v1)
		pair(k2, v2)
	})
}
