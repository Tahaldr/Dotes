package files

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
)

type model struct {
	w, h int
	dump bool // headless test mode: no timers, no programs started

	cwd         string
	entries     []entry
	hiddenN     int
	readErr     string
	cursor, top int
	parent      []entry
	parentIdx   int
	showHidden  bool
	memory      map[string]string // folder → name the cursor was on
	back, fwd   []string          // H / L history
	branch      string
	pending     string // first key of gg, gh, gd, ...
	note        string // one-line message in the status bar
	noteErr     bool

	pv      preview
	pscroll int
	cache   map[string]preview

	help       bool
	helpScroll int

	cwdFile   string
	lastClick time.Time
	clickIdx  int
	quitting  bool

	// batch 2: file actions
	sel          map[string]bool // selected paths (kept across folders)
	visual       int             // visNone / visSel / visUnset
	vAnchor      int
	yank         *yankState
	tasks        []*task
	taskSeq      int
	ticking      bool
	showTasks    bool
	taskCur      int
	undo         []undoStep
	prompt       *prompt
	question     *question
	pendingHover string // put the cursor here once a paste is done

	// batch 3: find and jump
	live      bool   // running in a real terminal (tests never touch zoxide)
	virtual   string // "" = a real folder, "search" or "drives"
	vTitle    string // header text of a virtual list
	filter    string // f: only names containing this
	find      string // / and ?: what n / N look for
	sortBy    string
	sortRev   bool
	lineMode  string // what the right side of each row shows
	searchCmd *exec.Cmd
	searching bool
	vAll      []entry // every search result (the filter picks from these)
	searchSeq int
	bookmarks []string
	showMarks bool
	markCur   int
	hintsOn   bool // the key hint bar (h on the ~ page hides it)

	// batch 4: tabs, git, file info
	tabs     []tabSnap // empty = just one tab
	tabIdx   int
	gitMarks map[string]string
	gitSeq   int
	gitStale bool
	spot     *spotState

	// real images (gfx.go)
	gfx                 string // "blocks" or "sixel"
	cellW, cellH        int    // a cell in pixels
	imgWant, imgShown   imgSpot
	sixelKey, sixelData string
	lastImgKey          string
	imgFlip             bool
	gfxForced           bool // images = blocks in config.txt

	// batch 5
	pick   pickArgs   // --pick: chosen from TermNook's nook -p
	hidden bool       // another Dotes part is on screen
	menu   *menuState // open-with menu

	// filled by View, used by the mouse
	colX  [3][2]int // x start / end of parent, current, preview columns
	bodyY int       // first row of the columns
	bodyH int
}

type previewMsg struct{ p preview }
type tickMsg struct{}
type editorDoneMsg struct{ err error }

func newModel(start string) *model {
	m := &model{memory: map[string]string{}, cache: map[string]preview{}, clickIdx: -1, sel: map[string]bool{},
		sortBy: "natural", lineMode: "size", bookmarks: loadBookmarks(), hintsOn: true,
		gfx: "blocks", cellW: 10, cellH: 20}
	if cfg.sortBy != "" { // settings from config.txt (tests run without them)
		m.showHidden, m.sortBy, m.sortRev, m.lineMode, m.hintsOn = cfg.hidden, cfg.sortBy, cfg.sortRev, cfg.lineMode, cfg.hints
	}
	if abs, err := filepath.Abs(start); err == nil {
		start = abs
	}
	if fi, err := os.Stat(start); err == nil && !fi.IsDir() {
		m.cd(filepath.Dir(start), filepath.Base(start), false)
	} else {
		m.cd(start, "", false)
	}
	if m.cwd == "" { // couldn't open it: start in the current folder, or home (the note says why)
		if wd, err := os.Getwd(); err == nil {
			m.cd(wd, "", false)
		}
		if m.cwd == "" {
			home, _ := os.UserHomeDir()
			m.cd(home, "", false)
		}
	}
	return m
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.previewCmd(), m.tick(), m.gitRefresh(), m.detectGfx())
}

