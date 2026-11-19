// Package paths finds file paths typed in text (a todo, a note) and suggests the rest of a
// path while you type it. A path starts with ~/ , a drive (C:\ or C:/) or, off Windows, /.
//
//	look at ~/Documents/Coding/Dotes/PLAN.md   →   ~/Documents/Coding/Dotes/PLAN.md is a link
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// Span is a path found in text: text[Start:End] (bytes).
type Span struct {
	Start, End int
	Path       string // as typed
}

// startsPath: s (a word) begins like a path
func startsPath(s string) bool {
	switch {
	case s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`):
		return true
	case len(s) >= 3 && isLetter(s[0]) && s[1] == ':' && (s[2] == '\\' || s[2] == '/'):
		return true
	case runtime.GOOS != "windows" && strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//"):
		return true
	}
	return false
}

func isLetter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }

// word boundaries: a path follows a space or an opening bracket / quote
func opener(r rune) bool { return r == ' ' || r == '\t' || strings.ContainsRune("([{\"'`<", r) }

// Find returns the paths in text, in order. Trailing punctuation (".", ",", ")" …) isn't
// part of a path.
func Find(text string) []Span {
	var out []Span
	i := 0
	for i < len(text) {
		r, n := utf8.DecodeRuneInString(text[i:])
		if i > 0 {
			pr, _ := utf8.DecodeLastRuneInString(text[:i])
			if !opener(pr) {
				i += n
				continue
			}
		}
		if opener(r) || !startsPath(text[i:]) {
			i += n
			continue
		}
		end := i
		for end < len(text) {
			c, cn := utf8.DecodeRuneInString(text[end:])
			if c == ' ' || c == '\t' || strings.ContainsRune("\"'`<>|", c) {
				break
			}
			end += cn
		}
		for end > i && strings.ContainsRune(".,;:!?)]}", rune(text[end-1])) {
			end--
		}
		out = append(out, Span{i, end, text[i:end]})
		i = max(end, i+1)
	}
	return out
}

// Expand turns a typed path into a real one (~ is the home folder).
func Expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		p = home + p[1:]
	}
	return filepath.Clean(filepath.FromSlash(p))
}

// Exists: the typed path is a file or folder on this PC.
func Exists(p string) bool {
	_, err := os.Stat(Expand(p))
	return err == nil
}

// Word is the path being typed at cursor (runes): its start and the text so far. ok is
// false when the cursor isn't in a path.
func Word(text []rune, cursor int) (start int, word string, ok bool) {
	start = cursor
	for start > 0 && !opener(text[start-1]) {
		start--
	}
	end := cursor
	for end < len(text) && !opener(text[end]) {
		end++
	}
	w := string(text[start:end])
	if !startsPath(w) && w != "~" {
		return 0, "", false
	}
	return start, string(text[start:cursor]), true
}

// Suggest lists what can complete word (typed so far): the entries of its folder that start
// with what's after the last slash. Folders come first and end with a slash, the way word
// writes them. Hidden names only show when you start typing them with a dot.
func Suggest(word string, limit int) []string {
	if word == "~" {
		word += "/"
	}
	cut := strings.LastIndexAny(word, `/\`)
	if cut < 0 {
		return nil
	}
	dirTyped, part := word[:cut+1], word[cut+1:]
	sep := string(word[cut])
	ents, err := os.ReadDir(Expand(dirTyped))
	if err != nil {
		return nil
	}
	lp := strings.ToLower(part)
	var dirs, files []string
	for _, e := range ents {
		n := e.Name()
		if !strings.HasPrefix(strings.ToLower(n), lp) || n == strings.TrimRight(part, `/\`) && !e.IsDir() {
			continue
		}
		if strings.HasPrefix(n, ".") && !strings.HasPrefix(part, ".") {
			continue
		}
		if isDir(e, filepath.Join(Expand(dirTyped), n)) {
			dirs = append(dirs, dirTyped+n+sep)
		} else {
			files = append(files, dirTyped+n)
		}
	}
	less := func(s []string) {
		sort.Slice(s, func(i, j int) bool { return strings.ToLower(s[i]) < strings.ToLower(s[j]) })
	}
	less(dirs)
	less(files)
	out := append(dirs, files...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func isDir(e os.DirEntry, full string) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&os.ModeSymlink != 0 {
		if fi, err := os.Stat(full); err == nil {
			return fi.IsDir()
		}
	}
	return false
}

// Style draws text with st and the paths in it that exist with link.
func Style(text string, st, link lipgloss.Style) string {
	spans := Find(text)
	if len(spans) == 0 {
		return st.Render(text)
	}
	var b strings.Builder
	at := 0
	for _, s := range spans {
		if !Exists(s.Path) {
			continue
		}
		if s.Start > at {
			b.WriteString(st.Render(text[at:s.Start]))
		}
		b.WriteString(link.Render(s.Path))
		at = s.End
	}
	if at < len(text) {
		b.WriteString(st.Render(text[at:]))
	}
	return b.String()
}

// Real lists the paths in text that exist, without repeats.
func Real(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range Find(text) {
		if k := strings.ToLower(Expand(s.Path)); !seen[k] && Exists(s.Path) {
			seen[k] = true
			out = append(out, s.Path)
		}
	}
	return out
}
