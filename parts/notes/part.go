// Package notes is Dotes' Notes part: todo lists and notes (was TermNotes).
package notes

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/md"
	"github.com/Tahaldr/dotes/internal/ui"
	"github.com/charmbracelet/x/ansi"
)

// Options for starting the part.
type Options struct {
	Notebook string // another notebook folder (todos\ and notes\ inside)
	Dump     bool   // headless test mode
}

const Usage = `  dotes n [--notebook FOLDER]       Notes (another notebook folder: todos\ and notes\ inside)
  dotes n add "buy milk" [-l LIST] [-d fri] [-p high]
                                    add a todo (to your inbox list, or LIST: part of its name is enough)
  dotes n today                     print what's late, due today and this week
  dotes n lists                     print your todo lists and notes`

// LoadConfig reads notes.txt (made on first use). notebook overrides its folder.
func LoadConfig(notebook string) {
	cfg = loadConfig()
	if notebook != "" {
		cfg.notebook = expandHome(notebook)
	}
}

// New makes the Notes part. Call LoadConfig first.
func New(o Options) tea.Model {
	m := newModel()
	if o.Dump {
		m.dump = true
		return m
	}
	m.live = true
	return m
}

// Add, Today and Lists are the shell commands (dotes n add / today / lists).
func Add(args []string) { cliAdd(args) }
func Today()            { cliToday() }
func Lists()            { cliLists() }

// ---- test helpers ----

func drain(m tea.Model, first tea.Cmd) { ui.Drain(m, first) }

func parseInput(a string) tea.Msg { return ui.ParseInput(a) }

func runDump(w, h int, keys []string) string {
	m := newModel()
	m.dump = true
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	for _, a := range keys {
		m.View() // lay out the panes so clicks land
		drain(m, func() tea.Msg { return parseInput(a) })
	}
	out := m.View().Content
	if os.Getenv("DOTES_RAW") == "" {
		out = ansi.Strip(out)
	}
	return out
}

// renderMarkdown draws markdown the shared Dotes way
func renderMarkdown(s string, w int) []string { return md.Render(s, w) }
