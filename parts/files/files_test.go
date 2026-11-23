package files

import (
	"archive/zip"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestMain keeps tests away from the real config folder
func TestMain(m *testing.M) {
	d, _ := os.MkdirTemp("", "dotes-test-config")
	os.Setenv("DOTES_CONFIG", d)
	code := m.Run()
	os.RemoveAll(d)
	os.Exit(code)
}

// a small folder tree to browse
func testTree(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(d, "src", "inner"), 0o755))
	must(os.MkdirAll(filepath.Join(d, "empty"), 0o755))
	must(os.WriteFile(filepath.Join(d, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))
	must(os.WriteFile(filepath.Join(d, "img10.txt"), []byte("ten"), 0o644))
	must(os.WriteFile(filepath.Join(d, "img2.txt"), []byte("two"), 0o644))
	must(os.WriteFile(filepath.Join(d, ".secret"), []byte("hidden"), 0o644))
	must(os.WriteFile(filepath.Join(d, "blob.bin"), []byte{0, 1, 2, 3, 0}, 0o644))
	must(os.WriteFile(filepath.Join(d, "src", "a.txt"), []byte("hello"), 0o644))
	return d
}

func run(m *model, keys ...string) string {
	m.dump = true
	_, first := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	drain(m, tea.Batch(first, m.Init()))
	for _, k := range keys {
		m.View()
		drain(m, func() tea.Msg { return ParseInput(k) })
	}
	return ansi.Strip(m.View().Content)
}

func names(m *model) []string {
	var out []string
	for _, e := range m.entries {
		out = append(out, e.name)
	}
	return out
}

// every size draws without crashing, before and after previews load
func TestFirstFrameEverySize(t *testing.T) {
	d := testTree(t)
	for _, sz := range [][2]int{{20, 5}, {40, 8}, {60, 10}, {80, 24}, {200, 60}} {
		m := newModel(d)
		m.dump = true
		m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		m.View() // nothing loaded yet
		drain(m, m.Init())
		out := ansi.Strip(m.View().Content)
		if sz[0] >= 40 {
			for i, l := range strings.Split(out, "\n") {
				if w := ansi.StringWidth(l); w > sz[0] {
					t.Errorf("%dx%d: line %d is %d wide", sz[0], sz[1], i, w)
				}
			}
		}
	}
}

func TestOrderAndHidden(t *testing.T) {
	m := newModel(testTree(t))
	run(m)
	got := strings.Join(names(m), ",")
	want := "empty,src,blob.bin,img2.txt,img10.txt,main.go"
	if got != want {
		t.Fatalf("order: got %s, want %s", got, want)
	}
	if m.hiddenN != 1 {
		t.Fatalf("hidden count %d", m.hiddenN)
	}
	run(m, ".")
	if len(m.entries) != 7 {
		t.Fatalf("after . expected 7 entries, got %v", names(m))
	}
}

func TestNavigation(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m, "j", "l") // into src
	if filepath.Base(m.cwd) != "src" {
		t.Fatalf("l should enter src, cwd = %s", m.cwd)
	}
	run(m, "h") // back to the tree, cursor on src
	if m.cwd != d || m.current().name != "src" {
		t.Fatalf("h should return with the cursor on src: %s %s", m.cwd, m.current().name)
	}
	run(m, "H") // history: back to src
	if filepath.Base(m.cwd) != "src" {
		t.Fatalf("H should go back to src, cwd = %s", m.cwd)
	}
	run(m, "L")
	if m.cwd != d {
		t.Fatalf("L should go forward, cwd = %s", m.cwd)
	}
	run(m, "G")
	if m.current().name != "main.go" {
		t.Fatalf("G: %s", m.current().name)
	}
	run(m, "g", "g")
	if m.cursor != 0 {
		t.Fatalf("gg: cursor %d", m.cursor)
	}
}

