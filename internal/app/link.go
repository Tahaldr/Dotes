package app

// Links between parts: Files → Git / Notes, Git → Files, Notes → Files.

import (
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Open shows part id and opens path in it (see SwitchMsg).
func Open(id, path string) tea.Cmd {
	return func() tea.Msg { return SwitchMsg{ID: id, Path: path} }
}

// RepoRoot is the top folder of the git repo dir is in ("" when it isn't in one).
func RepoRoot(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return filepath.Clean(strings.TrimSpace(string(out)))
}

// OnGitHub: the repo at dir has a github.com remote called origin.
func OnGitHub(dir string) bool {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	return err == nil && strings.Contains(strings.ToLower(string(out)), "github.com")
}
