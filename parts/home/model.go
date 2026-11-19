package home

import (
	"fmt"
	"github.com/Tahaldr/dotes/internal/watch"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
	"github.com/Tahaldr/dotes/parts/git"
	"github.com/Tahaldr/dotes/parts/notes"
)

type zone struct {
	x0, x1, y0, y1 int // screen cells, inclusive
	idx            int // app index
}

// pzone: a glance panel (or one line of it) on screen
type pzone struct {
	zone
	part, path string
}

type model struct {
	cfg     config
	apps    []appItem // in the order shown (recent first when recent = on)
	sel     int
	perRow  int
	help    bool
	hintsOn bool
	note    string
	noteErr bool

	w, h        int
	now         time.Time
	prayer      *prayerData
	prayerRetry time.Time
	cwd         string
	cwdFile     string
	dump, live  bool
	zones       []zone
	watch       *watch.Watcher
	inWindow    bool // the project picker opens in the Files part (dotes h projects: its own program)

	// the glance at the other parts (only the picked ones)
	on     map[string]bool
	notesG *notes.GlanceInfo
	gitG   *git.GlanceInfo
	recent []string // recent projects
	branch string   // of the folder you're in
	pzones []pzone
	shown  []panel // the panels on screen now (for the keys)

	inPanels   bool // the keyboard is on the panels (space), not the cards
	psel       int  // which panel
	pline      int  // which line in it (-1 = the panel itself)
	ticks      int
	looking    bool // a glance is being gathered in the background
	glanceDump bool // --dump: gather it at once (tests leave it off: no git, no zoxide)
}

// glanceMsg: what the background look found
type glanceMsg struct {
	git    *git.GlanceInfo
	recent []string
	branch string
}

// look gathers the slow part of the glance (git in every repo, zoxide) in the background
func (m *model) look() tea.Cmd {
	if m.looking {
		return nil
	}
	m.looking = true
	on, cwd := m.on, m.cwd
	roots, search, skip := m.cfg.roots, m.cfg.search, m.cfg.skip
	return func() tea.Msg {
		out := glanceMsg{branch: gitBranch(cwd)}
		if on["git"] {
			g := git.Glance()
			out.git = &g
		}
		if on["files"] {
			mm := &model{cfg: config{roots: roots, search: search, skip: skip}}
			for _, d := range mm.projects() { // real projects only (zoxide also knows Documents, ...)
				if len(out.recent) < panelRows && isProject(d) {
					out.recent = append(out.recent, d)
				}
			}
		}
		return out
	}
}

// isProject: a folder with a project marker (.git, go.mod, package.json, ...)
func isProject(d string) bool {
	for _, mk := range markers {
		if _, err := os.Stat(filepath.Join(d, mk)); err == nil {
			return true
		}
	}
	return false
}

// lookNotes reads the notebook (fast: done right here)
func (m *model) lookNotes() {
	if m.on["notes"] && (!m.dump || notes.Ready()) { // tests: only with a test notebook
		g := notes.Glance(panelRows)
		m.notesG = &g
	}
}

type tickMsg struct{}
type ranMsg struct {
	name string
	err  error
}
type pickedMsg struct {
	out, cwdf, projf, openf string
	err                     error
}
type projectDoneMsg struct {
	dir string
	err error
}

func newModel() *model {
	m := &model{cfg: loadConfig(), hintsOn: app.LoadShared().Hints, now: time.Now()}
	m.cwd, _ = os.Getwd()
	m.orderApps()
	if m.cfg.prayer != nil {
		m.prayer = loadPrayer(m.cfg.prayer, m.now)
	}
	return m
}

// reloadSettings: home.txt, the logo, recent apps, the style or the hints changed outside
func (m *model) reloadSettings() {
	old := m.cfg.prayer
	m.cfg = loadConfig()
	m.hintsOn = app.LoadShared().Hints
	m.orderApps()
	m.sel = max(0, min(m.sel, len(m.apps)-1))
	if p := m.cfg.prayer; p != nil && (old == nil || prayerKey(p, m.now) != prayerKey(old, m.now)) {
		m.prayer = loadPrayer(p, m.now)
		m.prayerRetry = time.Time{} // another city: get its times on the next tick
	}
}

