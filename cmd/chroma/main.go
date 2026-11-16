package main

import (
	"fmt"
	"math"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/ngbcoder/Chroma/internal/app"
	"github.com/ngbcoder/Chroma/internal/config"
	"github.com/ngbcoder/Chroma/internal/media"
	"github.com/ngbcoder/Chroma/internal/render"
	"github.com/ngbcoder/Chroma/internal/render/gdiplus"
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

func getScreenSize() (int, int) {
	cx, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("GetSystemMetrics").Call(0)
	cy, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("GetSystemMetrics").Call(1)
	return int(cx), int(cy)
}

func main() {
	runtime.LockOSThread()
	fmt.Println("Starting Chroma...")

	w, err := ui.NewWindow(640, 260)
	if err != nil {
		fmt.Println("NewWindow:", err)
		return
	}
	fmt.Println("Window created successfully, HWND:", w.Hwnd)
	defer w.Destroy()

	cfg, _ := config.Load("")
	w.Locked = cfg.Behavior.Locked

	if cfg.WindowX == 0 && cfg.WindowY == 0 {
		sw, sh := getScreenSize()
		winW, winH := 640, 260
		var px, py int
		switch cfg.PresetLocation {
		case "top_left":
			px, py = 20, 10
		case "top_right":
			px, py = sw - winW - 20, 10
		case "bottom_center":
			px, py = (sw - winW) / 2, sh - winH - 50
		case "bottom_left":
			px, py = 20, sh - winH - 50
		case "bottom_right":
			px, py = sw - winW - 20, sh - winH - 50
		default: // "top_center"
			px, py = (sw - winW) / 2, 10
		}
		w.Move(px, py)
	} else {
		w.Move(cfg.WindowX, cfg.WindowY)
	}
	defer func() {
		l, t, _, _ := w.Rect()
		cfg.WindowX = l
		cfg.WindowY = t
		cfg.Save()
	}()

	store := app.NewStore()
	defer store.Close()

	layout := ui.NewLayout()
	dwell := ui.NewDwellController()
	dwell.ExpandDelay = time.Duration(cfg.Behavior.DwellDelayMs) * time.Millisecond
	dwell.CollapseDelay = time.Duration(cfg.Behavior.LeaveDelayMs) * time.Millisecond
	if cfg.Theme.Snappiness > 0 {
		k := cfg.Theme.Snappiness * 420.0
		layout.Width.SetParams(k, 22)
		layout.Height.SetParams(k, 22)
	}
	cfg.Watch(func() {
		if newCfg, err := config.Load(""); err == nil {
			dwell.ExpandDelay = time.Duration(newCfg.Behavior.DwellDelayMs) * time.Millisecond
			dwell.CollapseDelay = time.Duration(newCfg.Behavior.LeaveDelayMs) * time.Millisecond
			if newCfg.Theme.Snappiness > 0 {
				k := newCfg.Theme.Snappiness * 420.0
				layout.Width.SetParams(k, 22)
				layout.Height.SetParams(k, 22)
			}
		}
	})

	dwell.OnExpand = func() {
		layout.GoTo(ui.StateExpanded)
		layout.Glow.SetTarget(1.0)
	}
	dwell.OnCollapse = func() {
		layout.GoTo(ui.StateCollapsed)
		layout.Glow.SetTarget(0.0)
	}

	w.OnClick = func(x, y int) {
		sz := layout.SizeNow()
		if sz.W < 300 {
			return
		}
		cx := (float64(w.Width) - sz.W) / 2
		cy := (float64(w.Height) - sz.H) / 2
		by := cy + 92
		if float64(y) < by || float64(y) > by+28 {
			return
		}
		ctrlX := cx + sz.W/2 - 60
		relX := float64(x) - ctrlX
		if relX >= 0 && relX < 40 {
			if s, err := media.FetchSession(); err == nil {
				media.Prev(s)
				s.Release()
			}
			return
		}
		if relX >= 40 && relX < 80 {
			st := store.State()
			if s, err := media.FetchSession(); err == nil {
				if st.Media.Status == 4 {
					media.Pause(s)
				} else {
					media.Play(s)
				}
				s.Release()
			}
			return
		}
		if relX >= 80 && relX <= 120 {
			if s, err := media.FetchSession(); err == nil {
				media.Next(s)
				s.Release()
			}
			return
		}
	}

	procGetWindowRectMain := syscall.NewLazyDLL("user32.dll").NewProc("GetWindowRect")
	poller := ui.NewCursorPoller(dwell, func(x, y int) bool {
		var rc struct{ L, T, R, B int32 }
		procGetWindowRectMain.Call(w.Hwnd, uintptr(unsafe.Pointer(&rc)))
		if rc.R > rc.L {
			return int32(x) >= rc.L-40 && int32(x) <= rc.R+40 && int32(y) >= rc.T-40 && int32(y) <= rc.B+40
		}
		return x >= 400 && x <= 1200 && y >= 0 && y <= 120
	})
	poller.Start()
	defer poller.Stop()

	var r render.Backend
	gdiR, err := gdiplus.New()
	if err == nil {
		r = gdiR
	} else {
		r = raster.New()
	}

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
					PositionUpdatedAt: info.PositionUpdatedAt,
					Thumbnail:         info.Thumbnail,
				}))
				store.Send(app.Health("smtc", "ok"))
			case <-lc.Context().Done():
				return
			}
		}
	}()
	defer lc.Shutdown()

	// Main loop: pump messages + drive frames
	quit := false
	firstFrame := true
	for !quit {
		if pumpMessages() {
			quit = true
			break
		}

		state := store.State()

		// advance layout
		layout.Step(1.0 / 60.0)

		// render
		r.Begin(w.Width, w.Height)
		r.Clear()
		sz := layout.SizeNow()
		cx := (float64(w.Width) - sz.W) / 2
		cy := (float64(w.Height) - sz.H) / 2

		glowVal := layout.Glow.Value()
		if state.Media.Status == 4 {
			t := float64(time.Now().UnixMilli()) / 1000.0
			pulse := math.Sin(t*3.5)*0.3 + 0.7
			glowVal = glowVal * pulse
		}
		if glowVal > 0.02 {
			blurRadius := 20.0
			if sz.W > 300 {
				blurRadius = 28.0
				r.Glow(cx-4, cy-4, sz.W+8, sz.H+8, blurRadius, render.RGBA(100, 180, 255, uint8(30*glowVal)))
			} else {
				r.Glow(cx-2, cy-2, sz.W+4, sz.H+4, blurRadius, render.RGBA(100, 180, 255, uint8(40*glowVal)))
			}
		}

		// background pill
		r.FillRoundedRect(cx, cy, sz.W, sz.H, layout.Radius.Value(), render.RGBA(15, 15, 18, 220))
		if state.Media.Available && sz.W > 300 {
			if state.Media.Thumbnail != 0 {
				r.DrawBitmap(state.Media.Thumbnail, cx+16, cy+18, 64, 64)
			} else {
				r.FillRoundedRect(cx+16, cy+18, 64, 64, 8, render.RGBA(35, 35, 45, 255))
				r.FillRoundedRect(cx+36, cy+38, 24, 24, 4, render.RGBA(60, 60, 80, 200))
			}

			title := state.Media.Title
			r.DrawText(title, cx+92, cy+20, render.TextStyle{Size: 13, Bold: true, Color: render.RGB(255, 255, 255), Width: sz.W - 108})

			artist := state.Media.Artist
			r.DrawText(artist, cx+92, cy+42, render.TextStyle{Size: 11, Color: render.RGB(180, 180, 180), Width: sz.W - 108})

			album := state.Media.Album
			if album != "" && album != artist {
				r.DrawText(album, cx+92, cy+62, render.TextStyle{Size: 10, Color: render.RGB(130, 130, 145), Width: sz.W - 108})
			}

			pos := state.Media.Position
			if state.Media.Status == 4 { // 4 = Playing
				updated := state.Media.PositionUpdatedAt
				if updated.IsZero() {
					updated = time.Now()
				}
				pos += time.Since(updated)
				if state.Media.Duration > 0 && pos > state.Media.Duration {
					pos = state.Media.Duration
				}
				if pos < 0 {
					pos = 0
				}
			}

			// Media control bar background (Prev, Play/Pause, Next)
			ctrlBgX := cx + sz.W/2 - 60
			ctrlBgY := cy + 92
			r.FillRoundedRect(ctrlBgX, ctrlBgY, 120, 28, 14, render.RGBA(25, 25, 34, 230))

			// Prev button
			r.DrawText("«", ctrlBgX+14, ctrlBgY+4, render.TextStyle{Size: 13, Bold: true, Color: render.RGB(210, 210, 225)})

			// Play/Pause button
			playPauseText := "▶"
			if state.Media.Status == 4 {
				playPauseText = "‖"
			}
			r.DrawText(playPauseText, ctrlBgX+55, ctrlBgY+4, render.TextStyle{Size: 13, Bold: true, Color: render.RGB(255, 255, 255)})

			// Next button
			r.DrawText("»", ctrlBgX+96, ctrlBgY+4, render.TextStyle{Size: 13, Bold: true, Color: render.RGB(210, 210, 225)})

			if state.Media.Duration > 0 {
				frac := float64(pos) / float64(state.Media.Duration)
				if frac < 0 {
					frac = 0
				}
				if frac > 1 {
					frac = 1
				}
				r.FillRoundedRect(cx+16, cy+sz.H-16, sz.W-32, 4, 2, render.RGBA(255, 255, 255, 60))
				r.FillRoundedRect(cx+16, cy+sz.H-16, (sz.W-32)*frac, 4, 2, render.RGB(100, 180, 255))
			}
		} else if state.Media.Available {
			// Collapsed pill: Show Album Art Thumbnail + Title & Artist stacked inside pill
			textX := cx + 12.0
			textW := sz.W - 24.0

			thumbSize := 24.0
			thumbY := cy + (sz.H-thumbSize)/2.0
			if state.Media.Thumbnail != 0 {
				r.DrawBitmap(state.Media.Thumbnail, cx+8.0, thumbY, thumbSize, thumbSize)
				textX = cx + 38.0
				textW = sz.W - 46.0
			} else {
				r.FillRoundedRect(cx+8.0, thumbY, thumbSize, thumbSize, 5, render.RGBA(40, 40, 52, 255))
				r.FillRoundedRect(cx+15.0, thumbY+7.0, 10, 10, 2, render.RGBA(70, 70, 95, 200))
				textX = cx + 38.0
				textW = sz.W - 46.0
			}

			title := state.Media.Title
			artist := state.Media.Artist

			r.DrawText(title, textX, cy+4.0, render.TextStyle{
				Size:  10,
				Bold:  true,
				Color: render.RGB(255, 255, 255),
				Width: textW,
			})
			r.DrawText(artist, textX, cy+18.0, render.TextStyle{
				Size:  9,
				Color: render.RGB(170, 170, 185),
				Width: textW,
			})
		} else {
			textY := cy + (sz.H-16.0)/2.0
			r.DrawText(time.Now().Format("15:04"), cx, textY, render.TextStyle{
				Size:      12,
				Bold:      true,
				Color:     render.RGB(255, 255, 255),
				Alignment: 1, // Center
				Width:     sz.W,
			})
		}

		// copy frame buffer to window DIB
		if gdiR != nil {
			if w.Bits() != 0 {
				dst := unsafe.Slice((*byte)(unsafe.Pointer(w.Bits())), w.Width*w.Height*4)
				gdiR.CopyTo(dst)
			}
		} else {
			rastR := r.(*raster.Raster)
			if w.Bits() != 0 && len(rastR.Buf) == w.Width*w.Height*4 {
				dst := unsafe.Slice((*byte)(unsafe.Pointer(w.Bits())), len(rastR.Buf))
				copy(dst, rastR.Buf)
			}
		}
		var presentErr error
		if firstFrame {
			presentErr = w.Present(100, 50)
			firstFrame = false
		} else {
			presentErr = w.PresentAtCurrent()
		}
		if presentErr != nil {
			fmt.Println("Present error:", presentErr)
		}

		time.Sleep(16 * time.Millisecond)
	}
}
