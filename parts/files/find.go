package files

// Finding and jumping: filter (f), find (/ ? n N), search names and text (s S with fd / rg),
// fzf (z) and zoxide (Z) jumps, typing a path (g space), following links (gf), drives,
// sorting (,) and line modes (m).

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// smartMatch: case-insensitive unless the query has a capital letter (like fd and rg)
func smartMatch(name, q string) bool {
	if q == "" {
		return true
	}
	for _, r := range q {
		if unicode.IsUpper(r) {
			return strings.Contains(name, q)
		}
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(q))
}

func filterEntries(list []entry, q string) []entry {
	if q == "" {
		return list
	}
	out := list[:0:0]
	for _, e := range list {
		if smartMatch(e.name, q) {
			out = append(out, e)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// f: filter, live as you type
// ---------------------------------------------------------------------------

func (m *model) askFilter() {
	old := m.filter
	m.input("Filter", "only names containing this · a capital letter makes it case-sensitive", old, -1, func(m *model, s string) tea.Cmd {
		m.filter = s
		m.reload()
		return nil
	})
	m.prompt.onChange = func(m *model, s string) { m.filter = s; m.reload(); m.cursor, m.top = 0, 0 }
	m.prompt.onCancel = func(m *model) { m.filter = old; m.reload() }
}

// ---------------------------------------------------------------------------
// / ? n N: find a name in this folder
// ---------------------------------------------------------------------------

func (m *model) askFind(back bool) {
	start, old := m.cursor, m.find
	title := "Find"
	if back {
		title = "Find backwards"
	}
	m.input(title, "jumps as you type · n / N for the next / previous one", "", -1, func(m *model, s string) tea.Cmd {
		m.find = s
		if s != "" && m.findNext(m.cursor, !back, true) < 0 {
			m.setNote("nothing matches "+s, true)
		}
		return nil
	})
	m.prompt.onChange = func(m *model, s string) {
		m.find = s
		m.cursor = start
		m.findNext(start, !back, true)
	}
	m.prompt.onCancel = func(m *model) { m.cursor, m.find = start, old }
}

// findNext moves to the next match after (or before) from; same = from itself may match
func (m *model) findNext(from int, forward, same bool) int {
	n := len(m.entries)
	if m.find == "" || n == 0 {
		return -1
	}
	step := 1
	if !forward {
		step = -1
	}
	i := from
	if !same {
		i += step
	}
	for k := 0; k < n; k++ {
		j := ((i+k*step)%n + n) % n
		if smartMatch(m.entries[j].name, m.find) {
			m.cursor, m.pscroll = j, 0
			return j
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// s S: search below this folder (fd for names, rg for text); results are a list you can act on
// ---------------------------------------------------------------------------

type searchDoneMsg struct {
	seq     int
	kind    string
	query   string
	results []entry
	err     error
	capped  bool
}

const searchMax = 3000

func (m *model) askSearch(text bool) {
	title, hint := "Search names", "everything below this folder whose name matches (fd)"
	if text {
		title, hint = "Search text", "every file below this folder containing this (ripgrep)"
	}
	m.input(title, hint, "", -1, func(m *model, s string) tea.Cmd {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		return m.startSearch(text, s)
	})
}

func (m *model) startSearch(text bool, q string) tea.Cmd {
	m.stopSearch()
	m.searchSeq++
	seq, dir, hidden := m.searchSeq, m.cwd, m.showHidden
	kind := "names"
	if text {
		kind = "text"
	}
	var cmd *exec.Cmd
	switch {
	case text:
		if _, err := exec.LookPath("rg"); err != nil {
			m.setNote("S needs ripgrep: scoop install ripgrep", true)
			return nil
		}
		args := []string{"--color=never", "--line-number", "--no-heading", "--max-count=1", "--max-columns=300", "--smart-case"}
		if hidden {
			args = append(args, "--hidden")
		}
		cmd = exec.Command("rg", append(args, "-e", q)...)
	default:
		if _, err := exec.LookPath("fd"); err == nil {
			args := []string{"--color=never", "--absolute-path"}
			if hidden {
				args = append(args, "--hidden")
			}
			cmd = exec.Command("fd", append(args, "--", q)...)
		}
	}
	m.vTitle = fmt.Sprintf("searching %s for %q…", kind, q)
	m.searching = true
	if cmd != nil {
		cmd.Dir = dir
		m.searchCmd = cmd
	}
	return func() tea.Msg {
		res, capped, err := runSearch(cmd, dir, q, text, hidden)
		return searchDoneMsg{seq, kind, q, res, err, capped}
	}
}

func (m *model) stopSearch() {
	if m.searchCmd != nil && m.searchCmd.Process != nil {
		m.searchCmd.Process.Kill()
	}
	m.searchCmd = nil
	m.searching = false
}

func runSearch(cmd *exec.Cmd, dir, q string, text, hidden bool) ([]entry, bool, error) {
	if cmd == nil { // no fd: walk the folder ourselves
		var out []entry
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || p == dir {
				return nil
			}
			if !hidden && strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if smartMatch(d.Name(), q) {
				out = append(out, resultEntry(dir, p, 0, ""))
			}
			if len(out) >= searchMax {
				return filepath.SkipAll
			}
			return nil
		})
		return out, len(out) >= searchMax, nil
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	var out []entry
	capped := false
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			continue
		}
		if text {
			// path:line:text
			parts := strings.SplitN(line, ":", 3)
			if len(parts) < 3 {
				continue
			}
			n, _ := strconv.Atoi(parts[1])
			out = append(out, resultEntry(dir, filepath.Join(dir, parts[0]), n, strings.TrimSpace(parts[2])))
		} else {
			out = append(out, resultEntry(dir, filepath.Clean(line), 0, ""))
		}
		if len(out) >= searchMax {
			capped = true
			cmd.Process.Kill()
			break
		}
	}
	err = cmd.Wait()
	if capped || len(out) > 0 {
		err = nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		err = nil // rg / fd: "nothing found"
	}
	if err != nil && stderr.Len() > 0 {
		err = fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}
	return out, capped, err
}

func resultEntry(dir, p string, line int, text string) entry {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		rel = p
	}
	e := entry{name: rel, path: p, matchLine: line, matchText: text}
	if fi, err := os.Lstat(p); err == nil {
		e.size, e.mod, e.mode, e.btime = fi.Size(), fi.ModTime(), fi.Mode(), fileBtime(fi)
		e.isDir = fi.IsDir()
		if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			e.isLink = true
			if t, err := os.Stat(p); err == nil && t.IsDir() {
				e.isDir = true
			}
		}
	}
	return e
}

func (m *model) searchDone(msg searchDoneMsg) {
	if msg.seq != m.searchSeq {
		return // an older search
	}
	m.searchCmd, m.searching = nil, false
	if msg.err != nil {
		m.vTitle = ""
		m.setNote("search failed: "+msg.err.Error(), true)
		return
	}
	if c := m.current(); c != nil && m.virtual == "" {
		m.memory[m.cwd] = c.name
	}
	m.virtual, m.vAll, m.filter = "search", msg.results, ""
	m.entries = msg.results
	m.vTitle = fmt.Sprintf("%s %q · %d found", map[string]string{"names": "names matching", "text": "files containing"}[msg.kind], msg.query, len(msg.results))
	if msg.capped {
		m.vTitle += fmt.Sprintf(" (first %d)", searchMax)
	}
	m.cursor, m.top = 0, 0
	m.parent, m.parentIdx = nil, -1
	m.moveScrollToMatch()
	if len(msg.results) == 0 {
		m.setNote("nothing found · h goes back", false)
	}
}

// moveScrollToMatch: a text search result previews around its matching line
func (m *model) moveScrollToMatch() {
	if c := m.current(); c != nil && c.matchLine > 0 {
		m.pscroll = max(0, c.matchLine-4)
	}
}

// ---------------------------------------------------------------------------
// z: fzf over the files below · Z: zoxide's folders · g space: type a path
// ---------------------------------------------------------------------------

type jumpMsg struct {
	path string
	err  error
}

func (m *model) fzfJump() tea.Cmd {
	if _, err := exec.LookPath("fzf"); err != nil {
		m.setNote("z needs fzf: scoop install fzf", true)
		return nil
	}
	if m.dump {
		return nil
	}
	var out bytes.Buffer
	c := exec.Command("fzf", "--height=100%", "--layout=reverse", "--prompt=jump ❯ ")
	c.Dir = m.cwd
	c.Stdout = &out
	if _, err := exec.LookPath("fd"); err == nil {
		fdCmd := "fd --color=never"
		if m.showHidden {
			fdCmd += " --hidden"
		}
		c.Env = append(os.Environ(), "FZF_DEFAULT_COMMAND="+fdCmd)
	}
	dir := m.cwd
	return tea.ExecProcess(c, func(err error) tea.Msg {
		p := strings.TrimSpace(out.String())
		if p == "" {
			return jumpMsg{}
		}
		return jumpMsg{path: filepath.Join(dir, p)}
	})
}

func (m *model) zoxideJump() tea.Cmd {
	if _, err := exec.LookPath("zoxide"); err != nil {
		m.setNote("Z needs zoxide: scoop install zoxide", true)
		return nil
	}
	if m.dump {
		return nil
	}
	var out bytes.Buffer
	c := exec.Command("zoxide", "query", "-i")
	c.Stdout = &out
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return jumpMsg{path: strings.TrimSpace(out.String())}
	})
}

