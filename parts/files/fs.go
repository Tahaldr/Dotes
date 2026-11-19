package files

// Reading folders: entries, hidden files, sorting.

import (
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

type entry struct {
	name, path string
	isDir      bool // folders, and links that point to folders
	isLink     bool // symlinks and junctions (never followed when walking)
	isRepo     bool // folder with a .git inside
	hidden     bool // dot file, or Hidden / System on Windows
	drive      bool // a row in the drives view
	size       int64
	mod        time.Time
	btime      time.Time // created
	label      string    // drives: volume name and free space
	matchLine  int       // text search: first matching line (1-based, 0 = none)
	matchText  string
	mode       fs.FileMode
}

// readDir lists a folder: folders first, then files, natural order ("file2" before "file10").
// hidden entries are left out unless showHidden; hiddenN counts them either way.
func readDir(dir string, showHidden bool) (list []entry, hiddenN int, err error) {
	des, err := os.ReadDir(dir)
	if err != nil && len(des) == 0 {
		return nil, 0, err
	}
	list = make([]entry, 0, len(des))
	for _, de := range des {
		name := de.Name()
		p := filepath.Join(dir, name)
		e := entry{name: name, path: p}
		info, ierr := de.Info()
		if ierr == nil {
			e.size, e.mod, e.mode = info.Size(), info.ModTime(), info.Mode()
			e.hidden = isHidden(name, info)
			e.btime = fileBtime(info)
		} else {
			e.hidden = strings.HasPrefix(name, ".")
		}
		e.isDir = de.IsDir()
		if e.mode&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			e.isLink = true
			if t, err := os.Stat(p); err == nil && t.IsDir() {
				e.isDir = true
			}
		}
		if e.hidden {
			hiddenN++
			if !showHidden {
				continue
			}
		}
		if e.isDir && !e.isLink {
			if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
				e.isRepo = true
			}
		}
		list = append(list, e)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].isDir != list[j].isDir {
			return list[i].isDir
		}
		return natLess(list[i].name, list[j].name)
	})
	return list, hiddenN, nil
}

// natLess compares names without case, numbers by value: "img2" < "img10"
func natLess(a, b string) bool {
	ar, br := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		if unicode.IsDigit(ar[i]) && unicode.IsDigit(br[j]) {
			si, sj := i, j
			for i < len(ar) && unicode.IsDigit(ar[i]) {
				i++
			}
			for j < len(br) && unicode.IsDigit(br[j]) {
				j++
			}
			na := strings.TrimLeft(string(ar[si:i]), "0")
			nb := strings.TrimLeft(string(br[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ar[i] != br[j] {
			return ar[i] < br[j]
		}
		i++
		j++
	}
	if len(ar)-i != len(br)-j {
		return len(ar)-i < len(br)-j
	}
	return a < b
}

// humanSize: "812 B", "4.2 kB", "1.3 MB", "2.0 GB"
func humanSize(n int64) string {
	const k = 1000
	switch {
	case n < k:
		return fmt.Sprintf("%d B", n)
	case n < k*k:
		return fmt.Sprintf("%.1f kB", float64(n)/k)
	case n < k*k*k:
		return fmt.Sprintf("%.1f MB", float64(n)/k/k)
	}
	return fmt.Sprintf("%.1f GB", float64(n)/k/k/k)
}

// shortSize for list rows: "812", "4.2k", "1.3M"
func shortSize(n int64) string {
	const k = 1000
	switch {
	case n < k:
		return fmt.Sprint(n)
	case n < k*k:
		return fmt.Sprintf("%.1fk", float64(n)/k)
	case n < k*k*k:
		return fmt.Sprintf("%.1fM", float64(n)/k/k)
	}
	return fmt.Sprintf("%.1fG", float64(n)/k/k/k)
}

// prettyPath shows the home folder as ~
func prettyPath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && (strings.EqualFold(p, home) || strings.HasPrefix(strings.ToLower(p), strings.ToLower(home)+string(filepath.Separator))) {
		return "~" + p[len(home):]
	}
	return p
}

// isRoot: C:\ or /
func isRoot(dir string) bool { return filepath.Dir(dir) == dir }

// gitBranch finds the branch of the repo dir is in ("" outside a repo)
func gitBranch(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		head := filepath.Join(d, ".git", "HEAD")
		if b, err := os.ReadFile(head); err == nil {
			s := strings.TrimSpace(string(b))
			if r, ok := strings.CutPrefix(s, "ref: refs/heads/"); ok {
				return r
			}
			if len(s) > 7 {
				return s[:7] // detached: short commit
			}
			return s
		}
		if isRoot(d) {
			return ""
		}
	}
}

// sortEntries re-sorts a folder (folders stay first). by: natural, alpha, mtime, btime, ext, size, random
func sortEntries(list []entry, by string, rev bool) {
	if by == "random" {
		rand.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
		sort.SliceStable(list, func(i, j int) bool { return list[i].isDir && !list[j].isDir })
		return
	}
	less := func(a, b *entry) bool {
		switch by {
		case "alpha":
			return strings.ToLower(a.name) < strings.ToLower(b.name)
		case "mtime":
			return a.mod.After(b.mod) // newest first
		case "btime":
			return a.btime.After(b.btime)
		case "size":
			return a.size > b.size // biggest first
		case "ext":
			ea, eb := strings.ToLower(filepath.Ext(a.name)), strings.ToLower(filepath.Ext(b.name))
			if ea != eb {
				return ea < eb
			}
		}
		return natLess(a.name, b.name)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := &list[i], &list[j]
		if a.isDir != b.isDir {
			return a.isDir
		}
		if rev {
			return less(b, a)
		}
		return less(a, b)
	})
}

// shortTime: "Nov 17 14:02" this year, "Nov 17  2025" before
func shortTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if t.Year() == time.Now().Year() {
		return t.Format("Jan 02 15:04")
	}
	return t.Format("Jan 02  2006")
}
