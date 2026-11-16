package ui

import (
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var procGetCursorPos = syscall.NewLazyDLL("user32.dll").NewProc("GetCursorPos")

type pointI struct{ X, Y int32 }

func getCursorPos() (int, int, bool) {
	var p pointI
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		return 0, 0, false
	}
	return int(p.X), int(p.Y), true
}

// DwellController implements the 150ms expand / 400ms collapse debounce.
type DwellController struct {
	ExpandDelay  time.Duration
	CollapseDelay time.Duration

	mu sync.Mutex

	hovering   bool
	hoverStart time.Time
	leaveStart time.Time
	expanded   bool

	OnExpand   func()
	OnCollapse func()
}

func NewDwellController() *DwellController {
	return &DwellController{
		ExpandDelay:   150 * time.Millisecond,
		CollapseDelay: 400 * time.Millisecond,
	}
}

// Update samples hover state. inHot indicates the cursor is within the hot-zone.
// now is the sample time. Callbacks may fire.
func (d *DwellController) Update(inHot bool, now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if inHot {
		if !d.hovering {
			d.hovering = true
			d.hoverStart = now
			d.leaveStart = time.Time{}
		}
		if !d.expanded && now.Sub(d.hoverStart) >= d.ExpandDelay {
			d.expanded = true
			if d.OnExpand != nil {
				d.OnExpand()
			}
		}
	} else {
		if d.hovering {
			d.hovering = false
			d.leaveStart = now
		}
		if d.expanded && !d.hovering && !d.leaveStart.IsZero() && now.Sub(d.leaveStart) >= d.CollapseDelay {
			d.expanded = false
			d.leaveStart = time.Time{}
			if d.OnCollapse != nil {
				d.OnCollapse()
			}
		}
	}
}

func (d *DwellController) Expanded() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.expanded
}

// CursorPoller polls GetCursorPos at ~60 Hz while running.
type CursorPoller struct {
	Controller *DwellController
	// hot-zone test
	InHotZone func(x, y int) bool

	stop chan struct{}
}

func NewCursorPoller(c *DwellController, inHot func(int, int) bool) *CursorPoller {
	return &CursorPoller{Controller: c, InHotZone: inHot, stop: make(chan struct{})}
}

func (p *CursorPoller) Start() {
	go func() {
		tick := time.NewTicker(time.Second / 60)
		defer tick.Stop()
		for {
			select {
			case <-p.stop:
				return
			case now := <-tick.C:
				x, y, ok := getCursorPos()
				if !ok {
					continue
				}
				inHot := false
				if p.InHotZone != nil {
					inHot = p.InHotZone(x, y)
				}
				p.Controller.Update(inHot, now)
			}
		}
	}()
}

func (p *CursorPoller) Stop() {
	close(p.stop)
}
