package main

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/ngbcoder/Chroma/internal/app"
	"github.com/ngbcoder/Chroma/internal/media"
	"github.com/ngbcoder/Chroma/internal/render"
	"github.com/ngbcoder/Chroma/internal/render/raster"
	"github.com/ngbcoder/Chroma/internal/ui"
)

type msgW struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

var (
	procPeekMessageW2     = syscall.NewLazyDLL("user32.dll").NewProc("PeekMessageW")
	procTranslateMessage2 = syscall.NewLazyDLL("user32.dll").NewProc("TranslateMessage")
	procDispatchMessageW2 = syscall.NewLazyDLL("user32.dll").NewProc("DispatchMessageW")
)

func pumpMessages() bool {
	for {
		var m msgW
		r, _, _ := procPeekMessageW2.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
		if r == 0 {
			return false
		}
		if m.Message == 0x0012 { // WM_QUIT
			return true
		}
		procTranslateMessage2.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW2.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func main() {
	runtime.LockOSThread()

	w, err := ui.NewWindow(640, 260)
	if err != nil {
		fmt.Println("NewWindow:", err)
		return
	}
	defer w.Destroy()

	store := app.NewStore()
	defer store.Close()

	layout := ui.NewLayout()
	dwell := ui.NewDwellController()
	dwell.ExpandDelay = 150 * time.Millisecond
	dwell.CollapseDelay = 400 * time.Millisecond
	dwell.OnExpand = func() { layout.GoTo(ui.StateExpanded) }
	dwell.OnCollapse = func() { layout.GoTo(ui.StateCollapsed) }

	w.OnClick = func(x, y int) {
		sz := layout.SizeNow()
		if sz.W < 300 {
			return
		}
		cx := (float64(w.Width) - sz.W) / 2
		cy := (float64(w.Height) - sz.H) / 2
		by := cy + sz.H - 30
		if float64(y) < by || float64(y) > by+20 {
			return
		}
		if float64(x) >= cx+sz.W/2-40 && float64(x) <= cx+sz.W/2-20 {
			if s, err := media.FetchSession(); err == nil {
				media.Prev(s)
				s.Release()
			}
			return
		}
		if float64(x) >= cx+sz.W/2-10 && float64(x) <= cx+sz.W/2+10 {
			st := store.State()
			if s, err := media.FetchSession(); err == nil {
				if st.Media.Status == 3 {
					media.Pause(s)
				} else {
					media.Play(s)
				}
				s.Release()
			}
			return
		}
		if float64(x) >= cx+sz.W/2+20 && float64(x) <= cx+sz.W/2+40 {
			if s, err := media.FetchSession(); err == nil {
				media.Next(s)
				s.Release()
			}
			return
		}
	}

	poller := ui.NewCursorPoller(dwell, func(x, y int) bool {
		// hot-zone around top-center of primary screen (generous)
		return x >= 400 && x <= 1200 && y >= 0 && y <= 120
	})
	poller.Start()
	defer poller.Stop()

	r := raster.New()

	lc := app.NewLifecycle()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				info, err := media.FetchCurrent()
				if err != nil {
					store.Send(app.MediaUpdate(app.MediaState{Available: false}))
					store.Send(app.Health("smtc", "degraded"))
					continue
				}
				store.Send(app.MediaUpdate(app.MediaState{
					Available:         true,
					SourceApp:         info.SourceApp,
					Title:             info.Title,
					Artist:            info.Artist,
					Album:             info.Album,
					Status:            info.Status,
					Position:          info.Position,
					Duration:          info.Duration,
					PositionUpdatedAt: time.Now(),
				}))
				store.Send(app.Health("smtc", "ok"))
			case <-lc.Context().Done():
				return
			}
		}
	}()
	defer lc.Shutdown()

	// Main loop: pump messages + drive frames
	start := time.Now()
	_ = start
	// progress extrapolation state
	var basePos time.Duration
	var baseTime time.Time
	var lastTitle string
	var lastPolledPos time.Duration

	quit := false
	firstFrame := true
	for !quit {
		if pumpMessages() {
			quit = true
			break
		}

		state := store.State()

		// update base on track change or seek
		if state.Media.Available {
			if state.Media.Title != lastTitle {
				lastTitle = state.Media.Title
				basePos = state.Media.Position
				baseTime = time.Now()
				lastPolledPos = state.Media.Position
			} else if state.Media.Position != lastPolledPos {
				// seeked
				basePos = state.Media.Position
				baseTime = time.Now()
				lastPolledPos = state.Media.Position
			}
		}

		// advance layout
		layout.Step(1.0 / 60.0)

		// render
		r.Begin(w.Width, w.Height)
		r.Clear()
		sz := layout.SizeNow()
		cx := (float64(w.Width) - sz.W) / 2
		cy := (float64(w.Height) - sz.H) / 2
		// background pill
		r.FillRoundedRect(cx, cy, sz.W, sz.H, 18, render.RGBA(15, 15, 18, 220))
		if state.Media.Available && sz.W > 300 {
			title := state.Media.Title
			if len(title) > 42 {
				title = title[:39] + "..."
			}
			r.DrawText(title, cx+16, cy+22, render.TextStyle{Size: 13, Color: render.RGB(255, 255, 255)})
			artist := state.Media.Artist
			if len(artist) > 42 {
				artist = artist[:43] + "..."
			}
			r.DrawText(artist, cx+16, cy+44, render.TextStyle{Size: 11, Color: render.RGB(180, 180, 180)})
			pos := basePos
			if state.Media.Status != 2 && state.Media.Status != 0 {
				pos += time.Since(baseTime)
				if pos > state.Media.Duration {
					pos = state.Media.Duration
				}
				if pos < 0 {
					pos = 0
				}
			}
			if state.Media.Duration > 0 {
				frac := float64(pos) / float64(state.Media.Duration)
				if frac < 0 {
					frac = 0
				}
				if frac > 1 {
					frac = 1
				}
				r.FillRoundedRect(cx+12, cy+sz.H-14, sz.W-24, 3, 1.5, render.RGBA(255, 255, 255, 60))
				r.FillRoundedRect(cx+12, cy+sz.H-14, (sz.W-24)*frac, 3, 1.5, render.RGB(100, 180, 255))
			}
		} else if state.Media.Available {
			title := state.Media.Title
			if len(title) > 24 {
				title = title[:21] + "..."
			}
			// Vertically center text in collapsed pill (height 36px)
			textY := cy + (sz.H - 14) / 2
			r.DrawText(title, cx+14, textY, render.TextStyle{Size: 11, Color: render.RGB(255, 255, 255)})
		} else {
			textY := cy + (sz.H - 16) / 2
			r.DrawText(time.Now().Format("15:04"), cx+14, textY, render.TextStyle{Size: 12, Color: render.RGB(255, 255, 255)})
		}

		// copy raster buffer to window DIB
		if w.Bits() != 0 && len(r.Buf) == w.Width*w.Height*4 {
			dst := unsafe.Slice((*byte)(unsafe.Pointer(w.Bits())), len(r.Buf))
			copy(dst, r.Buf)
		}
		if firstFrame {
			w.Present(100, 50)
			firstFrame = false
		} else {
			w.PresentAtCurrent()
		}

		time.Sleep(16 * time.Millisecond)
	}
}