func TestPreviews(t *testing.T) {
	m := newModel(testTree(t))
	out := run(m, "G") // main.go
	if !strings.Contains(out, "package main") {
		t.Fatalf("code preview missing:\n%s", out)
	}
	m.moveToName("blob.bin")
	out = run(m)
	if !strings.Contains(out, "Binary file") {
		t.Fatalf("binary info card missing:\n%s", out)
	}
	m.moveToName("src")
	out = run(m)
	if !strings.Contains(out, "inner") || !strings.Contains(out, "a.txt") {
		t.Fatalf("folder preview missing:\n%s", out)
	}
}

func TestQuitWritesCwd(t *testing.T) {
	d := testTree(t)
	f := filepath.Join(t.TempDir(), "cwd")
	m := newModel(d)
	m.cwdFile = f
	run(m, "q")
	b, err := os.ReadFile(f)
	if err != nil || string(b) != d {
		t.Fatalf("cwd file: %q %v", b, err)
	}
}

func TestNatLess(t *testing.T) {
	if !natLess("file2", "file10") || natLess("file10", "file2") || !natLess("a", "B") {
		t.Fatal("natural order is wrong")
	}
}

// ---- batch 2: file actions ----

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func typeKeys(s string) []string {
	var out []string
	for _, r := range s {
		out = append(out, string(r))
	}
	return out
}

func TestCopyPasteAndUndo(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m)
	m.moveToName("main.go")
	run(m, "y", "p") // same folder: a copy named "main (1).go"
	if !exists(filepath.Join(d, "main (1).go")) {
		t.Fatalf("paste in the same folder should make main (1).go: %v", names(m))
	}
	run(m, "u") // undo: the copy goes to the Recycle Bin
	if exists(filepath.Join(d, "main (1).go")) {
		t.Fatal("undo should remove the copy")
	}
	// copy a folder into another one
	m.moveToName("src")
	run(m, "y")
	m.moveToName("empty")
	run(m, "l", "p")
	if !exists(filepath.Join(d, "empty", "src", "inner")) || !exists(filepath.Join(d, "empty", "src", "a.txt")) {
		t.Fatal("folder copy is missing files")
	}
}

func TestCutPasteAndUndo(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m)
	m.moveToName("img2.txt")
	run(m, "x")
	m.moveToName("src")
	run(m, "l", "p")
	if exists(filepath.Join(d, "img2.txt")) || !exists(filepath.Join(d, "src", "img2.txt")) {
		t.Fatal("cut + paste should move the file")
	}
	run(m, "u")
	if !exists(filepath.Join(d, "img2.txt")) || exists(filepath.Join(d, "src", "img2.txt")) {
		t.Fatal("undo should move it back")
	}
}

func TestIntoItself(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m)
	m.moveToName("src")
	run(m, "y", "l", "l") // into src, then into inner
	run(m, "p")
	if exists(filepath.Join(d, "src", "inner", "src")) || !m.noteErr {
		t.Fatalf("pasting a folder inside itself must fail (note %q)", m.note)
	}
}

func TestCreateRenameUndo(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m)
	run(m, append(append([]string{"a"}, typeKeys("new/deep/file.md")...), "enter")...)
	if !exists(filepath.Join(d, "new", "deep", "file.md")) {
		t.Fatalf("create with folders failed: %s", m.note)
	}
	if m.current().name != "new" {
		t.Fatalf("cursor should be on new, is on %s", m.current().name)
	}
	run(m, "a", "f", "/", "enter")
	if fi, err := os.Stat(filepath.Join(d, "f")); err != nil || !fi.IsDir() {
		t.Fatal("a name ending in / should make a folder")
	}
	// rename: the cursor starts before the extension
	m.moveToName("main.go")
	run(m, "r", "2", "enter")
	if !exists(filepath.Join(d, "main2.go")) {
		t.Fatalf("rename failed: %v (%s)", names(m), m.note)
	}
	run(m, "u")
	if !exists(filepath.Join(d, "main.go")) {
		t.Fatal("undo should restore the old name")
	}
}

