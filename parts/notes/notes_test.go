package notes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

const sample = `# Groceries

- [ ] milk @priority(high)
  - [x] the cheap one
  - [ ] oat
- [x] bread
- [ ] eggs
`

// TestMain keeps tests away from the real config folder
func TestMain(m *testing.M) {
	d, _ := os.MkdirTemp("", "dotes-test-config")
	os.Setenv("DOTES_CONFIG", d)
	code := m.Run()
	os.RemoveAll(d)
	os.Exit(code)
}

// notebook makes a test notebook with one todo list and one note; returns the list's path
func notebook(t *testing.T, todo string) string {
	t.Helper()
	dir := t.TempDir()
	cfg = config{notebook: dir, hints: true, indent: 4, inbox: "Inbox"}
	os.MkdirAll(filepath.Join(dir, "todos"), 0o755)
	os.MkdirAll(filepath.Join(dir, "notes"), 0o755)
	p := filepath.Join(dir, "todos", "Groceries.md")
	os.WriteFile(p, []byte(todo), 0o644)
	os.WriteFile(filepath.Join(dir, "notes", "Ideas.md"), []byte("# Ideas\n\nhello **there**\n"), 0o644)
	return p
}

func doc(t *testing.T, text string) *todoDoc {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.md")
	os.WriteFile(p, []byte(text), 0o644)
	d, err := loadDoc(p)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func body(d *todoDoc) string { return strings.Join(d.texts(), "\n") }

func find(d *todoDoc, s string) int {
	for i, l := range d.lines {
		if strings.Contains(l.s, s) {
			return i
		}
	}
	return -1
}

func TestSmartToggle(t *testing.T) {
	d := doc(t, sample)
	d.toggle(find(d, "oat")) // the last open sub-todo: the parent ticks too
	if !d.todoAt(find(d, "milk")).done {
		t.Fatal("parent should tick when all its sub-todos are done:\n" + body(d))
	}
	d.toggle(find(d, "cheap")) // one open again: the parent unticks
	if d.todoAt(find(d, "milk")).done {
		t.Fatal("parent should untick")
	}
	d.toggle(find(d, "milk")) // ticking a parent ticks its subs
	if !d.todoAt(find(d, "oat")).done || !d.todoAt(find(d, "cheap")).done {
		t.Fatal("subs should follow the parent:\n" + body(d))
	}
}

func TestAddCutPasteMove(t *testing.T) {
	d := doc(t, sample)
	at := d.addBelow(find(d, "milk"), false) // after milk's sub-todos
	if d.lines[at].s != "- [ ] " || !strings.Contains(d.lines[at-1].s, "oat") {
		t.Fatalf("new todo in the wrong place (%d):\n%s", at, body(d))
	}
	d.remove(at, at)
	at = d.addBelow(find(d, "bread"), true)
	if d.lines[at].s != "  - [ ] " {
		t.Fatalf("sub-todo should be indented: %q", d.lines[at].s)
	}
	d.remove(at, at)
	reg := d.cut(find(d, "milk"))
	if len(reg) != 3 || find(d, "oat") >= 0 {
		t.Fatalf("cut should take the sub-todos: %v", reg)
	}
	d.paste(find(d, "eggs"), reg, false)
	want := "# Groceries\n\n- [x] bread\n- [ ] eggs\n- [ ] milk @priority(high)\n  - [x] the cheap one\n  - [ ] oat"
	if body(d) != want {
		t.Fatalf("paste:\n%s", body(d))
	}
	i := d.moveUp(find(d, "milk"))
	if !strings.Contains(d.lines[i].s, "milk") || !strings.Contains(d.lines[i+3].s, "eggs") {
		t.Fatalf("move up:\n%s", body(d))
	}
	if d.moveDown(find(d, "oat")) != find(d, "oat") {
		t.Fatal("the last sub-todo can't move down")
	}
}

func TestShiftPriorityArchive(t *testing.T) {
	d := doc(t, sample)
	if !d.shift(find(d, "bread"), 1) || d.lines[find(d, "bread")].s != "  - [x] bread" {
		t.Fatalf("indent:\n%s", body(d))
	}
	d.shift(find(d, "bread"), -1)
	if d.shift(find(d, "bread"), -1) {
		t.Fatal("can't outdent below zero")
	}
	i := find(d, "eggs")
	for _, want := range []string{"low", "medium", "high", ""} {
		if got := d.cyclePriority(i); got != want {
			t.Fatalf("priority %q, want %q", got, want)
		}
	}
	if d.lines[i].s != "- [ ] eggs" {
		t.Fatalf("priority tag left behind: %q", d.lines[i].s)
	}
	if n := d.archiveDone(); n != 1 {
		t.Fatalf("archived %d, want 1 (bread)", n)
	}
	if !strings.HasSuffix(body(d), "## Archive\n- [x] bread") {
		t.Fatalf("archive:\n%s", body(d))
	}
	if done, all := progress(d.texts()); done != 1 || all != 4 {
		t.Fatalf("progress %d/%d should skip the archive", done, all)
	}
}

func TestSaveKeepsLineEndings(t *testing.T) {
	d := doc(t, strings.ReplaceAll(sample, "\n", "\r\n"))
	d.toggle(find(d, "eggs"))
	if err := d.save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(d.path)
	if !strings.Contains(string(b), "- [x] eggs\r\n") || strings.Count(string(b), "\r\n") != strings.Count(string(b), "\n") {
		t.Fatalf("CRLF lost: %q", b)
	}
}

// the app, driven by keys like a person would
func TestKeys(t *testing.T) {
	p := notebook(t, sample)
	read := func() string { b, _ := os.ReadFile(p); return string(b) }

	runDump(100, 24, []string{"enter", "G", "x"}) // tick eggs
	if !strings.Contains(read(), "- [x] eggs") {
		t.Fatal("x should tick:\n" + read())
	}
	runDump(100, 24, []string{"enter", "G", "o", "type:butter", "enter", "type:jam", "esc"})
	if !strings.Contains(read(), "- [x] eggs\n- [ ] butter\n- [ ] jam\n") {
		t.Fatal("o + typing should add todos:\n" + read())
	}
	runDump(100, 24, []string{"enter", "G", "o", "esc"}) // an empty new todo goes away
	if strings.Contains(read(), "- [ ] \n") {
		t.Fatal("empty todo left behind:\n" + read())
	}
	runDump(100, 24, []string{"enter", "G", "d", "d", "u"}) // delete + undo
	if !strings.Contains(read(), "- [ ] jam") {
		t.Fatal("undo should bring jam back:\n" + read())
	}
	runDump(100, 24, []string{"enter", "G", "c", "c", "type:honey", "enter"})
	if !strings.Contains(read(), "- [ ] honey\n") || strings.Contains(read(), "jam") {
		t.Fatal("cc should retype:\n" + read())
	}
	out := runDump(100, 24, []string{"enter", "tab"}) // fold milk
	if strings.Contains(out, "oat") || !strings.Contains(out, "1/2") {
		t.Fatal("tab should fold milk's sub-todos:\n" + out)
	}
	runDump(100, 24, []string{"enter", "s", "type:skim", "esc"})
	if !strings.Contains(read(), "  - [ ] oat\n  - [ ] skim\n") {
		t.Fatal("s should add a sub-todo:\n" + read())
	}
}

func TestListsCreateRenameDelete(t *testing.T) {
	notebook(t, sample)
	out := runDump(100, 24, []string{"a", "type:Work stuff", "enter", "type:first", "esc"})
	p := filepath.Join(cfg.notebook, "todos", "Work stuff.md")
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "# Work stuff\n\n- [ ] first\n" {
		t.Fatalf("new list: %q %v\n%s", b, err, out)
	}
	runDump(100, 24, []string{"r", "ctrl+u", "type:Job", "enter"}) // newest list is first
	if _, err := os.Stat(filepath.Join(cfg.notebook, "todos", "Job.md")); err != nil {
		t.Fatal("rename should rename the file")
	}
	out = runDump(100, 24, []string{"tab"})
	if !strings.Contains(out, "hello there") {
		t.Fatal("the note should show on the right:\n" + out)
	}
	runDump(100, 24, []string{"tab", "d", "y"})
	if _, err := os.Stat(filepath.Join(cfg.notebook, "notes", "Ideas.md")); err == nil {
		t.Fatal("d y should delete the note")
	}
}

func TestEverySize(t *testing.T) {
	notebook(t, sample)
	for _, sz := range [][2]int{{30, 5}, {50, 10}, {60, 12}, {120, 40}, {200, 60}} {
		out := runDump(sz[0], sz[1], []string{"enter", "~"})
		if len(strings.Split(out, "\n")) > sz[1] {
			t.Fatalf("%dx%d: too many lines", sz[0], sz[1])
		}
		runDump(sz[0], sz[1], []string{"enter", "j", "j", "tab", "esc", "tab"})
	}
}

func TestEditHidesPriority(t *testing.T) {
	p := notebook(t, sample)
	out := runDump(100, 24, []string{"enter", "a"})
	if strings.Contains(out, "@priority") {
		t.Fatal("the priority tag shouldn't show while typing:\n" + out)
	}
	runDump(100, 24, []string{"enter", "a", "type: and cream", "enter"})
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "- [ ] milk and cream @priority(high)\n") {
		t.Fatalf("the priority should stay after editing:\n%s", b)
	}
}

