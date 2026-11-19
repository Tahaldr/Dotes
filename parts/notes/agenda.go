package notes

// Due dates and the Today tab. A todo gets a date with t (stored as @due(2026-11-20), so the
// file stays plain Markdown), and the Today tab gathers what's late, due today and due in
// the next 7 days from every list.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const dateFmt = "2006-01-02"

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

func today() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.Local)
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.ParseInLocation(dateFmt, strings.TrimSpace(s), time.Local)
	return t, err == nil
}

// parseDueInput understands: today, tomorrow, mon…sun (the next one), +3 (days), +2w (weeks),
// 11-20 (this year, or next if it's past) and 2026-11-20. "" means no date.
func parseDueInput(in string, now time.Time) (string, error) {
	s := strings.ToLower(strings.TrimSpace(in))
	switch {
	case s == "":
		return "", nil
	case s == "today" || s == "tod" || s == "0":
		return now.Format(dateFmt), nil
	case s == "tomorrow" || s == "tom" || s == "tmr":
		return now.AddDate(0, 0, 1).Format(dateFmt), nil
	case strings.HasPrefix(s, "+"):
		n, unit := s[1:], 1
		if strings.HasSuffix(n, "w") {
			n, unit = strings.TrimSuffix(n, "w"), 7
		} else {
			n = strings.TrimSuffix(n, "d")
		}
		k, err := strconv.Atoi(n)
		if err != nil || k < 0 || k > 3650 {
			break
		}
		return now.AddDate(0, 0, k*unit).Format(dateFmt), nil
	}
	if len(s) >= 3 {
		if wd, ok := weekdays[s[:3]]; ok {
			days := (int(wd) - int(now.Weekday()) + 7) % 7
			if days == 0 {
				days = 7
			}
			return now.AddDate(0, 0, days).Format(dateFmt), nil
		}
	}
	if t, ok := parseDate(s); ok {
		return t.Format(dateFmt), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", fmt.Sprintf("%d-%s", now.Year(), s), time.Local); err == nil {
		if t.Before(now) {
			t = t.AddDate(1, 0, 0)
		}
		return t.Format(dateFmt), nil
	}
	return "", fmt.Errorf("%q isn't a date: try today, tomorrow, fri, +3, +2w, 11-20 or 2026-11-20", in)
}

// dueLabel: "3d late", "today", "tomorrow", "Fri", "Nov 20", and its color
func dueLabel(due string, done bool, now time.Time) (string, lipgloss.Style) {
	t, ok := parseDate(due)
	if !ok {
		return due, fg(cMut)
	}
	days := int(t.Sub(now).Hours() / 24)
	if t.Before(now) && days == 0 {
		days = -1
	}
	label, st := t.Format("Jan 2"), fg(cMut)
	switch {
	case days < 0:
		label, st = fmt.Sprintf("%dd late", -days), fg(cRed)
	case days == 0:
		label, st = "today", fg(cYellow)
	case days == 1:
		label, st = "tomorrow", fg(cAcc)
	case days < 7:
		label, st = t.Format("Mon"), fg(cHint)
	}
	if t.Year() != now.Year() && days >= 7 {
		label = t.Format("Jan 2 2006")
	}
	if done {
		st = fg(cMut)
	}
	return label, st
}

// ---------------------------------------------------------------------------
// the Today tab
// ---------------------------------------------------------------------------

type agendaItem struct {
	path, list string
	line       int
	raw, text  string
	prio, due  string
	group      int // 0 late, 1 today, 2 next 7 days
}

var agendaGroups = []string{"Late", "Today", "Next 7 days"}

// buildAgenda reads every todo list for open todos due within a week (or late)
func (m *model) buildAgenda() {
	now := today()
	m.agenda = nil
	for _, info := range m.docs {
		if info.kind != "todos" {
			continue
		}
		d, err := loadDoc(info.path)
		if err != nil {
			continue
		}
		a := d.archiveStart()
		for i := 0; i < a; i++ {
			t, ok := parseTodo(d.lines[i].s)
			if !ok || t.done {
				continue
			}
			plain, prio, due := splitTags(t.text)
			dt, ok := parseDate(due)
			if !ok {
				continue
			}
			g := 2
			switch {
			case dt.Before(now):
				g = 0
			case dt.Equal(now):
				g = 1
			case dt.After(now.AddDate(0, 0, 7)):
				continue
			}
			m.agenda = append(m.agenda, agendaItem{info.path, info.title, i, d.lines[i].s, plain, prio, due, g})
		}
	}
	sort.SliceStable(m.agenda, func(i, j int) bool {
		a, b := m.agenda[i], m.agenda[j]
		if a.due != b.due {
			return a.due < b.due
		}
		return prioRank("- [ ] @priority("+a.prio+")") < prioRank("- [ ] @priority("+b.prio+")")
	})
	m.acur = max(0, min(m.acur, len(m.agenda)-1))
}