func TestTrashDeleteAndRestore(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m)
	m.moveToName("img10.txt")
	run(m, "d")
	if m.question == nil {
		t.Fatal("d should ask first")
	}
	run(m, "n")
	if !exists(filepath.Join(d, "img10.txt")) {
		t.Fatal("n must keep the file")
	}
	run(m, "d", "y")
	if exists(filepath.Join(d, "img10.txt")) {
		t.Fatalf("trash failed: %s", m.note)
	}
	run(m, "u")
	if !exists(filepath.Join(d, "img10.txt")) && os.Getenv("CI") == "" { // CI machines have no working Recycle Bin
		t.Fatalf("undo should bring it back from the Recycle Bin: %s", m.note)
	}
	m.moveToName("blob.bin")
	run(m, "D", "y")
	if exists(filepath.Join(d, "blob.bin")) {
		t.Fatal("D + y should delete")
	}
}

func TestVisualSelection(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m, "v", "j", "j") // empty, src, blob.bin
	if m.selCount() != 3 {
		t.Fatalf("visual range: %d selected", m.selCount())
	}
	run(m, "esc")
	if len(m.sel) != 3 || m.visual != visNone {
		t.Fatal("esc should keep the range selected")
	}
	run(m, "esc")
	if len(m.sel) != 0 {
		t.Fatal("second esc clears")
	}
	run(m, "ctrl+a")
	if len(m.sel) != len(m.entries) {
		t.Fatal("ctrl+a selects all")
	}
	run(m, "ctrl+r")
	if len(m.sel) != 0 {
		t.Fatal("ctrl+r inverts")
	}
}

func TestApplyRenamesSwap(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "a.txt"), filepath.Join(d, "b.txt")
	os.WriteFile(a, []byte("A"), 0o644)
	os.WriteFile(b, []byte("B"), 0o644)
	done, err := applyRenames([]string{a, b}, []string{"b.txt", "a.txt"})
	if err != nil || len(done) != 2 {
		t.Fatalf("swap: %v %v", done, err)
	}
	if got, _ := os.ReadFile(a); string(got) != "B" {
		t.Fatal("names should be swapped")
	}
	if _, err := applyRenames([]string{a}, []string{"x", "y"}); err == nil {
		t.Fatal("a line count mismatch must fail")
	}
}

func TestCopyPathKeys(t *testing.T) {
	m := newModel(testTree(t))
	run(m)
	m.moveToName("main.go")
	run(m, "c", "n")
	if !strings.Contains(m.note, "main") || strings.Contains(m.note, ".go") {
		t.Fatalf("cn: %s", m.note)
	}
}

// ---- batch 3: find and jump ----

func TestFilterAndFind(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m, append(append([]string{"f"}, typeKeys("img")...), "enter")...)
	if got := strings.Join(names(m), ","); got != "img2.txt,img10.txt" {
		t.Fatalf("filter: %s", got)
	}
	run(m, "esc")
	if len(m.entries) != 6 {
		t.Fatal("esc should clear the filter")
	}
	run(m, "/", "m", "a", "i", "n", "enter")
	if m.current().name != "main.go" {
		t.Fatalf("find: on %s", m.current().name)
	}
	run(m, "/", "i", "m", "g", "enter", "n")
	if m.current().name != "img10.txt" {
		t.Fatalf("n: on %s", m.current().name)
	}
	run(m, "N")
	if m.current().name != "img2.txt" {
		t.Fatalf("N: on %s", m.current().name)
	}
}

func TestSearchNamesAndText(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m, "s", "a", ".", "t", "x", "t", "enter")
	if m.virtual != "search" || len(m.entries) != 1 || m.entries[0].name != filepath.Join("src", "a.txt") {
		t.Fatalf("name search: %v %v", m.virtual, names(m))
	}
	run(m, "h")
	if m.virtual != "" || m.cwd != d {
		t.Fatal("h should leave the results")
	}
	if _, err := exec.LookPath("rg"); err != nil {
		return // text search needs ripgrep
	}
	run(m, append(append([]string{"S"}, typeKeys("func main")...), "enter")...)
	if len(m.entries) != 1 || m.entries[0].matchLine != 3 {
		t.Fatalf("text search: %v (%s)", names(m), m.note)
	}
}

