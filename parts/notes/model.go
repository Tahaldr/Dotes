package notes

import (
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/internal/watch"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// a slot is one row you can stand on in the left list: a document, or "a adds one" for an
// empty section
type slot struct {
	kind string
	doc  int // index into m.docs, -1 = the empty section's placeholder
}

type editState struct {
	p     *prompt
	line  int
	isNew bool
	prio  string // the @priority tag, kept out of the text while typing

	comp     *compState // tab cycling through path suggestions
	sugWord  string     // the path the suggestions below are for
	sugItems []string
}

type model struct {
	w, h  int
	docs  []docInfo
	slots []slot
	sl    int       // the slot under the cursor
	focus int       // 0 = the lists, 1 = inside a todo list
	tab   string    // "todos" or "notes": what the left side lists
	tabX  [3][2]int // where the tabs are in the header (for clicks)

	agenda     []agendaItem // the Today tab
	acur       int
	agendaRows []int

	finder *finderState
	menu   *menuState

	doc  *todoDoc // the todo list on the right (nil for notes)
	cur  int      // the line the cursor is on, in doc
	top  int      // first screen line shown on the right
	ltop int      // first row shown on the left

	nscroll int // notes: preview scroll
	ncache  struct {
		path string
		mod  time.Time
		w    int
		out  []string
	}

	edit     *editState
	prompt   *prompt
	question *question
	pending  string
	reg      []line // what dd / yy took

	help       bool
	helpScroll int
	hintsOn    bool

	note    string
	noteErr bool

	dump, live, quitting bool

	// layout, set while drawing (for the mouse)
	bodyY, bodyH int
	cols         int        // cards per row on the board
	cards        []cardRect // where each card is
	pageX        int        // where the open list's page starts
	editY, editX int        // where the todo being typed is on screen (-1 = not shown)
	links        []linkZone // paths on screen (a click opens them in Files)
	watch        *watch.Watcher
	rightRows    []int // doc line per body row (-1 = none, -2 = the way back)
	boxX         []int // where the checkbox is on that row
	lastClick    time.Time
	lastY        int
	cardCache    map[string]*cardData
}

type tickMsg struct{}
type editorDoneMsg struct{ err error }

func newModel() *model {
	m := &model{hintsOn: cfg.hints, rightRows: nil}
	m.reloadDocs("")
	m.loadCurrent()
	m.buildAgenda()
	return m
}

func (m *model) Init() tea.Cmd {
	if m.dump {
		return nil
	}
	m.watch = watch.New("notes") // a list saved in Nvim or another window shows at once
	m.watch.Set(kindDir("todos"), kindDir("notes"))
	return tea.Batch(m.tick(), m.watch.Next())
}

func (m *model) tick() tea.Cmd {
	if m.dump {
		return nil
	}
	every := 2 * time.Second
	if m.watch.Watching() > 0 { // changes come from the watcher; the timer is a fallback
		every = 10 * time.Second
	}
	return tea.Tick(every, func(time.Time) tea.Msg { return tickMsg{} })
}

// checkDisk picks up lists and notes changed outside (not while you're typing or picking)
func (m *model) checkDisk() {
	if m.edit != nil || m.prompt != nil || m.finder != nil || m.menu != nil {
		return
	}
	if m.refresh() && m.doc == nil {
		m.loadCurrent()
	}
	m.buildAgenda()
	if m.doc != nil {
		if fi, err := os.Stat(m.doc.path); err == nil && !fi.ModTime().Equal(m.doc.mod) {
			m.reloadDoc()
		}
	}
}

func (m *model) setNote(s string, isErr bool) { m.note, m.noteErr = s, isErr }

// ---------------------------------------------------------------------------
// documents
// ---------------------------------------------------------------------------

// buildSlots lists the open tab's documents (todo lists or notes)
func (m *model) buildSlots() {
	if m.tab == "" {
		m.tab = "todos"
	}
	m.slots = nil
	for i, d := range m.docs {
		if d.kind == m.tab {
			m.slots = append(m.slots, slot{m.tab, i})
		}
	}
	m.slots = append(m.slots, slot{m.tab, -1}) // the "+ new" card
}

var tabs = []string{"todos", "notes", "today"}

// switchTab shows todo lists, notes or the Today tab
func (m *model) switchTab(kind string) {
	if kind == m.tab {
		return
	}
	m.tab, m.sl, m.ltop, m.focus, m.top = kind, 0, 0, 0, 0
	m.buildSlots()
	m.loadCurrent()
	if kind == "today" {
		m.buildAgenda()
	}
}

func (m *model) nextTab(fwd bool) {
	for i, t := range tabs {
		if t == m.tab {
			d := 1
			if !fwd {
				d = len(tabs) - 1
			}
			m.switchTab(tabs[(i+d)%len(tabs)])
			return
		}
	}
}

func (m *model) count(kind string) int {
	n := 0
	for _, d := range m.docs {
		if d.kind == kind {
			n++
		}
	}
	return n
}

// reloadDocs reads the notebook again; the cursor stays on keep (a path), or where it was
func (m *model) reloadDocs(keep string) {
	if keep == "" {
		if d := m.current(); d != nil {
			keep = d.path
		}
	}
	m.docs = listDocs()
	for _, d := range m.docs { // a new list / note opens its tab
		if strings.EqualFold(d.path, keep) {
			m.tab = d.kind
		}
	}
	m.buildSlots()
	m.sl = max(0, min(m.sl, len(m.slots)-1))
	for i, s := range m.slots {
		if s.doc >= 0 && strings.EqualFold(m.docs[s.doc].path, keep) {
			m.sl = i
			return
		}
	}
}

// refresh: the notebook changed on disk? (new / removed files reorder; edits just update)
func (m *model) refresh() bool {
	fresh := listDocs()
	same := len(fresh) == len(m.docs)
	byPath := map[string]docInfo{}
	for _, d := range fresh {
		byPath[strings.ToLower(d.path)] = d
	}
	if same {
		for _, d := range m.docs {
			if _, ok := byPath[strings.ToLower(d.path)]; !ok {
				same = false
			}
		}
	}
	if !same {
		m.reloadDocs("")
		return true
	}
	changed := false
	for i, d := range m.docs { // keep the order while you work
		if f := byPath[strings.ToLower(d.path)]; !f.mod.Equal(d.mod) || f.title != d.title || f.done != d.done || f.total != d.total {
			m.docs[i] = f
			changed = true
		}
	}
	return changed
}

func (m *model) current() *docInfo {
	if m.sl < 0 || m.sl >= len(m.slots) || m.slots[m.sl].doc < 0 {
		return nil
	}
	return &m.docs[m.slots[m.sl].doc]
}

func (m *model) section() string { return m.tab }

// loadCurrent shows the document under the cursor on the right
func (m *model) loadCurrent() {
	m.doc, m.nscroll, m.top = nil, 0, 0
	d := m.current()
	if d == nil || d.kind != "todos" {
		return
	}
	doc, err := loadDoc(d.path)
	if err != nil {
		m.setNote("can't read "+filepath.Base(d.path)+": "+err.Error(), true)
		return
	}
	m.doc = doc
	m.cur = -1
	m.fixCur()
}

// reloadDoc reads the open todo list again (changed outside), keeping the cursor and folds
func (m *model) reloadDoc() {
	if m.doc == nil {
		return
	}
	folds := map[string]bool{}
	for _, l := range m.doc.lines {
		if l.fold {
			folds[l.s] = true
		}
	}
	doc, err := loadDoc(m.doc.path)
	if err != nil {
		return
	}
	for i := range doc.lines {
		doc.lines[i].fold = folds[doc.lines[i].s]
	}
	m.doc = doc
	m.fixCur()
}

// syncInfo updates the left list's counts for the open todo list (after a change)
func (m *model) syncInfo() {
	d := m.current()
	if d == nil || m.doc == nil {
		return
	}
	d.done, d.total = progress(m.doc.texts())
	d.mod = m.doc.mod
	d.title = titleOf(m.doc.texts(), d.path)
}

// ---------------------------------------------------------------------------
// the cursor in a todo list
// ---------------------------------------------------------------------------

// fixCur puts the cursor on a shown row (the nearest one above, else the first)
func (m *model) fixCur() {
	if m.doc == nil {
		return
	}
	rows := m.doc.rows()
	if len(rows) == 0 {
		m.cur = -1
		return
	}
	best := rows[0]
	for _, r := range rows {
		if r <= m.cur {
			best = r
		}
	}
	m.cur = best
}

func (m *model) rowPos() (rows []int, k int) {
	rows = m.doc.rows()
	for i, r := range rows {
		if r == m.cur {
			return rows, i
		}
	}
	return rows, 0
}

func (m *model) moveRow(d int) {
	rows, k := m.rowPos()
	if len(rows) == 0 {
		return
	}
	m.cur = rows[max(0, min(len(rows)-1, k+d))]
}

func (m *model) moveSlot(d int) {
	n := max(0, min(len(m.slots)-1, m.sl+d))
	if n != m.sl {
		m.sl = n
		m.loadCurrent()
	}
}

func (m *model) half() int { return max(1, m.bodyH/2) }

// ---------------------------------------------------------------------------
// changes
// ---------------------------------------------------------------------------

// change runs f on the open list (with undo), and saves when f says something changed
func (m *model) change(f func(d *todoDoc) bool) {
	d := m.doc
	if d == nil {
		return
	}
	d.remember()
	if !f(d) {
		d.forget()
		return
	}
	m.save()
}

func (m *model) save() {
	if err := m.doc.save(); err != nil {
		m.setNote("couldn't save: "+err.Error(), true)
	}
	m.syncInfo()
}

// startEdit types into line i (pos -1 = cursor at the end); isNew = a todo being added
func (m *model) startEdit(i int, isNew bool, pos int, clear bool) {
	d := m.doc
	text, prio := d.lines[i].s, ""
	if t, ok := parseTodo(text); ok {
		text, prio = takeTags(t.text) // priority and due date stay, unseen
		prio = strings.TrimSpace(prio)
	}
	if clear {
		text = ""
	}
	m.cur = i
	m.edit = &editState{p: newPrompt(text, pos), line: i, isNew: isNew, prio: prio}
}

// addTodo adds an empty todo and starts typing in it
func (m *model) addTodo(how string) {
	d := m.doc
	if d == nil {
		return
	}
	d.remember()
	var at int
	switch how {
	case "below":
		at = d.addBelow(m.cur, false)
	case "above":
		at = d.addAbove(m.cur)
	case "sub":
		at = d.addBelow(m.cur, true)
	}
	m.startEdit(at, true, -1, false)
}

// rollback undoes an unsaved addition (an empty new todo)
func (d *todoDoc) rollback() {
	if n := len(d.undo); n > 0 {
		d.lines, d.undo = d.undo[n-1], d.undo[:n-1]
	}
}

func (m *model) editKey(k tea.KeyPressMsg) tea.Cmd {
	e, d := m.edit, m.doc
	switch s := k.String(); s {
	case "tab", "shift+tab": // in a path: put a suggestion in
		if e.complete(s == "tab") {
			return nil
		}
	case "enter", "esc": // after tab: keep the suggestion, go on typing
		if c := e.comp; c != nil && string(e.p.text) == c.text && e.p.pos == c.pos {
			e.comp = nil
			return nil
		}
	}
	switch k.String() {
	case "tab", "shift+tab":
		dir := 1
		if k.String() == "shift+tab" {
			dir = -1
		}
		if !e.isNew {
			d.remember()
		}
		if d.shift(e.line, dir) && !e.isNew {
			m.save()
		} else if !e.isNew {
			d.forget()
		}
		return nil
	}
	done, how := e.p.editKey(k)
	if !done {
		return nil
	}
	m.edit = nil
	text := strings.TrimSpace(string(e.p.text))
	t, isTodo := parseTodo(d.lines[e.line].s)
	switch {
	case how == "ctrl+c" && e.isNew, (how != "ctrl+c" && e.isNew && text == ""):
		d.rollback()
		m.fixCur()
		return nil
	case how == "ctrl+c":
		return nil
	}
	newLine := text
	if isTodo {
		t.text = text
		if e.prio != "" {
			t.text += " " + e.prio
		}
		newLine = t.String()
	} else {
		newLine = strings.Repeat(" ", indentOf(d.lines[e.line].s)) + text
	}
	if !e.isNew {
		if newLine == d.lines[e.line].s {
			return nil
		}
		d.remember()
	}
	d.lines[e.line].s = newLine
	m.save()
	if how == "enter" && e.isNew { // keep adding
		d.remember()
		at := d.addBelow(e.line, false)
		m.startEdit(at, true, -1, false)
	}
	return nil
}

// ---------------------------------------------------------------------------
// creating, renaming, deleting lists and notes
// ---------------------------------------------------------------------------

func (m *model) newDoc() tea.Cmd {
	kind := m.section()
	m.input("New "+kindOne[kind], "a title; the file is named after it, in "+prettyPath(kindDir(kind)), "", func(m *model, s string) tea.Cmd {
		if s == "" {
			return nil
		}
		p, err := createDoc(kind, s)
		if err != nil {
			m.setNote("couldn't create it: "+err.Error(), true)
			return nil
		}
		m.reloadDocs(p)
		m.loadCurrent()
		if kind == "notes" {
			m.focus = 1
			return m.editFile(p, 3)
		}
		m.focus = 1
		if m.doc != nil && len(m.doc.rows()) == 0 {
			m.addTodo("below")
		}
		return nil
	})
	return nil
}

func (m *model) renameDoc() {
	d := m.current()
	if d == nil {
		return
	}
	path := d.path
	m.input("Rename", "the title and the file name change together", d.title, func(m *model, s string) tea.Cmd {
		if s == "" {
			return nil
		}
		np, err := renameDoc(path, s)
		if err != nil {
			m.setNote(err.Error(), true)
		}
		m.reloadDocs(np)
		m.loadCurrent()
		return nil
	})
}

func (m *model) deleteDoc() {
	d := m.current()
	if d == nil {
		return
	}
	path, title := d.path, d.title
	m.ask("Delete the "+kindOne[d.kind]+" \""+title+"\"? It goes to the Recycle Bin.", true, func(m *model) tea.Cmd {
		if m.dump {
			os.Remove(path)
		} else if err := trash(path); err != nil {
			m.setNote("couldn't delete it: "+err.Error(), true)
			return nil
		}
		m.setNote("deleted "+title+" (it's in the Recycle Bin)", false)
		m.reloadDocs("")
		m.loadCurrent()
		return nil
	})
}

// editFile opens path in the editor (on line, when > 0) and comes back
func (m *model) editFile(path string, line int) tea.Cmd {
	if m.dump {
		m.setNote("would edit "+filepath.Base(path), false)
		return nil
	}
	args := []string{path}
	if line > 0 {
		args = []string{"+" + strconv.Itoa(line), path}
	}
	c := exec.Command(editor(), args...)
	c.Dir = filepath.Dir(path)
	return tea.ExecProcess(c, func(err error) tea.Msg { return editorDoneMsg{err} })
}

func prettyPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(strings.ToLower(p), strings.ToLower(home)) {
		return "~" + p[len(home):]
	}
	return p
}

