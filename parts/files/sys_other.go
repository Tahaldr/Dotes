//go:build !windows

package files

import (
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func isHidden(name string, _ fs.FileInfo) bool { return strings.HasPrefix(name, ".") }

func openDefault(path string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, path).Start()
}

func fileBtime(info fs.FileInfo) time.Time { return info.ModTime() }

func fileAtime(fs.FileInfo) time.Time { return time.Time{} }
func fileAttrs(fs.FileInfo) string    { return "" }

func fileOwner(path string) string {
	fi, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		if u, err := user.LookupId(strconv.Itoa(int(st.Uid))); err == nil {
			return u.Username
		}
	}
	return ""
}

func listDrives() []entry {
	out := []entry{{name: "/", path: "/", isDir: true, drive: true}}
	for _, base := range []string{"/mnt", "/media", "/Volumes"} {
		des, _ := os.ReadDir(base)
		for _, d := range des {
			if d.IsDir() {
				p := base + "/" + d.Name()
				out = append(out, entry{name: d.Name(), path: p, isDir: true, drive: true, label: p})
			}
		}
	}
	return out
}

func openRecycleBin() error {
	home, _ := os.UserHomeDir()
	return openDefault(home + "/.local/share/Trash/files")
}
