package ui

import (
	"fmt"
	"syscall"
	"unsafe"
)

// --- constants ---

const (
	wsPopup = 0x80000000

	wsExLayered    = 0x00080000
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	wsExNoActivate = 0x08000000

	cwUseDefault = 0x80000000

	wmDestroy       = 0x0002
	wmPaint         = 0x000F
	wmMouseActivate = 0x0021
	wmLButtonDown  = 0x0201
	wmMouseMove    = 0x0200
	wmLButtonUp    = 0x0202
	wmNCHitTest    = 0x0084

	maNoActivate = 3

	swShowNA = 8

	ulwAlpha = 0x00000002

	acSrcOver  = 0x00
	acSrcAlpha = 0x01

	biRgb = 0

	dibRgbColors = 0
)

// --- lazy DLL procs ---

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW   = user32.NewProc("RegisterClassExW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procShowWindow         = user32.NewProc("ShowWindow")
	procUpdateLayeredWnd   = user32.NewProc("UpdateLayeredWindow")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procLoadCursorW        = user32.NewProc("LoadCursorW")
	procGetWindowRect2     = user32.NewProc("GetWindowRect")
	procSetWindowPos2      = user32.NewProc("SetWindowPos")
	procGetCursorPos2      = user32.NewProc("GetCursorPos")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
)

// --- structs ---

type point struct {
	X, Y int32
}
type size struct {
	CX, CY int32
}
type rect struct {
	Left, Top, Right, Bottom int32
}
type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
	BmiColors [1]uint32
}

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

// --- Window ---

// Window wraps a layered HWND.
type Window struct {
	Hwnd   uintptr
	Width  int
	Height int

	OnClick func(x, y int)

	// drag state
	Dragging   bool
	DragOffX   int
	DragOffY   int
	DidDrag    bool

	className *uint16

	memDC  uintptr
	dib    uintptr
	bits   uintptr // pointer to pixel buffer
	oldObj uintptr
}

var (
	windowsByHwnd = map[uintptr]*Window{}
	mainWndProc   = syscall.NewCallback(wndProcCallback)
)

func wndProcCallback(hwnd, msgUint, wParam, lParam uintptr) uintptr {
	w, ok := windowsByHwnd[hwnd]
	switch uint32(msgUint) {
	case wmMouseActivate:
		return maNoActivate
	case wmLButtonDown:
		if ok && w != nil {
			// record drag offset
			var pt struct{ X, Y int32 }
			procGetCursorPos2.Call(uintptr(unsafe.Pointer(&pt)))
			var rc struct{ L, T, R, B int32 }
			procGetWindowRect2.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
			w.DragOffX = int(pt.X) - int(rc.L)
			w.DragOffY = int(pt.Y) - int(rc.T)
			w.Dragging = true
			w.DidDrag = false
		}
		return 0
	case wmMouseMove:
		if ok && w != nil && w.Dragging {
			// check movement threshold
			// move window
			var pt struct{ X, Y int32 }
			procGetCursorPos2.Call(uintptr(unsafe.Pointer(&pt)))
			newX := int(pt.X) - w.DragOffX
			newY := int(pt.Y) - w.DragOffY
			w.DidDrag = true
			procSetWindowPos2.Call(hwnd, 0, uintptr(newX), uintptr(newY), 0, 0, 0x0010|0x0001|0x0004) // SWP_NOZORDER|NOSIZE|NOACTIVATE
		}
		return 0
	case wmLButtonUp:
		if ok && w != nil {
			if w.Dragging && !w.DidDrag && w.OnClick != nil {
				x := int(int16(lParam & 0xFFFF))
				y := int(int16((lParam >> 16) & 0xFFFF))
				w.OnClick(x, y)
			}
			w.Dragging = false
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msgUint, wParam, lParam)
	return r
}

// NewWindow creates a layered, topmost, tool window of the given size.
func NewWindow(width, height int) (*Window, error) {
	inst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)

	className, _ := syscall.UTF16PtrFromString("ChromaLayeredWindow")

	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   mainWndProc,
		HInstance:     inst,
		LpszClassName: className,
	}
	// default arrow cursor
	cur, _, _ := procLoadCursorW.Call(0, uintptr(32512)) // IDC_ARROW
	wc.HCursor = cur

	r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		return nil, fmt.Errorf("RegisterClassExW failed: %w", err)
	}

	w := &Window{Width: width, Height: height, className: className}

	exStyle := uintptr(wsExLayered | wsExTopmost | wsExToolWindow | wsExNoActivate)
	style := uintptr(wsPopup)

	hwnd, _, err := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(mustUTF16Ptr("Chroma"))),
		style,
		cwUseDefault, cwUseDefault,
		uintptr(width), uintptr(height),
		0, 0, inst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowExW failed: %w", err)
	}
	w.Hwnd = hwnd
	windowsByHwnd[hwnd] = w

	// create DIB + mem DC sized to canvas
	if err := w.ensureDIB(); err != nil {
		return nil, err
	}

	// show without activating
	procShowWindow.Call(hwnd, swShowNA)
	return w, nil
}