func (m *model) Init() tea.Cmd {
	m.lookNotes()
	if m.dump && m.glanceDump { // dotes --dump home: the glance right away
		if g, ok := m.look()().(glanceMsg); ok {
			m.Update(g)
		}
	}
	cmds := []tea.Cmd{m.tick()}
	if !m.dump {
		cmds = append(cmds, m.look())
	}
	if !m.dump {
		m.watch = watch.New("home")
		m.watch.Set(app.Dir())
		cmds = append(cmds, m.watch.Next())
	}
	if m.cfg.prayer != nil && m.prayer == nil && !m.dump {
		cmds = append(cmds, fetchPrayer(m.cfg.prayer))
	}
	return tea.Batch(cmds...)
}

// tick once a second: the clock and the prayer countdown
func (m *model) tick() tea.Cmd {
	if m.dump {
		return nil
	}
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

// ---- recent apps: home-recent.txt holds names, last used first ----

func recentFile() string { return app.File("home-recent.txt") }

func (m *model) orderApps() {
	picked := app.LoadShared().Parts
	m.on = map[string]bool{}
	for _, id := range picked {
		m.on[id] = true
	}
	m.apps = withParts(m.cfg.apps, picked)
	if !m.cfg.recent {
		return
	}
	b, _ := os.ReadFile(recentFile())
	order := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	rank := func(a appItem) int {
		if a.run == "" { // "back to the prompt" stays last
			return 2000
		}
		for i, n := range order {
			if n == a.name {
				return i
			}
		}
		return 1000
	}
	sort.SliceStable(m.apps, func(i, j int) bool { return rank(m.apps[i]) < rank(m.apps[j]) })
}

func remember(name string) {
	b, _ := os.ReadFile(recentFile())
	out := []string{name}
	for _, n := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if n != "" && n != name && len(out) < 20 {
			out = append(out, n)
		}
	}
	os.WriteFile(recentFile(), []byte(strings.Join(out, "\n")), 0o644)
}

// ---- update ----

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		m.now = time.Now()
		m.ticks++
		var cmd tea.Cmd
		if m.ticks%5 == 0 {
			m.lookNotes()
		}
		if m.ticks%60 == 0 {
			cmd = m.look()
		}
		if m.cfg.prayer != nil && (m.prayer == nil || m.prayer.Key != prayerKey(m.cfg.prayer, m.now)) &&
			m.now.After(m.prayerRetry) && !m.dump {
			m.prayerRetry = m.now.Add(5 * time.Minute) // one try at a time; offline: again in 5 minutes
			cmd = tea.Batch(cmd, fetchPrayer(m.cfg.prayer))
		}
		return m, tea.Batch(m.tick(), cmd)
	case glanceMsg:
		m.looking = false
		if msg.git != nil {
			m.gitG = msg.git
		}
		m.recent, m.branch = msg.recent, msg.branch
	case app.ShownMsg: // back on Home: fresh news from the other parts
		if msg.ID == "home" {
			m.lookNotes()
			return m, m.look()
		}
	case watch.Msg:
		if msg.Key != "home" {
			return m, nil
		}
		for _, p := range msg.Paths {
			if b := strings.ToLower(filepath.Base(p)); strings.HasPrefix(b, "home") && b != "home-prayer.json" || b == "dotes.txt" {
				m.reloadSettings()
				break
			}
		}
		return m, m.watch.Next()
	case prayerMsg:
		if msg.data != nil {
			m.prayer = msg.data
		}
	case ranMsg:
		if msg.err != nil {
			m.setNote(msg.name+" failed: "+msg.err.Error(), true)
		}
		m.cfg = loadConfig() // pick up settings edited meanwhile
		m.orderApps()
		m.sel = 0
	case pickedMsg:
		return m, m.picked(msg)
	case app.PickedMsg: // picked in the Files part: open it from here
		return m, tea.Sequence(app.Open("home", ""), m.openPicked(msg.Opener, msg.Path))
	case projectDoneMsg:
		if msg.err != nil {
			m.setNote(msg.err.Error(), true)
			return m, nil
		}
		m.writeCwd(msg.dir)
		return m, tea.Quit
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case tea.MouseClickMsg:
		return m, m.click(msg.X, msg.Y)
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp {
			m.move(-1)
		} else {
			m.move(1)
		}
	}
	return m, nil
}