// ---------------------------------------------------------------------------
// messages
// ---------------------------------------------------------------------------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case app.ShownMsg:
		if msg.ID == "notes" {
			m.quitting = false // q went back to Home: go on when you come back
		}
	case app.OpenMsg: // Files' gn: this folder's note
		m.openProject(msg.Path)
	case tickMsg:
		m.checkDisk()
		return m, m.tick()
	case watch.Msg:
		if msg.Key != "notes" {
			return m, nil
		}
		m.checkDisk()
		return m, m.watch.Next()
	case editorDoneMsg:
		if msg.err != nil {
			m.setNote("editor: "+msg.err.Error(), true)
		}
		m.reloadDocs("")
		if m.doc != nil {
			m.reloadDoc()
		} else {
			m.loadCurrent()
		}
	case tea.PasteMsg:
		switch {
		case m.prompt != nil:
			m.prompt.insert(msg.Content)
		case m.finder != nil:
			m.finder.p.insert(msg.Content)
			m.findMatch()
		case m.edit != nil:
			m.edit.p.insert(msg.Content)
		}
	case tea.KeyPressMsg:
		cmd := m.key(msg)
		if m.quitting {
			return m, tea.Quit
		}
		return m, cmd
	case tea.MouseClickMsg:
		return m, m.click(msg)
	case tea.MouseWheelMsg:
		m.wheel(msg)
	}
	return m, nil
}

