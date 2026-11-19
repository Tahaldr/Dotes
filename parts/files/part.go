// Package files is Dotes' Files part: a yazi-style file manager (was TermFiles).
package files

import (
	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/ui"
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

// test helpers (shared with every part)
func drain(m tea.Model, first tea.Cmd) { ui.Drain(m, first) }

var ParseInput = ui.ParseInput
