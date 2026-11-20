package files

// The open-with menu (O / shift+enter), shell commands (; and :), and pick mode: TermNook's
// `nook -p` starts TermFiles with --pick, shows your recent projects, and gets back what you
// chose and which app should open it.

import (
	"fmt"
	"github.com/Tahaldr/dotes/internal/app"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ---------------------------------------------------------------------------
// pick mode (for TermNook)
// ---------------------------------------------------------------------------

type pickArgs struct {
	out      string   // where the choice goes: "index<TAB>path"
	inWindow bool     // Home's picker in the same window: the choice goes back as app.PickedMsg
	openers  []opener // TermNook's OpenWith apps; the first opens on enter
	projects []string
}

// readLines reads a file of one item per line
func readLines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(strings.TrimPrefix(l, "\uFEFF")); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// loadOpeners reads "key<TAB>name" lines (key may be empty)
func loadOpeners(path string) []opener {
	var out []opener
	for _, l := range readLines(path) {
		k, n, ok := strings.Cut(l, "\t")
		if !ok {
			k, n = "", l
		}
		out = append(out, opener{name: strings.TrimSpace(n), key: strings.ToLower(strings.TrimSpace(k))})
	}
	return out
}

func (m *model) picking() bool { return m.pick.out != "" || m.pick.inWindow }

// startPick: Home asks for a project (app.PickProjectsMsg)
func (m *model) startPick(p app.PickProjectsMsg) {
	m.pick = pickArgs{inWindow: true, projects: p.Projects}
	for _, o := range p.Openers {
		m.pick.openers = append(m.pick.openers, opener{key: o[0], name: o[1]})
	}
	m.showProjects()
}

// endPick: back to browsing as before the picker
func (m *model) endPick() {
	if !m.pick.inWindow {
		return
	}
	m.pick = pickArgs{}
	if m.virtual == "projects" {
		m.virtual = ""
		m.cd(m.cwd, "", false)
	}
}

func (m *model) showProjects() {
	if len(m.pick.projects) == 0 {
		return
	}
	if c := m.current(); c != nil && m.virtual == "" {
		m.memory[m.cwd] = c.name
	}
	var list []entry
	for _, p := range m.pick.projects {
		fi, err := os.Stat(p)
		if err != nil || !fi.IsDir() {
			continue
		}
		e := entry{name: filepath.Base(p), path: p, isDir: true, mod: fi.ModTime(), mode: fi.Mode(),
			label: prettyPath(filepath.Dir(p))}
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			e.isRepo = true
		}
		list = append(list, e)
	}
	m.searchMoved = true
	m.virtual, m.vTitle, m.vAll, m.entries, m.filter = "projects", "Recent projects", list, list, ""
	m.cursor, m.top = 0, 0
	m.parent, m.parentIdx = nil, -1
	m.gitStale = true
}

// choose ends pick mode: the path and the app (index into openers) go back to TermNook
func (m *model) choose(path string, idx int) {
	if m.pick.inWindow {
		m.pickedOut = &app.PickedMsg{Opener: idx, Path: path}
		m.endPick()
		return
	}
	if m.pick.out == "" {
		return
	}
	os.WriteFile(m.pick.out, []byte(fmt.Sprintf("%d\t%s", idx, path)), 0o644)
	m.quitting = true
}

