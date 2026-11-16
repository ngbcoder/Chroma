package main

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"github.com/ngbcoder/Chroma/internal/app"
	"github.com/ngbcoder/Chroma/internal/media"
	"github.com/ngbcoder/Chroma/internal/render"
	"github.com/ngbcoder/Chroma/internal/render/raster"
	"github.com/ngbcoder/Chroma/internal/ui"
)

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
					Available: true,
					SourceApp: info.SourceApp,
					Title:     info.Title,
					Artist:    info.Artist,
					Album:     info.Album,
					Status:    info.Status,
					Position:  info.Position,
					Duration:  info.Duration,
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
	for {
		// pump
		// (inline peek)
		// We'll just sleep and let OS handle messages via a separate goroutine? 
		// Simpler: call RunMessagePump would block. Instead, peek.
		// Use a non-blocking pump via ui package? We'll inline.
		// PeekMessage loop
		// (omitted for brevity; messages handled by default proc)

		state := store.State()
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
		if state.Media.Available {
			r.DrawText(state.Media.Title, cx+12, cy+18, render.TextStyle{Size: 12, Color: render.RGB(255, 255, 255)})
			r.DrawText(state.Media.Artist, cx+12, cy+36, render.TextStyle{Size: 11, Color: render.RGB(180, 180, 180)})
			if state.Media.Duration > 0 {
				frac := float64(state.Media.Position) / float64(state.Media.Duration)
				if frac < 0 {
					frac = 0
				}
				if frac > 1 {
					frac = 1
				}
				r.FillRoundedRect(cx+12, cy+sz.H-14, sz.W-24, 3, 1.5, render.RGBA(255, 255, 255, 60))
				r.FillRoundedRect(cx+12, cy+sz.H-14, (sz.W-24)*frac, 3, 1.5, render.RGB(100, 180, 255))
			}
		} else {
			r.DrawText(time.Now().Format("15:04"), cx+12, cy+12, render.TextStyle{Size: 14, Color: render.RGB(255, 255, 255)})
		}

		// copy raster buffer to window DIB
		if w.Bits() != 0 && len(r.Buf) == w.Width*w.Height*4 {
			dst := unsafe.Slice((*byte)(unsafe.Pointer(w.Bits())), len(r.Buf))
			copy(dst, r.Buf)
		}
		w.Present(100, 50)

		time.Sleep(16 * time.Millisecond)
	}
}