var zoxideOnce sync.Once
var zoxidePath string

// zoxideAdd ranks folders you visit, so Z (and z in your shell) finds them
func (m *model) zoxideAdd(dir string) {
	if !m.live {
		return
	}
	zoxideOnce.Do(func() { zoxidePath, _ = exec.LookPath("zoxide") })
	if zoxidePath != "" {
		go exec.Command(zoxidePath, "add", "--", dir).Run()
	}
}

// jumpTo goes to a folder, or to a file's folder with the cursor on it
func (m *model) jumpTo(p string) {
	if p == "" {
		return
	}
	fi, err := os.Stat(p)
	if err != nil {
		m.setNote("can't find "+p, true)
		return
	}
	if fi.IsDir() {
		m.cd(p, "", true)
	} else {
		m.cd(filepath.Dir(p), filepath.Base(p), true)
	}
}

// expandPath: ~, %VAR% and $VAR, relative to the current folder
func (m *model) expandPath(s string) string {
	s = strings.TrimSpace(strings.Trim(s, `"`))
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) {
		home, _ := os.UserHomeDir()
		s = home + s[1:]
	}
	s = os.Expand(s, os.Getenv)
	for {
		i := strings.Index(s, "%")
		j := strings.Index(s[i+1:], "%")
		if i < 0 || j < 0 {
			break
		}
		name := s[i+1 : i+1+j]
		v := os.Getenv(name)
		if v == "" {
			break
		}
		s = s[:i] + v + s[i+2+j:]
	}
	if !filepath.IsAbs(s) {
		s = filepath.Join(m.cwd, s)
	}
	return filepath.Clean(s)
}