// panelKey: the keyboard on the glance panels
func (m *model) panelKey(s string) tea.Cmd {
	n := len(m.shown)
	if n == 0 {
		m.inPanels = false
		return nil
	}
	m.psel = min(m.psel, n-1)
	p := m.shown[m.psel]
	switch s {
	case "tab", "right":
		m.psel, m.pline = (m.psel+1)%n, -1
	case "shift+tab", "left":
		m.psel, m.pline = (m.psel+n-1)%n, -1
	case "down":
		if m.pline+1 < len(p.lines) {
			m.pline++
		}
	case "up":
		if m.pline < 0 {
			m.inPanels = false // back up to the cards
		} else {
			m.pline--
		}
	case "space", "esc":
		m.inPanels = false
	case "enter":
		if p.part == "" {
			return nil
		}
		path := ""
		if m.pline >= 0 && m.pline < len(p.rowPaths) {
			path = p.rowPaths[m.pline]
		}
		return app.Open(p.part, path)
	case "q":
		return tea.Quit
	}
	return nil
}

func (m *model) setNote(s string, isErr bool) { m.note, m.noteErr = s, isErr }

func (m *model) count() int { return len(m.apps) }

func (m *model) move(d int) {
	if n := m.count(); n > 0 {
		m.sel = (m.sel + d + n) % n
	}
}

func (m *model) key(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	m.note = ""
	if m.help {
		switch s {
		case "h":
			m.hintsOn = !m.hintsOn
			if !m.dump {
				app.SetShared("hints", map[bool]string{true: "on", false: "off"}[m.hintsOn])
			}
		default:
			m.help = false
		}
		return nil
	}
	switch s {
	case "f1", "?", "~":
		m.help = true
		return nil
	case "x":
		app.ExitCode = app.ExitCloseTab // the shell function closes the terminal tab
		return tea.Quit
	case "ctrl+c":
		return tea.Quit
	}
	if m.inPanels {
		return m.panelKey(s)
	}
	n := len(m.apps)
	if s == "space" || (s == "down" && m.sel+m.perRow >= n && len(m.shown) > 0) { // to the panels
		if len(m.shown) > 0 {
			m.inPanels, m.pline = true, -1
		}
		return nil
	}
	rows := m.perRow > 0 && m.perRow < n // up / down jump a row of cards
	switch s {
	case "left", "shift+tab": // letters are app keys, so no h j k l here
		m.move(-1)
	case "right", "tab":
		m.move(1)
	case "up":
		if rows {
			if m.sel >= m.perRow {
				m.sel -= m.perRow
			}
		} else {
			m.move(-1)
		}
	case "down":
		if rows {
			m.sel = min(n-1, m.sel+m.perRow)
		} else {
			m.move(1)
		}
	case "enter":
		if m.sel < len(m.apps) {
			return m.open(m.apps[m.sel])
		}
	case "esc", "q":
		return tea.Quit
	default:
		for i, a := range m.apps {
			if a.key != "" && strings.EqualFold(s, a.key) {
				m.sel = i
				return m.open(a)
			}
		}
	}
	return nil
}

func (m *model) click(x, y int) tea.Cmd {
	if m.help {
		return nil
	}
	for _, z := range m.zones {
		if x >= z.x0 && x <= z.x1 && y >= z.y0 && y <= z.y1 {
			if m.sel == z.idx { // second click opens
				return m.open(m.apps[z.idx])
			}
			m.sel = z.idx
			return nil
		}
	}
	for i := len(m.pzones) - 1; i >= 0; i-- { // a line before its panel
		z := m.pzones[i]
		if x >= z.x0 && x <= z.x1 && y >= z.y0 && y <= z.y1 && z.part != "" {
			return app.Open(z.part, z.path)
		}
	}
	return nil
}