// dueSoon: how many are late or due today (shown on the tab)
func (m *model) dueSoon() int {
	n := 0
	for _, it := range m.agenda {
		if it.group < 2 {
			n++
		}
	}
	return n
}

// agendaView: a timeline down the page, one stop per day (all the late ones share the first)
func (m *model) agendaView(fullW, h int) []string {
	var out []string
	rowItem := []int{}
	curLine := 0
	add := func(s string, item int) { out = append(out, s); rowItem = append(rowItem, item) }
	w := min(fullW-4, 110)
	margin := strings.Repeat(" ", center(fullW, w))
	counts := [3]int{}
	for _, it := range m.agenda {
		counts[it.group]++
	}
	sum := "nothing late or due this week"
	if len(m.agenda) > 0 {
		var parts []string
		for g, n := range counts {
			if n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, strings.ToLower(agendaGroups[g])))
			}
		}
		sum = strings.Join(parts, " · ") + " · from every todo list"
	}
	now := today()
	add("", -1)
	add("  "+fg(cYellow).Render(icCalendar)+"  "+fg(cTxt).Bold(true).Render("Today")+"   "+fg(cMut).Render(now.Format("Monday, January 2")), -1)
	add("  "+fg(cMut).Render(sum), -1)
	add("", -1)
	if len(m.agenda) == 0 {
		add("     "+fg(cGreen).Render(icBoxDone)+"  "+fg(cTxt).Render("All clear."), -1)
		add("", -1)
		add("     "+fg(cMut).Render("In a todo list, t gives a todo a due date: today, tomorrow, fri, +3, 11-20…"), -1)
		add("     "+fg(cMut).Render("Late ones, today's and the next 7 days' show up here, on a timeline."), -1)
	}
	rail := fg(cDim).Render("│")
	stop := ""
	for i, it := range m.agenda {
		key := it.due
		if it.group == 0 {
			key = "late"
		}
		if key != stop {
			if stop != "" {
				add("    "+rail, -1)
			}
			stop = key
			node, col, label, sub := "○", fg(cAcc), "", ""
			switch it.group {
			case 0:
				node, col, label = "●", fg(cRed), "LATE"
			case 1:
				node, col, label = "●", fg(cYellow), "TODAY"
			default:
				dt, _ := parseDate(it.due)
				label = strings.ToUpper(dt.Format("Mon 2"))
				if l, _ := dueLabel(it.due, false, now); l == "tomorrow" {
					sub = "tomorrow"
				}
			}
			n := 0
			for _, o := range m.agenda {
				if (key == "late" && o.group == 0) || (key != "late" && o.due == key) {
					n++
				}
			}
			head := "    " + col.Render(node) + "  " + col.Bold(true).Render(label) + "  " + fg(cMut).Render(fmt.Sprint(n))
			if sub != "" {
				head += "  " + fg(cHint).Render(sub)
			}
			head += " "
			add(head+fg(cDim).Render(strings.Repeat("─", max(0, w-width(head)-1))), -1)
		}
		on := i == m.acur
		mark, textSt := "   ", fg(cTxt)
		if on {
			mark, textSt = fg(cAcc).Render(" ❯ "), fg(cAcc).Bold(true)
			curLine = len(out)
		}
		right := ""
		if it.prio != "" {
			right += prioChip(it.prio) + "   "
		}
		right += fg(cMut).Render(icTodos + " " + trunc(it.list, 24))
		if it.group == 0 {
			l, lst := dueLabel(it.due, false, now)
			right += "   " + lst.Render(pad(l, 8))
		}
		left := "    " + rail + mark + fg(cHint).Render(icBox) + "  "
		room := max(4, w-width(left)-width(right)-3)
		text := textSt.Render(trunc(it.text, room))
		add(left+text+strings.Repeat(" ", max(2, w-width(left)-width(text)-width(right)-1))+right, i)
	}
	if len(m.agenda) > 0 {
		add("    "+fg(cDim).Render("╵"), -1)
	}
	// scroll
	off := min(scrollOff, (h-1)/2)
	if curLine < m.top+off {
		m.top = max(0, curLine-off)
	}
	if curLine > m.top+h-1-off {
		m.top = curLine - h + 1 + off
	}
	m.top = max(0, min(m.top, len(out)-h))
	view := make([]string, h)
	m.agendaRows = make([]int, h)
	for r := 0; r < h; r++ {
		m.agendaRows[r] = -1
		if i := m.top + r; i < len(out) {
			view[r], m.agendaRows[r] = margin+out[i], rowItem[i]
		}
	}
	return view
}

