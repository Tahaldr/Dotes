package files

// File actions: select, copy / cut / paste, trash / delete, create, rename (one or many),
// links, copy a path, and undo.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// ---------------------------------------------------------------------------
// selection (kept across folders, like yazi)
// ---------------------------------------------------------------------------

const (
	visNone  = 0
	visSel   = 1 // v: the range gets selected
	visUnset = 2 // V: the range gets unselected
)

func (m *model) inVisual(i int) bool {
	if m.visual == visNone {
		return false
	}
	a, b := min(m.vAnchor, m.cursor), max(m.vAnchor, m.cursor)
	return i >= a && i <= b
}

// selectedAt: is row i of the current folder selected (counting a visual range in progress)?
func (m *model) selectedAt(i int) bool {
	if m.inVisual(i) {
		return m.visual == visSel
	}
	return i < len(m.entries) && m.sel[m.entries[i].path]
}

func (m *model) commitVisual() {
	if m.visual == visNone {
		return
	}
	for i := range m.entries {
		if m.inVisual(i) {
			if m.visual == visSel {
				m.sel[m.entries[i].path] = true
			} else {
				delete(m.sel, m.entries[i].path)
			}
		}
	}
	m.visual = visNone
}

// targets: the selected files, or the one under the cursor
func (m *model) targets() []string {
	m.commitVisual()
	var out []string
	for p := range m.sel {
		if _, err := os.Lstat(p); err == nil {
			out = append(out, p)
		}
	}
	if len(out) > 0 {
		sort.Slice(out, func(i, j int) bool { return natLess(out[i], out[j]) })
		return out
	}
	if c := m.current(); c != nil {
		return []string{c.path}
	}
	return nil
}

func (m *model) clearSel() { m.sel = map[string]bool{}; m.visual = visNone }

// selCount: how many are selected, counting a visual range in progress
func (m *model) selCount() int {
	n := len(m.sel)
	if m.visual == visNone {
		return n
	}
	for i := range m.entries {
		if m.inVisual(i) && m.sel[m.entries[i].path] != (m.visual == visSel) {
			if m.visual == visSel {
				n++
			} else {
				n--
			}
		}
	}
	return n
}

func plural(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %ss", n, one)
}

func nameList(paths []string) string {
	if len(paths) == 1 {
		return filepath.Base(paths[0])
	}
	return plural(len(paths), "item")
}

// ---------------------------------------------------------------------------
// copy / cut / paste
// ---------------------------------------------------------------------------

func (m *model) yankTargets(cut bool) {
	t := m.targets()
	if len(t) == 0 {
		return
	}
	m.yank = &yankState{paths: t, cut: cut}
	m.clearSel()
	if cut {
		m.setNote(nameList(t)+" cut · p to move here", false)
	} else {
		m.setNote(nameList(t)+" copied · p to paste", false)
	}
}

func (m *model) isYanked(p string) (bool, bool) {
	if m.yank == nil {
		return false, false
	}
	for _, y := range m.yank.paths {
		if y == p {
			return true, m.yank.cut
		}
	}
	return false, false
}

func (m *model) paste(force bool) tea.Cmd {
	if m.yank == nil {
		m.setNote("nothing copied: y copies, x cuts", true)
		return nil
	}
	t := &task{kind: taskCopy, force: force, undoOK: !force}
	if m.yank.cut {
		t.kind = taskMove
	}
	for _, src := range m.yank.paths {
		if _, err := os.Lstat(src); err != nil {
			continue
		}
		base := filepath.Base(src)
		dest := filepath.Join(m.cwd, base)
		if t.kind == taskMove && strings.EqualFold(filepath.Dir(src), m.cwd) {
			continue // cut and pasted in the same folder: nothing to do
		}
		if !force {
			dest = freeName(m.cwd, base)
		}
		t.items = append(t.items, item{src, dest})
	}
	if m.yank.cut {
		m.yank = nil // a cut is used once
	}
	if len(t.items) == 0 {
		m.setNote("already here", false)
		return nil
	}
	if first := filepath.Base(t.items[0].dest); first != "" {
		m.memory[m.cwd] = first
		m.pendingHover = first
	}
	return m.startTask(t)
}

// ---------------------------------------------------------------------------
// trash / delete
// ---------------------------------------------------------------------------

