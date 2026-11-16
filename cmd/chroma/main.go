package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	gioapp "gioui.org/app"
	giosystem "gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	chromaapp "github.com/ngbcoder/Chroma/internal/app"
	"github.com/ngbcoder/Chroma/internal/config"
	"github.com/ngbcoder/Chroma/internal/media"
	"github.com/ngbcoder/Chroma/internal/render"
	"github.com/ngbcoder/Chroma/internal/render/gdiplus"
	"github.com/ngbcoder/Chroma/internal/render/raster"
	"github.com/ngbcoder/Chroma/internal/system"
	"github.com/ngbcoder/Chroma/internal/ui"
)

const (
	trayCommandQuit uint32 = iota + 1
	trayCommandSettings
	trayCommandPause
	trayCommandLyrics
	trayCommandMusicRadar
	trayCommandStartup
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

func configuredWindowPosition(cfg *config.Config, width, height int) (int, int) {
	sw, sh := getScreenSize()
	switch cfg.DockMode {
	case "edge_left":
		return 0, (sh - height) / 2
	case "edge_right":
		return sw - width, (sh - height) / 2
	}
	if cfg.WindowX != 0 || cfg.WindowY != 0 {
		return cfg.WindowX, cfg.WindowY
	}
	switch cfg.PresetLocation {
	case "top_left":
		return 20, 10
	case "top_right":
		return sw - width - 20, 10
	case "bottom_center":
		return (sw - width) / 2, sh - height - 50
	case "bottom_left":
		return 20, sh - height - 50
	case "bottom_right":
		return sw - width - 20, sh - height - 50
	default:
		return (sw - width) / 2, 10
	}
}

func launchSettings() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	cmd := exec.Command(exe, "--settings")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start settings process: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release settings process: %w", err)
	}
	return nil
}

