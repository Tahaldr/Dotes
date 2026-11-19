package shell

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/internal/ui"
	"github.com/charmbracelet/x/ansi"
)

func TestMain(m *testing.M) {
	d, _ := os.MkdirTemp("", "dotes-test-config")
	os.Setenv("DOTES_CONFIG", d)
	code := m.Run()
	os.RemoveAll(d)
	os.Exit(code)
}

func run(m *Model, w, h int, keys ...string) string {
	_, c := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	ui.Drain(m, tea.Batch(c, m.Init()))
	for _, k := range keys {
		m.View()
		ui.Drain(m, func() tea.Msg { return ui.ParseInput(k) })
	}
	return ansi.Strip(m.View().Content)
}

// fake is a tiny part for switching tests
type fake struct {
	name string
	got  []string
}

func (f *fake) Init() tea.Cmd { return nil }
func (f *fake) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		f.got = append(f.got, msg.String())
	case tea.MouseClickMsg:
		f.got = append(f.got, "click y", string(rune('0'+msg.Y)))
	}
	return f, nil
}
func (f *fake) View() tea.View { return tea.NewView("inside " + f.name) }

func TestSetupFirstRun(t *testing.T) {
	os.Remove(app.File("dotes.txt"))
	m := New(Options{Notebook: t.TempDir(), FilesPath: t.TempDir(), Dump: true})
	if o := run(m, 100, 20); !strings.Contains(o, "Welcome to Dotes") {
		t.Fatalf("first run should show the picker:\n%s", o)
	}
	if app.HasShared() {
		t.Fatal("dotes.txt must not exist before the user picks")
	}
	o := run(m, 100, 30, "a", "space", "enter") // all off, pick Home only
	if !app.HasShared() || strings.Join(app.LoadShared().Parts, ",") != "home" {
		t.Fatalf("saved parts: %v\n%s", app.LoadShared().Parts, o)
	}
	if !strings.Contains(o, "1 Home") || !strings.Contains(o, "Nvim") {
		t.Fatalf("no ready part picked:\n%s", o)
	}
	m = New(Options{Notebook: t.TempDir(), FilesPath: t.TempDir(), Dump: true, Setup: true})
	o = run(m, 100, 20, "a", "a", "enter") // all on, all off: refused
	if !strings.Contains(o, "Pick at least one") {
		t.Fatalf("empty pick must be refused:\n%s", o)
	}
	app.SaveParts([]string{"files"})
}

func TestFirstFrameEverySize(t *testing.T) {
	app.SaveParts([]string{"files"})
	dir := t.TempDir()
	for _, sz := range [][2]int{{1, 1}, {10, 3}, {40, 8}, {80, 24}, {200, 60}} {
		m := New(Options{Notebook: t.TempDir(), FilesPath: dir, Dump: true})
		m.View() // before any size
		o := run(m, sz[0], sz[1])
		if sz[0] >= 80 && !strings.HasPrefix(o, " dotes  1 Files") {
			t.Fatalf("%v: no top bar:\n%s", sz, o)
		}
	}
	m := New(Options{Notebook: t.TempDir(), FilesPath: dir, Dump: true, Setup: true})
	m.View()
	run(m, 5, 2)
}

func TestSwitchAndMouse(t *testing.T) {
	app.SaveParts([]string{"files", "notes"})
	m := New(Options{Notebook: t.TempDir(), FilesPath: t.TempDir(), Dump: true})
	nf := &fake{name: "notes"}
	m.makers["notes"] = func() tea.Model { return nf }
	m.parts = nil
	m.loadParts(app.LoadShared().Parts, "files")
	o := run(m, 100, 20, "alt+2")
	if !strings.Contains(o, "inside notes") || !strings.Contains(o, "2 Notes") || !strings.Contains(o, "alt+1-2 switch") {
		t.Fatalf("alt+2 should open Notes:\n%s", o)
	}
	run(m, 100, 20, "j", "click:5,3")
	if strings.Join(nf.got, " ") != "j click y 2" {
		t.Fatalf("keys / mouse (moved up by the bar): %v", nf.got)
	}
	o = run(m, 100, 20, "click:10,0") // the Files tab
	if m.active != 0 || strings.Contains(o, "inside notes") {
		t.Fatalf("click on the Files tab:\n%s", o)
	}
	app.SaveParts([]string{"files"})
}

func TestNotesPart(t *testing.T) {
	app.SaveParts([]string{"files", "notes"})
	nb := t.TempDir()
	os.MkdirAll(nb+"/todos", 0o755)
	os.MkdirAll(nb+"/notes", 0o755)
	os.WriteFile(nb+"/todos/Groceries.md", []byte("# Groceries\n\n- [ ] milk\n"), 0o644)
	m := New(Options{FilesPath: t.TempDir(), Notebook: nb, Dump: true})
	o := run(m, 110, 20, "alt+2")
	if !strings.Contains(o, "Groceries") || !strings.Contains(o, "Todo lists") {
		t.Fatalf("alt+2 should show Notes:\n%s", o)
	}
	o = run(m, 110, 20, "enter")
	if !strings.Contains(o, "milk") {
		t.Fatalf("enter opens the list:\n%s", o)
	}
	o = run(m, 110, 20, "alt+1")
	if strings.Contains(o, "milk") || !strings.Contains(o, "NOR") {
		t.Fatalf("alt+1 should show Files again:\n%s", o)
	}
	for _, sz := range [][2]int{{1, 1}, {30, 5}, {50, 11}, {200, 60}} {
		m := New(Options{Start: "notes", Notebook: nb, Dump: true})
		m.View()
		run(m, sz[0], sz[1])
	}
	app.SaveParts([]string{"files"})
}

func TestHomeSwitchesParts(t *testing.T) {
	app.SaveParts([]string{"home", "files"})
	m := New(Options{Notebook: t.TempDir(), FilesPath: t.TempDir(), Dump: true})
	run(m, 110, 30)
	if m.cur().info.ID != "home" {
		t.Fatalf("dotes opens Home first, got %s", m.cur().info.ID)
	}
	run(m, 110, 30, "f") // the Files app on Home
	if m.cur().info.ID != "files" {
		t.Fatalf("f on Home should show Files, got %s", m.cur().info.ID)
	}
	run(m, 110, 30, "alt+1", "o") // Notes isn't picked: it's added for this run
	if m.cur().info.ID != "notes" || len(m.parts) != 3 {
		t.Fatalf("o on Home should open Notes, got %s (%d parts)", m.cur().info.ID, len(m.parts))
	}
	app.SaveParts([]string{"files"})
}