func (m *model) tick() tea.Cmd {
	if m.dump {
		return nil
	}
	return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

// ---------------------------------------------------------------------------
// folders
// ---------------------------------------------------------------------------

func (m *model) current() *entry {
	if m.cursor >= 0 && m.cursor < len(m.entries) {
		return &m.entries[m.cursor]
	}
	return nil
}

// readShaped reads a folder with the current sort and filter applied
func (m *model) readShaped(dir string, filter string) ([]entry, int, error) {
	list, n, err := readDir(dir, m.showHidden)
	if err != nil {
		return nil, 0, err
	}
	if m.sortBy != "natural" || m.sortRev {
		sortEntries(list, m.sortBy, m.sortRev)
	}
	return filterEntries(list, filter), n, nil
}

// linkDir: the folder under the cursor, or the one we're in
func (m *model) linkDir() string {
	if c := m.current(); c != nil && c.isDir && !c.drive && m.virtual == "" {
		return c.path
	}
	if c := m.current(); c != nil && m.virtual != "" { // search results: the file's folder
		if c.isDir {
			return c.path
		}
		return filepath.Dir(c.path)
	}
	return m.cwd
}

// openInGit (gi): the repo this folder is in, in the Git part
func (m *model) openInGit() tea.Cmd {
	dir := m.linkDir()
	root := app.RepoRoot(dir)
	switch {
	case root == "":
		m.setNote(prettyPath(dir)+" isn't in a git repo", true)
		return nil
	case !app.OnGitHub(root):
		m.setNote(filepath.Base(root)+" has no GitHub remote (origin), so Git can't open it", true)
		return nil
	}
	return app.Open("git", root)
}

// openNotes (gn): the project's note in Notes (made the first time)
func (m *model) openNotes() tea.Cmd {
	dir := m.linkDir()
	if root := app.RepoRoot(dir); root != "" {
		dir = root
	}
	return app.Open("notes", dir)
}

// cd goes to dir and puts the cursor on hover (or on where it was last time)
func (m *model) cd(dir, hover string, record bool) {
	dir = filepath.Clean(dir)
	filter := m.filter
	if dir != m.cwd || m.virtual != "" {
		filter = "" // a filter belongs to one folder
	}
	list, n, err := m.readShaped(dir, filter)
	if err != nil {
		m.setNote("can't open "+prettyPath(dir)+": "+errText(err), true)
		return
	}
	if m.cwd != "" {
		if c := m.current(); c != nil && m.virtual == "" {
			m.memory[m.cwd] = c.name
		}
		if record && (m.cwd != dir || m.virtual != "") {
			m.back = append(m.back, m.cwd)
			m.fwd = nil
		}
	}
	m.stopSearch()
	m.virtual, m.vTitle, m.filter = "", "", filter
	m.cwd, m.entries, m.hiddenN, m.readErr = dir, list, n, ""
	if hover == "" {
		hover = m.memory[dir]
	}
	m.cursor, m.top = 0, 0
	m.moveToName(hover)
	m.branch = gitBranch(dir)
	m.loadParent()
	m.pscroll = 0
	m.gitStale = true
	m.zoxideAdd(dir)
}

func (m *model) moveToName(name string) {
	for i, e := range m.entries {
		if e.name == name {
			m.cursor = i
			return
		}
	}
}

func (m *model) loadParent() {
	m.parent, m.parentIdx = nil, -1
	if m.virtual != "" {
		return
	}
	if isRoot(m.cwd) {
		m.parent = listDrives() // the drives, with this one marked
		for i, d := range m.parent {
			if strings.EqualFold(d.path, m.cwd) {
				m.parentIdx = i
			}
		}
		return
	}
	pdir := filepath.Dir(m.cwd)
	list, _, err := m.readShaped(pdir, "")
	if err != nil {
		return
	}
	m.parent = list
	base := filepath.Base(m.cwd)
	for i, e := range list {
		if e.name == base {
			m.parentIdx = i
		}
	}
}

// reload reads the folder again (after a change, or the hidden toggle), keeping the cursor on the same name
func (m *model) reload() {
	name := ""
	if c := m.current(); c != nil {
		name = c.name
	}
	old := m.cursor
	switch m.virtual {
	case "drives":
		m.entries = listDrives()
	case "search", "projects": // keep the results that still exist
		var keep []entry
		for _, e := range m.vAll {
			if fi, err := os.Lstat(e.path); err == nil {
				e.size, e.mod = fi.Size(), fi.ModTime()
				keep = append(keep, e)
			}
		}
		m.vAll = keep
		m.entries = filterEntries(keep, m.filter)
	default:
		list, n, err := m.readShaped(m.cwd, m.filter)
		if err != nil {
			m.readErr = errText(err)
			return
		}
		m.entries, m.hiddenN = list, n
	}
	m.cursor = min(old, max(0, len(m.entries)-1))
	m.moveToName(name)
	m.loadParent()
	m.gitStale = true
}

// sameList: did the folder change on disk?
func sameList(a, b []entry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name || a[i].size != b[i].size || !a[i].mod.Equal(b[i].mod) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// preview
// ---------------------------------------------------------------------------

func (m *model) previewCmd() tea.Cmd {
	c := m.current()
	if c == nil {
		m.pv = preview{}
		return nil
	}
	key := previewKey(*c, m.showHidden)
	if m.pv.key == key {
		return nil
	}
	if p, ok := m.cache[key]; ok {
		m.pv = p
		return nil
	}
	e, hidden := *c, m.showHidden
	return func() tea.Msg { return previewMsg{makePreview(e, hidden)} }
}

// ---------------------------------------------------------------------------
// update
// ---------------------------------------------------------------------------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.gfxMsg(msg) {
		return m, m.imgSyncLater()
	}
	if o, ok := msg.(app.OpenMsg); ok { // another part sent us somewhere
		m.jumpTo(o.Path)
		return m, m.imgSyncLater()
	}
	if s, ok := msg.(app.ShownMsg); ok {
		m.hidden = s.ID != "files"
		m.imgShown = imgSpot{} // the screen was redrawn: put the picture back when shown
		return m, m.imgSyncLater()
	}
	switch msg.(type) {
	case imgSyncMsg:
		return m, m.imgSync()
	case tea.WindowSizeMsg:
		m.imgShown = imgSpot{} // the whole screen gets drawn again
	}
	mm, cmd := m.update(msg)
	cmd = tea.Batch(cmd, m.imgSyncLater())
	if g := m.gitRefresh(); g != nil {
		cmd = tea.Batch(cmd, g)
	}
	return mm, cmd
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case previewMsg:
		if len(m.cache) > 300 {
			m.cache = map[string]preview{}
		}
		m.cache[msg.p.key] = msg.p
		if c := m.current(); c != nil && previewKey(*c, m.showHidden) == msg.p.key {
			m.pv = msg.p
		}
		return m, nil
	case tickMsg:
		if m.virtual != "" || m.sortBy == "random" {
			return m, m.tick()
		}
		if list, n, err := m.readShaped(m.cwd, m.filter); err == nil && (!sameList(list, m.entries) || n != m.hiddenN) {
			m.reload()
			return m, tea.Batch(m.previewCmd(), m.tick())
		}
		return m, m.tick()
	case editorDoneMsg:
		if msg.err != nil {
			m.setNote("editor: "+msg.err.Error(), true)
		}
		m.reload()
		return m, m.previewCmd()
	case bulkDoneMsg:
		m.bulkDone(msg)
		return m, m.previewCmd()
	case taskTickMsg:
		m.finishTasks()
		if m.running() > 0 {
			return m, tea.Batch(taskTick(), m.previewCmd())
		}
		m.ticking = false
		return m, m.previewCmd()
	case tea.PasteMsg:
		if p := m.prompt; p != nil {
			m.insertText(msg.Content)
			if p.onChange != nil {
				p.onChange(m, string(p.text))
			}
		}
		return m, m.previewCmd()
	case gitMsg:
		if msg.seq == m.gitSeq {
			m.gitMarks = msg.marks
		}
		return m, nil
	case spotSizeMsg:
		m.spotSize(msg)
		return m, nil
	case searchDoneMsg:
		m.searchDone(msg)
		return m, m.previewCmd()
	case jumpMsg:
		m.jumpTo(msg.path)
		return m, m.previewCmd()
	case tea.KeyPressMsg:
		cmd := m.key(msg)
		if m.quitting {
			return m, tea.Quit
		}
		return m, tea.Batch(cmd, m.previewCmd())
	case tea.MouseClickMsg:
		cmd := m.click(msg)
		return m, tea.Batch(cmd, m.previewCmd())
	case tea.MouseWheelMsg:
		m.wheel(msg)
		return m, m.previewCmd()
	}
	return m, nil
}