func runSettingsWindow() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Println("config load:", err)
		return
	}

	var darkMode widget.Bool
	darkMode.Value = cfg.Theme.DarkMode
	var quietMode widget.Bool
	quietMode.Value = cfg.Behavior.QuietFeedMode

	dockModes := []string{"top_island", "top_notch", "thin_line", "edge_left", "edge_right"}
	dockIndex := 0
	for i, mode := range dockModes {
		if mode == cfg.DockMode {
			dockIndex = i
			break
		}
	}

	var dockButton widget.Clickable
	var saveButton widget.Clickable
	var closeButton widget.Clickable

	go func() {
		var w gioapp.Window
		var ops op.Ops
		th := material.NewTheme()
		w.Option(gioapp.Title("Chroma Settings"), gioapp.Size(unit.Dp(420), unit.Dp(240)), gioapp.Decorated(false))
		for {
			e := w.Event()
			switch e := e.(type) {
			case gioapp.DestroyEvent:
				return
			case gioapp.FrameEvent:
				gtx := gioapp.NewContext(&ops, e)
				if dockButton.Clicked(gtx) {
					dockIndex = (dockIndex + 1) % len(dockModes)
					cfg.DockMode = dockModes[dockIndex]
				}
				if saveButton.Clicked(gtx) {
					cfg.Theme.DarkMode = darkMode.Value
					cfg.Behavior.QuietFeedMode = quietMode.Value
					if err := cfg.Save(); err != nil {
						fmt.Println("save config:", err)
					}
					w.Perform(giosystem.ActionClose)
					return
				}
				if closeButton.Clicked(gtx) {
					w.Perform(giosystem.ActionClose)
					return
				}

				layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceSides}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.H6(th, "Chroma Settings").Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return material.CheckBox(th, &darkMode, "Dark mode").Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return material.CheckBox(th, &quietMode, "Quiet feed mode").Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return material.Body1(th, "Dock mode: "+dockModes[dockIndex]).Layout(gtx)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return material.Button(th, &dockButton, "Cycle").Layout(gtx)
										}),
									)
								}),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Button(th, &closeButton, "Close").Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Button(th, &saveButton, "Save").Layout(gtx)
							}),
						)
					}),
				)
				e.Frame(gtx.Ops)
			}
		}
	}()
	gioapp.Main()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--settings" {
		runSettingsWindow()
		return
	}

	runtime.LockOSThread()
	fmt.Println("Starting Chroma...")

	startedAt := time.Now()
	cfg, err := config.Load("")
	if err != nil {
		fmt.Println("config load:", err)
		return
	}

	w, err := ui.NewWindow(640, 260)
	if err != nil {
		fmt.Println("NewWindow:", err)
		return
	}
	fmt.Println("Window created successfully, HWND:", w.Hwnd)
	defer w.Destroy()

	w.Locked = cfg.Behavior.Locked
	if err := system.AddTrayIcon(w.Hwnd, "Chroma"); err != nil {
		fmt.Println("tray icon:", err)
	} else {
		defer func() {
			if err := system.RemoveTrayIcon(w.Hwnd); err != nil {
				fmt.Println("remove tray icon:", err)
			}
		}()
	}

	x, y := configuredWindowPosition(cfg, w.Width, w.Height)
	w.Move(x, y)
	defer func() {
		l, t, _, _ := w.Rect()
		cfg.WindowX = l
		cfg.WindowY = t
		if err := cfg.Save(); err != nil {
			fmt.Println("save config on shutdown:", err)
		}
	}()

	store := chromaapp.NewStore()
	defer store.Close()

	layout := ui.NewLayout()
	if !layout.SetDockMode(cfg.DockMode) {
		fmt.Println("invalid dock mode:", cfg.DockMode)
		return
	}
	dwell := ui.NewDwellController()
	dwell.ExpandDelay = time.Duration(cfg.Behavior.DwellDelayMs) * time.Millisecond
	dwell.CollapseDelay = time.Duration(cfg.Behavior.LeaveDelayMs) * time.Millisecond
	if cfg.Theme.Snappiness > 0 {
		k := cfg.Theme.Snappiness * 420.0
		layout.Width.SetParams(k, 22)
		layout.Height.SetParams(k, 22)
	}
	configChanged := make(chan struct{}, 1)
	cfgWatcher, err := cfg.Watch(func() {
		select {
		case configChanged <- struct{}{}:
		default:
		}
	})
	if err != nil {
		fmt.Println("config watcher:", err)
	} else {
		defer func() {
			if err := cfgWatcher.Close(); err != nil {
				fmt.Println("close config watcher:", err)
			}
		}()
		go func() {
			for err := range cfgWatcher.Errors() {
				fmt.Println("config watcher:", err)
			}
		}()
	}

	dwellEvents := make(chan bool, 4)
	dwell.OnExpand = func() {
		select {
		case dwellEvents <- true:
		default:
		}
	}
	dwell.OnCollapse = func() {
		select {
		case dwellEvents <- false:
		default:
		}
	}

	paused := false
	w.OnTrayEvent = func(lParam uintptr) {
		if uint32(lParam) == system.TrayLeftDoubleClick {
			if err := launchSettings(); err != nil {
				fmt.Println("launch settings:", err)
			}
			return
		}
		if uint32(lParam) != system.TrayRightButtonUp && uint32(lParam) != system.TrayContextMenu {
			return
		}

		startupEnabled, err := system.StartupEnabled()
		if err != nil {
			fmt.Println("read startup setting:", err)
		}
		selected, err := system.ShowTrayMenu(w.Hwnd, []system.TrayMenuItem{
			{ID: trayCommandSettings, Label: "Settings"},
			{ID: trayCommandPause, Label: "Pause overlay", Checked: paused},
			{ID: trayCommandLyrics, Label: "Enable lyrics", Checked: cfg.Network.Lyrics},
			{ID: trayCommandMusicRadar, Label: "Enable Music Radar", Checked: cfg.Network.MusicRadar},
			{ID: trayCommandStartup, Label: "Start with Windows", Checked: startupEnabled},
			{Separator: true},
			{ID: trayCommandQuit, Label: "Quit Chroma"},
		})
		if err != nil {
			fmt.Println("show tray menu:", err)
			return
		}
		if selected == 0 {
			return
		}
		switch selected {
		case trayCommandQuit:
			w.Quit()
		case trayCommandSettings:
			if err := launchSettings(); err != nil {
				fmt.Println("launch settings:", err)
			}
		case trayCommandPause:
			paused = !paused
			if paused {
				dwell.SetEnabled(false)
				w.Hide()
				layout.GoTo(ui.StateCollapsed)
				layout.Glow.SetTarget(0)
			} else {
				dwell.SetEnabled(true)
				w.Show()
			}
		case trayCommandLyrics, trayCommandMusicRadar:
			updated, err := config.Load(cfg.Path())
			if err != nil {
				fmt.Println("load config for network toggle:", err)
				return
			}
			if selected == trayCommandLyrics {
				updated.Network.Lyrics = !updated.Network.Lyrics
			} else {
				updated.Network.MusicRadar = !updated.Network.MusicRadar
			}
			if err := updated.Save(); err != nil {
				fmt.Println("save network setting:", err)
				return
			}
			cfg = updated
		case trayCommandStartup:
			if err := system.SetStartupEnabled(!startupEnabled); err != nil {
				fmt.Println("update startup setting:", err)
			}
		}
	}

	w.OnClick = func(x, y int) {
		sz := layout.SizeNow()
		if sz.W < 300 {
			return
		}
		cx, cy := layout.Position(w.Width, w.Height)
		if float64(y) >= cy+72 && float64(y) <= cy+92 &&
			float64(x) >= cx+sz.W-96 && store.State().Media.SessionCount > 1 {
			if _, _, err := media.CycleSession(); err != nil {
				fmt.Println("cycle media session:", err)
			}
			return
		}
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
		fmt.Println("GDI+ renderer unavailable, using raster:", err)
		r = raster.New()
	}

	defer media.ClearThumbnailCache()
	lc := chromaapp.NewLifecycle()
	lc.Go(func(ctx context.Context) {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				info, err := media.FetchCurrent()
				if err != nil {
					store.Send(chromaapp.MediaUpdate(chromaapp.MediaState{Available: false}))
					store.Send(chromaapp.Health("smtc", "degraded"))
					continue
				}
				store.Send(chromaapp.MediaUpdate(chromaapp.MediaState{
					Available:         true,
					SourceApp:         info.SourceApp,
					TrackIdentity:     info.TrackIdentity,
					Title:             info.Title,
					Artist:            info.Artist,
					Album:             info.Album,
					Status:            info.Status,
					Position:          info.Position,
					Duration:          info.Duration,
					PositionUpdatedAt: info.PositionUpdatedAt,
					Thumbnail:         info.Thumbnail,
					SessionIndex:      info.SessionIndex,
					SessionCount:      info.SessionCount,
				}))
				store.Send(chromaapp.Health("smtc", "ok"))
			case <-ctx.Done():
				return
			}
		}
	})
	defer lc.Shutdown()

	// Main loop: pump messages + drive frames
	quit := false
	firstFrame := true
	lastTrackIdentity := ""
	lastArtworkBitmap := uintptr(0)
	accentColor := render.RGB(100, 180, 255)
	hasAccentColor := false
	for !quit {
		if pumpMessages() {
			quit = true
			break
		}
		select {
		case <-configChanged:
			newCfg, err := config.Load(cfg.Path())
			if err != nil {
				fmt.Println("reload config:", err)
			} else {
				oldDockMode := cfg.DockMode
				cfg = newCfg
				w.Locked = cfg.Behavior.Locked
				if !layout.SetDockMode(cfg.DockMode) {
					fmt.Println("invalid dock mode:", cfg.DockMode)
				}
				dwell.ExpandDelay = time.Duration(cfg.Behavior.DwellDelayMs) * time.Millisecond
				dwell.CollapseDelay = time.Duration(cfg.Behavior.LeaveDelayMs) * time.Millisecond
				if cfg.Theme.Snappiness > 0 {
					k := cfg.Theme.Snappiness * 420.0
					layout.Width.SetParams(k, 22)
					layout.Height.SetParams(k, 22)
				}
				if oldDockMode != cfg.DockMode {
					x, y := configuredWindowPosition(cfg, w.Width, w.Height)
					w.Move(x, y)
				}
			}
		default:
		}
		for {
			select {
			case expanded := <-dwellEvents:
				if paused {
					continue
				}
				if expanded {
					layout.GoTo(ui.StateExpanded)
					layout.Glow.SetTarget(1)
				} else {
					layout.GoTo(ui.StateCollapsed)
					layout.Glow.SetTarget(0)
				}
			default:
				goto dwellEventsDrained
			}
		}
	dwellEventsDrained:

		state := store.State()
		if state.Media.TrackIdentity != lastTrackIdentity ||
			(state.Media.Thumbnail != 0 && state.Media.Thumbnail != lastArtworkBitmap) {
			lastTrackIdentity = state.Media.TrackIdentity
			lastArtworkBitmap = state.Media.Thumbnail
			hasAccentColor = false
			if state.Media.Thumbnail != 0 {
				color, err := gdiplus.DominantColor(state.Media.Thumbnail)
				if err != nil {
					fmt.Println("read artwork color:", err)
				} else {
					accentColor = color
					hasAccentColor = true
				}
			}
		}

		// advance layout
		layout.Step(1.0 / 60.0)

		// render
		r.Begin(w.Width, w.Height)
		r.Clear()
		sz := layout.SizeNow()
		cx, cy := layout.Position(w.Width, w.Height)

		glowVal := layout.Glow.Value()
		if state.Media.Status == 4 {
			t := float64(time.Now().UnixMilli()) / 1000.0
			pulse := math.Sin(t*3.5)*0.3 + 0.7
			glowVal = glowVal * pulse
		}
		if glowVal > 0.02 {
			blurRadius := 20.0
			glowColor := render.RGBA(100, 180, 255, uint8(30*glowVal))
			if hasAccentColor {
				glowColor = render.RGBA(accentColor.R, accentColor.G, accentColor.B, uint8(30*glowVal))
			}
			if sz.W > 300 {
				blurRadius = 28.0
				r.Glow(cx-4, cy-4, sz.W+8, sz.H+8, blurRadius, glowColor)
			} else {
				glowColor.A = uint8(40 * glowVal)
				r.Glow(cx-2, cy-2, sz.W+4, sz.H+4, blurRadius, glowColor)
			}
		}

		// background pill
		r.FillRoundedRect(cx, cy, sz.W, sz.H, layout.Radius.Value(), render.RGBA(15, 15, 18, 220))
		if state.Media.Available && sz.W > 300 && sz.H > 20 {
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
			if state.Media.SessionCount > 1 {
				sessionLabel := fmt.Sprintf("%d / %d  ›", state.Media.SessionIndex+1, state.Media.SessionCount)
				r.FillRoundedRect(cx+sz.W-96, cy+72, 84, 18, 9, render.RGBA(28, 28, 36, 230))
				r.DrawText(sessionLabel, cx+sz.W-92, cy+76, render.TextStyle{
					Size: 9, Color: render.RGB(190, 200, 220), Alignment: 1, Width: 76,
				})
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
		} else if state.Media.Available && sz.H > 20 {
			// Collapsed pill: Show Album Art Thumbnail + Title & Artist stacked inside pill
			textX := cx + 12.0
			textW := sz.W - 24.0

			thumbSize := 24.0
			thumbY := cy + (sz.H-thumbSize)/2.0
			if state.Media.Thumbnail != 0 {
				r.DrawBitmap(state.Media.Thumbnail, cx+8.0, thumbY, thumbSize, thumbSize)
				textX = cx + 38.0
				textW = sz.W - 46.0
			} else if sz.H > 20 {
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
			presentErr = w.PresentAtCurrent()
			firstFrame = false
			if presentErr == nil {
				fmt.Printf("First frame presented in %s\n", time.Since(startedAt).Round(time.Millisecond))
			}
		} else {
			presentErr = w.PresentAtCurrent()
		}
		if presentErr != nil {
			fmt.Println("Present error:", presentErr)
		}

		time.Sleep(16 * time.Millisecond)
	}
}
