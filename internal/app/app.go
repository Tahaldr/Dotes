// Package app holds what every Dotes part shares: the version, the config folder and the
// shared settings file (dotes.txt).
package app

import (
	"os"
	"path/filepath"
	"strings"
)

const Version = "1.2.0"

// Parts, in tab order. Key is the short command (dotes f) and the alt+N order.
type PartInfo struct {
	ID, Name, Short, Desc string
	Ready                 bool // built into this version
}

var Parts = []PartInfo{
	{"home", "Home", "h", "dashboard, app launcher, projects, prayer times", true},
	{"files", "Files", "f", "a yazi-style file manager", true},
	{"notes", "Notes", "n", "todo lists and notes", true},
	{"git", "Git", "g", "GitHub and Git: repos, changes, PRs, issues, CI", true},
}

// Dir is the config folder: %APPDATA%\dotes (Windows), ~/.config/dotes (elsewhere).
// DOTES_CONFIG overrides it (tests).
func Dir() string {
	if d := os.Getenv("DOTES_CONFIG"); d != "" {
		return d
	}
	d, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".config")
	}
	return filepath.Join(d, "dotes")
}

// File is a file in the config folder.
func File(name string) string { return filepath.Join(Dir(), name) }

const sharedDefault = `# Dotes settings: key = value. Lines starting with # are ignored.
# Each part has its own file next to this one (files.txt, notes.txt, git.txt, home.txt).

# parts you use, in any order: home, files, notes, git  (dotes setup changes them)
parts = files

# key hints at the bottom of every part: on / off (h on the F1 page switches)
hints = on

# the part names on the right of the first line (Home · Files · Notes · Git): on / off
tabbar = on
`

// Shared settings from dotes.txt.
type Shared struct {
	Parts  []string // ids, in the order of Parts
	Hints  bool
	TabBar bool
}

// HasShared: has the user picked their parts yet (dotes.txt exists)?
func HasShared() bool {
	_, err := os.Stat(File("dotes.txt"))
	return err == nil
}

func LoadShared() Shared {
	s := Shared{Hints: true, TabBar: true}
	kv := map[string]string{}
	if HasShared() { // never made here: its absence means "show the part picker"
		kv = ReadKV(File("dotes.txt"), sharedDefault)
	} else {
		kv["parts"] = "files"
	}
	picked := map[string]bool{}
	for _, p := range strings.Split(kv["parts"], ",") {
		picked[strings.ToLower(strings.TrimSpace(p))] = true
	}
	for _, p := range Parts {
		if picked[p.ID] {
			s.Parts = append(s.Parts, p.ID)
		}
	}
	s.Hints = !off(kv["hints"])
	s.TabBar = !off(kv["tabbar"])
	return s
}

func off(v string) bool { v = strings.ToLower(v); return v == "off" || v == "false" || v == "no" }

// SetShared changes one line of dotes.txt.
func SetShared(key, value string) { SetKV(File("dotes.txt"), sharedDefault, key, value) }

// SaveParts writes the picked parts (setup).
func SaveParts(ids []string) {
	ReadKV(File("dotes.txt"), sharedDefault) // first time: the file with every option explained
	SetShared("parts", strings.Join(ids, ", "))
}

// ReadKV reads "key = value" lines (lower-case keys). A missing file is created from def.
// Repeated keys keep the last value; use ReadLines for lists.
func ReadKV(path, def string) map[string]string {
	out := map[string]string{}
	for _, kv := range ReadPairs(path, def) {
		out[kv[0]] = kv[1]
	}
	return out
}

// ReadPairs is ReadKV keeping every line, in order.
func ReadPairs(path, def string) [][2]string {
	b, err := os.ReadFile(path)
	if err != nil {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(def), 0o644)
		b = []byte(def)
	}
	var out [][2]string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if k, v, ok := strings.Cut(l, "="); ok {
			out = append(out, [2]string{strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)})
		}
	}
	return out
}

// SetKV changes one "key = value" line (adds it if missing), keeping everything else.
func SetKV(path, def, key, value string) {
	b, err := os.ReadFile(path)
	if err != nil {
		b = []byte(def)
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
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// ImportOld copies a settings file from the old standalone app (TermFiles, TermNotes, ...)
// into the Dotes folder the first time, so nothing has to be set up again. Its "hints" line
// is dropped (that switch lives in dotes.txt now). True = a file was copied.
func ImportOld(oldApp, oldName, newName string) bool {
	if os.Getenv("DOTES_CONFIG") != "" { // tests: never read the real old settings
		return false
	}
	if _, err := os.Stat(File(newName)); err == nil {
		return false
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	b, err := os.ReadFile(filepath.Join(d, oldApp, oldName))
	if err != nil {
		return false
	}
	var keep []string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if k, _, ok := strings.Cut(strings.TrimSpace(l), "="); ok && strings.EqualFold(strings.TrimSpace(k), "hints") {
			continue
		}
		keep = append(keep, l)
	}
	os.MkdirAll(Dir(), 0o755)
	return os.WriteFile(File(newName), []byte(strings.Join(keep, "\n")), 0o644) == nil
}

// ShownMsg goes to every running part when the visible part changes (ID = the one shown now).
type ShownMsg struct{ ID string }

// SwitchMsg asks the shell to show another part (Home's apps do this). With a Path the
// shell then hands that part an OpenMsg: Files goes there, Git opens the repo it's in,
// Notes opens the folder's project note.
type SwitchMsg struct{ ID, Path string }

// OpenMsg goes to the part just shown: open Path in it.
type OpenMsg struct{ Path string }

// PickProjectsMsg asks Files to show these projects to pick one from (Home's project picker,
// in the same window). Openers are the apps that can open it: key, name.
type PickProjectsMsg struct {
	Projects []string
	Openers  [][2]string
}

// PickedMsg: the project picked in Files, and which opener (index) should open it.
type PickedMsg struct {
	Opener int
	Path   string
}

// ExitCode is what Dotes exits with. ExitCloseTab tells the shell function to close the
// terminal tab too (x on Home).
var ExitCode int

const ExitCloseTab = 3