func (m *model) setNote(s string, isErr bool) { m.note, m.noteErr = s, isErr }

func (m *model) move(d int) {
	if len(m.entries) == 0 {
		return
	}
	m.cursor = max(0, min(len(m.entries)-1, m.cursor+d))
	m.pscroll = 0
	m.moveScrollToMatch()
}

func (m *model) half() int { return max(1, m.bodyH/2) }
func (m *model) page() int { return max(1, m.bodyH) }

func (m *model) key(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	switch {
	case m.question != nil:
		return m.questionKey(k)
	case m.prompt != nil:
		return m.promptKey(k)
	case m.showTasks:
		m.tasksKey(k)
		return nil
	case m.showMarks:
		m.marksKey(k)
		return nil
	case m.spot != nil:
		m.spotKey(k)
		return nil
	case m.menu != nil:
		return m.menuKey(k)
	}
	m.note = ""

	if m.help {
		switch s {
		case "h":
			m.toggleHints()
		case "e":
			m.help = false
			if m.dump {
				return nil
			}
			return m.editFile(entry{name: "config.txt", path: configFile()}, 0)
		case "~", "f1", "esc", "q":
			m.help = false
		case "j", "down":
			m.helpScroll++
		case "k", "up":
			m.helpScroll = max(0, m.helpScroll-1)
		case "ctrl+d", "pgdown":
			m.helpScroll += m.half()
		case "ctrl+u", "pgup":
			m.helpScroll = max(0, m.helpScroll-m.half())
		case "g", "home":
			m.helpScroll = 0
		}
		return nil
	}

	if m.pending != "" {
		p := m.pending
		m.pending = ""
		if p == "g" {
			home, _ := os.UserHomeDir()
			switch s {
			case "g":
				m.cursor, m.pscroll = 0, 0
			case "h":
				m.cd(home, "", true)
			case "d":
				m.cd(filepath.Join(home, "Downloads"), "", true)
			case "D":
				m.cd(filepath.Join(home, "Documents"), "", true)
			case "c":
				m.cd(configDir(), "", true)
			case "space", " ":
				m.askGoto()
			case "f":
				m.followLink()
			case "p":
				m.showProjects()
			case "i":
				return m.openInGit()
			case "n":
				return m.openNotes()
			case "t":
				if !m.dump {
					if err := openRecycleBin(); err != nil {
						m.setNote(err.Error(), true)
					}
				}
			}
		}
		if p == "," {
			m.setSort(s)
		}
		if p == "t" {
			switch s {
			case "t":
				m.newTab()
			case "r":
				m.renameTab()
			}
		}
		if p == "m" {
			m.setLineMode(s)
		}
		if p == "c" && (s == "c" || s == "d" || s == "f" || s == "n") {
			m.copyPath(s)
		}
		return nil
	}

	if m.pickKey(s) {
		return nil
	}
	switch s {
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "ctrl+d", "shift+pgdown":
		m.move(m.half())
	case "ctrl+u", "shift+pgup":
		m.move(-m.half())
	case "ctrl+f", "pgdown":
		m.move(m.page())
	case "ctrl+b", "pgup":
		m.move(-m.page())
	case "G", "end":
		m.move(len(m.entries))
	case "home":
		m.move(-len(m.entries))
	case "g":
		m.pending = "g"
	case "h", "left":
		m.leave()
	case "l", "right":
		if c := m.current(); c != nil {
			if c.isDir {
				m.cd(c.path, "", true)
			} else {
				return m.open(*c)
			}
		}
	case "enter", "o":
		if c := m.current(); c != nil {
			return m.open(*c)
		}
	case "H":
		if n := len(m.back); n > 0 {
			dest := m.back[n-1]
			m.back = m.back[:n-1]
			m.fwd = append(m.fwd, m.cwd)
			m.cd(dest, "", false)
		}
	case "L":
		if n := len(m.fwd); n > 0 {
			dest := m.fwd[n-1]
			m.fwd = m.fwd[:n-1]
			m.back = append(m.back, m.cwd)
			m.cd(dest, "", false)
		}
	case "J":
		m.pscroll += 5
	case "K":
		m.pscroll = max(0, m.pscroll-5)
	case ".":
		m.showHidden = !m.showHidden
		m.reload()
		if m.showHidden {
			m.setNote("showing hidden files", false)
		} else {
			m.setNote("hidden files hidden", false)
		}
	case "~", "f1":
		m.help, m.helpScroll = true, 0

	// select
	case "space", " ":
		if c := m.current(); c != nil {
			m.commitVisual()
			if m.sel[c.path] {
				delete(m.sel, c.path)
			} else {
				m.sel[c.path] = true
			}
			m.move(1)
		}
	case "ctrl+a":
		m.commitVisual()
		for _, e := range m.entries {
			m.sel[e.path] = true
		}
	case "ctrl+r":
		m.commitVisual()
		for _, e := range m.entries {
			if m.sel[e.path] {
				delete(m.sel, e.path)
			} else {
				m.sel[e.path] = true
			}
		}
	case "v", "V":
		mode := visSel
		if s == "V" {
			mode = visUnset
		}
		if m.visual == mode {
			m.commitVisual()
		} else {
			m.commitVisual()
			m.visual, m.vAnchor = mode, m.cursor
		}
	case "esc":
		switch {
		case m.visual != visNone:
			m.commitVisual()
		case len(m.sel) > 0:
			m.clearSel()
		case m.searching:
			m.stopSearch()
			m.vTitle = ""
		case m.filter != "":
			m.filter = ""
			m.reload()
		case m.find != "":
			m.find = ""
		case m.virtual == "search":
			m.leave()
		}

	// find and jump
	case "f":
		m.askFilter()
	case "/":
		m.askFind(false)
	case "?":
		m.askFind(true)
	case "n":
		if m.findNext(m.cursor, true, false) < 0 && m.find != "" {
			m.setNote("nothing matches "+m.find, true)
		}
	case "N":
		if m.findNext(m.cursor, false, false) < 0 && m.find != "" {
			m.setNote("nothing matches "+m.find, true)
		}
	case "s":
		m.askSearch(false)
	case "S":
		m.askSearch(true)
	case "ctrl+s":
		if m.searching {
			m.stopSearch()
			m.vTitle = ""
			m.setNote("search stopped", false)
		}
	case "z":
		return m.fzfJump()
	case "Z":
		return m.zoxideJump()
	case ",", "m", "t":
		m.pending = s
	case "tab":
		return m.openSpot()
	case "O", "shift+enter":
		m.openWithMenu()
	case ";":
		m.askShell(false)
	case ":":
		m.askShell(true)
	case "e":
		return m.askExtract()
	case "C":
		m.askCompress()
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		m.switchTab(int(s[0] - '1'))
	case "[":
		m.switchTab(m.tabIdx - 1)
	case "]":
		m.switchTab(m.tabIdx + 1)
	case "{":
		m.swapTab(-1)
	case "}":
		m.swapTab(1)
	case "b":
		m.toggleBookmark()
	case "'":
		m.showMarks, m.markCur = true, 0

	// files
	case "y":
		m.yankTargets(false)
	case "x":
		m.yankTargets(true)
	case "Y", "X":
		if m.yank != nil {
			m.yank = nil
			m.setNote("copy / cut canceled", false)
		}
	case "p":
		return m.paste(false)
	case "P":
		return m.paste(true)
	case "d", "delete":
		m.askTrash(false)
	case "D", "shift+delete":
		m.askTrash(true)
	case "a":
		m.askCreate()
	case "A":
		return m.bulkCreate()
	case "r", "f2":
		return m.askRename()
	case "-":
		m.link("abs")
	case "_":
		m.link("rel")
	case "ctrl+-", "ctrl+_":
		m.link("hard")
	case "c":
		m.pending = "c"
	case "u":
		return m.undoLast()
	case "w":
		m.showTasks = true
		m.taskCur = len(m.tasks) - 1
	case "q":
		m.writeCwd()
		m.quitting = true
	case "ctrl+c":
		if !m.closeTab() {
			m.quitting = true
		}
	case "Q":
		m.quitting = true
	}
	return nil
}

