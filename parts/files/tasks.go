package files

// Background jobs: copy, move, trash and delete run here so big folders never freeze the screen.
// `w` lists them with progress; `x` there cancels one.

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

type taskKind int

const (
	taskCopy taskKind = iota
	taskMove
	taskTrash
	taskDelete
	taskExtract
	taskCompress
)

var taskVerb = map[taskKind][2]string{
	taskCopy: {"Copying", "Copied"}, taskMove: {"Moving", "Moved"},
	taskTrash: {"Moving to Recycle Bin", "Moved to Recycle Bin"}, taskDelete: {"Deleting", "Deleted"},
	taskExtract: {"Extracting", "Extracted"}, taskCompress: {"Compressing", "Compressed"},
}

// one thing to do: src → dest (dest is empty for trash / delete)
type item struct{ src, dest string }

type task struct {
	id      int
	kind    taskKind
	items   []item
	force   bool   // P: overwrite what's there
	undoOK  bool   // add an undo step when it's done
	isUndo  bool   // this task is itself an undo
	label   string // "3 items" / "photo.png"
	archive string // compress: the archive being made
	total   atomic.Int64
	done    atomic.Int64
	cancel  atomic.Bool
	ended   atomic.Bool
	handled bool // the UI saw it end

	mu      sync.Mutex
	current string
	err     error
	made    []item // what actually happened (for undo)
}

var errCanceled = errors.New("canceled")

func (t *task) setCurrent(s string) { t.mu.Lock(); t.current = s; t.mu.Unlock() }
func (t *task) cur() string         { t.mu.Lock(); defer t.mu.Unlock(); return t.current }
func (t *task) addMade(it item)     { t.mu.Lock(); t.made = append(t.made, it); t.mu.Unlock() }

func (t *task) fail(err error) {
	t.mu.Lock()
	if t.err == nil {
		t.err = err
	}
	t.mu.Unlock()
}

func (t *task) title() string {
	v := taskVerb[t.kind]
	if t.ended.Load() {
		return v[1] + " " + t.label
	}
	return v[0] + " " + t.label
}

func (t *task) percent() int {
	tot := t.total.Load()
	if tot <= 0 {
		if t.ended.Load() {
			return 100
		}
		return 0
	}
	return int(min(100, t.done.Load()*100/tot))
}

type taskTickMsg struct{}

func taskTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return taskTickMsg{} })
}

// startTask runs t in the background (right away in test mode)
func (m *model) startTask(t *task) tea.Cmd {
	m.taskSeq++
	t.id = m.taskSeq
	if len(t.items) == 1 {
		t.label = filepath.Base(t.items[0].src)
	} else {
		t.label = fmt.Sprintf("%d items", len(t.items))
	}
	m.tasks = append(m.tasks, t)
	if len(m.tasks) > 30 {
		m.tasks = m.tasks[len(m.tasks)-30:]
	}
	if m.dump {
		runTask(t)
		m.finishTasks()
		return nil
	}
	go runTask(t)
	if m.ticking {
		return nil
	}
	m.ticking = true
	return taskTick()
}

func (m *model) running() int {
	n := 0
	for _, t := range m.tasks {
		if !t.ended.Load() {
			n++
		}
	}
	return n
}

// finishTasks: notes, undo steps and a reload for every task that just ended
func (m *model) finishTasks() {
	changed := false
	for _, t := range m.tasks {
		if !t.ended.Load() || t.handled {
			continue
		}
		t.handled, changed = true, true
		switch {
		case t.err == errCanceled:
			m.setNote(taskVerb[t.kind][0]+" "+t.label+" canceled", true)
		case t.err != nil:
			m.setNote(t.err.Error(), true)
		default:
			m.setNote(t.title(), false)
		}
		if t.undoOK && len(t.made) > 0 {
			m.pushUndo(undoStep{kind: undoKind[t.kind], items: t.made})
		}
	}
	if changed {
		m.reload()
		if m.pendingHover != "" {
			m.moveToName(m.pendingHover)
			m.pendingHover = ""
		}
	}
}

// ---------------------------------------------------------------------------
// the work (runs in its own goroutine)
// ---------------------------------------------------------------------------