func (m *model) key(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if s != "" && m.prompt == nil && m.question == nil {
		m.note = ""
	}
	switch {
	case m.question != nil:
		return m.questionKey(k)
	case m.prompt != nil:
		return m.promptKey(k)
	case m.finder != nil:
		return m.finderKey(k)
	case m.menu != nil:
		return m.menuKey(s)
	case m.edit != nil:
		return m.editKey(k)
	case m.help && s == "e":
		m.help = false
		return m.editFile(configFile(), 0)
	case m.help:
		m.helpKey(s)
		return nil
	}
	if m.pending != "" {
		combo := m.pending + s
		m.pending = ""
		return m.comboKey(combo)
	}
	switch s {
	case "q", "ctrl+c":
		m.quitting = true
		return nil
	case "1", "2", "3":
		m.switchTab(tabs[s[0]-'1'])
		return nil
	case "/":
		m.openFinder()
		return nil
	case "~", "f1":
		m.help, m.helpScroll = true, 0
		return nil
	case "g", "d", "y", "c":
		if s == "g" || m.focus == 1 {
			m.pending = s
			return nil
		}
	}
	if m.tab == "today" {
		if s == "tab" || s == "shift+tab" || s == "[" || s == "]" {
			m.nextTab(s == "tab" || s == "]")
			return nil
		}
		return m.agendaKey(s)
	}
	switch m.page() {
	case "todos":
		return m.todoKey(s)
	case "note":
		return m.noteKey(s)
	}
	return m.listKey(s)
}

