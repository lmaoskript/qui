package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/torisetxd/qui"
)

const totalRows = 1_000_000

var (
	rowServices = []string{"gateway", "auth", "billing", "search", "media", "cache", "queue", "mailer"}
	rowMessages = []string{
		"request completed", "retry budget exhausted", "connection pool resized", "token refreshed",
		"upstream timed out", "snapshot written", "leader elected", "cache warmed",
		"payload rejected", "worker restarted", "rate limit applied", "index compacted",
	}
	rowLevels = []string{"ALL", "INFO", "WARN", "ERROR"}
)

type logEntry struct {
	level, service, message int
	millis                  uint32
}

func rowHash(v uint32) uint32 {
	v ^= v >> 16
	v *= 0x7feb352d
	v ^= v >> 15
	v *= 0x846ca68b
	return v ^ v>>16
}

func entryFor(id uint32) logEntry {
	h := rowHash(id + 1)
	level := 1
	if roll := h >> 3 % 100; roll >= 92 {
		level = 3
	} else if roll >= 70 {
		level = 2
	}
	return logEntry{level, int(h % 8), int(h >> 7 % 12), h >> 12 % 900}
}

type rowsState struct {
	query       string
	level       int
	ids         []uint32
	filtered    bool
	filterMs    float32
	selected    int
	hot, hotAt  int
	generation  int
	atEnd       bool
	first, seen int
}

func (s *rowsState) setup() {
	s.selected = -1
	s.hot, s.hotAt = -1, -1
}

func (s *rowsState) refilter() {
	began := time.Now()
	s.filtered = s.query != "" || s.level != 0
	s.generation++
	s.atEnd = false
	s.selected = -1
	if !s.filtered {
		s.ids = nil
		s.filterMs = float32(time.Since(began).Seconds() * 1000)
		return
	}
	query := strings.ToLower(s.query)
	var matches [8 * 12]bool
	for service := range rowServices {
		for message := range rowMessages {
			matches[service*12+message] = query == "" || strings.Contains(rowServices[service]+" "+rowMessages[message], query)
		}
	}
	s.ids = s.ids[:0]
	for id := uint32(0); id < totalRows; id++ {
		entry := entryFor(id)
		if (s.level == 0 || entry.level == s.level) && matches[entry.service*12+entry.message] {
			s.ids = append(s.ids, id)
		}
	}
	s.filterMs = float32(time.Since(began).Seconds() * 1000)
}

func (s *rowsState) count() int {
	if s.filtered {
		return len(s.ids)
	}
	return totalRows
}

func (a *app) rowsWindow() {
	ui := a.ui
	s := &a.rows
	s.hot, s.hotAt = s.hotAt, -1
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Key: "rows"}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Pad: qui.Edges{Top: 8, Right: 10, Bottom: 6, Left: 10}, Gap: 5, Align: qui.Center, Key: "controls"}, func() {
			if ui.Input("filter", &s.query, qui.InputOptions{Style: qui.Style{W: qui.Fill(), H: qui.Px(26), Pad: qui.XY(8, 0), Bg: qui.RGBA(0, 0, 0, 90)}, Placeholder: "Filter a million rows", Size: 12}).Changed {
				s.refilter()
			}
			for i, name := range rowLevels {
				bg := qui.Color(ghost)
				if s.level == i {
					bg = qui.Item
				}
				if ui.Button(name, qui.Style{H: qui.Px(26), Pad: qui.XY(7, 0), Bg: bg, Key: name}).Clicked {
					s.level = i
					s.refilter()
				}
			}
			if ui.Button("Top", qui.Style{H: qui.Px(26), Pad: qui.XY(7, 0), Bg: ghost, Key: "top"}).Clicked {
				s.generation++
				s.atEnd = false
			}
			if ui.Button("End", qui.Style{H: qui.Px(26), Pad: qui.XY(7, 0), Bg: ghost, Key: "end"}).Clicked {
				s.generation++
				s.atEnd = true
			}
		})
		list := ui.List(fmt.Sprintf("log%d", s.generation), s.count(), 25, qui.ListOptions{Style: qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.XY(8, 0)}, Bottom: s.atEnd}, a.drawRow)
		s.first, s.seen = int(list.First), int(list.Count)
		ui.Row(qui.Style{W: qui.Fill(), H: qui.Px(26), Pad: qui.XY(12, 0), Align: qui.Center, Gap: 12, Key: "footer"}, func() {
			text := fmt.Sprintf("row %s of %s · %d in the tree", group(min(s.first+1, s.count())), group(s.count()), s.seen)
			if s.filtered {
				text += fmt.Sprintf(" · filtered in %.1f ms", s.filterMs)
			}
			ui.Text(text, qui.TextStyle{Size: 11.5, Color: qui.Muted, W: qui.Fill(), Ellipsis: true})
			if s.selected >= 0 {
				ui.Text(fmt.Sprintf("selected #%d", s.selected), qui.TextStyle{Size: 11.5, Mono: true, Color: qui.Ink})
			}
		})
	})
}

func (a *app) drawRow(index int) {
	ui := a.ui
	s := &a.rows
	id := uint32(index)
	if s.filtered {
		id = s.ids[index]
	}
	entry := entryFor(id)
	levelColor := qui.Color(qui.Muted)
	switch entry.level {
	case 2:
		levelColor = qui.RGB(240, 190, 90)
	case 3:
		levelColor = qui.Danger
	}
	bg := qui.Color(ghost)
	if s.selected == int(id) {
		bg = qui.Selection
	} else if s.hot == index {
		bg = qui.Hover
	}
	row := ui.Column(qui.Style{W: qui.Fill(), Pad: qui.XY(6, 3), Bg: bg, Radius: 5, Hit: true}, func() {
		ui.Row(qui.Style{W: qui.Fill(), Gap: 10, Align: qui.Center}, func() {
			ui.Text(fmt.Sprintf("%07d", id), qui.TextStyle{Size: 11.5, Mono: true, Color: qui.Muted, W: qui.Px(56)})
			ui.Text(rowLevels[entry.level], qui.TextStyle{Size: 11.5, Mono: true, Bold: true, Color: levelColor, W: qui.Px(40)})
			ui.Text(rowServices[entry.service], qui.TextStyle{Size: 11.5, Mono: true, Color: qui.Accent2, W: qui.Px(58)})
			ui.Text(rowMessages[entry.message], qui.TextStyle{Size: 12.5, W: qui.Fill(), Ellipsis: true})
			ui.Text(fmt.Sprintf("%d ms", entry.millis), qui.TextStyle{Size: 11.5, Mono: true, Color: qui.Muted})
		})
		if id%11 == 3 {
			ui.Text(fmt.Sprintf("trace %08x · attempt %d · region eu-%d · this row is taller, and the list measures it only when it scrolls into view", rowHash(id), id%5+1, id%3+1), qui.TextStyle{Size: 11, Color: qui.Muted, W: qui.Fill()})
		}
	})
	if row.Hovered {
		s.hotAt = index
	}
	if row.Clicked {
		s.selected = int(id)
	}
}