// agendaDoc opens the item's list and checks the line is still the same todo
func (m *model) agendaDoc(it agendaItem) (*todoDoc, bool) {
	d, err := loadDoc(it.path)
	if err != nil || it.line >= len(d.lines) || d.lines[it.line].s != it.raw {
		m.setNote("that list changed; the Today tab is fresh again", true)
		m.buildAgenda()
		return nil, false
	}
	return d, true
}

func (m *model) agendaKey(s string) tea.Cmd {
	n := len(m.agenda)
	switch s {
	case "j", "down":
		m.acur = min(n-1, m.acur+1)
	case "k", "up":
		m.acur = max(0, m.acur-1)
	case "G", "end":
		m.acur = n - 1
	case "home":
		m.acur = 0
	case "ctrl+d", "pgdown":
		m.acur = min(n-1, m.acur+m.half())
	case "ctrl+u", "pgup":
		m.acur = max(0, m.acur-m.half())
	}
	m.acur = max(0, m.acur)
	if n == 0 {
		return nil
	}
	it := m.agenda[m.acur]
	switch s {
	case "x", "space", "enter":
		if s == "enter" {
			m.goTo(it.path, it.line)
			return nil
		}
		if d, ok := m.agendaDoc(it); ok {
			d.toggle(it.line)
			m.saveOther(d)
			m.setNote("done: "+it.text, false)
		}
	case "t":
		m.askDue(it.text, func(m *model, date string) {
			if d, ok := m.agendaDoc(it); ok {
				d.setDue(it.line, date)
				m.saveOther(d)
			}
		})
	}
	return nil
}

// saveOther saves a list that isn't (necessarily) the open one, then refreshes what shows it
func (m *model) saveOther(d *todoDoc) {
	if err := d.save(); err != nil {
		m.setNote("couldn't save: "+err.Error(), true)
	}
	if m.doc != nil && strings.EqualFold(m.doc.path, d.path) {
		m.reloadDoc()
	}
	m.refresh()
	m.buildAgenda()
}

// askDue asks for a date (today, fri, +3…); onDate gets "" to remove it
func (m *model) askDue(what string, onDate func(m *model, date string)) {
	m.input("Due date", "for \""+trunc(what, 40)+"\": today, tomorrow, fri, +3, +2w, 11-20 or 2026-11-20 · empty removes it", "", func(m *model, s string) tea.Cmd {
		date, err := parseDueInput(s, today())
		if err != nil {
			m.setNote(err.Error(), true)
			return nil
		}
		onDate(m, date)
		if date == "" {
			m.setNote("due date removed", false)
		} else {
			l, _ := dueLabel(date, false, today())
			m.setNote("due "+l+" ("+date+")", false)
		}
		return nil
	})
}

// goTo opens a todo list on line (unfolding what hides it)
func (m *model) goTo(path string, line int) {
	m.tab = "todos"
	m.reloadDocs(path)
	m.loadCurrent()
	if m.doc == nil || !strings.EqualFold(m.doc.path, path) {
		return
	}
	m.focus = 1
	if line < len(m.doc.lines) {
		for p := m.doc.parentOf(line); p >= 0; p = m.doc.parentOf(p) {
			m.doc.lines[p].fold = false
		}
		m.cur = line
	}
	m.fixCur()
}