func runTask(t *task) {
	defer t.ended.Store(true)
	switch t.kind {
	case taskExtract:
		if err := extract(t, t.items[0].src, t.items[0].dest); err != nil {
			t.fail(err)
			os.RemoveAll(t.items[0].dest)
			return
		}
		t.addMade(t.items[0])
	case taskCompress:
		if err := compress(t, t.items, t.archive); err != nil {
			t.fail(err)
			os.Remove(t.archive)
			return
		}
		t.addMade(item{dest: t.archive})
	case taskTrash:
		t.total.Store(int64(len(t.items)))
		for _, it := range t.items {
			if t.cancel.Load() {
				t.fail(errCanceled)
				return
			}
			t.setCurrent(it.src)
			if err := trash(it.src); err != nil {
				t.fail(fmt.Errorf("couldn't move %s to the Recycle Bin: %v", filepath.Base(it.src), err))
				return
			}
			t.addMade(it)
			t.done.Add(1)
		}
	case taskDelete:
		t.total.Store(int64(len(t.items)))
		for _, it := range t.items {
			if t.cancel.Load() {
				t.fail(errCanceled)
				return
			}
			t.setCurrent(it.src)
			if err := os.RemoveAll(it.src); err != nil {
				t.fail(fmt.Errorf("couldn't delete %s: %v", filepath.Base(it.src), errText(err)))
				return
			}
			t.done.Add(1)
		}
	default:
		var total int64
		for _, it := range t.items {
			total += treeSize(it.src)
		}
		t.total.Store(total)
		for _, it := range t.items {
			if t.cancel.Load() {
				t.fail(errCanceled)
				return
			}
			if err := checkInto(it.src, it.dest); err != nil {
				t.fail(err)
				return
			}
			var err error
			if t.kind == taskMove {
				err = moveOne(t, it)
			} else {
				err = copyTree(t, it.src, it.dest)
			}
			if err != nil {
				t.fail(err)
				return
			}
			t.addMade(it)
		}
	}
}

// checkInto stops "paste a folder into itself"
func checkInto(src, dest string) error {
	s := strings.ToLower(filepath.Clean(src)) + string(filepath.Separator)
	d := strings.ToLower(filepath.Clean(dest)) + string(filepath.Separator)
	if strings.HasPrefix(d, s) && d != s {
		return fmt.Errorf("can't put %s inside itself", filepath.Base(src))
	}
	return nil
}

func treeSize(p string) int64 {
	var n int64
	filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func moveOne(t *task, it item) error {
	if strings.EqualFold(filepath.Clean(it.src), filepath.Clean(it.dest)) {
		return nil
	}
	if t.force {
		if fi, err := os.Lstat(it.dest); err == nil && !fi.IsDir() {
			os.Remove(it.dest)
		}
	}
	t.setCurrent(it.src)
	if err := os.Rename(it.src, it.dest); err == nil {
		t.done.Add(treeSize(it.dest))
		return nil
	}
	// another drive (or the target folder exists): copy, then remove the original
	if err := copyTree(t, it.src, it.dest); err != nil {
		return err
	}
	if err := os.RemoveAll(it.src); err != nil {
		return fmt.Errorf("copied, but couldn't remove the original %s: %v", filepath.Base(it.src), errText(err))
	}
	return nil
}

// copyTree copies a file or a folder; links are copied as links, never followed
func copyTree(t *task, src, dest string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("can't read %s: %v", filepath.Base(p), errText(err))
		}
		if t.cancel.Load() {
			return errCanceled
		}
		rel, _ := filepath.Rel(src, p)
		to := filepath.Join(dest, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err == nil {
				err = os.Symlink(target, to)
			}
			if err != nil && !os.IsExist(err) {
				return fmt.Errorf("can't copy the link %s: %v", d.Name(), errText(err))
			}
			return nil
		case d.IsDir():
			if err := os.MkdirAll(to, info.Mode().Perm()|0o700); err != nil {
				return fmt.Errorf("can't create %s: %v", d.Name(), errText(err))
			}
			return nil
		case info.Mode()&fs.ModeIrregular != 0:
			return filepath.SkipDir // a junction: don't walk into it
		}
		t.setCurrent(p)
		return copyFile(t, p, to, info)
	})
}

func copyFile(t *task, src, dest string, info fs.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("can't read %s: %v", filepath.Base(src), errText(err))
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm()|0o600)
	if err != nil {
		return fmt.Errorf("can't write %s: %v", filepath.Base(dest), errText(err))
	}
	buf := make([]byte, 1<<20)
	for {
		if t.cancel.Load() {
			out.Close()
			os.Remove(dest)
			return errCanceled
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return fmt.Errorf("can't write %s: %v", filepath.Base(dest), errText(werr))
			}
			t.done.Add(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return fmt.Errorf("can't read %s: %v", filepath.Base(src), errText(rerr))
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	os.Chtimes(dest, info.ModTime(), info.ModTime())
	return nil
}

// freeName: "photo.png" → "photo (1).png" when the name is taken
func freeName(dir, name string) string {
	p := filepath.Join(dir, name)
	if _, err := os.Lstat(p); err != nil {
		return p
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if ext == name { // ".gitignore"
		stem, ext = name, ""
	}
	for i := 1; ; i++ {
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Lstat(p); err != nil {
			return p
		}
	}
}
