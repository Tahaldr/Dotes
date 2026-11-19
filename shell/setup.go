package shell

// The part picker: the first time Dotes runs (and with `dotes setup`) you choose the parts
// you want. The others stay hidden; nothing is forced on you.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/internal/ui"
)

type setup struct {
	on     map[string]bool
	cursor int
	first  bool   // first run: welcome text
	note   string // "pick at least one"
	rowY   int    // screen row of the first part (mouse)
}

func newSetup(picked []string, first bool) *setup {
	s := &setup{on: map[string]bool{}, first: first}
	for _, id := range picked {
		s.on[id] = true
	}
	if first { // suggest everything; the user unpicks what they don't want
		for _, p := range app.Parts {
			s.on[p.ID] = true
		}
	}
	return s
}

func (m *Model) setupKey(k tea.KeyPressMsg) tea.Cmd {
	s := m.setup
	s.note = ""
	switch k.String() {
	case "j", "down", "tab":
		s.cursor = (s.cursor + 1) % len(app.Parts)
	case "k", "up", "shift+tab":
		s.cursor = (s.cursor + len(app.Parts) - 1) % len(app.Parts)
	case "space", "x":
		id := app.Parts[s.cursor].ID
		s.on[id] = !s.on[id]
	case "a":
		all := true
		for _, p := range app.Parts {
			all = all && s.on[p.ID]
		}
		for _, p := range app.Parts {
			s.on[p.ID] = !all
		}
	case "enter":
		return m.setupDone()
	case "q", "esc", "ctrl+c":
		if s.first || m.only {
			return tea.Quit // nothing saved: the picker comes back next time
		}
		m.setup = nil
		return m.open()
	}
	return nil
}

func (m *Model) setupClick(y int) tea.Cmd {
	s := m.setup
	if i := y - s.rowY; i >= 0 && i < len(app.Parts) {
		s.cursor = i
		id := app.Parts[i].ID
		s.on[id] = !s.on[id]
	}
	return nil
}

func (m *Model) setupDone() tea.Cmd {
	s := m.setup
	var ids []string
	for _, p := range app.Parts {
		if s.on[p.ID] {
			ids = append(ids, p.ID)
		}
	}
	if len(ids) == 0 {
		s.note = "Pick at least one part."
		return nil
	}
	app.SaveParts(ids)
	if m.only {
		return tea.Quit
	}
	start := ""
	if p := m.cur(); p != nil && s.on[p.info.ID] { // keep the open part only if it's still picked
		start = p.info.ID
	}
	m.setup = nil
	m.loadParts(ids, start)
	return m.open()
}

func (m *Model) setupView() string {
	s := m.setup
	var lines []string
	add := func(l string) { lines = append(lines, l) }
	add("")
	if s.first {
		add("  " + ui.FG(ui.Acc).Bold(true).Render("Welcome to Dotes"))
		add("")
		add("  " + ui.FG(ui.Txt).Render("Pick the parts you want. The others stay hidden."))
		add("  " + ui.FG(ui.Mut).Render("You can change this any time with  dotes setup."))
	} else {
		add("  " + ui.FG(ui.Acc).Bold(true).Render("Dotes parts"))
		add("")
		add("  " + ui.FG(ui.Txt).Render("Pick the parts you want. The others stay hidden."))
	}
	add("")
	s.rowY = len(lines)
	nameW := 0
	for _, p := range app.Parts {
		nameW = max(nameW, ui.Width(p.Name))
	}
	for i, p := range app.Parts {
		mark := ui.FG(ui.Mut).Render("·")
		if s.on[p.ID] {
			mark = ui.FG(ui.Green).Render("✓")
		}
		cur, name := "  ", ui.FG(ui.Txt).Render(ui.Pad(p.Name, nameW))
		if i == s.cursor {
			cur, name = ui.FG(ui.Acc).Render("❯ "), ui.FG(ui.Acc).Bold(true).Render(ui.Pad(p.Name, nameW))
		}
		desc := ui.FG(ui.Hint).Render(p.Desc)
		if !p.Ready {
			desc += ui.FG(ui.Mut).Render("  (coming in a later version)")
		}
		add("  " + cur + mark + " " + name + "   " + desc)
	}
	add("")
	if s.note != "" {
		add("  " + ui.FG(ui.Red).Render(s.note))
	} else {
		add("")
	}
	key := func(k, d string) string { return ui.FG(ui.Acc).Render(k) + " " + ui.FG(ui.Mut).Render(d) }
	add("  " + strings.Join([]string{key("j/k", "move"), key("space", "pick"), key("a", "all"),
		key("enter", "done"), key("q", "cancel")}, ui.FG(ui.Dim).Render("  ·  ")))
	for i := range lines {
		lines[i] = ui.Pad(lines[i], m.w)
	}
	return strings.Join(lines, "\n")
}
