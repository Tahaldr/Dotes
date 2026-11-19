// Package files is Dotes' Files part: a yazi-style file manager (was TermFiles).
package files

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// OriginY: rows the shell draws above this part (its top bar). Real images (Sixel) are placed
// with absolute screen rows, so they need it.
var OriginY int

// Options for starting the part.
type Options struct {
	CwdFile  string // on q, write the last folder here (cd-on-quit shell function)
	Pick     string // pick mode: write the choice here (TermNook's nook -p)
	Projects string // pick mode: file with recent projects, one per line
	Openers  string // pick mode: file with "key<TAB>name" apps
	Dump     bool   // headless test mode: no terminal, no background work
}

const Usage = `  dotes f [folder or file]          Files, starting there (default: the current folder)
  dotes f --cwd-file PATH           on q, write the last folder to PATH (for a cd-on-quit shell function)
  dotes f --pick OUT [--projects FILE] [--openers FILE]
                                    pick a file or folder for another program
  dotes f --sixel-test IMAGE        check if this terminal shows real images`

// New makes the Files part. Call LoadConfig first (once).
func New(start string, o Options) tea.Model {
	m := newModel(start)
	m.cwdFile = o.CwdFile
	if o.Pick != "" {
		pk := pickArgs{out: o.Pick, openers: loadOpeners(o.Openers), projects: readLines(o.Projects)}
		if len(pk.openers) == 0 {
			pk.openers = []opener{{name: "Editor"}}
		}
		m.pick = pk
		m.showProjects()
	}
	if o.Dump {
		m.dump = true
		return m
	}
	m.live = true
	m.zoxideAdd(m.cwd)
	return m
}

// LoadConfig reads files.txt (made on first use).
func LoadConfig() { cfg = loadConfig() }

// SixelTest prints a picture straight to the terminal.
func SixelTest(path string) { sixelTest(path) }

func drain(m tea.Model, first tea.Cmd) {
	Drain(m, first)
}

// Drain runs a command and everything it leads to, feeding the messages to m (tests, dumps).
func Drain(m tea.Model, first tea.Cmd) {
	queue := []tea.Cmd{first}
	for steps := 0; len(queue) > 0 && steps < 200; steps++ {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case tea.QuitMsg:
			return
		default:
			_, next := m.Update(msg)
			queue = append(queue, next)
		}
	}
}

// ParseInput turns "j", "ctrl+d", "alt+2", "enter", "click:10,5", "wheel:10,5,up" into a message
func ParseInput(a string) tea.Msg {
	if strings.HasPrefix(a, "click:") || strings.HasPrefix(a, "wheel:") {
		p := strings.Split(a[6:], ",")
		x, _ := strconv.Atoi(p[0])
		y, _ := strconv.Atoi(p[1])
		if a[0] == 'c' {
			return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
		}
		b := tea.MouseWheelDown
		if len(p) > 2 && p[2] == "up" {
			b = tea.MouseWheelUp
		}
		return tea.MouseWheelMsg{X: x, Y: y, Button: b}
	}
	named := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "down": tea.KeyDown, "up": tea.KeyUp,
		"left": tea.KeyLeft, "right": tea.KeyRight, "backspace": tea.KeyBackspace, "home": tea.KeyHome,
		"end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "f1": tea.KeyF1, "space": tea.KeySpace,
	}
	if r, ok := named[a]; ok {
		return tea.KeyPressMsg{Code: r}
	}
	if strings.HasPrefix(a, "ctrl+") {
		return tea.KeyPressMsg{Code: rune(a[5]), Mod: tea.ModCtrl}
	}
	if strings.HasPrefix(a, "alt+") && len(a) > 4 {
		return tea.KeyPressMsg{Code: []rune(a[4:])[0], Mod: tea.ModAlt}
	}
	r := []rune(a)[0]
	return tea.KeyPressMsg{Code: r, Text: a}
}