func (m *model) askGoto() {
	start := prettyPath(m.cwd)
	if !strings.HasSuffix(start, string(filepath.Separator)) {
		start += string(filepath.Separator)
	}
	m.input("Go to", "tab completes · ~, %APPDATA% and relative paths work", start, -1, func(m *model, s string) tea.Cmd {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		m.jumpTo(m.expandPath(s))
		return nil
	})
	m.prompt.onTab = func(m *model) { m.completePath() }
}

// completePath: tab in "Go to" finishes the folder name (or as much as all matches share)
func (m *model) completePath() {
	p := m.prompt
	text := string(p.text)
	sep := strings.LastIndexAny(text, `/\`)
	dirPart, leaf := "", text
	if sep >= 0 {
		dirPart, leaf = text[:sep+1], text[sep+1:]
	}
	dir := m.cwd
	if dirPart != "" {
		dir = m.expandPath(dirPart)
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var hits []string
	for _, d := range des {
		if !d.IsDir() && !isDirPath(filepath.Join(dir, d.Name())) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(d.Name()), strings.ToLower(leaf)) {
			if !m.showHidden && strings.HasPrefix(d.Name(), ".") && !strings.HasPrefix(leaf, ".") {
				continue
			}
			hits = append(hits, d.Name())
		}
	}
	switch len(hits) {
	case 0:
		p.hint = "no folder starts with " + leaf
		return
	case 1:
		text = dirPart + hits[0] + string(filepath.Separator)
		p.hint = ""
	default:
		common := hits[0]
		for _, h := range hits[1:] {
			for !strings.HasPrefix(strings.ToLower(h), strings.ToLower(common)) {
				common = common[:len(common)-1]
			}
		}
		text = dirPart + common
		if len(hits) > 8 {
			p.hint = strings.Join(hits[:8], "  ") + fmt.Sprintf("  … %d more", len(hits)-8)
		} else {
			p.hint = strings.Join(hits, "  ")
		}
	}
	p.text = []rune(text)
	p.pos = len(p.text)
}

// gf: go to where a link points
func (m *model) followLink() {
	c := m.current()
	if c == nil || !c.isLink {
		m.setNote("gf works on links (shown in italics)", true)
		return
	}
	target, err := filepath.EvalSymlinks(c.path)
	if err != nil {
		m.setNote("broken link: "+errText(err), true)
		return
	}
	m.jumpTo(target)
}

// ---------------------------------------------------------------------------
// drives: h at C:\ shows them all
// ---------------------------------------------------------------------------

func (m *model) showDrives() {
	cur := filepath.VolumeName(m.cwd)
	if c := m.current(); c != nil && m.virtual == "" {
		m.memory[m.cwd] = c.name
	}
	m.stopSearch()
	m.back = append(m.back, m.cwd)
	m.virtual, m.vTitle, m.filter = "drives", "This PC", ""
	m.entries = listDrives()
	m.parent, m.parentIdx = nil, -1
	m.cursor, m.top = 0, 0
	for i, d := range m.entries {
		if strings.EqualFold(d.name, cur) {
			m.cursor = i
		}
	}
}

// ---------------------------------------------------------------------------
// , sorting and m line modes
// ---------------------------------------------------------------------------

var sortNames = map[string]string{
	"natural": "name (2 before 10)", "alpha": "name (A to Z)", "mtime": "modified (newest first)",
	"btime": "created (newest first)", "ext": "extension", "size": "size (biggest first)", "random": "random",
}

func (m *model) setSort(key string) {
	by := map[string]string{"m": "mtime", "b": "btime", "e": "ext", "a": "alpha", "n": "natural", "s": "size", "r": "random"}
	k := strings.ToLower(key)
	b, ok := by[k]
	if !ok {
		return
	}
	m.sortBy, m.sortRev = b, key != k
	switch b { // like yazi, these show what they sort by
	case "mtime", "btime", "size":
		m.lineMode = b
	}
	m.reload()
	m.loadParent()
	note := "sorted by " + sortNames[b]
	if m.sortRev {
		note += ", reversed"
	}
	m.setNote(note, false)
}

func (m *model) setLineMode(key string) {
	mode := map[string]string{"s": "size", "p": "perm", "b": "btime", "m": "mtime", "o": "owner", "n": "none"}[key]
	if mode == "" {
		return
	}
	m.lineMode = mode
	m.setNote("rows show "+map[string]string{"size": "sizes", "perm": "permissions", "btime": "created dates",
		"mtime": "modified dates", "owner": "owners", "none": "nothing extra"}[mode], false)
}

// lineInfo: the text at the right end of a row
func lineInfo(e *entry, mode string) string {
	if e.drive {
		return e.label
	}
	switch mode {
	case "size":
		if e.matchLine > 0 {
			return ":" + strconv.Itoa(e.matchLine)
		}
		if !e.isDir {
			return shortSize(e.size)
		}
	case "perm":
		return e.mode.String()
	case "mtime":
		return shortTime(e.mod)
	case "btime":
		return shortTime(e.btime)
	case "owner":
		return fileOwner(e.path)
	}
	if e.isLink {
		return icLink
	}
	return ""
}