func (m *model) askTrash(forever bool) {
	t := m.targets()
	if len(t) == 0 {
		return
	}
	if forever {
		m.ask("Delete "+nameList(t)+" forever? This can't be undone.", true, func(m *model) tea.Cmd {
			m.clearSel()
			return m.startTask(&task{kind: taskDelete, items: itemsOf(t)})
		})
		return
	}
	m.ask("Move "+nameList(t)+" to the Recycle Bin?", false, func(m *model) tea.Cmd {
		m.clearSel()
		return m.startTask(&task{kind: taskTrash, items: itemsOf(t), undoOK: true})
	})
}

func itemsOf(paths []string) []item {
	out := make([]item, len(paths))
	for i, p := range paths {
		out[i] = item{src: p}
	}
	return out
}

// ---------------------------------------------------------------------------
// create / rename
// ---------------------------------------------------------------------------

func (m *model) askCreate() {
	m.input("Create", "end with / for a folder · a/b/c.txt makes the folders too", "", -1, func(m *model, s string) tea.Cmd {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		top, err := create(m.cwd, s)
		if err != nil {
			m.setNote(err.Error(), true)
			return nil
		}
		m.pushUndo(undoStep{kind: "create", items: []item{{dest: top}}})
		m.reload()
		m.moveToName(filepath.Base(top))
		m.setNote("created "+strings.TrimRight(s, `/\`), false)
		return nil
	})
}

// create makes a file (or folder, ending in /) and any folders on the way;
// top is the first new thing inside dir (what undo removes)
func create(dir, rel string) (top string, err error) {
	isDir := strings.HasSuffix(rel, "/") || strings.HasSuffix(rel, `\`)
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if _, err := os.Lstat(p); err == nil {
		return "", fmt.Errorf("%s already exists", filepath.Base(p))
	}
	// the first part of the path that doesn't exist yet
	parts := strings.Split(filepath.Clean(filepath.FromSlash(rel)), string(filepath.Separator))
	cur := dir
	for _, part := range parts {
		cur = filepath.Join(cur, part)
		if _, err := os.Lstat(cur); err != nil {
			top = cur
			break
		}
	}
	if isDir {
		err = os.MkdirAll(p, 0o755)
	} else {
		if err = os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
			var f *os.File
			if f, err = os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err == nil {
				f.Close()
			}
		}
	}
	if err != nil {
		return "", fmt.Errorf("couldn't create %s: %v", filepath.Base(p), errText(err))
	}
	return top, nil
}

func (m *model) askRename() tea.Cmd {
	t := m.targets()
	if len(t) > 1 {
		return m.bulkRename(t)
	}
	c := m.current()
	if c == nil || len(t) == 0 {
		return nil
	}
	old := t[0]
	name := filepath.Base(old)
	cur := len([]rune(name))
	if ext := filepath.Ext(name); ext != "" && ext != name && !isDirPath(old) {
		cur -= len([]rune(ext)) // cursor before the extension, like yazi
	}
	m.input("Rename", "", name, cur, func(m *model, s string) tea.Cmd {
		s = strings.TrimSpace(s)
		if s == "" || s == name {
			return nil
		}
		if strings.ContainsAny(s, `/\`) {
			m.setNote("a name can't have / or \\ (use x and p to move)", true)
			return nil
		}
		dest := filepath.Join(filepath.Dir(old), s)
		if _, err := os.Lstat(dest); err == nil && !strings.EqualFold(dest, old) {
			m.setNote(s+" already exists", true)
			return nil
		}
		if err := os.Rename(old, dest); err != nil {
			m.setNote("couldn't rename: "+errText(err), true)
			return nil
		}
		m.clearSel()
		m.pushUndo(undoStep{kind: "move", items: []item{{old, dest}}})
		m.reload()
		m.moveToName(s)
		m.setNote("renamed to "+s, false)
		return nil
	})
	return nil
}

func isDirPath(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// ---------------------------------------------------------------------------
// many at once, in the editor: r with a selection, and A
// ---------------------------------------------------------------------------

type bulkDoneMsg struct {
	kind string // "rename" or "create"
	file string
	olds []string
	err  error
}

func (m *model) bulkRename(paths []string) tea.Cmd {
	// names only when everything is in this folder, full paths otherwise
	local := true
	for _, p := range paths {
		if !strings.EqualFold(filepath.Dir(p), m.cwd) {
			local = false
		}
	}
	lines := make([]string, len(paths))
	for i, p := range paths {
		if local {
			lines[i] = filepath.Base(p)
		} else {
			lines[i] = p
		}
	}
	return m.editLines("rename", lines, paths)
}

func (m *model) bulkCreate() tea.Cmd {
	return m.editLines("create", nil, nil)
}

func (m *model) editLines(kind string, lines, olds []string) tea.Cmd {
	f, err := os.CreateTemp("", "dotes-files-"+kind+"-*.txt")
	if err != nil {
		m.setNote(err.Error(), true)
		return nil
	}
	f.WriteString(strings.Join(lines, "\n") + "\n")
	f.Close()
	if m.dump {
		os.Remove(f.Name())
		m.setNote("would open the editor to "+kind, false)
		return nil
	}
	c := exec.Command(editor(), f.Name())
	c.Dir = m.cwd
	name := f.Name()
	return tea.ExecProcess(c, func(err error) tea.Msg { return bulkDoneMsg{kind, name, olds, err} })
}

func (m *model) bulkDone(msg bulkDoneMsg) {
	defer os.Remove(msg.file)
	if msg.err != nil {
		m.setNote("editor: "+msg.err.Error(), true)
		return
	}
	b, err := os.ReadFile(msg.file)
	if err != nil {
		m.setNote(err.Error(), true)
		return
	}
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		lines = append(lines, strings.TrimSpace(l))
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if msg.kind == "create" {
		var made []item
		for _, l := range lines {
			if l == "" {
				continue
			}
			top, err := create(m.cwd, l)
			if err != nil {
				m.setNote(err.Error(), true)
				continue
			}
			made = append(made, item{dest: top})
		}
		if len(made) > 0 {
			m.pushUndo(undoStep{kind: "create", items: made})
			if m.note == "" {
				m.setNote("created "+plural(len(made), "item"), false)
			}
		}
		m.reload()
		return
	}
	done, err := applyRenames(msg.olds, lines)
	if len(done) > 0 {
		m.pushUndo(undoStep{kind: "move", items: done})
		m.clearSel()
	}
	if err != nil {
		m.setNote(err.Error(), true)
	} else if len(done) == 0 {
		m.setNote("no names changed", false)
	} else {
		m.setNote("renamed "+plural(len(done), "item"), false)
	}
	m.reload()
}

// applyRenames renames olds[i] to news[i] (a name, or a full path). Names may swap:
// everything goes to a temporary name first.
func applyRenames(olds, news []string) ([]item, error) {
	if len(news) != len(olds) {
		return nil, fmt.Errorf("%d lines for %d files: nothing renamed", len(news), len(olds))
	}
	var todo []item
	seen := map[string]bool{}
	for i, o := range olds {
		n := news[i]
		if n == "" {
			return nil, fmt.Errorf("line %d is empty: nothing renamed", i+1)
		}
		if !filepath.IsAbs(n) {
			n = filepath.Join(filepath.Dir(o), n)
		}
		if seen[strings.ToLower(n)] {
			return nil, fmt.Errorf("%s is used twice: nothing renamed", filepath.Base(n))
		}
		seen[strings.ToLower(n)] = true
		if n != o {
			todo = append(todo, item{o, n})
		}
	}
	moving := map[string]bool{}
	for _, it := range todo {
		moving[strings.ToLower(it.src)] = true
	}
	for _, it := range todo {
		if _, err := os.Lstat(it.dest); err == nil && !moving[strings.ToLower(it.dest)] {
			return nil, fmt.Errorf("%s already exists: nothing renamed", filepath.Base(it.dest))
		}
	}
	tmp := make([]string, len(todo))
	for i, it := range todo {
		tmp[i] = it.src + fmt.Sprintf(".dotes-%d~", i)
		if err := os.Rename(it.src, tmp[i]); err != nil {
			for j := i - 1; j >= 0; j-- { // put back what moved
				os.Rename(tmp[j], todo[j].src)
			}
			return nil, fmt.Errorf("couldn't rename %s: %v", filepath.Base(it.src), errText(err))
		}
	}
	var done []item
	var firstErr error
	for i, it := range todo {
		os.MkdirAll(filepath.Dir(it.dest), 0o755)
		if err := os.Rename(tmp[i], it.dest); err != nil {
			os.Rename(tmp[i], it.src)
			if firstErr == nil {
				firstErr = fmt.Errorf("couldn't rename %s: %v", filepath.Base(it.src), errText(err))
			}
			continue
		}
		done = append(done, it)
	}
	return done, firstErr
}

// ---------------------------------------------------------------------------
// links: - absolute, _ relative, ctrl+- hard link (made here, pointing at what's copied)
// ---------------------------------------------------------------------------

func (m *model) link(kind string) {
	if m.yank == nil {
		m.setNote("copy something first (y), then make a link to it here", true)
		return
	}
	var made []item
	for _, src := range m.yank.paths {
		dest := freeName(m.cwd, filepath.Base(src))
		var err error
		switch kind {
		case "hard":
			err = os.Link(src, dest)
		case "rel":
			target, rerr := filepath.Rel(m.cwd, src)
			if rerr != nil {
				target = src
			}
			err = os.Symlink(target, dest)
		default:
			err = os.Symlink(src, dest)
		}
		if err != nil {
			msg := errText(err)
			if kind != "hard" && strings.Contains(strings.ToLower(msg), "privilege") {
				msg = "Windows needs Developer Mode (or admin) for links"
			}
			m.setNote("couldn't link "+filepath.Base(src)+": "+msg, true)
			break
		}
		made = append(made, item{dest: dest})
	}
	if len(made) > 0 {
		m.pushUndo(undoStep{kind: "create", items: made})
		if !m.noteErr {
			m.setNote("made "+plural(len(made), "link"), false)
		}
		m.reload()
	}
}

// ---------------------------------------------------------------------------
// copy a path: cc path, cd folder, cf name, cn name without extension
// ---------------------------------------------------------------------------

func (m *model) copyPath(what string) {
	var parts []string
	for _, p := range m.targets() {
		switch what {
		case "d":
			parts = append(parts, filepath.Dir(p))
		case "f":
			parts = append(parts, filepath.Base(p))
		case "n":
			b := filepath.Base(p)
			parts = append(parts, strings.TrimSuffix(b, filepath.Ext(b)))
		default:
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return
	}
	text := strings.Join(parts, "\n")
	if !m.dump {
		if err := clipboard.WriteAll(text); err != nil {
			m.setNote("clipboard: "+err.Error(), true)
			return
		}
	}
	m.clearSel()
	m.setNote("copied "+trunc(strings.ReplaceAll(text, "\n", ", "), 60), false)
}

// ---------------------------------------------------------------------------
// undo
// ---------------------------------------------------------------------------

type undoStep struct {
	kind  string // "copy" / "create": remove what was made · "move": move back · "trash": restore
	items []item
}

var undoKind = map[taskKind]string{taskCopy: "copy", taskMove: "move", taskTrash: "trash", taskExtract: "copy", taskCompress: "copy"}

func (m *model) pushUndo(u undoStep) {
	m.undo = append(m.undo, u)
	if len(m.undo) > 50 {
		m.undo = m.undo[1:]
	}
}

func (m *model) undoLast() tea.Cmd {
	n := len(m.undo)
	if n == 0 {
		m.setNote("nothing to undo", false)
		return nil
	}
	u := m.undo[n-1]
	m.undo = m.undo[:n-1]
	switch u.kind {
	case "copy", "create":
		// what was made goes to the Recycle Bin (so even an undo can be undone from there)
		var its []item
		for _, it := range u.items {
			its = append(its, item{src: it.dest})
		}
		return m.startTask(&task{kind: taskTrash, items: its, isUndo: true})
	case "move":
		var its []item
		for i := len(u.items) - 1; i >= 0; i-- {
			its = append(its, item{src: u.items[i].dest, dest: u.items[i].src})
		}
		if len(its) == 1 && strings.EqualFold(filepath.Dir(its[0].src), filepath.Dir(its[0].dest)) {
			// a rename: do it right away
			if err := os.Rename(its[0].src, its[0].dest); err != nil {
				m.setNote("couldn't undo: "+errText(err), true)
				return nil
			}
			m.reload()
			m.moveToName(filepath.Base(its[0].dest))
			m.setNote("undone: "+filepath.Base(its[0].dest)+" has its old name", false)
			return nil
		}
		return m.startTask(&task{kind: taskMove, items: its, isUndo: true})
	case "trash":
		back := 0
		for _, it := range u.items {
			if err := restore(it.src); err != nil {
				m.setNote("couldn't restore "+filepath.Base(it.src)+": "+err.Error(), true)
				continue
			}
			back++
		}
		m.reload()
		if back > 0 {
			m.moveToName(filepath.Base(u.items[0].src))
			if !m.noteErr {
				m.setNote("restored "+plural(back, "item")+" from the Recycle Bin", false)
			}
		}
	}
	return nil
}
