package files

import (
	"fmt"

	"github.com/Tahaldr/dotes/internal/rtl"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const scrollOff = 5 // rows kept visible above / below the cursor, like yazi

func (m *model) View() tea.View {
	var content string
	switch {
	case m.w < 40 || m.h < 8:
		content = fg(cMut).Render("Files needs a bigger window (at least 40 x 8).")
	case m.help:
		content = m.helpView()
	default:
		content = m.overlays(m.mainView())
	}
	v := tea.NewView(rtl.Frame(content)) // Arabic names readable (Windows Terminal has no right-to-left)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "Dotes · " + filepath.Base(m.cwd)
	return v
}

func (m *model) mainView() string {
	m.imgWant = imgSpot{}                          // set again below if the preview is a picture
	hints := []string{" " + keyHints("~", "keys")} // hidden hints: only the way to the full list
	if m.hintsOn {
		hints = hintRows(m.w, 2, m.contextHints())
	}
	tabRow := m.tabBar()
	m.bodyY = 1
	if tabRow != "" {
		m.bodyY = 2
	}
	m.bodyH = max(1, m.h-1-m.bodyY-len(hints))

	// column widths 1 : 4 : 3, like yazi (no parent column in narrow windows)
	avail := m.w - 2
	pw := avail / 8
	if m.w < 70 {
		pw = 0
		avail = m.w - 1
	}
	cw := avail / 2
	if pw == 0 {
		cw = avail * 4 / 7
	}
	prw := avail - pw - cw
	x := 0
	if pw > 0 {
		m.colX[0] = [2]int{0, pw}
		x = pw + 1
	} else {
		m.colX[0] = [2]int{0, 0}
	}
	m.colX[1] = [2]int{x, x + cw}
	m.colX[2] = [2]int{x + cw + 1, m.w}

	// keep the cursor away from the edges
	h := m.bodyH
	off := min(scrollOff, (h-1)/2)
	if m.cursor < m.top+off {
		m.top = max(0, m.cursor-off)
	}
	if m.cursor > m.top+h-1-off {
		m.top = m.cursor - h + 1 + off
	}
	m.top = max(0, min(m.top, len(m.entries)-h))

	var parentCol []string
	if pw > 0 {
		parentCol = renderList(m.parent, m.parentIdx, listTop(len(m.parent), m.parentIdx, h), pw, h, nil)
	}
	curCol := renderList(m.entries, m.cursor, m.top, cw, h, &listOpts{m.rowMark, m.lineMode, m.find, m.gitMarks})
	if len(m.entries) == 0 {
		msg := "(empty folder)"
		if m.readErr != "" {
			msg = "can't read: " + m.readErr
		} else if m.filter != "" {
			msg = "nothing matches " + m.filter + " · esc clears the filter"
		} else if m.virtual == "search" {
			msg = "nothing found · h goes back"
		} else if m.hiddenN > 0 {
			msg = fmt.Sprintf("(only %d hidden, press . to show)", m.hiddenN)
		}
		curCol[0] = "   " + fg(cMut).Render(msg)
	}
	prevCol := m.previewCol(prw, h)

	sep := fg(cDim).Render("│")
	lines := make([]string, 0, m.h)
	if tabRow != "" {
		lines = append(lines, tabRow)
	}
	lines = append(lines, m.header())
	for r := 0; r < h; r++ {
		row := ""
		if pw > 0 {
			row = pad(parentCol[r], pw) + sep
		}
		row += pad(curCol[r], cw) + sep + pad(prevCol[r], prw)
		lines = append(lines, row)
	}
	lines = append(lines, m.statusBar())
	lines = append(lines, hints...)
	return strings.Join(lines, "\n")
}

// listTop for side columns: show the hovered row about a third down
func listTop(n, idx, h int) int {
	if idx < 0 || n <= h {
		return 0
	}
	return max(0, min(idx-h/3, n-h))
}

// rowMark: the bar left of a row: green = selected, yellow = copied, red = cut
func (m *model) rowMark(i int) string {
	e := &m.entries[i]
	if m.selectedAt(i) {
		return fg(cGreen).Render("▌")
	}
	if y, cut := m.isYanked(e.path); y {
		if cut {
			return fg(cRed).Render("▌")
		}
		return fg(cYellow).Render("▌")
	}
	return " "
}

// listOpts: only the current folder has them (marks, cursor marker, row info, find matches)
type listOpts struct {
	marks    func(int) string
	lineMode string
	hl       string
	git      map[string]string // git marks by lower-case path
}

// renderList draws a column of entries; o == nil for the side columns
func renderList(list []entry, cursor, top, w, h int, o *listOpts) []string {
	main := o != nil
	out := make([]string, h)
	for r := 0; r < h; r++ {
		i := top + r
		if i >= len(list) {
			break
		}
		e := &list[i]
		on := i == cursor
		ic := iconFor(e)
		name := e.name
		nameSt := fg(cTxt)
		switch {
		case on:
			nameSt = fg(cAcc).Bold(true)
		case main && o.hl != "" && smartMatch(e.name, o.hl):
			nameSt = fg(cYellow)
		case e.hidden:
			nameSt = fg(cMut)
		}
		gm := ""
		if main && o.git != nil {
			gm = o.git[strings.ToLower(e.path)]
			if gm == "!" && !on { // ignored by git: faded
				nameSt = fg(cMut)
			}
		}
		if e.isLink {
			nameSt = nameSt.Italic(true)
		}
		icSt := fg(ic.col)
		if e.hidden && !on {
			icSt = fg(cMut)
		}
		mark := " "
		if main {
			mark = o.marks(i) + "  "
			if on {
				mark = o.marks(i) + fg(cAcc).Render("❯") + " "
			}
		}
		right := ""
		if main {
			right = lineInfo(e, o.lineMode)
			if gm != "" && gm != "!" {
				right = strings.TrimSpace(gm + "  " + right)
			}
		} else if e.drive && w >= 40 {
			right = e.label
		}
		left := mark + icSt.Render(ic.glyph) + " "
		room := w - width(left) - 1
		if right != "" {
			room -= width(right) + 1
		}
		row := left + nameSt.Render(trunc(name, max(1, room)))
		if right != "" {
			gap := w - width(row) - width(right) - 1
			styled := fg(cMut).Render(right)
			if gm != "" && gm != "!" && strings.HasPrefix(right, gm) { // the git mark in its color
				styled = gitMarkStyle(gm) + fg(cMut).Render(right[len(gm):])
			}
			row += strings.Repeat(" ", max(1, gap)) + styled
		}
		out[r] = row
	}
	return out
}

func (m *model) previewCol(w, h int) []string {
	out := make([]string, h)
	c := m.current()
	if c == nil {
		return out
	}
	if m.pv.key != previewKey(*c, m.showHidden) {
		out[0] = " " + fg(cMut).Render("loading…")
		return out
	}
	if m.pv.dir {
		if len(m.pv.entries) == 0 {
			msg := "(empty folder)"
			if m.pv.hiddenN > 0 {
				msg = fmt.Sprintf("(%d hidden)", m.pv.hiddenN)
			}
			out[0] = " " + fg(cMut).Render(msg)
			return out
		}
		idx := m.previewIdx()
		return renderList(m.pv.entries, idx, listTop(len(m.pv.entries), idx, h), w, h, nil)
	}
	lines := m.pv.lines
	switch {
	case m.pv.kind == "image" && m.gfx == "sixel" && !m.popupOpen():
		m.pscroll = 0 // the picture sits at a fixed spot
		lines = m.sixelPreview(w-1, h, m.colX[2][0]+1, m.bodyY)
	case m.pv.kind != "":
		lines = m.pv.richLines(w-1, h)
	}
	m.pscroll = max(0, min(m.pscroll, len(lines)-h))
	for r := 0; r < h && m.pscroll+r < len(lines); r++ {
		lead := " "
		if c.matchLine > 0 && m.pscroll+r+1 == c.matchLine { // the line a text search found
			lead = fg(cYellow).Render("▌")
		}
		out[r] = lead + trunc(lines[m.pscroll+r], w-1)
	}
	return out
}

func (m *model) header() string {
	p := prettyPath(m.cwd)
	dir, last := filepath.Split(p)
	if last == "" { // a drive root: C:\
		dir, last = "", p
	}
	icon := icFolder
	if strings.HasPrefix(p, "~") {
		icon = icHome
	}
	left := " " + fg(cAcc).Render(icon) + "  " + fg(cHint).Render(dir) + fg(cAcc).Bold(true).Render(last)
	switch m.virtual {
	case "drives":
		left = " " + fg(cAcc).Render(icPC) + "  " + fg(cAcc).Bold(true).Render(m.vTitle)
	case "projects":
		left = " " + fg(cAcc).Render(icRepo) + "  " + fg(cAcc).Bold(true).Render(m.vTitle) + fg(cMut).Render("   h browses "+prettyPath(m.cwd))
	case "search":
		left = " " + fg(cAcc).Render(icSearch) + "  " + fg(cHint).Render(prettyPath(m.cwd)+"  ") + fg(cAcc).Bold(true).Render(m.vTitle)
	}
	if m.searching {
		left += "  " + fg(cYellow).Render(icSearch+" "+m.vTitle+"  ctrl+s stops")
	}
	if m.virtual == "" && m.bookmarkIndex(m.cwd) >= 0 {
		left += "  " + fg(cYellow).Render(icBookmark)
	}
	right := ""
	if m.filter != "" {
		right += fg(cYellow).Render(icFilter+" "+m.filter) + "   "
	}
	if m.find != "" {
		right += fg(cYellow).Render(icSearch+" "+m.find+"  n/N") + "   "
	}
	if m.sortBy != "natural" || m.sortRev {
		label := strings.Fields(sortNames[m.sortBy])[0]
		if m.sortRev {
			label += " ↑"
		}
		right += fg(cHint).Render(icSort+" "+label) + "   "
	}
	if m.branch != "" && m.virtual == "" {
		right += fg(cPurple).Render(icBranch+" "+m.branch) + "   "
	}
	pos := "0/0"
	if len(m.entries) > 0 {
		pos = fmt.Sprintf("%d/%d", m.cursor+1, len(m.entries))
	}
	right += fg(cMut).Render(pos) + " "
	return spread(left, right, m.w)
}

func (m *model) statusBar() string {
	mode, mc := " NOR ", cAcc
	switch m.visual {
	case visSel:
		mode, mc = " VIS ", cPurple
	case visUnset:
		mode, mc = " UNS ", cRed
	}
	left := " " + lipgloss.NewStyle().Background(mc).Foreground(cBg).Bold(true).Render(mode)
	if c := m.current(); c != nil && c.drive {
		left += "  " + fg(cHint).Render(c.path+"  "+c.label)
	} else if c != nil {
		parts := []string{c.mode.String()}
		if !c.isDir {
			parts = append(parts, humanSize(c.size))
		}
		if !c.mod.IsZero() {
			parts = append(parts, c.mod.Format("2006-01-02 15:04"))
		}
		left += "  " + fg(cHint).Render(strings.Join(parts, "  "))
	}
	if m.pending != "" {
		left += "  " + fg(cYellow).Render(m.pending+"…")
	}
	// what's going on: selection, clipboard, tasks
	var chips []string
	if n := m.selCount(); n > 0 {
		chips = append(chips, fg(cGreen).Render(fmt.Sprintf("%d selected", n)))
	}
	if m.yank != nil {
		if m.yank.cut {
			chips = append(chips, fg(cRed).Render(fmt.Sprintf("%d cut", len(m.yank.paths))))
		} else {
			chips = append(chips, fg(cYellow).Render(fmt.Sprintf("%d copied", len(m.yank.paths))))
		}
	}
	if n := m.running(); n > 0 {
		pct := 0
		for _, t := range m.tasks {
			if !t.ended.Load() {
				pct = t.percent()
				break
			}
		}
		chips = append(chips, fg(cAcc).Render(fmt.Sprintf("%s %d task · %d%% · w", icTasks, n, pct)))
	}
	right := ""
	switch {
	case m.note != "" && m.noteErr:
		right = fg(cRed).Render(m.note)
	case m.note != "":
		right = fg(cGreen).Render(m.note)
	case !m.showHidden && m.hiddenN > 0:
		right = fg(cMut).Render(fmt.Sprintf("%d hidden · . to show", m.hiddenN))
	}
	if len(chips) > 0 {
		sep := fg(cDim).Render("  │  ")
		if right != "" {
			right += sep
		}
		right += strings.Join(chips, sep)
	}
	return spread(left, right+" ", m.w)
}

// spread puts left and right on one line of width w (right is cut first if it doesn't fit)
func spread(left, right string, w int) string {
	gap := w - width(left) - width(right)
	if gap < 1 {
		right = trunc(right, max(0, w-width(left)-1))
		gap = w - width(left) - width(right)
	}
	return pad(left+strings.Repeat(" ", max(0, gap))+right, w)
}

func (m *model) helpView() string {
	var body []string
	group := ""
	for _, k := range keyTable {
		if k.group != group {
			if group != "" {
				body = append(body, "")
			}
			group = k.group
			body = append(body, "  "+fg(cAcc).Bold(true).Render(strings.ToUpper(group)))
		}
		body = append(body, "    "+pad(fg(cTxt).Render(k.keys), 32)+fg(cHint).Render(k.desc))
	}
	h := m.h - 2
	m.helpScroll = max(0, min(m.helpScroll, len(body)-h))
	lines := []string{spread(" "+fg(cAcc).Bold(true).Render("Files keys"), fg(cMut).Render("yazi keys, TermNook look")+" ", m.w)}
	for r := 0; r < h; r++ {
		if i := m.helpScroll + r; i < len(body) {
			lines = append(lines, pad(body[i], m.w))
		} else {
			lines = append(lines, "")
		}
	}
	hl := "hide the key hints"
	if !m.hintsOn {
		hl = "show the key hints"
	}
	lines = append(lines, " "+keyHints("j/k", "scroll", "h", hl, "e", "edit settings", "esc", "close"))
	return strings.Join(lines, "\n")
}