func mustUTF16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func (w *Window) ensureDIB() error {
	if w.memDC != 0 {
		return nil
	}
	// memory DC compatible with screen
	screenDC, _, _ := procGetDC.Call(0)
	memDC, _, err := procCreateCompatibleDC.Call(screenDC)
	procReleaseDC.Call(0, screenDC)
	if memDC == 0 {
		return fmt.Errorf("CreateCompatibleDC failed: %w", err)
	}
	w.memDC = memDC

	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       int32(w.Width),
			BiHeight:      -int32(w.Height), // top-down
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: biRgb,
		},
	}
	var bits uintptr
	dib, _, err := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bi)), dibRgbColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 {
		return fmt.Errorf("CreateDIBSection failed: %w", err)
	}
	w.dib = dib
	w.bits = bits
	old, _, _ := procSelectObject.Call(memDC, dib)
	w.oldObj = old
	return nil
}

// Bits returns the raw pixel buffer (premul 32-bit BGRA, top-down).
func (w *Window) Bits() uintptr { return w.bits }

func (w *Window) Stride() int { return w.Width * 4 }

// PresentAtCurrent uploads the DIB keeping the window's current position.
func (w *Window) PresentAtCurrent() error {
	// get current rect
	var rc struct{ L, T, R, B int32 }
	procGetWindowRect2.Call(w.Hwnd, uintptr(unsafe.Pointer(&rc)))
	return w.Present(int(rc.L), int(rc.T))
}

// Present uploads the DIB to the layered window at (x,y).
func (w *Window) Present(x, y int) error {
	ptDst := point{X: int32(x), Y: int32(y)}
	sz := size{CX: int32(w.Width), CY: int32(w.Height)}
	ptSrc := point{X: 0, Y: 0}
	blend := blendFunction{
		BlendOp:             acSrcOver,
		SourceConstantAlpha: 255,
		AlphaFormat:         acSrcAlpha,
	}
	r, _, err := procUpdateLayeredWnd.Call(
		w.Hwnd,
		0, // hdcDst (screen)
		uintptr(unsafe.Pointer(&ptDst)),
		uintptr(unsafe.Pointer(&sz)),
		w.memDC,
		uintptr(unsafe.Pointer(&ptSrc)),
		0, // crKey
		uintptr(unsafe.Pointer(&blend)),
		ulwAlpha,
	)
	if r == 0 {
		return fmt.Errorf("UpdateLayeredWindow failed: %w", err)
	}
	return nil
}

// Destroy releases resources.
func (w *Window) Destroy() {
	if w.memDC != 0 {
		if w.oldObj != 0 {
			procSelectObject.Call(w.memDC, w.oldObj)
		}
		if w.dib != 0 {
			procDeleteObject.Call(w.dib)
		}
		procDeleteDC.Call(w.memDC)
	}
	if w.Hwnd != 0 {
		procDestroyWindow.Call(w.Hwnd)
		delete(windowsByHwnd, w.Hwnd)
	}
}

// RunMessagePump runs the Win32 message loop until WM_QUIT.
func RunMessagePump() {
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// DPI helpers placeholder
func ScaleForDPI(v float64, dpi int) float64 {
	return v * float64(dpi) / 96.0
}