func TestSortAndLineMode(t *testing.T) {
	d := testTree(t)
	os.WriteFile(filepath.Join(d, "big.txt"), []byte(strings.Repeat("x", 5000)), 0o644)
	m := newModel(d)
	run(m, ",", "s")
	if got := names(m); got[2] != "big.txt" || m.lineMode != "size" {
		t.Fatalf("size sort: %v", got)
	}
	run(m, ",", "A")
	if got := names(m); got[len(got)-1] != "big.txt" {
		t.Fatalf("Z to A: %v", got)
	}
	out := run(m, "m", "p")
	if !strings.Contains(out, "-rw") {
		t.Fatal("mp should show permissions")
	}
}

func TestDrivesAndGoto(t *testing.T) {
	d := testTree(t)
	m := newModel(filepath.VolumeName(d) + string(filepath.Separator))
	run(m, "h")
	if m.virtual != "drives" || len(m.entries) == 0 {
		t.Fatalf("h at a drive root should list drives: %v", m.virtual)
	}
	run(m, "enter")
	if m.virtual != "" {
		t.Fatal("enter on a drive opens it")
	}
	m = newModel(d)
	run(m, "g", "space")
	m.prompt.text = []rune(d + string(filepath.Separator) + "sr")
	m.prompt.pos = len(m.prompt.text)
	run(m, "tab", "enter")
	if filepath.Base(m.cwd) != "src" {
		t.Fatalf("g space + tab should complete and go to src: %s", m.cwd)
	}
}

// ---- batch 4: tabs, previews, git, info ----

func TestTabs(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m, "t", "t") // tab 2, same folder
	if len(m.tabs) != 2 || m.tabIdx != 1 {
		t.Fatalf("tt: %d tabs, on %d", len(m.tabs), m.tabIdx)
	}
	m.moveToName("src")
	run(m, "l")
	run(m, "1")
	if m.cwd != d {
		t.Fatalf("tab 1 should still be in the tree: %s", m.cwd)
	}
	run(m, "]")
	if filepath.Base(m.cwd) != "src" {
		t.Fatalf("tab 2 should be in src: %s", m.cwd)
	}
	out := run(m)
	if !strings.Contains(out, "1 ") || !strings.Contains(out, "2 src") {
		t.Fatalf("tab bar missing:\n%s", out)
	}
	run(m, "ctrl+c")
	if len(m.tabs) != 0 || m.quitting {
		t.Fatal("ctrl+c closes the tab, and doesn't quit while one is left")
	}
}

func TestRichPreviews(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "a.md"), []byte("# Title\n\n- [x] done\n- [ ] todo\n"), 0o644)
	os.WriteFile(filepath.Join(d, "b.csv"), []byte("name,age\nali,30\nsara,25\n"), 0o644)
	os.WriteFile(filepath.Join(d, "c.json"), []byte(`{"a":1,"b":[1,2]}`), 0o644)
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	f, _ := os.Create(filepath.Join(d, "d.png"))
	png.Encode(f, img)
	f.Close()
	zf, _ := os.Create(filepath.Join(d, "e.zip"))
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("inside/readme.txt")
	w.Write([]byte("hi"))
	zw.Close()
	zf.Close()

	m := newModel(d)
	checks := map[string]string{"a.md": "■", "b.csv": "sara", "c.json": `"b": [`, "d.png": "PNG image · 8 × 8", "e.zip": "readme.txt"}
	for name, want := range checks {
		m.moveToName(name)
		out := run(m)
		if !strings.Contains(out, want) {
			t.Errorf("%s preview should show %q:\n%s", name, want, out)
		}
	}
}

