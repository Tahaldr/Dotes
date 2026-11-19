// Package watch tells a part when files change on disk (saved in Nvim, made in another
// window, a git commit...), so it can redraw at once instead of waiting for a timer.
//
// A Watcher watches a set of folders (not their subfolders). Changes that come close together
// are joined: the part gets one Msg about 150ms after the last one.
//
//	w := watch.New("files")
//	w.Set(cwd)            // what to watch now (replaces the old set)
//	return w.Next()       // a tea.Cmd: waits for the next change → watch.Msg{Key: "files"}
//
// When the system can't watch (too many folders, a network drive), Set returns false and
// the part keeps polling.
package watch

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

// Msg: something changed in a watched folder. Paths are the changed files (at most 50).
type Msg struct {
	Key   string
	Paths []string
}

type Watcher struct {
	key  string
	mu   sync.Mutex
	fw   *fsnotify.Watcher
	dirs map[string]bool
	out  chan Msg
	skip func(path string) bool
}

const settle = 150 * time.Millisecond

// New makes a watcher; nil when the system can't watch at all.
func New(key string) *Watcher {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil
	}
	w := &Watcher{key: key, fw: fw, dirs: map[string]bool{}, out: make(chan Msg, 1)}
	go w.loop()
	return w
}

// Skip ignores changes to paths it says yes to (lock files, .git/objects ...).
func (w *Watcher) Skip(f func(path string) bool) {
	if w != nil {
		w.skip = f
	}
}

// Set watches exactly these folders. false = some couldn't be watched.
func (w *Watcher) Set(dirs ...string) bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	want := map[string]bool{}
	for _, d := range dirs {
		if d != "" {
			want[filepath.Clean(d)] = true
		}
	}
	for d := range w.dirs {
		if !want[d] {
			w.fw.Remove(d)
			delete(w.dirs, d)
		}
	}
	ok := true
	for d := range want {
		if w.dirs[d] {
			continue
		}
		if err := w.fw.Add(d); err != nil {
			ok = false
			continue
		}
		w.dirs[d] = true
	}
	return ok
}

// Watching: how many folders are watched now.
func (w *Watcher) Watching() int {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.dirs)
}

// Next waits for the next change. Run it again after each Msg (like a tick).
func (w *Watcher) Next() tea.Cmd {
	if w == nil {
		return nil
	}
	return func() tea.Msg { return <-w.out }
}

// Close stops watching.
func (w *Watcher) Close() {
	if w != nil {
		w.fw.Close()
	}
}

func (w *Watcher) loop() {
	var timer *time.Timer
	var fire <-chan time.Time
	var paths []string
	seen := map[string]bool{}
	for {
		select {
		case ev, ok := <-w.fw.Events:
			if !ok {
				return
			}
			if ev.Op == fsnotify.Chmod || (w.skip != nil && w.skip(ev.Name)) {
				continue
			}
			if k := strings.ToLower(ev.Name); !seen[k] && len(paths) < 50 {
				seen[k] = true
				paths = append(paths, ev.Name)
			}
			if timer == nil {
				timer = time.NewTimer(settle)
			} else {
				timer.Reset(settle)
			}
			fire = timer.C
		case <-fire:
			fire = nil
			msg := Msg{Key: w.key, Paths: paths}
			paths, seen = nil, map[string]bool{}
			select { // the part hasn't taken the last one yet: it'll see this change then
			case w.out <- msg:
			default:
			}
		case _, ok := <-w.fw.Errors:
			if !ok {
				return
			}
		}
	}
}
