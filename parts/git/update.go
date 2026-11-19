package git

// Updates are Dotes' job (dotes update, the chip in the top bar). This keeps Git's
// "Update" command in the palette working with them.

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/Tahaldr/dotes/internal/app"
)

var version = app.Version

func newer(a, b string) bool { return app.Newer(a, b) }

type updateMsg struct{ latest string }

// checkUpdate: Dotes checks; Git only remembers what it found
func (m *model) checkUpdate() tea.Cmd {
	m.newVersion = app.NewerSaved()
	return nil
}

func (m *model) onUpdate(msg updateMsg) tea.Cmd {
	if newer(msg.latest, version) {
		m.newVersion = msg.latest
	}
	return nil
}

type updatedMsg struct{ err error }

// askUpdate: run dotes update, then close (the new version starts next time)
func (m *model) askUpdate() tea.Cmd {
	v := strings.TrimPrefix(m.newVersion, "v")
	m.prompt = &prompt{label: "Update Dotes to " + v + " now? Your settings and data stay as they are. Dotes closes after; just open it again.", confirm: true,
		done: func(m *model, _ string) tea.Cmd {
			if m.dump {
				return m.setStatus("Would update to "+v, false)
			}
			exe, err := os.Executable()
			if err != nil {
				return m.setStatus(err.Error(), true)
			}
			c := exec.Command(exe, "update")
			return tea.ExecProcess(c, func(err error) tea.Msg { return updatedMsg{err} })
		}}
	return nil
}

func (m *model) onUpdated(msg updatedMsg) tea.Cmd {
	if msg.err != nil {
		return m.setStatus("The update didn't work: "+msg.err.Error(), true)
	}
	m.exitMsg = "Dotes was updated. Open it again to use the new version."
	return tea.Quit
}

// ExitMessage is printed after Dotes closes ("" = nothing).
func (m *model) ExitMessage() string { return m.exitMsg }