func TestTabs(t *testing.T) {
	notebook(t, sample)
	out := runDump(100, 24, nil)
	if !strings.Contains(out, "Groceries") || strings.Contains(out, "Ideas  ") {
		t.Fatal("the todo tab should list only todo lists:\n" + out)
	}
	for _, keys := range [][]string{{"tab"}, {"2"}, {"click:35,0"}} {
		out = runDump(100, 24, keys)
		if !strings.Contains(out, "hello there") || strings.Contains(out, "2/3") {
			t.Fatalf("%v should open the Notes tab:\n%s", keys, out)
		}
	}
}

func TestDueInput(t *testing.T) {
	now := time.Date(2026, 11, 17, 0, 0, 0, 0, time.Local) // a Tuesday
	for in, want := range map[string]string{
		"today": "2026-11-17", "tomorrow": "2026-11-18", "fri": "2026-11-20", "tue": "2026-11-24",
		"+3": "2026-11-20", "+2w": "2026-12-01", "11-20": "2026-11-20", "01-05": "2027-01-05",
		"2026-12-25": "2026-12-25", "": "",
	} {
		if got, err := parseDueInput(in, now); err != nil || got != want {
			t.Errorf("%q: got %q %v, want %q", in, got, err, want)
		}
	}
	if _, err := parseDueInput("someday", now); err == nil {
		t.Error("someday isn't a date")
	}
	if l, _ := dueLabel("2026-11-15", false, now); l != "2d late" {
		t.Errorf("late label %q", l)
	}
}