func TestGitMarksAndInfo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	d := t.TempDir()
	exec.Command("git", "-C", d, "init", "-q").Run()
	os.MkdirAll(filepath.Join(d, "sub"), 0o755)
	os.WriteFile(filepath.Join(d, "sub", "new.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(d, "top.txt"), []byte("line1\nline2\n"), 0o644)
	m := newModel(d)
	run(m)
	if m.gitMarks[strings.ToLower(filepath.Join(d, "top.txt"))] != "?" {
		t.Fatalf("untracked file should be ?: %v", m.gitMarks)
	}
	if m.gitMarks[strings.ToLower(filepath.Join(d, "sub"))] == "" {
		t.Fatalf("a folder with new files inside should be marked: %v", m.gitMarks)
	}
	m.moveToName("top.txt")
	out := run(m, "tab")
	if !strings.Contains(out, "2 lines") || !strings.Contains(out, "new (not tracked)") {
		t.Fatalf("info popup:\n%s", out)
	}
}

func TestSixelImages(t *testing.T) {
	d := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for i := range img.Pix {
		img.Pix[i] = 180
	}
	f, _ := os.Create(filepath.Join(d, "pic.png"))
	png.Encode(f, img)
	f.Close()
	os.WriteFile(filepath.Join(d, "z.txt"), []byte("text"), 0o644)
	m := newModel(d)
	m.gfx = "sixel"
	run(m)
	if m.imgWant.key == "" || m.imgWant.w == 0 {
		t.Fatal("a sixel terminal should get a picture spot")
	}
	raw, ok := m.imgSync()().(tea.RawMsg)
	if !ok || !strings.Contains(fmt.Sprint(raw.Msg), "\x1bP") {
		t.Fatal("imgSync should send a Sixel picture")
	}
	if m.imgSync() != nil {
		t.Fatal("the same picture isn't sent twice")
	}
	run(m, "j") // to the text file: no picture wanted
	if m.imgWant.key != "" {
		t.Fatal("no picture for a text file")
	}
	run(m, "k", "~") // a popup hides it
	if m.popupOpen() == false {
		t.Fatal("help counts as a popup")
	}
}

// ---- batch 5: TermNook pick mode, archives, open with, config ----

func TestPickMode(t *testing.T) {
	d := testTree(t)
	out := filepath.Join(t.TempDir(), "pick.txt")
	m := newModel(d)
	m.pick = pickArgs{out: out, openers: []opener{{name: "Nvim"}, {name: "VS Code", key: "c"}},
		projects: []string{filepath.Join(d, "src"), filepath.Join(d, "empty")}}
	m.showProjects()
	o := run(m)
	if m.virtual != "projects" || len(m.entries) != 2 || !strings.Contains(o, "enter Nvim") || !strings.Contains(o, "alt+c VS Code") {
		t.Fatalf("projects view:\n%s", o)
	}
	run(m, "l") // into src: browsing still works
	if filepath.Base(m.cwd) != "src" {
		t.Fatalf("l should go into the project: %s", m.cwd)
	}
	m.moveToName("a.txt")
	drain(m, func() tea.Msg { return tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt} })
	b, _ := os.ReadFile(out)
	if string(b) != "1\t"+filepath.Join(d, "src", "a.txt") || !m.quitting {
		t.Fatalf("alt+c should pick a.txt for opener 1: %q", b)
	}
}

func TestExtractCompress(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m)
	m.moveToName("src")
	run(m, "C", "enter") // src.zip
	if !exists(filepath.Join(d, "src.zip")) {
		t.Fatalf("compress: %s", m.note)
	}
	m.moveToName("src.zip")
	run(m, "e") // into "src (1)" since src exists
	if !exists(filepath.Join(d, "src (1)", "src", "inner")) || !exists(filepath.Join(d, "src (1)", "src", "a.txt")) {
		t.Fatalf("extract: %s %v", m.note, names(m))
	}
	run(m, "u")
	if exists(filepath.Join(d, "src (1)")) {
		t.Fatal("undo removes what was extracted")
	}
}

