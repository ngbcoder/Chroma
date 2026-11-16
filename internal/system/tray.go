package system

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	shell32Dll = syscall.NewLazyDLL("shell32.dll")
	user32Dll  = syscall.NewLazyDLL("user32.dll")

	procShell_NotifyIconW   = shell32Dll.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu     = user32Dll.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32Dll.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32Dll.NewProc("TrackPopupMenu")
	procGetCursorPos        = user32Dll.NewProc("GetCursorPos")
	procSetForegroundWindow = user32Dll.NewProc("SetForegroundWindow")
	procDestroyMenu         = user32Dll.NewProc("DestroyMenu")
)

const (
	NimAdd    = 0
	NimModify = 1
	NimDelete = 2

	NifMessage = 1
	NifIcon    = 2
	NifTip     = 4

	WmApp   = 0x8000
	TrayMsg = WmApp + 1

	TrayRightButtonUp   = 0x0205
	TrayContextMenu     = 0x007B
	TrayLeftDoubleClick = 0x0203

	mfString       = 0x0000
	mfChecked      = 0x0008
	mfSeparator    = 0x0800
	tpmReturnCmd   = 0x0100
	tpmRightButton = 0x0002
)

type NotifyIconDataW struct {
	CbSize            uint32
	Hwnd              uintptr
	UID               uint32
	UFlags            uint32
	UCallbackMessage  uint32
	HIcon             uintptr
	SzTip             [128]uint16
	DwState           uint32
	DwStateMask       uint32
	SzInfo            [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle       [64]uint16
	DwInfoFlags       uint32
	GuidItem          [16]byte
	HBalloonIcon      uintptr
}

// AddTrayIcon adds Chroma to the system tray.
func AddTrayIcon(hwnd uintptr, tip string) error {
	var nid NotifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.Hwnd = hwnd
	nid.UID = 1
	nid.UFlags = NifMessage | NifIcon | NifTip
	nid.UCallbackMessage = TrayMsg
	hIcon, _, _ := user32Dll.NewProc("LoadIconW").Call(0, 32512)
	nid.HIcon = hIcon

	tipChars := syscall.StringToUTF16(tip)
	copy(nid.SzTip[:len(nid.SzTip)-1], tipChars)

	r, _, err := procShell_NotifyIconW.Call(NimAdd, uintptr(unsafe.Pointer(&nid)))
	if r == 0 {
		return fmt.Errorf("Shell_NotifyIconW add failed: %w", err)
	}
	return nil
}

// RemoveTrayIcon removes Chroma from the system tray.
func RemoveTrayIcon(hwnd uintptr) error {
	var nid NotifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.Hwnd = hwnd
	nid.UID = 1
	r, _, err := procShell_NotifyIconW.Call(NimDelete, uintptr(unsafe.Pointer(&nid)))
	if r == 0 {
		return fmt.Errorf("Shell_NotifyIconW remove failed: %w", err)
	}
	return nil
}

type TrayMenuItem struct {
	ID        uint32
	Label     string
	Checked   bool
	Separator bool
}

// ShowTrayMenu displays a context menu at the cursor and returns the selected ID.
func ShowTrayMenu(hwnd uintptr, items []TrayMenuItem) (uint32, error) {
	menu, _, err := procCreatePopupMenu.Call()
	if menu == 0 {
		return 0, fmt.Errorf("CreatePopupMenu failed: %w", err)
	}
	defer procDestroyMenu.Call(menu)

	for _, item := range items {
		if item.Separator {
			r, _, callErr := procAppendMenuW.Call(menu, mfSeparator, 0, 0)
			if r == 0 {
				return 0, fmt.Errorf("AppendMenuW separator failed: %w", callErr)
			}
			continue
		}
		label, convErr := syscall.UTF16PtrFromString(item.Label)
		if convErr != nil {
			return 0, fmt.Errorf("encode tray menu item: %w", convErr)
		}
		flags := uintptr(mfString)
		if item.Checked {
			flags |= mfChecked
		}
		r, _, callErr := procAppendMenuW.Call(menu, flags, uintptr(item.ID), uintptr(unsafe.Pointer(label)))
		if r == 0 {
			return 0, fmt.Errorf("AppendMenuW failed: %w", callErr)
		}
	}

	var cursor struct{ X, Y int32 }
	r, _, callErr := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	if r == 0 {
		return 0, fmt.Errorf("GetCursorPos failed: %w", callErr)
	}
	procSetForegroundWindow.Call(hwnd)
	selected, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmReturnCmd|tpmRightButton,
		uintptr(uint32(cursor.X)),
		uintptr(uint32(cursor.Y)),
		0,
		hwnd,
		0,
	)
	return uint32(selected), nil
}