func TestSortByPriority(t *testing.T) {
	d := doc(t, "# L\n\n- [ ] a\n- [ ] b @priority(low)\n  - [ ] b2\n  - [ ] b1 @priority(high)\n- [ ] c @priority(high)\n\n## Archive\n- [x] z @priority(high)\n")
	if !d.sortByPriority() {
		t.Fatal("nothing sorted")
	}
	want := "# L\n\n- [ ] c @priority(high)\n- [ ] b @priority(low)\n  - [ ] b1 @priority(high)\n  - [ ] b2\n- [ ] a\n\n## Archive\n- [x] z @priority(high)"
	if body(d) != want {
		t.Fatalf("sorted:\n%s", body(d))
	}
}

func TestDueMoveFindToday(t *testing.T) {
	p := notebook(t, sample)
	work := filepath.Join(cfg.notebook, "todos", "Work.md")
	os.WriteFile(work, []byte("# Work\n\n- [ ] report\n"), 0o644)
	os.Chtimes(work, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)) // Groceries stays first
	read := func(f string) string { b, _ := os.ReadFile(f); return string(b) }

	runDump(100, 24, []string{"enter", "G", "t", "type:today", "enter"}) // eggs due today
	if !strings.Contains(read(p), "- [ ] eggs @due("+today().Format(dateFmt)+")") {
		t.Fatal("t should set a due date:\n" + read(p))
	}
	out := runDump(110, 24, []string{"3"})
	if !strings.Contains(out, "eggs") || !strings.Contains(out, "TODAY") || !strings.Contains(out, "Today 1") {
		t.Fatal("the Today tab should list eggs:\n" + out)
	}
	runDump(110, 24, []string{"3", "x"})
	if !strings.Contains(read(p), "- [x] eggs") {
		t.Fatal("x in Today should tick it in its list:\n" + read(p))
	}
	runDump(100, 24, []string{"enter", "m", "enter"}) // milk (with subs) to Work
	if strings.Contains(read(p), "milk") || !strings.Contains(read(work), "- [ ] report\n- [ ] milk @priority(high)\n  - [x] the cheap one\n") {
		t.Fatalf("m should move milk to Work:\n%s\n---\n%s", read(p), read(work))
	}
	out = runDump(100, 24, []string{"/", "type:report", "enter"})
	if !strings.Contains(out, "❯ 󰄱 report") {
		t.Fatal("finding report should open Work on it:\n" + out)
	}
}