func (m *model) leave() {
	switch {
	case m.virtual == "search" || m.virtual == "projects":
		m.cd(m.cwd, m.memory[m.cwd], false)
		return
	case m.virtual == "drives":
		return
	case isRoot(m.cwd):
		m.showDrives()
		return
	}
	m.cd(filepath.Dir(m.cwd), filepath.Base(m.cwd), true)
}

// open: folders go in, text goes to the editor, everything else to its default app
func (m *model) open(e entry) tea.Cmd {
	if e.isDir {
		m.cd(e.path, "", true)
		return nil
	}
	if isTextFile(e) {
		return m.editFile(e, e.matchLine)
	}
	if m.dump {
		m.setNote("would open "+e.name, false)
		return nil
	}
	if err := openDefault(e.path); err != nil {
		m.setNote("couldn't open "+e.name+": "+err.Error(), true)
	} else {
		m.setNote("opened "+e.name, false)
	}
	return nil
}

// editFile opens e in the editor (on line, when > 0: a text search result) and comes back
func (m *model) editFile(e entry, line int) tea.Cmd {
	if m.dump {
		m.setNote("would edit "+e.name, false)
		return nil
	}
	args := []string{e.path}
	if line > 0 {
		args = []string{"+" + strconv.Itoa(line), e.path}
	}
	c := exec.Command(editor(), args...)
	c.Dir = m.cwd
	return tea.ExecProcess(c, func(err error) tea.Msg { return editorDoneMsg{err} })
}