func TestOpenWithAndConfig(t *testing.T) {
	m := newModel(testTree(t))
	run(m)
	m.moveToName("main.go")
	o := run(m, "O")
	if m.menu == nil || !strings.Contains(o, "Default app") {
		t.Fatalf("open-with menu:\n%s", o)
	}
	run(m, "esc")
	c := parseConfig("hidden = true\nsort = size reverse\nlinemode = mtime\nhints = off\nopener = Paint | mspaint \"%p\"\n",
		config{sortBy: "natural", lineMode: "size", hints: true})
	if !c.hidden || c.sortBy != "size" || !c.sortRev || c.lineMode != "mtime" || !c.hints || len(c.openers) != 1 {
		t.Fatalf("config: %+v", c)
	}
}

// g space: suggestions as you type, ↑↓ picks one, enter goes there
func TestGotoSuggestions(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m, "g", "space")
	if len(m.prompt.sugs) == 0 || m.prompt.sugs[0].name != "empty" && m.prompt.sugs[0].name != "src" {
		t.Fatalf("folders first: %+v", m.prompt.sugs)
	}
	run(m, "i", "m", "g")
	if len(m.prompt.sugs) != 2 || m.prompt.sugs[0].isDir {
		t.Fatalf("img: %+v", m.prompt.sugs)
	}
	run(m, "ctrl+u")
	o := run(m, append(typeKeys(d+string(filepath.Separator)+"rc"), "down")...)
	if !strings.Contains(o, "src") || m.prompt.sugSel != 0 { // "rc" is inside "src"
		t.Fatalf("contains match:\n%s", o)
	}
	run(m, "enter")
	if filepath.Base(m.cwd) != "src" {
		t.Fatalf("enter on the picked one: %s", m.cwd)
	}
}

// a search runs on while you browse; its results wait for ctrl+s instead of pulling you away
func TestSearchWhileBrowsing(t *testing.T) {
	d := testTree(t)
	m := newModel(d)
	run(m)
	cmd := m.startSearch(false, "a.txt")
	m.cd(filepath.Join(d, "src"), "", true)                          // you go somewhere while it runs
	m.cwd = filepath.Join(d, strings.Repeat("long-folder-name-", 8)) // a long path gives way
	if o := run(m); !strings.Contains(o, "searching names") || !m.searching {
		t.Fatalf("the header should show the search:\n%s", o)
	}
	drain(m, cmd)
	if m.virtual == "search" || m.searchReady == nil || !strings.Contains(m.note, "ctrl+s") {
		t.Fatalf("results should wait: %v %q", m.virtual, m.note)
	}
	run(m, "ctrl+s")
	if m.virtual != "search" || len(m.entries) != 1 || m.cwd != d {
		t.Fatalf("ctrl+s shows them: %v %v", m.virtual, names(m))
	}
}

func TestFullScreenReader(t *testing.T) {
	d := testTree(t)
	long := strings.Repeat("x", 95)
	os.WriteFile(filepath.Join(d, "a.csv"), []byte("id,name,notes\n1,Sara,"+long+"END\n"), 0o644)
	m := newModel(d)
	m.moveToName("a.csv")
	out := run(m, "F")
	if m.rd == nil || !strings.Contains(out, "│ name") || !strings.Contains(out, "col 1") {
		t.Fatalf("csv full screen: a table that scrolls sideways:\n%s", out)
	}
	out = run(m, "L") // to the right end
	if !strings.Contains(out, "END") {
		t.Fatalf("L should show the end of the row:\n%s", out)
	}
	run(m, "q")
	if m.rd != nil {
		t.Fatal("q should leave full screen")
	}
}

func TestDragColumns(t *testing.T) {
	m := newModel(testTree(t))
	run(m)
	sep := m.colX[1][1]
	run(m, fmt.Sprintf("click:%d,3", sep), "drag:80,3", "release:80,3")
	if m.colX[1][1] <= sep || m.drag != 0 {
		t.Fatalf("dragging the line should widen the folder column: %d -> %d", sep, m.colX[1][1])
	}
	run(m, "=")
	if m.colX[1][1] != sep {
		t.Fatalf("= should put the widths back: %d, want %d", m.colX[1][1], sep)
	}
}
