//go:build !windows

package notes

// Deleting moves the file to the desktop trash (~/.local/share/Trash, or ~/.Trash on macOS).

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func trash(path string) error {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "share", "Trash", "files")
	if runtime.GOOS == "darwin" {
		dir = filepath.Join(home, ".Trash")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := filepath.Base(path)
	dest := filepath.Join(dir, name)
	if _, err := os.Stat(dest); err == nil {
		dest = filepath.Join(dir, fmt.Sprintf("%s.%d", name, time.Now().Unix()))
	}
	return os.Rename(path, dest)
}