func (m *model) comboKey(c string) tea.Cmd {
	switch c {
	case "gg":
		if m.tab == "today" {
			m.acur = 0
		} else if m.page() == "note" {
			m.nscroll = 0
		} else if m.focus == 1 && m.doc != nil {
			if rows := m.doc.rows(); len(rows) > 0 {
				m.cur = rows[0]
			}
		} else {
			m.sl = 0
			m.loadCurrent()
		}
	case "dd":
		if m.doc.isTodo(m.cur) {
			m.change(func(d *todoDoc) bool { m.reg = d.cut(m.cur); return true })
			m.fixCur()
			m.setNote("deleted · p puts it back · u undoes", false)
		}
	case "yy":
		if m.doc.isTodo(m.cur) {
			m.reg = m.doc.copyTree(m.cur)
			m.setNote("copied · p pastes below, P above", false)
		}
	case "gf":
		return m.followPaths()
	case "cc":
		if m.cur >= 0 {
			m.startEdit(m.cur, false, -1, true)
		}
	}
	return nil
}

func (m *model) listKey(s string) tea.Cmd {
	switch s {
	case "j", "down":
		m.moveSlot(max(1, m.cols))
	case "k", "up":
		m.moveSlot(-max(1, m.cols))
	case "l", "right":
		m.moveSlot(1)
	case "h", "left":
		m.moveSlot(-1)
	case "G", "end":
		m.moveSlot(len(m.slots))
	case "home":
		m.moveSlot(-len(m.slots))
	case "ctrl+d", "pgdown":
		m.moveSlot(2 * max(1, m.cols))
	case "ctrl+u", "pgup":
		m.moveSlot(-2 * max(1, m.cols))
	case "tab", "shift+tab", "[", "]":
		m.nextTab(s == "tab" || s == "]")
	case "enter", "space":
		return m.openCurrent()
	case "a":
		return m.newDoc()
	case "r":
		m.renameDoc()
	case "d", "delete":
		m.deleteDoc()
	case "e":
		if d := m.current(); d != nil {
			return m.editFile(d.path, 0)
		}
	}
	return nil
}