// pickKey: the keys that work differently in pick mode; true = handled
func (m *model) pickKey(s string) bool {
	if !m.picking() {
		return false
	}
	c := m.current()
	switch {
	case s == "enter":
		if c != nil {
			m.choose(c.path, 0)
		}
		return true
	case strings.HasPrefix(s, "alt+") && len(s) == 5:
		for i, o := range m.pick.openers {
			if o.key != "" && o.key == s[4:] && c != nil {
				m.choose(c.path, i)
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// open with… (O)
// ---------------------------------------------------------------------------

type menuItem struct {
	name, hint string
	do         func(m *model) tea.Cmd
}

type menuState struct {
	title string
	items []menuItem
	cur   int
}

func (m *model) openWithMenu() {
	c := m.current()
	if c == nil {
		return
	}
	e := *c
	var items []menuItem
	if m.picking() {
		for i, o := range m.pick.openers {
			i := i
			hint := ""
			if i == 0 {
				hint = "enter"
			} else if o.key != "" {
				hint = "alt+" + o.key
			}
			items = append(items, menuItem{o.name, hint, func(m *model) tea.Cmd { m.choose(e.path, i); return nil }})
		}
	} else {
		ed := filepath.Base(editor())
		ed = strings.TrimSuffix(ed, filepath.Ext(ed))
		if !e.isDir {
			items = append(items, menuItem{"Editor (" + ed + ")", "", func(m *model) tea.Cmd { return m.editFile(e, 0) }})
		}
		items = append(items, menuItem{"Default app", "like double-clicking it", func(m *model) tea.Cmd {
			if err := openDefault(e.path); err != nil {
				m.setNote(err.Error(), true)
			}
			return nil
		}})
		if runtime.GOOS == "windows" && !e.isDir {
			items = append(items, menuItem{"Choose an app…", "the Windows list", func(m *model) tea.Cmd {
				exec.Command("rundll32.exe", "shell32.dll,OpenAs_RunDLL", e.path).Start()
				return nil
			}})
		}
		if runtime.GOOS == "windows" {
			items = append(items, menuItem{"Show in Explorer", "", func(m *model) tea.Cmd {
				exec.Command("explorer.exe", "/select,", e.path).Start()
				return nil
			}})
		}
		if _, err := exec.LookPath("code"); err == nil {
			items = append(items, menuItem{"VS Code", "", func(m *model) tea.Cmd {
				exec.Command("code", e.path).Start()
				return nil
			}})
		}
		for _, o := range cfg.openers {
			o := o
			items = append(items, menuItem{o.name, "from config.txt", func(m *model) tea.Cmd {
				return m.runShell(strings.ReplaceAll(o.run, "%p", e.path), false)
			}})
		}
	}
	m.menu = &menuState{title: "Open " + e.name + " with", items: items}
}

func (m *model) menuKey(k tea.KeyPressMsg) tea.Cmd {
	mn := m.menu
	n := len(mn.items)
	s := k.String()
	switch s {
	case "esc", "q", "O":
		m.menu = nil
	case "j", "down":
		mn.cur = (mn.cur + 1) % n
	case "k", "up":
		mn.cur = (mn.cur - 1 + n) % n
	case "enter", "l":
		m.menu = nil
		return mn.items[mn.cur].do(m)
	default:
		if i, err := strconv.Atoi(s); err == nil && i >= 1 && i <= n {
			m.menu = nil
			return mn.items[i-1].do(m)
		}
	}
	return nil
}

func (m *model) menuPopup(lines []string) {
	mn := m.menu
	if mn == nil {
		return
	}
	w := m.popupWidth(44, 70)
	inner := w - 4
	var body []string
	for i, it := range mn.items {
		mark, st := "  ", fg(cTxt)
		if i == mn.cur {
			mark, st = fg(cAcc).Render("❯ "), fg(cAcc).Bold(true)
		}
		row := mark + fg(cHint).Render(strconv.Itoa(i+1)+" ") + st.Render(it.name)
		hint := fg(cMut).Render(it.hint)
		body = append(body, row+strings.Repeat(" ", max(2, inner-width(row)-width(hint)))+hint)
	}
	body = append(body, fg(cDim).Render(strings.Repeat("─", inner)), keyHints("1-9/enter", "open", "j/k", "move", "esc", "close"))
	box := popupBox(fg(cAcc).Bold(true).Render(icOpen+" "+trunc(mn.title, inner-4)), w, body, fg(cAcc))
	put(lines, (m.w-w)/2, 2, box, w)
}

// ---------------------------------------------------------------------------
// ; and : run a shell command here (: waits for enter before coming back)
// ---------------------------------------------------------------------------

func (m *model) askShell(wait bool) {
	title := "Shell"
	if wait {
		title = "Shell (waits)"
	}
	m.input(title, "runs in this folder · %h = the file under the cursor, %s = the selected ones", "", -1, func(m *model, s string) tea.Cmd {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		return m.runShell(m.expandShell(s), wait)
	})
}

func quoteArg(p string) string {
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(p, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}

func (m *model) expandShell(s string) string {
	if c := m.current(); c != nil {
		s = strings.ReplaceAll(s, "%h", quoteArg(c.path))
	}
	if strings.Contains(s, "%s") {
		var q []string
		for _, p := range m.targets() {
			q = append(q, quoteArg(p))
		}
		s = strings.ReplaceAll(s, "%s", strings.Join(q, " "))
	}
	return s
}

func (m *model) runShell(command string, wait bool) tea.Cmd {
	if m.dump {
		m.setNote("would run "+command, false)
		return nil
	}
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		sh := "pwsh"
		if _, err := exec.LookPath(sh); err != nil {
			sh = "powershell"
		}
		if wait {
			command += "; Write-Host ''; Read-Host 'press enter to go back to Dotes' | Out-Null"
		}
		c = exec.Command(sh, "-NoLogo", "-NoProfile", "-Command", command)
	} else {
		if wait {
			command += `; printf '\npress enter to go back to Dotes'; read _`
		}
		c = exec.Command("sh", "-c", command)
	}
	c.Dir = m.cwd
	return tea.ExecProcess(c, func(err error) tea.Msg { return editorDoneMsg{err} })
}
