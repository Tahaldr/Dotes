// Package shell is the outer Dotes app: the top bar, switching between parts, and the
// first-run part picker. Each part is a Bubble Tea model drawn under the bar.
package shell

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/internal/ui"
	"github.com/Tahaldr/dotes/parts/files"
	"github.com/Tahaldr/dotes/parts/git"
	"github.com/Tahaldr/dotes/parts/home"
	"github.com/Tahaldr/dotes/parts/notes"
	"github.com/charmbracelet/x/ansi"
)

// Maker builds a part the first time it's opened.
type Maker func() tea.Model

type part struct {
	info app.PartInfo
	make Maker
	m    tea.Model // nil until first opened
}

type Model struct {
	parts   []*part // the picked parts that this version has, in tab order
	active  int
	w, h    int
	bar     bool   // top bar shown
	newVer  string // a newer Dotes is out
	setup   *setup // the part picker (first run / dotes setup)
	only    bool   // dotes setup: quit after picking
	dump    bool
	tabX    [][2]int // x start / end of each part in the switcher, on screen (mouse)
	makers  map[string]Maker
	start   string // the part asked for on the command line: q there leaves Dotes
	cwdFile string
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
	m := &Model{dump: o.Dump, only: o.Setup, start: o.Start, cwdFile: o.Files.CwdFile}
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

// barH: the parts draw the switcher's row themselves now (their tab row), so nothing is
// added above them
func (m *Model) barH() int { return 0 }

// switcherOn: the other parts are shown at the right of the part's tab row (not on Home,
// which is the place you pick from; not with one part)
func (m *Model) switcherOn() bool {
	p := m.cur()
	return m.bar && m.setup == nil && len(m.parts) > 1 && p != nil && p.info.ID != "home"
}

func (m *Model) switchItems() []ui.SwitchItem {
	var items []ui.SwitchItem
	for i, p := range m.parts {
		items = append(items, ui.SwitchItem{Name: p.info.Name, On: i == m.active})
	}
	if m.newVer != "" {
		items = append(items, ui.SwitchItem{Name: "⬆ " + strings.TrimPrefix(m.newVer, "v") + " dotes update"})
	}
	return items
}

// setReserve tells the parts how much of their first line the switcher takes
func (m *Model) setReserve() {
	ui.Reserve = 0
	if m.switcherOn() {
		var names []string
		for _, it := range m.switchItems() {
			names = append(names, it.Name)
		}
		if w := ui.SwitcherWidth(names); w < m.w/2 {
			ui.Reserve = w
		}
	}
}

// home: where q goes back to (-1 = q leaves Dotes: no Home, or this part was asked for
// on the command line)
func (m *Model) home() int {
	p := m.cur()
	if p == nil || p.info.ID == "home" || p.info.ID == m.start {
		return -1
	}
	for i, q := range m.parts {
		if q.info.ID == "home" {
			return i
		}
	}
	return -1
}

type backMsg struct{}

// catchQuit turns a part's quit into "back to Home"
func catchQuit(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case tea.QuitMsg:
			return backMsg{}
		case tea.BatchMsg:
			out := make(tea.BatchMsg, len(msg))
			for i, c := range msg {
				out[i] = catchQuit(c)
			}
			return out
		default:
			return msg
		}
	}
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
	case backMsg: // q in a part: Home again (the part keeps where it was)
		if m.cwdFile != "" {
			os.WriteFile(m.cwdFile, nil, 0o644) // Files wrote its folder for a quit that didn't happen
		}
		if h := m.home(); h >= 0 {
			return m, m.switchTo(h)
		}
		return m, tea.Quit
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
		if msg.Y == 0 && m.switcherOn() && ui.Reserve > 0 && msg.X >= m.w-ui.Reserve {
			for i, x := range m.tabX {
				if msg.X >= x[0] && msg.X < x[1] && i < len(m.parts) {
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
	if m.home() >= 0 {
		cmd = catchQuit(cmd)
	}
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
	m.setReserve()
	var v tea.View
	if p == nil || p.m == nil {
		v = tea.NewView(m.emptyView())
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		v.WindowTitle = "Dotes"
	} else {
		v = p.m.View()
	}
	if ui.Reserve > 0 {
		v.Content = m.withSwitcher(v.Content)
	}
	return v
}

// withSwitcher draws the parts over the right end of the first line
func (m *Model) withSwitcher(content string) string {
	first, rest, _ := strings.Cut(content, "\n")
	sw, xs := ui.Switcher(m.switchItems())
	x0 := m.w - ui.Reserve
	m.tabX = m.tabX[:0]
	for _, x := range xs {
		m.tabX = append(m.tabX, [2]int{x0 + x[0], x0 + x[1]})
	}
	left := ansi.Truncate(first, x0, "")
	left += strings.Repeat(" ", max(0, x0-ui.Width(left)))
	return left + "\x1b[0m" + sw + "\n" + rest
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
