package notes

// Settings in the Dotes config folder (notes.txt), made with every option explained on first start.
// e on the ~ page opens it in your editor.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tahaldr/dotes/internal/app"
)

type config struct {
	notebook string
	editor   string
	hints    bool
	indent   int    // screen columns per sub-todo level
	inbox    string // where dotes n add puts todos without -l
}

const defaultNotebook = "~/Documents/Notebook"

const defaultConfig = `# Notes settings (Dotes): key = value. Lines starting with # are ignored.
# Changes apply the next time Dotes starts. Shared settings (hints, parts) are in dotes.txt.

# the folder with todos\ and notes\ inside (each list or note is a .md file)
notebook = ` + defaultNotebook + `

# editor for notes (empty = $EDITOR, else nvim)
editor =

# how far sub-todos are indented on screen, in columns (the file keeps 2 spaces, for Nvim)
indent = 4

# the list  dotes n add "…"  uses when you don't name one (made when missing)
inbox = Inbox
`

func configFile() string { return app.File("notes.txt") }

func loadConfig() config {
	b, err := os.ReadFile(configFile())
	if err != nil && app.ImportOld("termnotes", "config.txt", "notes.txt") { // settings from TermNotes
		b, err = os.ReadFile(configFile())
	}
	if err != nil {
		os.MkdirAll(filepath.Dir(configFile()), 0o755)
		os.WriteFile(configFile(), []byte(defaultConfig), 0o644)
		b = []byte(defaultConfig)
	}
	return parseConfig(string(b))
}

func parseConfig(text string) config {
	c := config{notebook: defaultNotebook, hints: app.LoadShared().Hints, indent: 4, inbox: "Inbox"}
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch k {
		case "notebook":
			if v != "" {
				c.notebook = v
			}
		case "editor":
			c.editor = v
		case "indent":
			if n, err := strconv.Atoi(v); err == nil && n >= 2 && n <= 8 {
				c.indent = n
			}
		case "inbox":
			if v != "" {
				c.inbox = v
			}
		}
	}
	c.notebook = expandHome(c.notebook)
	return c
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[1:])
	}
	return filepath.Clean(os.ExpandEnv(p))
}

// setConfigValue changes one "key = value" line (adds it if missing), keeping everything else
func setConfigValue(key, value string) {
	b, err := os.ReadFile(configFile())
	if err != nil {
		b = []byte(defaultConfig)
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	done := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if k, _, ok := strings.Cut(t, "="); ok && !strings.HasPrefix(t, "#") && strings.EqualFold(strings.TrimSpace(k), key) {
			lines[i] = key + " = " + value
			done = true
		}
	}
	if !done {
		lines = append(lines, key+" = "+value)
	}
	os.MkdirAll(filepath.Dir(configFile()), 0o755)
	os.WriteFile(configFile(), []byte(strings.Join(lines, "\n")), 0o644)
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
	for _, e := range []string{"nvim", "vim"} {
		if p, err := exec.LookPath(e); err == nil {
			return p
		}
	}
	return "notepad"
}

var cfg config