// noteKey: reading a note
func (m *model) noteKey(s string) tea.Cmd {
	switch s {
	case "j", "down":
		m.nscroll++
	case "k", "up":
		m.nscroll = max(0, m.nscroll-1)
	case "ctrl+d", "pgdown", "space":
		m.nscroll += m.half()
	case "ctrl+u", "pgup":
		m.nscroll = max(0, m.nscroll-m.half())
	case "G", "end":
		m.nscroll = 1 << 20
	case "home":
		m.nscroll = 0
	case "h", "esc", "left", "backspace":
		m.focus = 0
	case "enter", "e", "E":
		if d := m.current(); d != nil {
			return m.editFile(d.path, m.nscroll+1)
		}
	case "tab", "shift+tab", "[", "]":
		m.nextTab(s == "tab" || s == "]")
	case "r":
		m.renameDoc()
	case "d", "delete":
		m.deleteDoc()
	}
	return nil
}

func (m *model) openCurrent() tea.Cmd {
	d := m.current()
	if d == nil {
		return m.newDoc()
	}
	if d.kind == "notes" {
		m.focus, m.nscroll = 1, 0
		return nil
	}
	if m.doc == nil {
		m.loadCurrent()
	}
	if m.doc != nil {
		m.focus = 1
		m.fixCur()
		if m.cur < 0 {
			m.addTodo("below")
		}
	}
	return nil
}

