package ui

// The one tab row every part draws on its first line: the part's name, its own tabs, a
// little info on the right, and (drawn by the shell) the other parts to switch to.
//
//	 󰂾 Notes  󰄲 Todo lists 1   󰠮 Notes   󰃭 Today         Tahaldr    Home  Files  Notes  Git
//	          ━━━━━━━━━━━━━━━━ (the open tab is a pill)               └ the shell's switcher

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Tab is one tab in the row.
type Tab struct {
	Icon, Name string
	Count      string // after the name, muted ("3")
	Warn       bool   // the count needs you (Today's due todos): yellow
	Off        bool   // not ready yet: muted
}

// PartColor: each part's own color (muted onedark), for its icon and its name in the switcher
var PartColor = map[string]color.Color{"Home": Acc, "Files": Yellow, "Notes": Purple, "Git": Orange}

func partFG(name string) lipgloss.Style {
	if c, ok := PartColor[name]; ok {
		return FG(c)
	}
	return FG(Acc)
}

// Reserve is how many columns at the right of the first line the shell keeps for the part
// switcher (0 = none, e.g. Home, or one part). Parts leave them empty.
var Reserve int

// TabRow draws the first line w columns wide; xs says where each tab is (for clicks).
func TabRow(icon, name string, tabs []Tab, active int, right string, w int) (line string, xs [][2]int) {
	left := " " + partFG(name).Render(icon) + " " + FG(Txt).Bold(true).Render(name) + "  "
	if len(tabs) > 0 {
		left += FG(Dim).Render("│") + " "
	}
	x := Width(left)
	for i, t := range tabs {
		label := t.Name
		if t.Icon != "" {
			label = t.Icon + " " + label
		}
		st, cst := FG(Hint), FG(Mut)
		switch {
		case t.Off:
			st = FG(Mut)
		case t.Warn:
			st, cst = FG(Yellow), FG(Yellow)
		}
		var s string
		if i == active {
			pill := lipgloss.NewStyle().Background(Dim)
			s = pill.Foreground(Acc).Bold(true).Render(" " + label)
			if t.Count != "" {
				s += pill.Foreground(Acc).Render(" " + t.Count)
			}
			s += pill.Render(" ")
		} else {
			s = " " + st.Render(label)
			if t.Count != "" {
				s += " " + cst.Render(t.Count)
			}
			s += " "
		}
		xs = append(xs, [2]int{x, x + Width(s)})
		left += s + " "
		x += Width(s) + 1
	}
	room := max(0, w-Reserve)
	if right != "" {
		right += " "
	}
	return Pad(Spread(left, right, room), w), xs
}

// Rule is the thin line under a header row.
func Rule(w int) string { return FG(Dim).Render(strings.Repeat("─", max(0, w))) }

// ---- the shell's part switcher, drawn over the right end of the first line ----

// SwitchItem is one part in the switcher.
type SwitchItem struct {
	Name string
	On   bool
}

// Switcher: " Home  Files  Notes  Git " and where each name is (relative to its start).
func Switcher(items []SwitchItem) (string, [][2]int) {
	s, x := "", 0
	var xs [][2]int
	for i, it := range items {
		if i > 0 {
			s += FG(Dim).Render("·")
			x++
		}
		st := FG(Mut)
		switch {
		case it.On:
			st = partFG(it.Name).Bold(true)
		case strings.HasPrefix(it.Name, "⬆"):
			st = FG(Green)
		}
		label := " " + it.Name + " "
		xs = append(xs, [2]int{x, x + Width(label)})
		s += st.Render(label)
		x += Width(label)
	}
	return s + " ", xs
}

// SwitcherWidth: how wide Switcher draws these names.
func SwitcherWidth(names []string) int {
	w := 1
	for i, n := range names {
		if i > 0 {
			w++
		}
		w += Width(n) + 2
	}
	return w
}
