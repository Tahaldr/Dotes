package notes

// The board (todo lists and notes as cards), a note's reading page, and the details panel
// next to an open todo list.

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

const (
	cardMinW = 30
	cardMaxW = 46
	cardH    = 8 // borders included
	gapX     = 2
	gridTop  = 3 // body rows above the cards
)

type cardRect struct{ x, y, w, h, slot int }

// cardData: what a card shows, read once per change of the file
type cardData struct {
	mod   time.Time
	w     int
	lines []string // already drawn, without the card's sides
}

// ---------------------------------------------------------------------------
// the board
// ---------------------------------------------------------------------------

func (m *model) board(w, h int) []string {
	out := make([]string, h)
	// the title row, with a summary of everything in this tab
	if m.tab == "notes" {
		out[1] = "  " + fg(cPurple).Render(icNotes) + "  " + fg(cTxt).Bold(true).Render("Notes") + "   " +
			fg(cMut).Render(plural(m.count("notes"), "note"))
	} else {
		done, all := 0, 0
		for _, d := range m.docs {
			if d.kind == "todos" {
				done, all = done+d.done, all+d.total
			}
		}
		sum := plural(m.count("todos"), "list") + fmt.Sprintf(" · %d of %d done", done, all)
		if n := m.dueSoon(); n > 0 {
			sum += fg(cYellow).Render(fmt.Sprintf(" · %d due", n))
		}
		out[1] = "  " + fg(cGreen).Render(icTodos) + "  " + fg(cTxt).Bold(true).Render("Todo lists") + "   " + fg(cMut).Render(sum)
	}

	avail := w - 4
	cols := max(1, min((avail+gapX)/(cardMinW+gapX), len(m.slots)))
	cw := min(cardMaxW, (avail-gapX*(cols-1))/cols)
	if cols == 1 {
		cw = min(avail, cardMaxW+20)
	}
	m.cols = cols
	nrows := (len(m.slots) + cols - 1) / cols
	vis := max(1, (h-gridTop+1)/(cardH+1))
	selRow := m.sl / cols
	if selRow < m.ltop {
		m.ltop = selRow
	}
	if selRow >= m.ltop+vis {
		m.ltop = selRow - vis + 1
	}
	m.ltop = max(0, min(m.ltop, nrows-vis))
	for r := 0; m.ltop+r < nrows; r++ {
		y := gridTop + r*(cardH+1)
		if y >= h {
			break
		}
		for c := 0; c < cols; c++ {
			i := (m.ltop+r)*cols + c
			if i >= len(m.slots) {
				break
			}
			x := 2 + c*(cw+gapX)
			for k, l := range m.card(i, cw) {
				if y+k < h {
					out[y+k] = pad(out[y+k], x) + l
				}
			}
			m.cards = append(m.cards, cardRect{x, m.bodyY + y, cw, cardH, i})
		}
	}
	if left := len(m.slots) - (m.ltop+vis)*cols; left > 0 { // more below, said on the title row
		more := fg(cMut).Render(fmt.Sprintf("↓ %d more ", left))
		out[1] = spread(out[1], more, w-2)
	}
	return out
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// card draws slot i as a box cw wide
func (m *model) card(i, cw int) []string {
	s := m.slots[i]
	on := i == m.sl
	bc, tst := fg(cDim), fg(cTxt)
	if on {
		bc, tst = fg(cAcc), fg(cAcc).Bold(true)
	}
	inner := cw - 4
	out := make([]string, cardH)
	if s.doc < 0 { // the "new" card, dashed
		what := "New todo list"
		if s.kind == "notes" {
			what = "New note"
		}
		out[0] = bc.Render("╭" + strings.Repeat("┄", cw-2) + "╮")
		for k := 1; k < cardH-1; k++ {
			out[k] = bc.Render("┊") + strings.Repeat(" ", cw-2) + bc.Render("┊")
		}
		mid := (cardH - 1) / 2
		label := tst.Render("+ " + what)
		out[mid] = bc.Render("┊") + pad(strings.Repeat(" ", center(cw-2, width(label)))+label, cw-2) + bc.Render("┊")
		keyL := fg(cMut).Render("a")
		out[mid+1] = bc.Render("┊") + pad(strings.Repeat(" ", center(cw-2, 1))+keyL, cw-2) + bc.Render("┊")
		out[cardH-1] = bc.Render("╰" + strings.Repeat("┄", cw-2) + "╯")
		return out
	}
	d := m.docs[s.doc]
	icon, ic := icTodos, fg(cGreen)
	badge := ""
	if d.kind == "notes" {
		icon, ic = icNotes, fg(cPurple)
		badge = fg(cMut).Render(ago(d.mod))
	} else if late, soon := m.dueIn(d.path); late+soon > 0 {
		st := fg(cYellow)
		if late > 0 {
			st = fg(cRed)
		}
		badge = st.Render(fmt.Sprintf("%s %d", icCalendar, late+soon))
	}
	if !on {
		ic = fg(cHint)
	}
	// ╭─ 󰄲 Title ───────── badge ─╮
	tail := bc.Render("─╮")
	if badge != "" {
		tail = " " + badge + " " + bc.Render("─╮")
	}
	title := trunc(d.title, max(4, cw-6-width(tail)-3))
	head := bc.Render("╭─ ") + ic.Render(icon) + " " + tst.Render(title) + " "
	out[0] = head + bc.Render(strings.Repeat("─", max(0, cw-width(head)-width(tail)))) + tail
	body := m.cardBody(d, inner)
	for k := 1; k < cardH-1; k++ {
		l := ""
		if k-1 < len(body) {
			l = body[k-1]
		}
		out[k] = bc.Render("│") + " " + pad(l, inner) + " " + bc.Render("│")
	}
	out[cardH-1] = bc.Render("╰" + strings.Repeat("─", cw-2) + "╯")
	return out
}

// dueIn: how many todos in the list at path are late, and due today
func (m *model) dueIn(path string) (late, soon int) {
	for _, it := range m.agenda {
		if strings.EqualFold(it.path, path) {
			switch it.group {
			case 0:
				late++
			case 1:
				soon++
			}
		}
	}
	return
}

// cardBody: a todo list's progress and first todos, or the start of a note
func (m *model) cardBody(d docInfo, w int) []string {
	if m.cardCache == nil {
		m.cardCache = map[string]*cardData{}
	}
	key := strings.ToLower(d.path)
	if c := m.cardCache[key]; c != nil && c.mod.Equal(d.mod) && c.w == w {
		return c.lines
	}
	var lines []string
	if d.kind == "notes" {
		lines = noteSnippet(d.path, w, cardH-2)
	} else {
		lines = todoSnippet(d, w, cardH-2)
	}
	m.cardCache[key] = &cardData{d.mod, w, lines}
	return lines
}

func todoSnippet(info docInfo, w, n int) []string {
	count := fmt.Sprintf("%d/%d", info.done, info.total)
	lines := []string{bar(info.done, info.total, max(4, w-width(count)-2)) + "  " + fg(cMut).Render(count)}
	d, err := loadDoc(info.path)
	if err != nil {
		return append(lines, fg(cRed).Render("can't read it"))
	}
	var open, done []string
	now := today()
	for i := 0; i < d.archiveStart(); i++ {
		t, ok := parseTodo(d.lines[i].s)
		if !ok || d.parentOf(i) >= 0 {
			continue
		}
		plain, prio, due := splitTags(t.text)
		if t.done {
			done = append(done, fg(cGreen).Render(icBoxDone)+" "+fg(cMut).Strikethrough(true).Render(trunc(plain, w-2)))
			continue
		}
		box := fg(cHint)
		if st, ok := prioColor[prio]; ok {
			box = st
		}
		right := ""
		if due != "" {
			l, st := dueLabel(due, false, now)
			right = st.Render(l)
		}
		left := box.Render(icBox) + " " + fg(cTxt).Render(trunc(plain, max(4, w-2-width(right)-2)))
		if right != "" {
			left += strings.Repeat(" ", max(1, w-width(left)-width(right))) + right
		}
		open = append(open, left)
	}
	items := append(open, done...)
	room := n - 1
	if len(items) == 0 {
		return append(lines, "", fg(cMut).Render("empty · enter, then o"))
	}
	if len(items) > room {
		more := len(items) - room + 1
		items = append(items[:room-1], fg(cMut).Render(fmt.Sprintf("+ %d more", more)))
	}
	return append(lines, items...)
}

func noteSnippet(path string, w, n int) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return []string{fg(cRed).Render("can't read it")}
	}
	ls, _, _ := splitLines(string(b))
	if len(ls) > 0 && strings.HasPrefix(ls[0], "# ") {
		ls = ls[1:]
	}
	text := strings.TrimSpace(strings.Join(ls, "\n"))
	if text == "" {
		return []string{fg(cMut).Render("empty")}
	}
	var out []string
	for _, l := range renderMarkdown(text, w) {
		if len(out) == 0 && strings.TrimSpace(ansi.Strip(l)) == "" {
			continue
		}
		out = append(out, trunc(l, w))
		if len(out) == n {
			break
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// a note, to read (enter opens it in the editor); an outline on the right when there's room
// ---------------------------------------------------------------------------

func (m *model) notePage(w, h int) []string {
	d := m.current()
	out := make([]string, h)
	m.rightRows = make([]int, h)
	for r := range m.rightRows {
		m.rightRows[r] = -1
	}
	m.rightRows[0] = -2 // the way back
	area, side := w, 0
	heads := noteHeadings(d.path)
	if w >= 120 && len(heads) > 1 {
		side = detailsW
		area = w - side - 1
	}
	pw := min(area-4, 96)
	x := center(area, pw)
	c := &m.ncache
	if c.path != d.path || !c.mod.Equal(d.mod) || c.w != pw {
		b, err := os.ReadFile(d.path)
		c.path, c.mod, c.w = d.path, d.mod, pw
		if err != nil {
			c.out = []string{fg(cRed).Render("can't read it: " + err.Error())}
		} else {
			ls, _, _ := splitLines(string(b))
			if len(ls) > 0 && strings.HasPrefix(ls[0], "# ") { // the title is drawn above
				ls = ls[1:]
			}
			text := strings.Join(ls, "\n")
			c.out = renderMarkdown(text, pw-4)
			if strings.TrimSpace(text) == "" {
				c.out = []string{fg(cMut).Render("empty · enter opens it in your editor")}
			}
		}
	}
	when := "edited " + ago(d.mod) + " ago"
	if ago(d.mod) == "now" {
		when = "edited just now"
	}
	head := paneTitle("Notes", icNotes, fg(cPurple), d.title, fg(cMut).Render(fmt.Sprintf("%s · %d lines · enter edits", when, d.lines)))
	body := c.out
	m.nscroll = max(0, min(m.nscroll, len(body)-(h-len(head))+1))
	margin := strings.Repeat(" ", x)
	r := 0
	for ; r < len(head) && r < h; r++ {
		out[r] = margin + head[r]
	}
	for i := m.nscroll; r < h && i < len(body); i, r = i+1, r+1 {
		out[r] = margin + "  " + trunc(body[i], pw-3)
	}
	if side > 0 {
		panel := []string{"", fg(cPurple).Bold(true).Render("OUTLINE"), ""}
		for _, hd := range heads {
			lv := min(3, hd.level)
			st := fg(cHint)
			if lv == 1 {
				st = fg(cTxt).Bold(true)
			}
			panel = append(panel, strings.Repeat("  ", lv-1)+st.Render(trunc(hd.text, side-4-2*lv)))
		}
		for r := range out {
			l := ""
			if r < len(panel) {
				l = panel[r]
			}
			out[r] = pad(out[r], area) + fg(cDim).Render("│") + "  " + l
		}
	}
	return out
}

type heading struct {
	level int
	text  string
}

func noteHeadings(path string) []heading {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []heading
	fence := false
	ls, _, _ := splitLines(string(b))
	if len(ls) > 0 && strings.HasPrefix(ls[0], "# ") { // the title
		ls = ls[1:]
	}
	for _, l := range ls {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
		}
		if hm := headingRe.FindStringSubmatch(l); !fence && hm != nil {
			out = append(out, heading{len(hm[1]), hm[2]})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// the details panel: everything about the todo under the cursor, and the list
// ---------------------------------------------------------------------------

func (m *model) details(w, h int) []string {
	d := m.doc
	now := today()
	out := []string{""}
	section := func(name string) { out = append(out, fg(cAcc).Bold(true).Render(name), "") }
	row := func(icon string, ist, tst lipgloss.Style, text string) {
		out = append(out, ist.Render(icon)+"  "+tst.Render(trunc(text, w-3)))
	}
	if d.isTodo(m.cur) {
		t := d.todoAt(m.cur)
		plain, prio, due := splitTags(t.text)
		section("THIS TODO")
		for _, l := range wrapPlain(plain, w) {
			out = append(out, fg(cTxt).Bold(true).Render(l))
		}
		out = append(out, "")
		if t.done {
			row(icBoxDone, fg(cGreen), fg(cHint), "done")
		} else {
			row(icBox, fg(cHint), fg(cHint), "open")
		}
		if prio != "" {
			row("●", prioColor[prio], fg(cHint), prio+" priority")
		} else {
			row("○", fg(cMut), fg(cMut), "no priority  (!)")
		}
		if due != "" {
			l, st := dueLabel(due, t.done, now)
			date := due
			if dt, ok := parseDate(due); ok {
				date = dt.Format("Mon, Jan 2")
			}
			row(icCalendar, st, fg(cHint), l+" · "+date)
		} else {
			row(icCalendar, fg(cMut), fg(cMut), "no due date  (t)")
		}
		if d.hasSubs(m.cur) {
			sd, sa := d.subCount(m.cur)
			row("󰘍", fg(cHint), fg(cHint), fmt.Sprintf("%d of %d sub-todos done", sd, sa))
			out = append(out, bar(sd, sa, min(w, 24)))
		}
		if p := d.parentOf(m.cur); p >= 0 {
			pt, _, _ := splitTags(d.todoAt(p).text)
			row("󰁝", fg(cMut), fg(cMut), "under "+pt)
		}
		out = append(out, "", "")
	}
	section("THIS LIST")
	done, all := progress(d.texts())
	out = append(out, bar(done, all, min(w, 24))+"  "+fg(cMut).Render(fmt.Sprintf("%d/%d", done, all)))
	if info := m.current(); info != nil {
		late, soon := m.dueIn(info.path)
		if late > 0 {
			row(icCalendar, fg(cRed), fg(cHint), plural(late, "late todo"))
		}
		if soon > 0 {
			row(icCalendar, fg(cYellow), fg(cHint), fmt.Sprintf("%d due today", soon))
		}
		row("󰥔", fg(cMut), fg(cMut), "edited "+ago(info.mod)+map[bool]string{true: "", false: " ago"}[ago(info.mod) == "now"])
	}
	res := make([]string, h)
	copy(res, out)
	return res
}
