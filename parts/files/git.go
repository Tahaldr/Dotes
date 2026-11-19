package files

// Git marks in the list, no plugin needed: M changed, A added, ? new, D deleted, R renamed,
// U conflict, a dot on folders with changes inside, ignored files faded.

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type gitMsg struct {
	seq   int
	marks map[string]string // absolute path (lower case) → mark
}

// gitRefresh asks git again when the folder (or what's in it) changed
func (m *model) gitRefresh() tea.Cmd {
	if !m.gitStale {
		return nil
	}
	m.gitStale = false
	m.gitSeq++
	if m.branch == "" || m.virtual != "" {
		m.gitMarks = nil
		return nil
	}
	seq, dir := m.gitSeq, m.cwd
	return func() tea.Msg { return gitMsg{seq, gitStatus(dir)} }
}

func gitStatus(dir string) map[string]string {
	if _, err := exec.LookPath("git"); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	top, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel", "--show-prefix").Output()
	if err != nil {
		return nil
	}
	lines := strings.SplitN(strings.TrimRight(string(top), "\r\n"), "\n", 2)
	root := filepath.Clean(strings.TrimSpace(lines[0]))
	// the repo's top as this folder spells it: git resolves links and short names
	// (RUNNER~1, /var → /private/var), but the list shows the path you went in by
	base := filepath.Clean(dir)
	if len(lines) == 2 {
		for _, part := range strings.Split(strings.Trim(strings.TrimSpace(lines[1]), "/"), "/") {
			if part != "" {
				base = filepath.Dir(base)
			}
		}
	}
	out, err := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain=v1", "-z",
		"--ignored=matching", "--untracked-files=normal").Output()
	if err != nil {
		return nil
	}
	marks := map[string]string{}
	recs := bytes.Split(out, []byte{0})
	for i := 0; i < len(recs); i++ {
		r := string(recs[i])
		if len(r) < 4 {
			continue
		}
		xy, rel := r[:2], r[3:]
		if xy[0] == 'R' || xy[0] == 'C' {
			i++ // the old name comes next
		}
		abs := strings.ToLower(filepath.Join(base, filepath.FromSlash(strings.TrimSuffix(rel, "/"))))
		mark := gitMark(xy)
		marks[abs] = mark
		if mark == "!" {
			continue
		}
		// folders above a change get a dot
		for d := filepath.Dir(abs); len(d) > len(base) && strings.HasPrefix(d, strings.ToLower(base)); d = filepath.Dir(d) {
			if _, ok := marks[d]; !ok {
				marks[d] = "•"
			}
		}
	}
	return marks
}

func gitMark(xy string) string {
	switch {
	case xy == "??":
		return "?"
	case xy == "!!":
		return "!"
	case strings.ContainsRune(xy, 'U') || xy == "AA" || xy == "DD":
		return "U"
	case xy[1] != ' ':
		return string(xy[1])
	}
	return string(xy[0])
}

func gitMarkStyle(mk string) string {
	c := cYellow
	switch mk {
	case "A", "?":
		c = cGreen
	case "D", "U":
		c = cRed
	case "R", "C":
		c = cCyan
	case "•":
		c = cOrange
	}
	return fg(c).Render(mk)
}
