//go:build windows

package files

import (
	"io/fs"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// isHidden: dot files, and files Windows marks Hidden or System (desktop.ini, NTUSER.DAT, AppData)
func isHidden(name string, info fs.FileInfo) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.FileAttributes&(syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM) != 0
	}
	return false
}

func fileBtime(info fs.FileInfo) time.Time {
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, d.CreationTime.Nanoseconds())
	}
	return info.ModTime()
}

func fileAtime(info fs.FileInfo) time.Time {
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, d.LastAccessTime.Nanoseconds())
	}
	return time.Time{}
}

// fileAttrs: the Windows attributes in words
func fileAttrs(info fs.FileInfo) string {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return ""
	}
	var out []string
	for _, a := range []struct {
		bit  uint32
		name string
	}{{syscall.FILE_ATTRIBUTE_READONLY, "read-only"}, {syscall.FILE_ATTRIBUTE_HIDDEN, "hidden"},
		{syscall.FILE_ATTRIBUTE_SYSTEM, "system"}, {syscall.FILE_ATTRIBUTE_ARCHIVE, "archive"},
		{0x800, "compressed"}, {0x4000, "encrypted"}, {0x400, "link / junction"}, {0x1000, "offline"},
		{0x400000, "online only (cloud)"}} {
		if d.FileAttributes&a.bit != 0 {
			out = append(out, a.name)
		}
	}
	return strings.Join(out, ", ")
}

// owner: "DOMAIN\user" of a file (cached: it's a slow call)
var ownerCache sync.Map

func fileOwner(path string) string {
	if v, ok := ownerCache.Load(path); ok {
		return v.(string)
	}
	name := ""
	if sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION); err == nil {
		if sid, _, err := sd.Owner(); err == nil && sid != nil {
			if acc, _, _, err := sid.LookupAccount(""); err == nil {
				name = acc
			}
		}
	}
	ownerCache.Store(path, name)
	return name
}

// listDrives: C:\, D:\ ... with their name and free space
func listDrives() []entry {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []entry
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letter := string(rune('A' + i))
		root := letter + `:\`
		e := entry{name: letter + ":", path: root, isDir: true, drive: true}
		rp, _ := windows.UTF16PtrFromString(root)
		var vol [261]uint16
		var serial, maxLen, flags uint32
		if windows.GetVolumeInformation(rp, &vol[0], 261, &serial, &maxLen, &flags, nil, 0) == nil {
			e.label = windows.UTF16ToString(vol[:])
		}
		var free, total, totalFree uint64
		if windows.GetDiskFreeSpaceEx(rp, &free, &total, &totalFree) == nil && total > 0 {
			e.size = int64(total)
			e.label = strings.TrimSpace(e.label + "  " + humanSize(int64(free)) + " free of " + humanSize(int64(total)))
		}
		out = append(out, e)
	}
	return out
}

// openRecycleBin shows the Recycle Bin in Explorer (it isn't a normal folder)
func openRecycleBin() error {
	return exec.Command("explorer.exe", "shell:RecycleBinFolder").Start()
}

// openDefault opens a file in the app Windows uses for it (like double-clicking it in Explorer)
func openDefault(path string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
