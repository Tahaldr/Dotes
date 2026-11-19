// Package git is Dotes' Git part: GitHub and Git in the terminal (was TermGit).
package git

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/ui"
)

// LoggedIn: is there a GitHub login (through the GitHub CLI)?
func LoggedIn() bool { return loggedIn() }

// notReady is shown instead of Git when there's no GitHub login yet.
type notReady struct{ w, h int }

func (n notReady) Init() tea.Cmd { return nil }
func (n notReady) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		n.w, n.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if s := msg.String(); s == "q" || s == "ctrl+c" {
			return n, tea.Quit
		}
	}
	return n, nil
}
func (n notReady) View() tea.View {
	msg := "Run  dotes g login  in your shell (it uses the GitHub CLI), then open Dotes again."
	if !haveGh() {
		msg = "It logs in through the GitHub CLI (gh). Install it (winget install --id GitHub.cli), then run  dotes g login ."
	}
	lines := []string{"", "  " + ui.FG(ui.Acc).Bold(true).Render("Git needs a GitHub login"), "",
		"  " + ui.FG(ui.Txt).Render(msg), "  " + ui.FG(ui.Mut).Render("q quits.")}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// ---- test helpers ----

func drain(m tea.Model, first tea.Cmd) { ui.Drain(m, first) }

func parseInput(a string) tea.Msg { return ui.ParseInput(a) }

// Solo: is it a file read full screen?
func (l *Launch) Solo() bool { return l != nil && l.solo != nil }