func (m *model) todoKey(s string) tea.Cmd {
	d := m.doc
	switch s {
	case "j", "down":
		m.moveRow(1)
	case "k", "up":
		m.moveRow(-1)
	case "G", "end":
		m.moveRow(1 << 20)
	case "home":
		m.moveRow(-(1 << 20))
	case "ctrl+d", "pgdown":
		m.moveRow(m.half())
	case "ctrl+u", "pgup":
		m.moveRow(-m.half())
	case "h", "esc", "left":
		m.focus = 0
	case "enter", "space", "x":
		if d.isTodo(m.cur) {
			m.change(func(d *todoDoc) bool { d.toggle(m.cur); return true })
		}
	case "o":
		m.addTodo("below")
	case "O":
		m.addTodo("above")
	case "s":
		m.addTodo("sub")
	case "i", "I":
		if m.cur >= 0 {
			m.startEdit(m.cur, false, 0, false)
		}
	case "a", "e":
		if m.cur >= 0 {
			m.startEdit(m.cur, false, -1, false)
		}
	case "p", "P":
		if len(m.reg) > 0 {
			reg := m.reg
			m.change(func(d *todoDoc) bool { m.cur = d.paste(m.cur, reg, s == "P"); return true })
		}
	case ">", "<":
		dir := 1
		if s == "<" {
			dir = -1
		}
		m.change(func(d *todoDoc) bool { return d.shift(m.cur, dir) })
	case "J", "K":
		cur := m.cur
		m.change(func(d *todoDoc) bool {
			if !d.isTodo(cur) {
				return false
			}
			if s == "J" {
				m.cur = d.moveDown(cur)
			} else {
				m.cur = d.moveUp(cur)
			}
			return m.cur != cur
		})
	case "tab":
		switch {
		case d.hasSubs(m.cur):
			d.lines[m.cur].fold = !d.lines[m.cur].fold
		case d.isTodo(m.cur) && d.parentOf(m.cur) >= 0:
			m.cur = d.parentOf(m.cur)
			d.lines[m.cur].fold = true
		}
	case "shift+tab":
		any := false
		for _, l := range d.lines {
			any = any || l.fold
		}
		for i := range d.lines {
			d.lines[i].fold = !any && d.hasSubs(i)
		}
		m.fixCur()
	case "!":
		if d.isTodo(m.cur) {
			p := ""
			m.change(func(d *todoDoc) bool { p = d.cyclePriority(m.cur); return true })
			if p == "" {
				p = "none"
			}
			m.setNote("priority: "+p, false)
		}
	case "A":
		n := 0
		m.change(func(d *todoDoc) bool { n = d.archiveDone(); return n > 0 })
		if n == 0 {
			m.setNote("nothing to archive: only fully done todos move", false)
		} else {
			m.fixCur()
			m.setNote("archived "+strconv.Itoa(n)+" · u undoes", false)
		}
	case "u":
		if d.undoLast() {
			m.save()
			m.fixCur()
		} else {
			m.setNote("nothing to undo", false)
		}
	case "ctrl+r":
		if d.redoLast() {
			m.save()
			m.fixCur()
		} else {
			m.setNote("nothing to redo", false)
		}
	case "m":
		m.moveMenu()
	case "t":
		if d.isTodo(m.cur) {
			text, _, _ := splitTags(d.todoAt(m.cur).text)
			line := m.cur
			m.askDue(text, func(m *model, date string) {
				m.change(func(d *todoDoc) bool { d.setDue(line, date); return true })
				m.buildAgenda()
			})
		}
	case "S":
		m.change(func(d *todoDoc) bool { return d.sortByPriority() })
		m.setNote("sorted by priority · u undoes", false)
	case "E":
		return m.editFile(d.path, m.cur+1)
	}
	return nil
}

