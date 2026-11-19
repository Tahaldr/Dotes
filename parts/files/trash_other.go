//go:build !windows

package files

// The freedesktop trash (~/.local/share/Trash), used by Linux file managers.

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func trashDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "Trash")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "Trash")
}

func trash(path string) error {
	dir := trashDir()
	os.MkdirAll(filepath.Join(dir, "files"), 0o700)
	os.MkdirAll(filepath.Join(dir, "info"), 0o700)
	dest := freeName(filepath.Join(dir, "files"), filepath.Base(path))
	name := filepath.Base(dest)
	info := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n", (&url.URL{Path: path}).EscapedPath(), time.Now().Format("2006-01-02T15:04:05"))
	if err := os.WriteFile(filepath.Join(dir, "info", name+".trashinfo"), []byte(info), 0o600); err != nil {
		return err
	}
	return os.Rename(path, dest)
}

func restore(orig string) error {
	if _, err := os.Lstat(orig); err == nil {
		return fmt.Errorf("%s exists again", filepath.Base(orig))
	}
	dir := trashDir()
	des, _ := os.ReadDir(filepath.Join(dir, "info"))
	best, bestDate := "", ""
	for _, de := range des {
		b, err := os.ReadFile(filepath.Join(dir, "info", de.Name()))
		if err != nil {
			continue
		}
		var p, date string
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, "Path="); ok {
				p, _ = url.PathUnescape(v)
			}
			if v, ok := strings.CutPrefix(l, "DeletionDate="); ok {
				date = v
			}
		}
		if p == orig && date >= bestDate {
			best, bestDate = strings.TrimSuffix(de.Name(), ".trashinfo"), date
		}
	}
	if best == "" {
		return errors.New("not in the trash any more")
	}
	if err := os.Rename(filepath.Join(dir, "files", best), orig); err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, "info", best+".trashinfo"))
}
