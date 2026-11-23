package files

// Settings in the Dotes config folder (files.txt), made with every option explained on first start.
// e on the ~ page opens it in your editor.

import (
	"github.com/Tahaldr/dotes/internal/app"
	"os"
	"path/filepath"
	"strings"
)

type opener struct {
	name, run string // run: a command, %p = the file's path
	key       string // pick mode: alt+key (from TermNook's OpenWith)
}

type config struct {
	editor   string
	hidden   bool
	sortBy   string
	sortRev  bool
	lineMode string
	images   string // auto, sixel, blocks
	hints    bool
	columns  [2]float64
	openers  []opener
}

const defaultConfig = `# Files settings (Dotes): key = value. Lines starting with # are ignored.
# Changes apply the next time Dotes starts. Shared settings (hints, parts) are in dotes.txt.

# editor for text files (empty = $EDITOR, else nvim)
editor =

# show hidden files at start (. switches)
hidden = false

# sort: natural, alpha, mtime, btime, ext, size or random; add "reverse" to flip (, switches)
sort = natural

# what rows show on the right: size, perm, mtime, btime, owner, none (m switches)
linemode = size

# images: auto (real pictures when the terminal can, blocks otherwise), sixel, blocks
images = auto

# column widths (parent, current folder) as parts of the width; drag the lines between the
# columns with the mouse to set them, = (or empty) puts back 1 : 4 : 3
columns =

# extra apps for the open-with menu (O). %p is the file's path. One per line:
# opener = Notepad | notepad "%p"
# opener = Paint | mspaint "%p"
`

func configFile() string { return app.File("files.txt") }

func loadConfig() config {
	c := config{sortBy: "natural", lineMode: "size", images: "auto", hints: app.LoadShared().Hints}
	b, err := os.ReadFile(configFile())
	if err != nil && app.ImportOld("termfiles", "config.txt", "files.txt") { // settings from TermFiles
		b, err = os.ReadFile(configFile())
	}
	if err != nil {
		os.MkdirAll(filepath.Dir(configFile()), 0o755)
		os.WriteFile(configFile(), []byte(defaultConfig), 0o644)
		b = []byte(defaultConfig)
	}
	return parseConfig(string(b), c)
}

// parseConfig reads "key = value" lines over the defaults in c
func parseConfig(text string, c config) config {
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
		case "editor":
			c.editor = v
		case "hidden":
			c.hidden = v == "true" || v == "on" || v == "yes"
		case "sort":
			f := strings.Fields(strings.ToLower(v))
			if len(f) > 0 && sortNames[f[0]] != "" {
				c.sortBy = f[0]
				c.sortRev = len(f) > 1 && f[1] == "reverse"
			}
		case "linemode":
			switch v {
			case "size", "perm", "mtime", "btime", "owner", "none":
				c.lineMode = v
			}
		case "images":
			c.images = strings.ToLower(v)
		case "columns":
			c.columns = parseColumns(v)
		case "opener":
			if name, run, ok := strings.Cut(v, "|"); ok && strings.TrimSpace(run) != "" {
				c.openers = append(c.openers, opener{name: strings.TrimSpace(name), run: strings.TrimSpace(run)})
			}
		}
	}
	return c
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

var cfg config
