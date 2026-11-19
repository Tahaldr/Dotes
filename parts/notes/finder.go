package notes

// / opens the finder: type to search every todo list and note (titles and every line), enter
// jumps there. And a small menu popup (m: move a todo to another list).

import (
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type hit struct {
	kind, path, title string
	line              int // -1 = the document itself
	text              string
	done              bool
}

type finderState struct {
	p      *prompt
	corpus []hit
	hits   []hit
	cur    int
	top    int
}

func (m *model) openFinder() {
	f := &finderState{p: newPrompt("", -1)}
	for _, d := range m.docs {
		f.corpus = append(f.corpus, hit{d.kind, d.path, d.title, -1, d.title, false})
		b, err := os.ReadFile(d.path)
		if err != nil {
			continue
		}
		ls, _, _ := splitLines(string(b))
		for i, s := range ls {
			if blank(s) || (i == 0 && strings.HasPrefix(s, "# ")) {
				continue
			}
			h := hit{d.kind, d.path, d.title, i, strings.TrimSpace(s), false}
			if t, ok := parseTodo(s); ok {
				h.text, _, _ = splitTags(t.text)
				h.done = t.done
			} else if hm := headingRe.FindStringSubmatch(s); hm != nil {
				h.text = hm[2]
			}
			f.corpus = append(f.corpus, h)
		}
	}
	m.finder = f
	m.findMatch()
}

// findMatch: every word typed must be in the line (any case); nothing typed = the documents
func (m *model) findMatch() {
	f := m.finder
	q := strings.Fields(strings.ToLower(string(f.p.text)))
	f.hits = nil
	for _, h := range f.corpus {
		if len(q) == 0 {
			if h.line < 0 {
				f.hits = append(f.hits, h)
			}
			continue
		}
		low := strings.ToLower(h.text)
		ok := true
		for _, w := range q {
			if !strings.Contains(low, w) {
				ok = false
				break
			}
		}
		if ok {
			f.hits = append(f.hits, h)
		}
		if len(f.hits) >= 500 {
			break
		}
	}
	f.cur, f.top = 0, 0
}

func (m *model) finderKey(k tea.KeyPressMsg) tea.Cmd {
	f := m.finder
	switch k.String() {
	case "down", "ctrl+n", "ctrl+j", "tab":
		f.cur = min(len(f.hits)-1, f.cur+1)
		return nil
	case "up", "ctrl+p", "ctrl+k", "shift+tab":
		f.cur = max(0, f.cur-1)
		return nil
	case "pgdown":
		f.cur = min(len(f.hits)-1, f.cur+10)
		return nil
	case "pgup":
		f.cur = max(0, f.cur-10)
		return nil
	}
	before := string(f.p.text)
	done, how := f.p.editKey(k)
	if !done {
		if string(f.p.text) != before {
			m.findMatch()
		}
		return nil
	}
	m.finder = nil
	if how != "enter" || len(f.hits) == 0 {
		return nil
	}
	return m.openHit(f.hits[max(0, f.cur)])
}

func (m *model) openHit(h hit) tea.Cmd {
	switch {
	case h.kind == "todos" && h.line >= 0:
		m.goTo(h.path, h.line)
	case h.kind == "notes" && h.line >= 0:
		m.tab = "notes"
		m.reloadDocs(h.path)
		m.loadCurrent()
		return m.editFile(h.path, h.line+1)
	default:
		m.tab = h.kind
		m.focus = 0
		m.reloadDocs(h.path)
		m.loadCurrent()
	}
	return nil
}

func (m *model) finderPopup(lines []string) {
	f := m.finder
	if f == nil {
		return
	}
	w := min(m.w-4, max(60, m.w*70/100))
	inner := w - 4
	listH := max(3, m.h-10)
	body := []string{fg(cAcc).Bold(true).Render(icSearch+" ") + f.p.field(inner-3, fg(cTxt)), fg(cDim).Render(strings.Repeat("─", inner))}
	if len(f.hits) == 0 {
		body = append(body, fg(cMut).Render("nothing matches"))
	}
	if f.cur < f.top {
		f.top = f.cur
	}
	if f.cur >= f.top+listH {
		f.top = f.cur - listH + 1
	}
	for i := f.top; i < len(f.hits) && i < f.top+listH; i++ {
		h := f.hits[i]
		on := i == f.cur
		mark, st := "  ", fg(cTxt)
		if on {
			mark, st = fg(cAcc).Render("❯ "), fg(cAcc).Bold(true)
		}
		ic, icSt := icNotes, fg(cPurple)
		switch {
		case h.line < 0 && h.kind == "todos":
			ic, icSt = icTodos, fg(cGreen)
		case h.kind == "todos" && h.done:
			ic, icSt, st = icBoxDone, fg(cGreen), fg(cMut).Strikethrough(true)
		case h.kind == "todos" && h.line >= 0:
			ic, icSt = icBox, fg(cHint)
		case h.line >= 0:
			ic, icSt = "·", fg(cMut)
		}
		right := ""
		if h.line >= 0 {
			right = fg(cMut).Render(trunc(h.title, 22))
		} else {
			right = fg(cMut).Render(kindOne[h.kind])
		}
		left := mark + icSt.Render(ic) + " "
		text := st.Render(trunc(h.text, max(4, inner-width(left)-width(right)-2)))
		body = append(body, left+text+strings.Repeat(" ", max(1, inner-width(left)-width(text)-width(right)))+right)
	}
	for len(body) < listH+2 {
		body = append(body, "")
	}
	count := strconv.Itoa(len(f.hits))
	if len(f.hits) >= 500 {
		count = "500+"
	}
	body = append(body, fg(cDim).Render(strings.Repeat("─", inner)),
		spread(keyHints("enter", "go there", "↑/↓", "move", "esc", "close"), fg(cMut).Render(count+" found"), inner))
	box := popupBox(fg(cAcc).Bold(true).Render(icSearch+" Find in all todos and notes"), w, body, fg(cAcc))
	put(lines, (m.w-w)/2, 1, box, w)
}

// ---------------------------------------------------------------------------
// a small menu popup
// ---------------------------------------------------------------------------

type menuItem struct {
	name, hint string
	do         func(m *model) tea.Cmd
}

type menuState struct {
	title string
	items []menuItem
	cur   int
}

func (m *model) menuKey(s string) tea.Cmd {
	mn := m.menu
	n := len(mn.items)
	switch s {
	case "esc", "q":
		m.menu = nil
	case "j", "down", "tab":
		mn.cur = (mn.cur + 1) % n
	case "k", "up", "shift+tab":
		mn.cur = (mn.cur - 1 + n) % n
	case "enter", "l":
		m.menu = nil
		return mn.items[mn.cur].do(m)
	default:
		if i, err := strconv.Atoi(s); err == nil && i >= 1 && i <= n {
			m.menu = nil
			return mn.items[i-1].do(m)
		}
	}
	return nil
}

func (m *model) menuPopup(lines []string) {
	mn := m.menu
	if mn == nil {
		return
	}
	w := m.popupWidth(44, 64)
	inner := w - 4
	var body []string
	for i, it := range mn.items {
		mark, st := "  ", fg(cTxt)
		if i == mn.cur {
			mark, st = fg(cAcc).Render("❯ "), fg(cAcc).Bold(true)
		}
		num := " "
		if i < 9 {
			num = strconv.Itoa(i + 1)
		}
		row := mark + fg(cHint).Render(num+" ") + st.Render(it.name)
		body = append(body, spread(row, fg(cMut).Render(it.hint), inner))
	}
	body = append(body, fg(cDim).Render(strings.Repeat("─", inner)), keyHints("enter/1-9", "choose", "j/k", "move", "esc", "close"))
	box := popupBox(fg(cAcc).Bold(true).Render(mn.title), w, body, fg(cAcc))
	put(lines, (m.w-w)/2, 2, box, w)
}

// moveMenu: m sends the todo (with its sub-todos) to another list
func (m *model) moveMenu() {
	d := m.doc
	if d == nil || !d.isTodo(m.cur) {
		return
	}
	text, _, _ := splitTags(d.todoAt(m.cur).text)
	var items []menuItem
	for _, info := range m.docs {
		if info.kind != "todos" || strings.EqualFold(info.path, d.path) {
			continue
		}
		target := info
		items = append(items, menuItem{target.title, fmtCount(target.done, target.total), func(m *model) tea.Cmd {
			m.moveTo(target.path, target.title)
			return nil
		}})
	}
	items = append(items, menuItem{"+ New list…", "", func(m *model) tea.Cmd {
		m.input("New todo list", "the todo moves into it", "", func(m *model, s string) tea.Cmd {
			if s == "" {
				return nil
			}
			p, err := createDoc("todos", s)
			if err != nil {
				m.setNote("couldn't create it: "+err.Error(), true)
				return nil
			}
			m.moveTo(p, s)
			return nil
		})
		return nil
	}})
	m.menu = &menuState{title: icMove + " Move \"" + trunc(text, 30) + "\" to", items: items}
}

func (m *model) moveTo(path, title string) {
	t, err := loadDoc(path)
	if err != nil {
		m.setNote("can't read "+title+": "+err.Error(), true)
		return
	}
	var moved []line
	m.change(func(d *todoDoc) bool { moved = d.takeOut(m.cur); return true })
	t.appendTodos(moved)
	if err := t.save(); err != nil {
		m.setNote("couldn't save "+title+": "+err.Error(), true)
		return
	}
	m.fixCur()
	m.refresh()
	m.setNote("moved to "+title, false)
}

func fmtCount(done, all int) string { return strconv.Itoa(done) + "/" + strconv.Itoa(all) }
