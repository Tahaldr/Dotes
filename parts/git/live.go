package git

// Live: Git redraws the moment a repo on this PC changes, whoever changed it (Nvim, another
// terminal, lazygit, a pull in VS Code). The open repo is watched folder by folder (the ones
// git knows: tracked or not ignored) plus its .git (index = staging, HEAD and refs =
// commits, branches, pulls); the other repos on this PC only at the top, for their badges.

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	fswatch "github.com/Tahaldr/dotes/internal/watch"
)

const maxWatched = 3000 // folders; bigger repos fall back to the 5-second check

// liveDirs: what to watch now
func (m *model) liveDirs() []string {
	var dirs []string
	seen := map[string]bool{}
	add := func(d string) {
		if k := strings.ToLower(filepath.Clean(d)); !seen[k] {
			seen[k] = true
			dirs = append(dirs, d)
		}
	}
	gitDirs := func(root string) {
		g := filepath.Join(root, ".git")
		add(g)
		for _, sub := range []string{"refs/heads", "refs/remotes/origin", "refs/tags"} {
			if fi, err := os.Stat(filepath.Join(g, filepath.FromSlash(sub))); err == nil && fi.IsDir() {
				add(filepath.Join(g, filepath.FromSlash(sub)))
			}
		}
	}
	if dir := m.chDir(); dir != "" {
		gitDirs(dir)
		for _, d := range m.treeDirs(dir) {
			if len(dirs) >= maxWatched {
				break
			}
			add(d)
		}
	}
	roots := make([]string, 0, len(m.local))
	for _, d := range m.local {
		roots = append(roots, d)
	}
	sort.Strings(roots)
	for _, r := range roots {
		if len(dirs) >= maxWatched {
			break
		}
		add(r)
		gitDirs(r)
	}
	return dirs
}

// treeDirs: the repo's folders that git cares about (cached until the repo changes)
func (m *model) treeDirs(root string) []string {
	if c := m.liveTree; c.root == root && time.Since(c.at) < time.Minute {
		return c.dirs
	}
	out, err := exec.Command("git", "-C", root, "ls-files", "-co", "--exclude-standard", "-z").Output()
	dirs := []string{root}
	if err == nil {
		seen := map[string]bool{"": true, ".": true}
		for _, f := range strings.Split(string(out), "\x00") {
			for d := filepath.Dir(filepath.FromSlash(f)); !seen[d]; d = filepath.Dir(d) {
				seen[d] = true
				dirs = append(dirs, filepath.Join(root, d))
				if len(dirs) >= maxWatched {
					break
				}
			}
		}
	}
	m.liveTree.root, m.liveTree.at, m.liveTree.dirs = root, time.Now(), dirs
	return dirs
}

// liveSync watches what's on screen now (cheap when nothing changed)
func (m *model) liveSync() {
	if m.fsw == nil {
		return
	}
	key := m.chDir() + "|" + strings.Repeat("·", len(m.local))
	if key == m.liveKey {
		return
	}
	m.liveKey = key
	m.fsw.Set(m.liveDirs()...)
}

// skipLive: changes that don't matter (git's own scratch files)
func skipLive(p string) bool {
	b := strings.ToLower(filepath.Base(p))
	return strings.HasSuffix(b, ".lock") || b == "objects" || b == "logs" || b == "fsmonitor--daemon" ||
		strings.HasSuffix(b, "~") || strings.HasSuffix(b, ".swp") || strings.HasPrefix(b, ".#")
}

// onDiskChange: something changed in a watched repo
func (m *model) onDiskChange(msg fswatch.Msg) tea.Cmd {
	next := m.fsw.Next()
	if m.prompt != nil || m.palette { // you're in the middle of something: after it
		m.liveLater = true
		return next
	}
	changed := map[string]bool{}
	for _, p := range msg.Paths {
		for _, d := range m.local {
			if pre := strings.ToLower(filepath.Clean(d)) + string(filepath.Separator); strings.HasPrefix(strings.ToLower(p), pre) {
				changed[d] = true
			}
		}
		if strings.Contains(strings.ToLower(p), string(filepath.Separator)+".git") && filepath.Base(filepath.Dir(p)) != ".git" {
			m.liveTree.at = time.Time{} // a branch switch can bring new folders
		}
	}
	return tea.Batch(next, m.liveRefresh(changed))
}

// liveRefresh reloads what the change touched: Changes, commits, branches and the badges
func (m *model) liveRefresh(dirs map[string]bool) tea.Cmd {
	m.liveLater = false
	for d := range dirs {
		m.chForget(d)
	}
	if len(dirs) == 0 {
		for d := range m.changes {
			m.chForget(d)
		}
	}
	m.liveKey = "" // folders may have come or gone
	cmd := m.scanLocalInfo()
	if dir := m.chDir(); dir != "" && (len(dirs) == 0 || dirs[dir]) {
		cmd = tea.Batch(cmd, m.exLoad())
	}
	return cmd
}