func editor() string {
	if cfg.editor != "" {
		return cfg.editor
	}
	for _, v := range []string{"VISUAL", "EDITOR"} {
		if e := os.Getenv(v); e != "" {
			return e
		}
	}
	if p, err := exec.LookPath("nvim"); err == nil {
		return p
	}
	if p, err := exec.LookPath("vim"); err == nil {
		return p
	}
	return "notepad"
}

func configDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func (m *model) writeCwd() {
	if m.cwdFile != "" {
		os.WriteFile(m.cwdFile, []byte(m.cwd), 0o644)
	}
}

// ---------------------------------------------------------------------------
// mouse
// ---------------------------------------------------------------------------

// rowAt: the list index under row y of a column showing list with the given top
func (m *model) column(x int) int {
	for i, c := range m.colX {
		if c[1] > c[0] && x >= c[0] && x < c[1] {
			return i
		}
	}
	return -1
}

func (m *model) click(c tea.MouseClickMsg) tea.Cmd {
	if m.help || m.prompt != nil || m.question != nil || m.showTasks || m.showMarks || m.spot != nil || m.menu != nil || c.Button != tea.MouseLeft {
		return nil
	}
	row := c.Y - m.bodyY
	if row < 0 || row >= m.bodyH {
		return nil
	}
	switch m.column(c.X) {
	case 0: // parent: go to that folder
		top := listTop(len(m.parent), m.parentIdx, m.bodyH)
		if i := top + row; i < len(m.parent) && m.parent[i].isDir {
			m.cd(m.parent[i].path, "", true)
		}
	case 1:
		i := m.top + row
		if i >= len(m.entries) {
			return nil
		}
		double := i == m.clickIdx && time.Since(m.lastClick) < 450*time.Millisecond
		m.clickIdx, m.lastClick = i, time.Now()
		if i == m.cursor && (double || m.dump) {
			m.clickIdx = -1
			return m.open(m.entries[i])
		}
		m.cursor, m.pscroll = i, 0
	case 2: // a folder's preview: go in, on the clicked entry
		if c := m.current(); c != nil && c.isDir && m.pv.dir {
			top := listTop(len(m.pv.entries), m.previewIdx(), m.bodyH)
			if i := top + row; i < len(m.pv.entries) {
				m.cd(c.path, m.pv.entries[i].name, true)
			}
		}
	}
	return nil
}

func (m *model) wheel(w tea.MouseWheelMsg) {
	if m.prompt != nil || m.question != nil || m.showTasks || m.showMarks || m.spot != nil || m.menu != nil {
		return
	}
	if m.help {
		if w.Button == tea.MouseWheelDown {
			m.helpScroll += 3
		} else {
			m.helpScroll = max(0, m.helpScroll-3)
		}
		return
	}
	d := 1
	if w.Button == tea.MouseWheelUp {
		d = -1
	}
	switch m.column(w.X) {
	case 2:
		m.pscroll = max(0, m.pscroll+3*d)
	default:
		m.move(d)
	}
}

// previewIdx: in a folder preview, the entry the cursor was on last time (-1 if none)
func (m *model) previewIdx() int {
	c := m.current()
	if c == nil {
		return -1
	}
	name := m.memory[c.path]
	for i, e := range m.pv.entries {
		if e.name == name {
			return i
		}
	}
	return -1
}
