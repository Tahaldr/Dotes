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
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	dir     string
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
	m.searchLabel = fmt.Sprintf("searching %s for %q", kind, q)
	m.searching, m.searchDir, m.searchMoved, m.searchReady, m.searchSpin = true, dir, false, nil, 0
	found := &atomic.Int64{}
	m.searchFound = found
	if cmd != nil {
		cmd.Dir = dir
		m.searchCmd = cmd
	}
	return tea.Batch(func() tea.Msg {
		res, capped, err := runSearch(cmd, dir, q, text, hidden, found)
		return searchDoneMsg{seq, dir, kind, q, res, err, capped}
	}, searchTick())
}

type searchTickMsg struct{}

func searchTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return searchTickMsg{} })
}

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// searchStatus: the header's "⠹ searching names for "x" · 120 found · ctrl+s stops"
func (m *model) searchStatus() string {
	s := spinFrames[m.searchSpin%len(spinFrames)] + " " + m.searchLabel
	if m.searchFound != nil {
		s += fmt.Sprintf(" · %d found", m.searchFound.Load())
	}
	if m.searchMoved {
		s += " in " + filepath.Base(m.searchDir)
	}
	return s + " · ctrl+s stops"
}

func (m *model) stopSearch() {
	if m.searchCmd != nil && m.searchCmd.Process != nil {
		m.searchCmd.Process.Kill()
	}
	m.searchCmd = nil
	m.searching = false
	m.searchSeq++ // what the killed search found so far is dropped
}

func runSearch(cmd *exec.Cmd, dir, q string, text, hidden bool, found *atomic.Int64) ([]entry, bool, error) {
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
				found.Add(1)
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
		found.Store(int64(len(out)))
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
		m.setNote("search failed: "+msg.err.Error(), true)
		return
	}
	if m.searchMoved { // you're busy elsewhere: don't pull you away
		m.searchReady = &msg
		m.setNote(fmt.Sprintf("search done: %d found · ctrl+s shows them", len(msg.results)), false)
		return
	}
	m.showResults(msg)
}

// showResults: the search's results become the list
func (m *model) showResults(msg searchDoneMsg) {
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

// pathSug: a suggestion under "Go to": a folder or file there, or a folder zoxide knows
type pathSug struct {
	name, path string
	isDir      bool
	where      string // zoxide's: the folder it's in (shown dim)
}

func (m *model) askGoto() {
	start := prettyPath(m.cwd)
	if !strings.HasSuffix(start, string(filepath.Separator)) {
		start += string(filepath.Separator)
	}
	m.input("Go to", "", start, -1, func(m *model, s string) tea.Cmd {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		m.jumpTo(m.expandPath(s))
		return nil
	})
	p := m.prompt
	p.sugSel = -1
	if m.live { // a word with no slash also finds the folders you use (zoxide)
		if _, err := exec.LookPath("zoxide"); err == nil {
			if b, err := exec.Command("zoxide", "query", "--list").Output(); err == nil {
				for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
					if l = strings.TrimSpace(l); l != "" {
						p.known = append(p.known, l)
					}
				}
			}
		}
	}
	p.onTab = func(m *model) { m.completePath() }
	p.onChange = func(m *model, s string) { m.suggestPaths() }
	m.suggestPaths()
}

const sugMax = 8

// suggestPaths: what's in the folder typed so far that starts with (then contains) the
// last part, folders first; a bare word also matches the folders zoxide knows
func (m *model) suggestPaths() {
	p := m.prompt
	text := string(p.text)
	sep := strings.LastIndexAny(text, `/\`)
	dirPart, leaf := "", text
	if sep >= 0 {
		dirPart, leaf = text[:sep+1], text[sep+1:]
	}
	p.sugs, p.sugSel, p.hint, p.sugFor = nil, -1, "", text
	dir := m.cwd
	if dirPart != "" {
		dir = m.expandPath(dirPart)
	}
	low := strings.ToLower(leaf)
	var starts, has []pathSug
	if des, err := os.ReadDir(dir); err == nil {
		for _, d := range des {
			n := d.Name()
			if !m.showHidden && strings.HasPrefix(n, ".") && !strings.HasPrefix(leaf, ".") {
				continue
			}
			full := filepath.Join(dir, n)
			s := pathSug{name: n, path: full, isDir: d.IsDir() || isDirPath(full)}
			switch ln := strings.ToLower(n); {
			case strings.HasPrefix(ln, low):
				starts = append(starts, s)
			case low != "" && strings.Contains(ln, low):
				has = append(has, s)
			}
		}
	}
	dirsFirst := func(l []pathSug) {
		sort.SliceStable(l, func(i, j int) bool { return l[i].isDir && !l[j].isDir })
	}
	dirsFirst(starts)
	dirsFirst(has)
	all := append(starts, has...)
	if sep < 0 && leaf != "" { // a bare word: the folders you use, by name
		seen := map[string]bool{}
		for _, s := range all {
			seen[strings.ToLower(s.path)] = true
		}
		for _, k := range p.known {
			if b := strings.ToLower(filepath.Base(k)); strings.Contains(b, low) && !seen[strings.ToLower(k)] {
				all = append(all, pathSug{name: filepath.Base(k), path: k, isDir: true, where: prettyPath(filepath.Dir(k))})
			}
		}
	}
	if len(all) == 0 && leaf != "" {
		p.hint = "nothing here starts with " + leaf
	}
	p.more = max(0, len(all)-sugMax)
	p.sugs = all[:min(len(all), sugMax)]
}

// completePath: tab takes the picked suggestion (the first if none): a folder gets its
// slash so the next tab goes on inside it
func (m *model) completePath() {
	p := m.prompt
	if p.sugFor != string(p.text) {
		m.suggestPaths()
	}
	if len(p.sugs) == 0 {
		return
	}
	s := p.sugs[max(0, p.sugSel)]
	text := string(p.text)
	dirPart := ""
	if sep := strings.LastIndexAny(text, `/\`); sep >= 0 {
		dirPart = text[:sep+1]
	}
	if s.where != "" { // zoxide's: the whole path
		text = prettyPath(s.path)
	} else {
		text = dirPart + s.name
	}
	if s.isDir {
		text += string(filepath.Separator)
	}
	p.text = []rune(text)
	p.pos = len(p.text)
	m.suggestPaths()
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
	m.searchMoved = true
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