// ---- opening things ----

// partOf: "dotes f" / "dotes notes" → the part id ("" if it's not a part)
func partOfRun(run string) string {
	f := strings.Fields(run)
	if len(f) != 2 || f[0] != "dotes" {
		return ""
	}
	for _, p := range app.Parts {
		if f[1] == p.ID || f[1] == p.Short {
			return p.ID
		}
	}
	return ""
}

func (m *model) open(a appItem) tea.Cmd {
	switch {
	case a.run == "":
		return tea.Quit // back to the prompt
	case a.run == "projects":
		return m.pickProject()
	}
	if m.cfg.recent && !m.dump {
		remember(a.name)
	}
	if id := partOfRun(a.run); id != "" {
		return func() tea.Msg { return app.SwitchMsg{ID: id} }
	}
	if m.dump {
		m.setNote("would run: "+a.run, false)
		return nil
	}
	c := command(a.run, m.cwd)
	name := a.name
	return tea.ExecProcess(c, func(err error) tea.Msg { return ranMsg{name, err} })
}

var shellChars = "|&;<>()$`'\"{}*?"

// command runs what home.txt says: directly when it's a plain program, else through the shell
func command(run, dir string) *exec.Cmd {
	var c *exec.Cmd
	f := strings.Fields(run)
	if len(f) > 0 && f[0] == "dotes" {
		if exe, err := os.Executable(); err == nil {
			f[0] = exe
		}
	}
	if _, err := exec.LookPath(f[0]); err == nil && !strings.ContainsAny(run, shellChars) {
		c = exec.Command(f[0], f[1:]...)
	} else if runtime.GOOS == "windows" {
		sh := "pwsh"
		if _, err := exec.LookPath(sh); err != nil {
			sh = "powershell"
		}
		c = exec.Command(sh, "-NoLogo", "-Command", run)
	} else {
		c = exec.Command("sh", "-c", run)
	}
	c.Dir = dir
	return c
}

func (m *model) writeCwd(dir string) {
	if m.cwdFile != "" && dir != "" {
		os.WriteFile(m.cwdFile, []byte(dir), 0o644)
	}
}

// pickProject opens Files on your recent projects (in its own run, like TermNook's nook -p):
// enter opens the project in the first app, alt+key in another, q goes there in the shell.
func (m *model) pickProject() tea.Cmd {
	ow := m.cfg.openWith
	if len(ow) == 0 {
		m.setNote("project_open_with in home.txt has no apps", true)
		return nil
	}
	if m.dump && !m.inWindow {
		m.setNote("would open the project picker", false)
		return nil
	}
	if m.inWindow { // the Files part, in this window: instant
		pp := app.PickProjectsMsg{Projects: m.projects()}
		for _, o := range ow {
			pp.Openers = append(pp.Openers, [2]string{o.key, o.name})
		}
		return tea.Sequence(app.Open("files", ""), func() tea.Msg { return pp })
	}
	tmp := os.TempDir()
	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	p := pickedMsg{out: filepath.Join(tmp, "dotes-pick-"+id+".txt"), cwdf: filepath.Join(tmp, "dotes-pick-cwd-"+id+".txt"),
		projf: filepath.Join(tmp, "dotes-projects-"+id+".txt"), openf: filepath.Join(tmp, "dotes-openers-"+id+".txt")}
	os.WriteFile(p.projf, []byte(strings.Join(m.projects(), "\n")), 0o644)
	var ol []string
	for _, o := range ow {
		ol = append(ol, o.key+"\t"+o.name)
	}
	os.WriteFile(p.openf, []byte(strings.Join(ol, "\n")), 0o644)
	exe, err := os.Executable()
	if err != nil {
		m.setNote(err.Error(), true)
		return nil
	}
	c := exec.Command(exe, "f", "--pick", p.out, "--projects", p.projf, "--openers", p.openf, "--cwd-file", p.cwdf, m.cfg.search)
	return tea.ExecProcess(c, func(err error) tea.Msg { p.err = err; return p })
}

