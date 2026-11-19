// Package shell is the outer Dotes app: the top bar, switching between parts, and the
// first-run part picker. Each part is a Bubble Tea model drawn under the bar.
package shell

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/internal/ui"
	"github.com/Tahaldr/dotes/parts/files"
	"github.com/Tahaldr/dotes/parts/git"
	"github.com/Tahaldr/dotes/parts/home"
	"github.com/Tahaldr/dotes/parts/notes"
)

// Maker builds a part the first time it's opened.
type Maker func() tea.Model

type part struct {
	info app.PartInfo
	make Maker
	m    tea.Model // nil until first opened
}

type Model struct {
	parts  []*part // the picked parts that this version has, in tab order
	active int
	w, h   int
	bar    bool   // top bar shown
	newVer string // a newer Dotes is out
	setup  *setup // the part picker (first run / dotes setup)
	only   bool   // dotes setup: quit after picking
	dump   bool
	tabX   [][2]int // x start / end of each tab in the bar (mouse)
	makers map[string]Maker
}

// Options for starting Dotes.
type Options struct {
	Start     string // part id to open first ("" = the first picked)
	Files     files.Options
	FilesPath string
	Notebook  string      // Notes: another notebook folder
	Git       *git.Launch // Git: what dotes g ... asked for
	Projects  bool        // Home: open the project picker straight away
	NoBar     bool        // pick mode: just the part
	Setup     bool        // dotes setup: only the picker
	Dump      bool
}

// New makes the shell. The first time (no dotes.txt) it opens the part picker first.
func New(o Options) *Model {
	m := &Model{dump: o.Dump, only: o.Setup}
	m.makers = map[string]Maker{
		"home": func() tea.Model {
			return home.New(home.Options{CwdFile: o.Files.CwdFile, Projects: o.Projects, Dump: o.Dump})
		},
		"files": func() tea.Model {
			files.LoadConfig()
			fo := o.Files
			fo.Dump = o.Dump
			return files.New(o.FilesPath, fo)
		},
		"notes": func() tea.Model {
			notes.LoadConfig(o.Notebook)
			return notes.New(notes.Options{Dump: o.Dump})
		},
		"git": func() tea.Model {
			git.LoadConfig()
			return git.New(o.Git, o.Dump)
		},
	}
	sh := app.LoadShared()
	m.bar = sh.TabBar && !o.NoBar
	if o.Setup || !app.HasShared() {
		m.setup = newSetup(sh.Parts, !app.HasShared())
	}
	m.loadParts(sh.Parts, o.Start)
	return m
}

// loadParts keeps the picked parts this version has. A part asked for on the command line
// (dotes f) is opened even when it isn't picked.
func (m *Model) loadParts(picked []string, start string) {
	want := map[string]bool{start: true}
	for _, id := range picked {
		want[id] = true
	}
	old := map[string]*part{}
	for _, p := range m.parts {
		old[p.info.ID] = p
	}
	m.parts = nil
	for _, info := range app.Parts {
		if !want[info.ID] || m.makers[info.ID] == nil {
			continue
		}
		if p := old[info.ID]; p != nil {
			m.parts = append(m.parts, p)
		} else {
			m.parts = append(m.parts, &part{info: info, make: m.makers[info.ID]})
		}
	}
	m.active = 0
	for i, p := range m.parts {
		if p.info.ID == start {
			m.active = i
		}
	}
}

func (m *Model) barH() int {
	if m.bar && m.setup == nil {
		return 1
	}
	return 0
}

func (m *Model) cur() *part {
	if m.active < len(m.parts) {
		return m.parts[m.active]
	}
	return nil
}

// open starts the active part if it hasn't run yet
func (m *Model) open() tea.Cmd {
	p := m.cur()
	if p == nil || p.m != nil {
		return nil
	}
	files.OriginY = m.barH()
	p.m = p.make()
	cmd := p.m.Init()
	if m.w > 0 {
		var c2 tea.Cmd
		p.m, c2 = p.m.Update(tea.WindowSizeMsg{Width: m.w, Height: m.h - m.barH()})
		cmd = tea.Batch(cmd, c2)
	}
	return cmd
}

func (m *Model) Init() tea.Cmd {
	m.newVer = app.NewerSaved()
	var cmd tea.Cmd
	if !m.dump {
		cmd = app.CheckUpdate()
	}
	if m.setup != nil {
		return cmd
	}
	return tea.Batch(cmd, m.open())
}