func (m *model) helpKey(s string) {
	switch s {
	case "j", "down":
		m.helpScroll++
	case "k", "up":
		m.helpScroll = max(0, m.helpScroll-1)
	case "ctrl+d", "pgdown", "space":
		m.helpScroll += m.h / 2
	case "ctrl+u", "pgup":
		m.helpScroll = max(0, m.helpScroll-m.h/2)
	case "h":
		m.toggleHints()
	case "esc", "q", "~", "f1":
		m.help = false
	}
}

// ---------------------------------------------------------------------------
// mouse
// ---------------------------------------------------------------------------

func (m *model) click(c tea.MouseClickMsg) tea.Cmd {
	if m.help || m.prompt != nil || m.question != nil || m.finder != nil || m.menu != nil || c.Button != tea.MouseLeft {
		return nil
	}
	double := time.Since(m.lastClick) < 400*time.Millisecond && m.lastY == c.Y
	m.lastClick, m.lastY = time.Now(), c.Y
	if c.Y == 1 && m.focus == 1 { // where you are: back to the board
		m.focus = 0
		return nil
	}
	if c.Y == 0 { // the tabs in the header
		for i, kind := range tabs {
			if c.X >= m.tabX[i][0] && c.X < m.tabX[i][1] {
				m.switchTab(kind)
			}
		}
		return nil
	}
	for _, z := range m.links {
		if c.Y == z.y && c.X >= z.x0 && c.X < z.x1 {
			return openInFiles(z.path)
		}
	}
	r := c.Y - m.bodyY
	if r < 0 || r >= m.bodyH {
		return nil
	}
	if m.tab == "today" {
		if r < len(m.agendaRows) && m.agendaRows[r] >= 0 {
			m.acur = m.agendaRows[r]
			if double {
				it := m.agenda[m.acur]
				m.goTo(it.path, it.line)
			}
		}
		return nil
	}
	if m.edit != nil { // clicking elsewhere ends typing, like esc
		m.editKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	}
	switch m.page() {
	case "board":
		for _, c2 := range m.cards {
			if c.X >= c2.x && c.X < c2.x+c2.w && c.Y >= c2.y && c.Y < c2.y+c2.h {
				if c2.slot != m.sl {
					m.sl = c2.slot
					m.loadCurrent()
				}
				if double {
					return m.openCurrent()
				}
			}
		}
		return nil
	case "note":
		if r == 0 {
			m.focus = 0
		} else if double {
			return m.noteKey("enter")
		}
		return nil
	}
	if r >= len(m.rightRows) {
		return nil
	}
	switch ln := m.rightRows[r]; {
	case ln == -2:
		m.focus = 0
	case ln >= 0:
		m.cur = ln
		if x := m.boxX[r]; x >= 0 && c.X >= x && c.X <= x+1 && m.doc.isTodo(m.cur) {
			m.change(func(d *todoDoc) bool { d.toggle(m.cur); return true })
		}
	}
	return nil
}

func (m *model) wheel(w tea.MouseWheelMsg) {
	d := 3
	if w.Button == tea.MouseWheelUp {
		d = -3
	}
	if m.help {
		m.helpScroll = max(0, m.helpScroll+d)
		return
	}
	if m.edit != nil || m.prompt != nil || m.finder != nil || m.menu != nil {
		return
	}
	if m.tab == "today" {
		m.acur = max(0, min(len(m.agenda)-1, m.acur+d/3))
		return
	}
	switch m.page() {
	case "board":
		m.moveSlot(d / 3 * max(1, m.cols))
	case "todos":
		m.moveRow(d / 3)
	case "note":
		m.nscroll = max(0, m.nscroll+d)
	}
}
