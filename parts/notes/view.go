package notes

import (
	"fmt"
	"github.com/Tahaldr/dotes/internal/paths"
	"os"
	"strings"

	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/internal/rtl"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const scrollOff = 3

// The layout zooms, like a desk: a board of cards, and a card opens into a full page.
//
//	 󰂾 Notes         󰄲 Todo lists 3    󰠮 Notes 3    󰃭 Today 2           ~\Documents\Coding\Notebook
//	────────────────━━━━━━━━━━━━━━━━──────────────────────────────────────────────────────────────────
//	  󰄲  Todo lists   3 lists · 6 of 14 done · 2 due soon
//
//	  ╭─ 󰄲 Groceries ────── 󰃭 1 ─╮  ╭─ 󰄲 Work ──────── 󰃭 1 ─╮  ╭┄ + ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄╮
//	  │ ━━━━━━━━──────────  3/8  │  │ ━━━━━─────────── 1/4  │  ┊      New todo list    ┊
//	  │ 󰄱 milk           today   │  │ 󰄱 finish …    1d late │  ┊           a           ┊
//	  ╰──────────────────────────╯  ╰───────────────────────╯  ╰┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄╯
//
// enter opens a card: a todo list (with a details panel on wide windows) or a note to read.

func (m *model) View() tea.View {
	var content string
	switch {
	case m.w < 50 || m.h < 10:
		content = fg(cMut).Render("Notes needs a bigger window (at least 50 x 10).")
	case m.help:
		content = m.helpView()
	default:
		content = m.overlays(m.mainView())
	}
	v := tea.NewView(rtl.Frame(content)) // Arabic readable (Windows Terminal has no right-to-left)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "Dotes · Notes"
	if d := m.current(); d != nil {
		v.WindowTitle = "Dotes · " + d.title
	}
	return v
}

// page: what fills the body
func (m *model) page() string {
	switch {
	case m.tab == "today":
		return "today"
	case m.focus == 1 && m.doc != nil:
		return "todos"
	case m.focus == 1 && m.tab == "notes" && m.current() != nil:
		return "note"
	}
	return "board"
}

func (m *model) hintCtx() string {
	if m.edit != nil {
		return "edit"
	}
	if p := m.page(); p != "board" {
		return p
	}
	return "lists"
}

func (m *model) mainView() string {
	hints := []string{" " + keyHints("~", "keys")} // hidden hints: only the way to the full list
	if m.hintsOn {
		hints = hintRows(m.hintCtx(), m.w, 2)
	}
	m.bodyY = 2
	m.bodyH = max(1, m.h-3-len(hints))
	lines := []string{m.header(), m.tabRule()}
	m.cards, m.rightRows, m.agendaRows = nil, nil, nil
	m.editY = -1
	var body []string
	switch m.page() {
	case "today":
		body = m.agendaView(m.w, m.bodyH)
	case "todos":
		body = m.todoPage(m.w, m.bodyH)
	case "note":
		body = m.notePage(m.w, m.bodyH)
	default:
		body = m.board(m.w, m.bodyH)
	}
	for r := 0; r < m.bodyH; r++ {
		l := ""
		if r < len(body) {
			l = body[r]
		}
		lines = append(lines, pad(l, m.w))
	}
	lines = append(lines, m.statusBar())
	lines = append(lines, hints...)
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// the header: the name, three tabs (the active one underlined in the rule below) and the folder
// ---------------------------------------------------------------------------

func (m *model) header() string {
	left := " " + fg(cAcc).Render(icBook) + " " + fg(cTxt).Bold(true).Render("Notes") + "    "
	for i, kind := range tabs {
		name, icon, n := "Todo lists", icTodos, m.count(kind)
		switch kind {
		case "notes":
			name, icon = "Notes", icNotes
		case "today":
			name, icon, n = "Today", icCalendar, m.dueSoon()
		}
		on := kind == m.tab
		st, cst := fg(cHint), fg(cMut)
		switch {
		case on:
			st, cst = fg(cAcc).Bold(true), fg(cAcc)
		case kind == "today" && n > 0:
			st, cst = fg(cYellow), fg(cYellow)
		}
		label := " " + st.Render(icon+" "+name)
		if n > 0 {
			label += " " + cst.Render(fmt.Sprint(n))
		}
		label += " "
		x := width(left)
		left += label + "  "
		m.tabX[i] = [2]int{x, x + width(label)}
	}
	right := fg(cMut).Render(prettyPath(cfg.notebook)) + " "
	return spread(left, right, m.w)
}

// tabRule: the line under the header, thick and blue under the open tab
func (m *model) tabRule() string {
	a, b := 0, 0
	for i, kind := range tabs {
		if kind == m.tab {
			a, b = m.tabX[i][0], m.tabX[i][1]
		}
	}
	a, b = max(0, min(a, m.w)), max(0, min(b, m.w))
	return fg(cDim).Render(strings.Repeat("─", a)) + fg(cAcc).Render(strings.Repeat("━", b-a)) +
		fg(cDim).Render(strings.Repeat("─", m.w-b))
}

// shown: how far a line is indented on screen (each sub-todo level is cfg.indent wide)
func shown(s string) int { return indentOf(s) * cfg.indent / 2 }

// bar draws a progress bar of w cells
func bar(done, all, w int) string {
	if all == 0 {
		return fg(cDim).Render(strings.Repeat("─", w))
	}
	fill := w * done / all
	col := cAcc
	if done == all {
		col = cGreen
	}
	return fg(col).Render(strings.Repeat("━", fill)) + fg(cDim).Render(strings.Repeat("─", w-fill))
}

var prioColor = map[string]lipgloss.Style{
	"high": fg(cRed).Bold(true), "medium": fg(cOrange), "low": fg(cAcc),
}

func prioChip(p string) string {
	st, ok := prioColor[p]
	if !ok {
		st = fg(cHint)
	}
	return st.Render("● " + p)
}

// paneTitle: the way back, then the big title with a line under it
func paneTitle(crumb, icon string, col lipgloss.Style, title, under string) []string {
	return []string{fg(cMut).Render("  ‹ " + crumb), "", "  " + col.Render(icon) + "  " + fg(cTxt).Bold(true).Render(title), "  " + under, ""}
}

// center: the left margin that centers a column of w in total
func center(total, w int) int { return max(0, (total-w)/2) }

// ---------------------------------------------------------------------------
// an open todo list: a centered page, and a details panel on the right when there's room
// ---------------------------------------------------------------------------

const detailsW = 36

func (m *model) todoPage(w, h int) []string {
	area, side := w, 0
	if w >= 120 {
		side = detailsW
		area = w - side - 1
	}
	pw := min(area-4, 104)
	m.pageX = center(area, pw)
	page := m.todoPane(pw, h)
	var det []string
	if side > 0 {
		det = m.details(side-3, h)
	}
	out := make([]string, h)
	margin := strings.Repeat(" ", m.pageX)
	for r := range out {
		out[r] = margin + page[r]
		if side > 0 {
			out[r] = pad(out[r], area) + fg(cDim).Render("│") + "  " + det[r]
		}
	}
	return out
}

func (m *model) todoPane(w, h int) []string {
	d := m.doc
	focused := m.focus == 1
	now := today()
	var out []string
	var rowOf, boxOf []int
	add := func(s string, ln, box int) {
		out = append(out, s)
		rowOf = append(rowOf, ln)
		boxOf = append(boxOf, box)
	}
	title := titleOf(d.texts(), d.path)
	done, all := progress(d.texts())
	summary := "no todos yet"
	if all > 0 {
		summary = fmt.Sprintf("%d of %d done", done, all)
	}
	bw := max(6, min(40, w-width(summary)-8))
	for k, l := range paneTitle("Todo lists", icTodos, fg(cGreen), title, bar(done, all, bw)+"  "+fg(cMut).Render(summary)) {
		ln := -1
		if k == 0 {
			ln = -2 // the way back (a click goes back)
		}
		add(l, ln, -1)
	}

	curStart, curEnd := -1, -1
	editAt, editCol := -1, 0 // the row being typed, and where its path starts (the popup goes there)
	markEdit := func(x, fw int) {
		editAt = len(out)
		col := 0
		if start, _, ok := paths.Word(m.edit.p.text, m.edit.p.pos); ok {
			col = min(width(string(m.edit.p.text[:start])), max(0, fw-12))
		}
		editCol = m.pageX + x + col
	}
	rows := d.rows()
	arch := d.archiveStart()
	if len(rows) == 0 && m.edit == nil {
		add("     "+fg(cMut).Render("o adds the first todo"), -1, -1)
	}
	for k, i := range rows {
		s := d.lines[i].s
		on := i == m.cur
		archived := i > arch
		if isHeading(s) && k > 0 {
			add("", -1, -1)
		}
		mark := "    "
		if on && focused {
			mark = fg(cAcc).Render("  ❯ ")
		} else if on {
			mark = fg(cDim).Render("  ❯ ")
		}
		start := len(out)
		switch t, ok := parseTodo(s); {
		case ok:
			lead := mark + strings.Repeat(" ", shown(s))
			boxSt, box := fg(cHint), icBox
			textSt := fg(cTxt)
			if t.done {
				boxSt, box, textSt = fg(cGreen), icBoxDone, fg(cMut).Strikethrough(true)
			}
			if on && focused && !t.done {
				textSt = fg(cAcc).Bold(true)
			}
			if archived {
				boxSt, textSt = fg(cMut), fg(cMut)
			}
			prefix := lead + boxSt.Render(box) + " "
			pw := width(prefix)
			boxAt := width(lead) + m.pageX
			if m.edit != nil && m.edit.line == i {
				markEdit(pw, max(4, w-pw-2))
				add(prefix+m.edit.p.field(max(4, w-pw-2), fg(cTxt)), i, boxAt)
				break
			}
			plain, prio, due := splitTags(t.text)
			// chips on the right: priority, due date, sub-todos
			var chips []string
			if prio != "" {
				chips = append(chips, prioChip(prio))
			}
			if due != "" {
				l, st := dueLabel(due, t.done || archived, now)
				chips = append(chips, st.Render(icCalendar+" "+l))
			}
			if d.hasSubs(i) {
				sd, sa := d.subCount(i)
				c := fmt.Sprintf("%d/%d", sd, sa)
				if d.lines[i].fold {
					c = icFold + " " + c
				}
				chips = append(chips, fg(cMut).Render(c))
			}
			right := strings.Join(chips, "   ")
			rw := width(right)
			avail := max(8, w-pw-2-rw-min(rw, 3))
			if rw > 0 && avail < 12 { // no room: the chips get their own line
				avail = max(4, w-pw-2)
			}
			for c, ch := range wrapPlain(plain, avail) {
				p, bx := strings.Repeat(" ", pw), -1
				if c == 0 {
					p, bx = prefix, boxAt
				}
				row := p + paths.Style(ch, textSt, linkStyle(textSt))
				if c == 0 && rw > 0 && width(row)+rw+3 <= w-1 {
					row += strings.Repeat(" ", w-1-width(row)-rw) + right
					right = ""
				}
				add(row, i, bx)
			}
			if right != "" {
				add(strings.Repeat(" ", max(pw, w-1-rw))+right, i, -1)
			}
		case isArchive(s):
			n := 0
			for j := i + 1; j < len(d.lines); j++ {
				if d.isTodo(j) && d.parentOf(j) < 0 {
					n++
				}
			}
			add(mark+fg(cMut).Render(icArchive+"  Archive  ")+fg(cDim).Render(strings.Repeat("─", max(0, w-30))+fmt.Sprintf(" %d", n)), i, -1)
		case isHeading(s):
			hm := headingRe.FindStringSubmatch(s)
			add(mark+fg(cPurple).Bold(true).Render(icHeading+"  "+hm[2]), i, -1)
		default:
			text := strings.TrimSpace(s)
			if strings.HasPrefix(text, "- ") || strings.HasPrefix(text, "* ") {
				text = "• " + text[2:]
			}
			st := fg(cHint)
			if archived {
				st = fg(cMut)
			}
			if m.edit != nil && m.edit.line == i {
				markEdit(width(mark)+shown(s), w-6-shown(s))
				add(mark+strings.Repeat(" ", shown(s))+m.edit.p.field(w-6-shown(s), fg(cTxt)), i, -1)
				break
			}
			for c, ch := range wrapPlain(text, max(4, w-6-shown(s))) {
				lead := mark
				if c > 0 {
					lead = "    "
				}
				add(lead+strings.Repeat(" ", shown(s))+paths.Style(ch, st, linkStyle(st)), i, -1)
			}
		}
		if on {
			curStart, curEnd = start, len(out)-1
		}
	}
	// scroll so the cursor's lines stay in view (the title scrolls too)
	off := min(scrollOff, (h-1)/2)
	if curStart >= 0 {
		if curStart < m.top+off {
			m.top = max(0, curStart-off)
		}
		if curEnd > m.top+h-1-off {
			m.top = curEnd - h + 1 + off
		}
	}
	m.top = max(0, min(m.top, len(out)-h))
	if editAt >= m.top && editAt < m.top+h {
		m.editY, m.editX = m.bodyY+editAt-m.top, editCol
	}
	view := make([]string, h)
	m.rightRows, m.boxX = make([]int, h), make([]int, h)
	for r := 0; r < h; r++ {
		m.rightRows[r], m.boxX[r] = -1, -1
		if i := m.top + r; i < len(out) {
			view[r] = out[i]
			m.rightRows[r], m.boxX[r] = rowOf[i], boxOf[i]
		}
	}
	return view
}

// ---------------------------------------------------------------------------
// status bar, help
// ---------------------------------------------------------------------------

func (m *model) statusBar() string {
	mode, mc := " BOARD ", cAcc
	switch m.page() {
	case "todos":
		mode, mc = " TODOS ", cGreen
	case "note":
		mode, mc = " NOTE ", cPurple
	case "today":
		mode, mc = " TODAY ", cYellow
	}
	if m.edit != nil {
		mode, mc = " TYPE ", cPurple
	}
	left := " " + lipgloss.NewStyle().Background(mc).Foreground(cBg).Bold(true).Render(mode)
	if d := m.current(); d != nil && m.tab != "today" {
		rel := strings.TrimPrefix(strings.TrimPrefix(d.path, cfg.notebook), string(os.PathSeparator))
		left += "  " + fg(cHint).Render(rel)
	}
	if m.pending != "" {
		left += "  " + fg(cYellow).Render(m.pending+"…")
	}
	right := ""
	switch {
	case m.note != "" && m.noteErr:
		right = fg(cRed).Render(m.note)
	case m.note != "":
		right = fg(cGreen).Render(m.note)
	case len(m.reg) > 0 && m.focus == 1:
		right = fg(cYellow).Render(fmt.Sprintf("%d line(s) to paste · p", len(m.reg)))
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
		body = append(body, "    "+pad(fg(cTxt).Render(k.keys), 24)+fg(cHint).Render(k.desc))
	}
	h := m.h - 3
	m.helpScroll = max(0, min(m.helpScroll, len(body)-h))
	lines := []string{spread(" "+fg(cAcc).Render(icBook)+" "+fg(cAcc).Bold(true).Render("Notes keys"), fg(cMut).Render("Dotes "+app.Version)+" ", m.w),
		fg(cDim).Render(strings.Repeat("─", m.w))}
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

// linkStyle: how a path that exists looks inside text drawn with st
func linkStyle(st lipgloss.Style) lipgloss.Style { return st.Foreground(cAcc).Underline(true) }
