package ui

// Headless helpers for tests and `dotes --dump`: run a model without a terminal.

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Drain runs a command and everything it leads to, feeding the messages to m.
func Drain(m tea.Model, first tea.Cmd) {
	queue := []tea.Cmd{first}
	for steps := 0; len(queue) > 0 && steps < 200; steps++ {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case tea.QuitMsg:
			return
		default:
			_, next := m.Update(msg)
			queue = append(queue, next)
		}
	}
}

// ParseInput turns "j", "ctrl+d", "alt+2", "shift+tab", "enter", "type:some text",
// "click:10,5", "wheel:10,5,up" into a message.
func ParseInput(a string) tea.Msg {
	if strings.HasPrefix(a, "type:") {
		return tea.PasteMsg{Content: a[5:]}
	}
	if strings.HasPrefix(a, "click:") || strings.HasPrefix(a, "wheel:") {
		p := strings.Split(a[6:], ",")
		x, _ := strconv.Atoi(p[0])
		y, _ := strconv.Atoi(p[1])
		if a[0] == 'c' {
			return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
		}
		b := tea.MouseWheelDown
		if len(p) > 2 && p[2] == "up" {
			b = tea.MouseWheelUp
		}
		return tea.MouseWheelMsg{X: x, Y: y, Button: b}
	}
	named := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "down": tea.KeyDown, "up": tea.KeyUp,
		"left": tea.KeyLeft, "right": tea.KeyRight, "backspace": tea.KeyBackspace, "home": tea.KeyHome,
		"end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "f1": tea.KeyF1, "space": tea.KeySpace,
	}
	if r, ok := named[a]; ok {
		return tea.KeyPressMsg{Code: r}
	}
	if a == "shift+tab" {
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	}
	if strings.HasPrefix(a, "ctrl+") {
		return tea.KeyPressMsg{Code: rune(a[5]), Mod: tea.ModCtrl}
	}
	if strings.HasPrefix(a, "alt+") && len(a) > 4 {
		return tea.KeyPressMsg{Code: []rune(a[4:])[0], Mod: tea.ModAlt}
	}
	if a == "" {
		return nil
	}
	r := []rune(a)[0]
	return tea.KeyPressMsg{Code: r, Text: a}
}
