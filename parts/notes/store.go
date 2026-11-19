package notes

// The notebook folder (the same one the Nvim popup uses): todos\ holds the todo lists,
// notes\ the notes, one Markdown file each. A file's title is its first "# heading".

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type docInfo struct {
	kind        string // "todos" or "notes"
	path, title string
	mod         time.Time
	done, total int    // todo lists: ticked / all
	lines       int    // notes: how long
	preview     string // notes: the first line of text
}

var kindLabel = map[string]string{"todos": "Todo lists", "notes": "Notes"}
var kindOne = map[string]string{"todos": "todo list", "notes": "note"}

func kindDir(kind string) string {
	d := filepath.Join(cfg.notebook, kind)
	os.MkdirAll(d, 0o755)
	return d
}

// titleOf: the first "# heading", else the file name
func titleOf(lines []string, path string) string {
	if len(lines) > 0 {
		if m := headingRe.FindStringSubmatch(lines[0]); m != nil && len(m[1]) == 1 && m[2] != "" {
			return m[2]
		}
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// listDocs: todo lists then notes, last edited first in each
func listDocs() []docInfo {
	var out []docInfo
	for _, kind := range []string{"todos", "notes"} {
		var part []docInfo
		des, _ := os.ReadDir(kindDir(kind))
		for _, de := range des {
			if de.IsDir() || !strings.EqualFold(filepath.Ext(de.Name()), ".md") {
				continue
			}
			fi, err := de.Info()
			if err != nil {
				continue
			}
			p := filepath.Join(kindDir(kind), de.Name())
			b, _ := os.ReadFile(p)
			ls, _, _ := splitLines(string(b))
			d := docInfo{kind: kind, path: p, title: titleOf(ls, p), mod: fi.ModTime(), lines: len(ls), preview: firstText(ls)}
			if kind == "todos" {
				d.done, d.total = progress(ls)
			}
			part = append(part, d)
		}
		sort.SliceStable(part, func(i, j int) bool { return part[i].mod.After(part[j].mod) })
		out = append(out, part...)
	}
	return out
}

func fileName(title string) string {
	n := strings.TrimSpace(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 32 {
			return -1
		}
		return r
	}, title))
	n = strings.TrimRight(n, ". ")
	if n == "" {
		n = "untitled"
	}
	return n
}

// createDoc makes "# title" in the kind's folder (an existing one with that name is reused)
func createDoc(kind, title string) (string, error) {
	p := filepath.Join(kindDir(kind), fileName(title)+".md")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	return p, os.WriteFile(p, []byte("# "+title+"\n\n"), 0o644)
}

// renameDoc changes the title (the heading) and the file name to match
func renameDoc(path, title string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return path, err
	}
	ls, crlf, _ := splitLines(string(b))
	if len(ls) > 0 && strings.HasPrefix(ls[0], "# ") {
		ls[0] = "# " + title
	} else {
		ls = append([]string{"# " + title, ""}, ls...)
	}
	nl := "\n"
	if crlf {
		nl = "\r\n"
	}
	text := strings.Join(ls, nl) + nl
	if err := writeAtomic(path, []byte(text)); err != nil {
		return path, err
	}
	np := filepath.Join(filepath.Dir(path), fileName(title)+".md")
	if np == path {
		return path, nil
	}
	if _, err := os.Stat(np); err == nil && !strings.EqualFold(np, path) {
		return path, fmt.Errorf("the title changed, but %s already exists so the file keeps its name", filepath.Base(np))
	}
	if err := os.Rename(path, np); err != nil {
		return path, err
	}
	return np, nil
}

// ago: "now", "5m", "3h", "2d", "4w", or the date
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/24/7))
	}
	return t.Format("2006-01-02")
}

// firstText: the first line of a note that isn't a heading, without Markdown marks
func firstText(ls []string) string {
	for _, s := range ls {
		s = strings.TrimSpace(s)
		if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, "```") || strings.HasPrefix(s, "---") {
			continue
		}
		s = strings.TrimLeft(s, "-*+> ")
		if t, ok := parseTodo("- " + s); ok {
			s = t.text
		}
		return strings.NewReplacer("**", "", "__", "", "`", "", "[ ] ", "", "[x] ", "").Replace(s)
	}
	return ""
}