func (m *model) picked(p pickedMsg) tea.Cmd {
	defer func() {
		for _, f := range []string{p.out, p.cwdf, p.projf, p.openf} {
			os.Remove(f)
		}
	}()
	if p.err != nil {
		m.setNote("the project picker failed: "+p.err.Error(), true)
		return nil
	}
	b, err := os.ReadFile(p.out)
	if err != nil { // q in the picker: back to Home (dotes h projects quits then)
		return nil
	}
	idxS, target, _ := strings.Cut(string(b), "\t")
	idx, _ := strconv.Atoi(idxS)
	return m.openPicked(idx, target)
}

// openPicked opens a picked project with opener idx (project_open_with)
func (m *model) openPicked(idx int, target string) tea.Cmd {
	target = strings.TrimRight(strings.TrimSpace(target), `\/`)
	if target == "" || len(m.cfg.openWith) == 0 {
		return nil
	}
	o := m.cfg.openWith[min(max(0, idx), len(m.cfg.openWith)-1)]
	dir, file := target, ""
	if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
		dir, file = filepath.Dir(target), target
	}
	if _, err := exec.LookPath("zoxide"); err == nil { // rank it higher next time
		exec.Command("zoxide", "add", "--", dir).Run()
	}
	run := o.run
	if run == "" { // Shell: just go there
		m.writeCwd(dir)
		return tea.Quit
	}
	if file != "" && strings.HasSuffix(run, " .") { // "nvim ." opens the picked file
		run = strings.TrimSuffix(run, " .") + ` "` + file + `"`
	}
	if m.cfg.recent {
		remember(o.name)
	}
	c := command(run, dir)
	return tea.ExecProcess(c, func(err error) tea.Msg { return projectDoneMsg{dir, err} })
}

// projects: folders you visit often (zoxide) first, then projects found under project_roots
var markers = []string{".git", "package.json", "composer.json", "go.mod", "Cargo.toml", "pyproject.toml", "requirements.txt", "pubspec.yaml"}

func (m *model) projects() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if k := strings.ToLower(p); !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	h := home("")
	if _, err := exec.LookPath("zoxide"); err == nil {
		if b, err := exec.Command("zoxide", "query", "--list").Output(); err == nil {
			for _, p := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
				if len(p) > 3 && !strings.EqualFold(p, h) && isDir(p) {
					add(p)
				}
			}
		}
	}
	var found []string
	for _, r := range m.cfg.roots {
		scan(r, 0, &found)
	}
	sort.Strings(found)
	for _, p := range found {
		add(p)
	}
	return out
}

func scan(dir string, depth int, out *[]string) {
	for _, mk := range markers {
		if _, err := os.Stat(filepath.Join(dir, mk)); err == nil {
			*out = append(*out, dir)
			return
		}
	}
	if depth >= 4 {
		return
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		n := e.Name()
		if !e.IsDir() || strings.HasPrefix(n, ".") {
			continue
		}
		switch n {
		case "node_modules", "vendor", "dist", "build", "bin", "obj", "target":
			continue
		}
		scan(filepath.Join(dir, n), depth+1, out)
	}
}

// gitBranch reads .git/HEAD of the current folder or a parent (no git process: instant)
func gitBranch(dir string) string {
	for d := dir; d != ""; {
		if b, err := os.ReadFile(filepath.Join(d, ".git", "HEAD")); err == nil {
			ref := strings.TrimSpace(string(b))
			if r, ok := strings.CutPrefix(ref, "ref: refs/heads/"); ok {
				return r
			}
			return ref[:min(7, len(ref))]
		}
		p := filepath.Dir(d)
		if p == d {
			break
		}
		d = p
	}
	return ""
}

func prettyPath(p string) string {
	if h := home(""); h != "" && strings.HasPrefix(strings.ToLower(p), strings.ToLower(h)) {
		p = "~" + p[len(h):]
	}
	if r := []rune(p); len(r) > 32 {
		p = "…" + string(r[len(r)-31:])
	}
	return p
}

var _ = fmt.Sprint
