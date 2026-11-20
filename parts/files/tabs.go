package files

// Tabs, like yazi: tt new, tr rename, 1-9 switch, [ ] previous / next, { } move, ctrl+c close.
// Each tab remembers its folder, cursor, history, filter and search results.

import (
	"fmt"
	"github.com/Tahaldr/dotes/internal/ui"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type tabSnap struct {
	name            string
	custom          bool // renamed with tr (otherwise named after its folder)
	cwd             string
	cursor, top     int
	back, fwd       []string
	filter, find    string
	virtual, vTitle string
	vAll, entries   []entry
}

func (m *model) snap() tabSnap {
	s := tabSnap{cwd: m.cwd, cursor: m.cursor, top: m.top, back: m.back, fwd: m.fwd, filter: m.filter,
		find: m.find, virtual: m.virtual, vTitle: m.vTitle, vAll: m.vAll, entries: m.entries}
	if len(m.tabs) > m.tabIdx {
		s.name, s.custom = m.tabs[m.tabIdx].name, m.tabs[m.tabIdx].custom
	}
	return s
}

func (m *model) restore(s tabSnap) {
	m.searchMoved = true
	m.commitVisual()
	m.cwd, m.back, m.fwd, m.filter, m.find = s.cwd, s.back, s.fwd, s.filter, s.find
	m.virtual, m.vTitle, m.vAll, m.entries = s.virtual, s.vTitle, s.vAll, s.entries
	m.cursor, m.top = s.cursor, s.top
	m.branch = gitBranch(m.cwd)
	m.reload()
	m.cursor = min(s.cursor, max(0, len(m.entries)-1))
	m.pscroll = 0
	m.gitStale = true
}

func (m *model) tabName(i int) string {
	t := m.tabs[i]
	if t.custom {
		return t.name
	}
	switch t.virtual {
	case "drives":
		return "This PC"
	case "search":
		return "search"
	}
	if isRoot(t.cwd) {
		return t.cwd
	}
	return filepath.Base(t.cwd)
}

func (m *model) newTab() {
	if len(m.tabs) == 0 {
		m.tabs = []tabSnap{m.snap()}
	}
	if len(m.tabs) >= 9 {
		m.setNote("9 tabs is the most", true)
		return
	}
	m.tabs[m.tabIdx] = m.snap()
	t := m.snap()
	t.name, t.custom = "", false
	t.back, t.fwd = nil, nil
	m.tabs = append(m.tabs[:m.tabIdx+1], append([]tabSnap{t}, m.tabs[m.tabIdx+1:]...)...)
	m.tabIdx++
	m.setNote(fmt.Sprintf("tab %d opened", m.tabIdx+1), false)
}

func (m *model) switchTab(i int) {
	if len(m.tabs) < 2 || i < 0 || i >= len(m.tabs) || i == m.tabIdx {
		return
	}
	m.tabs[m.tabIdx] = m.snap()
	m.tabIdx = i
	m.restore(m.tabs[i])
}

// closeTab closes this tab; false = it was the last one
func (m *model) closeTab() bool {
	if len(m.tabs) < 2 {
		return false
	}
	m.tabs = append(m.tabs[:m.tabIdx], m.tabs[m.tabIdx+1:]...)
	m.tabIdx = min(m.tabIdx, len(m.tabs)-1)
	m.restore(m.tabs[m.tabIdx])
	if len(m.tabs) == 1 {
		m.tabs = nil
		m.tabIdx = 0
	}
	return true
}

func (m *model) swapTab(d int) {
	j := m.tabIdx + d
	if len(m.tabs) < 2 || j < 0 || j >= len(m.tabs) {
		return
	}
	m.tabs[m.tabIdx] = m.snap()
	m.tabs[m.tabIdx], m.tabs[j] = m.tabs[j], m.tabs[m.tabIdx]
	m.tabIdx = j
}

func (m *model) renameTab() {
	if len(m.tabs) == 0 {
		m.tabs = []tabSnap{m.snap()}
	}
	m.input("Rename tab", "empty = named after its folder again", m.tabName(m.tabIdx), -1, func(m *model, s string) tea.Cmd {
		s = strings.TrimSpace(s)
		m.tabs[m.tabIdx].name, m.tabs[m.tabIdx].custom = s, s != ""
		if len(m.tabs) == 1 && s == "" {
			m.tabs = nil
		}
		return nil
	})
}

// tabBar: the Dotes tab row: "Files", then the folder tabs when there's more than one
// (or a named one), and what's being copied / moved on the right
func (m *model) tabBar() string {
	var ts []ui.Tab
	if len(m.tabs) > 0 {
		m.tabs[m.tabIdx] = m.snap()
		for i := range m.tabs {
			ts = append(ts, ui.Tab{Name: fmt.Sprintf("%d %s", i+1, trunc(m.tabName(i), 20))})
		}
	}
	right := ""
	if n := m.running(); n > 0 {
		right = fg(cYellow).Render(fmt.Sprintf("%d task%s · w", n, map[bool]string{true: "s"}[n > 1]))
	}
	line, xs := ui.TabRow(icFolder, "Files", ts, m.tabIdx, right, m.w)
	m.tabXs = xs
	return line
}