// switchTo opens part i
func (m *Model) switchTo(i int) tea.Cmd {
	if i < 0 || i >= len(m.parts) || i == m.active {
		return nil
	}
	m.active = i
	cmd := m.open()
	// tell every part which one is on screen (Files hides its pictures when it isn't)
	return tea.Batch(cmd, m.broadcast(app.ShownMsg{ID: m.parts[i].info.ID}))
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		files.OriginY = m.barH()
		return m, m.broadcast(tea.WindowSizeMsg{Width: m.w, Height: m.h - m.barH()})
	case app.SwitchMsg: // Home opens another part; Files, Git and Notes link to each other
		at := -1
		for i, p := range m.parts {
			if p.info.ID == msg.ID {
				at = i
			}
		}
		if at < 0 && m.makers[msg.ID] != nil { // not picked: add it for this run
			for _, info := range app.Parts {
				if info.ID == msg.ID {
					m.parts = append(m.parts, &part{info: info, make: m.makers[msg.ID]})
					at = len(m.parts) - 1
				}
			}
		}
		if at < 0 {
			return m, nil
		}
		cmd := m.switchTo(at)
		if msg.Path != "" {
			cmd = tea.Batch(cmd, m.toActive(app.OpenMsg{Path: msg.Path}))
		}
		return m, cmd
	case app.UpdateMsg:
		if app.Newer(msg.Latest, app.Version) {
			m.newVer = msg.Latest
		}
		return m, nil
	case tea.KeyPressMsg:
		if m.setup != nil {
			return m, m.setupKey(msg)
		}
		if msg.Mod == tea.ModAlt && msg.Code >= '1' && msg.Code <= '9' {
			return m, m.switchTo(int(msg.Code - '1'))
		}
		return m, m.toActive(msg)
	case tea.MouseClickMsg:
		if m.setup != nil {
			return m, m.setupClick(msg.Y)
		}
		if msg.Y < m.barH() {
			for i, x := range m.tabX {
				if msg.X >= x[0] && msg.X < x[1] {
					return m, m.switchTo(i)
				}
			}
			return m, nil
		}
		mm := tea.Mouse(msg)
		mm.Y -= m.barH()
		return m, m.toActive(tea.MouseClickMsg(mm))
	case tea.MouseReleaseMsg:
		mm := tea.Mouse(msg)
		mm.Y -= m.barH()
		return m, m.toActive(tea.MouseReleaseMsg(mm))
	case tea.MouseMotionMsg:
		mm := tea.Mouse(msg)
		mm.Y -= m.barH()
		return m, m.toActive(tea.MouseMotionMsg(mm))
	case tea.MouseWheelMsg:
		if m.setup != nil {
			return m, nil
		}
		mm := tea.Mouse(msg)
		mm.Y -= m.barH()
		return m, m.toActive(tea.MouseWheelMsg(mm))
	case tea.PasteMsg:
		return m, m.toActive(msg)
	}
	// anything else (answers to a part's background work, terminal replies) goes to every
	// running part: each one only reacts to its own messages
	return m, m.broadcast(msg)
}

func (m *Model) toActive(msg tea.Msg) tea.Cmd {
	p := m.cur()
	if p == nil || p.m == nil {
		if k, ok := msg.(tea.KeyPressMsg); ok && (k.String() == "q" || k.String() == "ctrl+c") {
			return tea.Quit
		}
		return nil
	}
	var cmd tea.Cmd
	p.m, cmd = p.m.Update(msg)
	return cmd
}

func (m *Model) broadcast(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for _, p := range m.parts {
		if p.m != nil {
			var c tea.Cmd
			p.m, c = p.m.Update(msg)
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) View() tea.View {
	if m.setup != nil {
		v := tea.NewView(m.setupView())
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		v.WindowTitle = "Dotes · setup"
		return v
	}
	p := m.cur()
	var v tea.View
	if p == nil || p.m == nil {
		v = tea.NewView(m.emptyView())
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		v.WindowTitle = "Dotes"
	} else {
		v = p.m.View()
	}
	if m.barH() > 0 {
		v.Content = m.barView() + "\n" + v.Content
		if v.Cursor != nil {
			v.Cursor.Y += m.barH()
		}
	}
	return v
}

// barView: " dotes  1 Files │ 2 Notes            ⬆ 0.2.0 dotes update  alt+1-2 "
func (m *Model) barView() string {
	left := " " + ui.FG(ui.Acc).Bold(true).Render("dotes") + "  "
	x := ui.Width(left)
	m.tabX = m.tabX[:0]
	for i, p := range m.parts {
		if i > 0 {
			sep := ui.FG(ui.Dim).Render(" │ ")
			left += sep
			x += ui.Width(sep)
		}
		num := ui.FG(ui.Mut).Render(strconv.Itoa(i+1) + " ")
		name := ui.FG(ui.Hint).Render(p.info.Name)
		if i == m.active {
			num = ui.FG(ui.Acc).Render(strconv.Itoa(i+1) + " ")
			name = ui.FG(ui.Acc).Bold(true).Underline(true).Render(p.info.Name)
		}
		tab := num + name
		m.tabX = append(m.tabX, [2]int{x, x + ui.Width(tab)})
		left += tab
		x += ui.Width(tab)
	}
	right := ""
	if m.newVer != "" {
		right += ui.FG(ui.Green).Render("⬆ "+strings.TrimPrefix(m.newVer, "v")+" dotes update") + "   "
	}
	if len(m.parts) > 1 {
		right += ui.FG(ui.Mut).Render("alt+1-"+strconv.Itoa(len(m.parts))+" switch") + " "
	}
	return ui.Spread(left, right, m.w)
}

// emptyView: none of the picked parts is in this version yet
func (m *Model) emptyView() string {
	lines := []string{"", "  " + ui.FG(ui.Acc).Bold(true).Render("Nothing to show yet"), "",
		"  " + ui.FG(ui.Txt).Render("The parts you picked come in a later Dotes version."),
		"  " + ui.FG(ui.Mut).Render("Run  dotes setup  to pick a part that is ready now.  q quits."),
	}
	return strings.Join(lines, "\n")
}

// ExitMessages: what parts want printed after Dotes closes
func (m *Model) ExitMessages() []string {
	var out []string
	for _, p := range m.parts {
		if e, ok := p.m.(interface{ ExitMessage() string }); ok && e.ExitMessage() != "" {
			out = append(out, e.ExitMessage())
		}
	}
	return out
}