func TestCLIAdd(t *testing.T) {
	p := notebook(t, sample)
	cliAdd([]string{"jam", "-l", "groc", "-p", "high", "-d", "2026-12-01"})
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "- [ ] eggs\n- [ ] jam @priority(high) @due(2026-12-01)\n") {
		t.Fatalf("add to Groceries:\n%s", b)
	}
	cliAdd([]string{"call", "mom"})
	b, err := os.ReadFile(filepath.Join(cfg.notebook, "todos", "Inbox.md"))
	if err != nil || string(b) != "# Inbox\n\n- [ ] call mom\n" {
		t.Fatalf("add to a new Inbox: %q %v", b, err)
	}
}

func TestBoardAndPages(t *testing.T) {
	notebook(t, sample)
	out := runDump(120, 30, nil)
	if !strings.Contains(out, "╭─ 󰄲 Groceries") || !strings.Contains(out, "+ New todo list") || !strings.Contains(out, "BOARD") {
		t.Fatal("the todo lists should be cards:\n" + out)
	}
	out = runDump(120, 30, []string{"enter"})
	if !strings.Contains(out, "‹ Todo lists") || !strings.Contains(out, "THIS TODO") {
		t.Fatal("enter should open the list with its details:\n" + out)
	}
	if out = runDump(120, 30, []string{"enter", "esc"}); !strings.Contains(out, "BOARD") {
		t.Fatal("esc should go back to the board:\n" + out)
	}
	out = runDump(120, 30, []string{"2", "enter"})
	if !strings.Contains(out, "‹ Notes") || !strings.Contains(out, "hello there") {
		t.Fatal("enter on a note card should show the note:\n" + out)
	}
	if out = runDump(120, 30, []string{"l", "enter"}); !strings.Contains(out, "New todo list") || !strings.Contains(out, "a title") {
		t.Fatal("the + card should ask for a new list:\n" + out)
	}
}

// a list saved in another window shows without a key press
func TestLiveReload(t *testing.T) {
	p := notebook(t, sample)
	m := newModel()
	m.Init()
	if m.watch == nil {
		t.Skip("no watching here")
	}
	defer m.watch.Close()
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m.openCurrent()
	got := make(chan tea.Msg, 1)
	go func() { got <- m.watch.Next()() }()
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(p, []byte(sample+"- [ ] butter from nvim\n"), 0o644)
	select {
	case msg := <-got:
		m.Update(msg)
	case <-time.After(3 * time.Second):
		t.Fatal("the change wasn't seen")
	}
	if v := m.View().Content; !strings.Contains(v, "butter from nvim") {
		t.Fatalf("not reloaded:\n%s", v)
	}
}
