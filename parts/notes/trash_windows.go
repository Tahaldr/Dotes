//go:build windows

package notes

// Deleting a list or note moves it to the real Recycle Bin (like deleting in Explorer).

import (
	"fmt"
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

func trash(path string) error {
	from, err := windows.UTF16FromString(path)
	if err != nil {
		return err
	}
	from = append(from, 0) // the list ends with a second 0
	// FO_DELETE, silent + no confirm + allow undo (Recycle Bin) + no error UI
	op := shFileOp{wFunc: 3, pFrom: &from[0], fFlags: 0x0004 | 0x0010 | 0x0040 | 0x0400}
	if r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op))); r != 0 {
		return fmt.Errorf("error 0x%x", r)
	}
	if op.anyAborted != 0 {
		return fmt.Errorf("canceled")
	}
	return nil
}
