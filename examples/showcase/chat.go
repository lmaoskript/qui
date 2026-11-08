package main

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/torisetxd/qui"
)

var chatReplies = []string{
	"This reply is coming from a goroutine. It appends one word at a time and calls ui.Wake, and that is the entire threading story.",
	"Every bubble is a row in a virtual list pinned to the bottom, so a chat with a million messages costs the same as this one.",
	"Drag a window by its title bar. Nothing here is a widget object: the desktop is a loop over a slice of structs, rebuilt every frame.",
	"Try editing the shader in Shader Lab and break it on purpose. The compiler's message comes back to Go, and the last good version keeps running.",
	"Shift+Enter makes a new line and Enter sends. Emoji, 日本語 and العربية all go through the same shaper on native, and the browser's own on the web.",
	"Press Ctrl+K for the command palette, or try a different theme and watch the wallpaper follow.",
}

type chatMessage struct {
	mine bool
	text string
}

type chatState struct {
	mu        sync.Mutex
	messages  []chatMessage
	draft     string
	next      int
	streaming atomic.Bool
	wake      func()
}

func (s *chatState) setup(wake func()) {
	s.wake = wake
	s.messages = []chatMessage{
		{false, "Hi, I'm a very small chat bot that lives in this window. こんにちは 👋 مرحبا"},
		{true, "Is this running on a real GPU or in a browser?"},
		{false, "Whichever you started. The same Go source produces a native OpenGL window, and with GOOS=js it produces real DOM elements."},
		{true, "So screen readers work on the web version?"},
		{false, "The browser gets actual buttons, inputs and headings, and focus, selection and IME are the browser's own. Canvas would have thrown all of that away."},
		{true, "And what happens when nothing is moving?"},
		{false, "The loop blocks until something changes. Press Calm in the top bar and this whole desktop stops using the CPU."},
		{true, "Nice 🎉"},
	}
}

func (s *chatState) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
}

func (s *chatState) respond() {
	s.streaming.Store(true)
	defer s.streaming.Store(false)
	time.Sleep(280 * time.Millisecond)
	s.mu.Lock()
	reply := chatReplies[s.next%len(chatReplies)]
	s.next++
	s.messages = append(s.messages, chatMessage{false, ""})
	at := len(s.messages) - 1
	s.mu.Unlock()
	for i, word := range strings.Fields(reply) {
		s.mu.Lock()
		if at >= len(s.messages) {
			s.mu.Unlock()
			return
		}
		if i > 0 {
			s.messages[at].text += " "
		}
		s.messages[at].text += word
		s.mu.Unlock()
		s.wake()
		time.Sleep(38 * time.Millisecond)
	}
}

func (a *app) chatWindow() {
	ui := a.ui
	s := &a.chat
	s.mu.Lock()
	defer s.mu.Unlock()
	ui.Column(qui.Style{W: qui.Fill(), H: qui.Fill(), Key: "chat"}, func() {
		ui.List("log", len(s.messages), 56, qui.ListOptions{Style: qui.Style{W: qui.Fill(), H: qui.Fill(), Pad: qui.All(10), Gap: 8}, Bottom: true}, func(i int) {
			m := s.messages[i]
			justify, bg, fg := qui.Start, qui.Color(qui.Item), qui.Color(qui.Ink)
			if m.mine {
				justify, bg, fg = qui.End, qui.Accent, qui.Strong
			}
			ui.Row(qui.Style{W: qui.Fill(), Justify: justify}, func() {
				ui.Column(qui.Style{MaxW: 300, Pad: qui.XY(11, 7), Bg: bg, Radius: 10}, func() {
					text := m.text
					if text == "" {
						text = "…"
					}
					ui.Text(text, qui.TextStyle{W: qui.Fill(), Size: 13, Color: fg})
				})
			})
		})
		ui.Row(qui.Style{W: qui.Fill(), Pad: qui.All(10), Gap: 6, Align: qui.End, Key: "composer"}, func() {
			field := ui.Input("draft", &s.draft, qui.InputOptions{Style: qui.Style{W: qui.Fill(), MaxH: 110, Bg: qui.RGBA(0, 0, 0, 90)}, Placeholder: "Message the bot", Multiline: true, EnterSubmits: true})
			send := ui.Button("Send", qui.Style{H: qui.Px(32), Bg: qui.Accent, Fg: qui.Strong, Key: "send"})
			if (field.Submitted || send.Clicked) && strings.TrimSpace(s.draft) != "" {
				s.messages = append(s.messages, chatMessage{true, strings.TrimSpace(s.draft)})
				s.draft = ""
				go s.respond()
			}
		})
	})
}
