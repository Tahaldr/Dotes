package notes

// Paths in todos and notes. Type ~/ or C:\ in a todo and the folder's entries pop up under
// it (tab / shift+tab put them in, like Nvim's command line); a path that exists shows as a
// link and gf opens it in Files. Files' gn opens a project's note here.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/internal/paths"
)

const icFolder, icFile = "\U000F024B", "\U000F0214" // 󰉋 󰈔

// compState: tab is cycling through suggestions for the path that starts at start
type compState struct {
	start int
	items []string
	idx   int
	text  string // the text after the last tab (another key ends the cycling)
	pos   int
}

// suggestions for the path being typed (nil when the cursor isn't in one). sel is the one
// tab put in (-1 = none yet).
func (e *editState) suggestions() (items []string, sel int) {
	if c := e.comp; c != nil && string(e.p.text) == c.text && e.p.pos == c.pos {
		return c.items, c.idx
	}
	_, word, ok := paths.Word(e.p.text, e.p.pos)
	if !ok {
		return nil, -1
	}
	if word != e.sugWord || e.sugItems == nil {
		e.sugWord, e.sugItems = word, paths.Suggest(word, 200)
		if e.sugItems == nil {
			e.sugItems = []string{}
		}
	}
	return e.sugItems, -1
}

// complete: tab / shift+tab in a path puts the next / previous suggestion in. false = the
// cursor isn't in a path (tab indents instead).
func (e *editState) complete(fwd bool) bool {
	p := e.p
	c := e.comp
	if c == nil || string(p.text) != c.text || p.pos != c.pos {
		start, word, ok := paths.Word(p.text, p.pos)
		if !ok {
			return false
		}
		items := paths.Suggest(word, 200)
		if len(items) == 0 {
			return true // nothing to put in, but don't indent in the middle of a path
		}
		c = &compState{start: start, items: items, idx: -1}
		e.comp = c
	}
	n := len(c.items)
	if fwd {
		c.idx = (c.idx + 1) % n
	} else {
		c.idx = (c.idx - 1 + n) % n
	}
	ins := []rune(c.items[c.idx])
	p.text = append(append(append([]rune{}, p.text[:c.start]...), ins...), p.text[p.pos:]...)
	p.pos = c.start + len(ins)
	c.text, c.pos = string(p.text), p.pos
	return true
}

// suggestPopup: the folder's entries under the todo being typed
func (m *model) suggestPopup(lines []string) {
	e := m.edit
	if e == nil || m.editY < 0 || m.prompt != nil || m.question != nil {
		return
	}
	items, sel := e.suggestions()
	if len(items) == 0 {
		return
	}
	const rows = 8
	first := 0
	if sel >= rows {
		first = sel - rows + 1
	}
	last := min(len(items), first+rows)
	w := 0
	for _, it := range items[first:last] {
		w = max(w, width(filepath.Base(strings.TrimRight(it, `/\`)))+4)
	}
	w = min(max(w+4, 24), m.w-2)
	var body []string
	for i := first; i < last; i++ {
		name := filepath.Base(strings.TrimRight(items[i], `/\`))
		ic, st := fg(cHint).Render(icFile), fg(cTxt)
		if strings.HasSuffix(items[i], "/") || strings.HasSuffix(items[i], `\`) {
			ic = fg(cAcc).Render(icFolder)
			name += "/"
		}
		if i == sel {
			st = fg(cAcc).Bold(true)
		}
		body = append(body, ic+" "+st.Render(trunc(name, w-6)))
	}
	more := keyHints("tab", "next", "shift+tab", "back")
	if n := strconv.Itoa(len(items)); sel >= 0 {
		more = fg(cMut).Render(strconv.Itoa(sel+1)+"/"+n) + "  " + keyHints("tab", "next", "enter", "keep")
	} else if len(items) > last-first {
		more = fg(cMut).Render(n) + "  " + more
	}
	body = append(body, more)
	box := popupBox("", w, body, fg(cDim))
	y := m.editY + 1
	if y+len(box) > m.bodyY+m.bodyH && m.editY-len(box) >= m.bodyY { // no room below: above it
		y = m.editY - len(box)
	}
	x := min(max(0, m.editX), max(0, m.w-w-1))
	put(lines, x, y, box, w)
}

// followPaths (gf): open the path in the todo under the cursor, or in the note being read,
// in Files. More than one: pick from a menu.
func (m *model) followPaths() tea.Cmd {
	var found []string
	switch m.page() {
	case "todos":
		if m.cur >= 0 && m.cur < len(m.doc.lines) {
			found = paths.Real(m.doc.lines[m.cur].s)
		}
	case "note":
		if d := m.current(); d != nil {
			if b, err := os.ReadFile(d.path); err == nil {
				found = paths.Real(string(b))
			}
		}
	default:
		return nil
	}
	switch len(found) {
	case 0:
		m.setNote("no path to a file or folder on this PC here", true)
		return nil
	case 1:
		return openInFiles(found[0])
	}
	var items []menuItem
	for _, p := range found {
		p := p
		hint := "file"
		if fi, err := os.Stat(paths.Expand(p)); err == nil && fi.IsDir() {
			hint = "folder"
		}
		items = append(items, menuItem{name: p, hint: hint, do: func(m *model) tea.Cmd { return openInFiles(p) }})
	}
	m.menu = &menuState{title: icFolder + " Open in Files", items: items}
	return nil
}

func openInFiles(p string) tea.Cmd { return app.Open("files", paths.Expand(p)) }

// openProject: Files' gn on a folder opens its note (a todo list or note with the folder's
// name), made the first time with the folder's path in it.
func (m *model) openProject(dir string) {
	name := filepath.Base(dir)
	for _, d := range m.docs {
		if strings.EqualFold(d.title, name) {
			m.goToDoc(d.path)
			return
		}
	}
	p, err := createDoc("notes", name)
	if err != nil {
		m.setNote("couldn't make a note for "+name+": "+err.Error(), true)
		return
	}
	if b, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(b)) == "# "+name {
		os.WriteFile(p, []byte("# "+name+"\n\n"+prettyPath(dir)+"\n"), 0o644)
	}
	m.reloadDocs(p)
	m.goToDoc(p)
	m.setNote("new note for "+name+" · enter edits it", false)
}

// goToDoc opens a list or note by its file
func (m *model) goToDoc(path string) {
	m.edit, m.menu, m.finder, m.help = nil, nil, nil, false
	m.reloadDocs(path)
	m.loadCurrent()
	m.focus = 1
	m.nscroll = 0
	if m.doc != nil {
		m.fixCur()
	}
}
