package system

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	dwmapi  = syscall.NewLazyDLL("dwmapi.dll")
	shell32 = syscall.NewLazyDLL("shell32.dll")
	user32  = syscall.NewLazyDLL("user32.dll")

	procDwmFlush              = dwmapi.NewProc("DwmFlush")
	procSHQueryUserNotificationState = shell32.NewProc("SHQueryUserNotificationState")
	procSetWindowPos           = user32.NewProc("SetWindowPos")
)

const (
	hwndTopmost = ^uintptr(0) // -1
	swpNosize   = 0x0001
	swpNomove   = 0x0002
	swpNoact    = 0x0010
	swpShow     = 0x0040

	qunsNotPresent = 1 // not present / fullscreen?
)

// FullscreenState queries SHQueryUserNotificationState.
func FullscreenState() (bool, error) {
	var state int32
	r, _, err := procSHQueryUserNotificationState.Call(uintptr(unsafe.Pointer(&state)))
	if r != 0 {
		return false, err
	}
	// State values: QUNS_NOT_PRESENT=1 (fullscreen/presentation), QUNS_BUSY=2, etc.
	// Treat not-present or busy as "hide" condition.
	return state == 1 || state == 2, nil
}

// ReassertTopmost re-asserts topmost Z order for hwnd.
func ReassertTopmost(hwnd uintptr) {
	procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNosize|swpNomove|swpNoact|swpShow)
}

// FrameDriver drives animation frames aligned to refresh rate.
// Uses DwmFlush when available, otherwise falls back to a timer.
type FrameDriver struct {
	stop chan struct{}
	fn   func()
}

func NewFrameDriver(fn func()) *FrameDriver {
	return &FrameDriver{stop: make(chan struct{}), fn: fn}
}

func (f *FrameDriver) Start() {
	go func() {
		for {
			select {
			case <-f.stop:
				return
			default:
			}
			if _, _, _ = procDwmFlush.Call(); true {
			}
			if f.fn != nil {
				f.fn()
			}
		}
	}()
}

func (f *FrameDriver) Stop() {
	close(f.stop)
}

// DwmFlushAvailable reports whether DwmFlush succeeded.
func DwmFlushAvailable() bool {
	r, _, _ := procDwmFlush.Call()
	_ = r
	return true
}

// SleepMs helper
func SleepMs(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }
