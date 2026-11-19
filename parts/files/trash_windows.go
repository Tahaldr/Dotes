//go:build windows

package files

// The real Windows Recycle Bin: trash() is the same as deleting in Explorer, and restore()
// puts a file back (used by undo) by reading the Recycle Bin's own records.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procSHFileOperationW = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

type shFileOp struct {
	hwnd          uintptr
	wFunc         uint32
	pFrom         *uint16
	pTo           *uint16
	fFlags        uint16
	anyAborted    int32
	nameMappings  uintptr
	progressTitle *uint16
}

const (
	foDelete         = 3
	fofSilent        = 0x0004
	fofNoConfirm     = 0x0010
	fofAllowUndo     = 0x0040
	fofNoErrorUI     = 0x0400
	fofWantNukeWarn  = 0x4000
	trashFlags       = fofSilent | fofNoConfirm | fofAllowUndo | fofNoErrorUI
	trashFlagsStrict = trashFlags | fofWantNukeWarn
)

func trash(path string) error {
	from, err := windows.UTF16FromString(path) // ends with one 0
	if err != nil {
		return err
	}
	from = append(from, 0) // the list ends with a second 0
	op := shFileOp{wFunc: foDelete, pFrom: &from[0], fFlags: trashFlags}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if r != 0 {
		return fmt.Errorf("error 0x%x", r)
	}
	if op.anyAborted != 0 {
		return errCanceled
	}
	return nil
}

// restore finds the newest Recycle Bin copy of orig and moves it back
func restore(orig string) error {
	if _, err := os.Lstat(orig); err == nil {
		return fmt.Errorf("%s exists again", filepath.Base(orig))
	}
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return err
	}
	bin := filepath.VolumeName(orig) + `\$Recycle.Bin\` + u.User.Sid.String()
	des, err := os.ReadDir(bin)
	if err != nil {
		return fmt.Errorf("can't read the Recycle Bin: %v", errText(err))
	}
	bestI, bestTime := "", int64(-1)
	for _, de := range des {
		if !strings.HasPrefix(de.Name(), "$I") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(bin, de.Name()))
		if err != nil || len(b) < 28 {
			continue
		}
		name, when := parseRecycleInfo(b)
		if strings.EqualFold(name, orig) && when > bestTime {
			bestI, bestTime = de.Name(), when
		}
	}
	if bestI == "" {
		return errors.New("not in the Recycle Bin any more")
	}
	if err := os.Rename(filepath.Join(bin, "$R"+bestI[2:]), orig); err != nil {
		return fmt.Errorf("couldn't restore %s: %v", filepath.Base(orig), errText(err))
	}
	os.Remove(filepath.Join(bin, bestI))
	return nil
}

// parseRecycleInfo reads a $I file: version, size, deletion time, original path
func parseRecycleInfo(b []byte) (string, int64) {
	ver := binary.LittleEndian.Uint64(b[0:8])
	when := int64(binary.LittleEndian.Uint64(b[16:24]))
	var raw []byte
	if ver >= 2 {
		n := int(binary.LittleEndian.Uint32(b[24:28]))
		raw = b[28:min(len(b), 28+2*n)]
	} else {
		raw = b[24:min(len(b), 24+520)]
	}
	u := make([]uint16, len(raw)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(raw[2*i:])
	}
	s := string(utf16.Decode(u))
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return s, when
}
