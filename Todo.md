# Chroma — Todo

Canonical project name: **Chroma** (repo: github.com/ngbcoder/Chroma).
Module path: `github.com/ngbcoder/Chroma`. Go target: 1.27.

Status legend: `[ ]` not started · `[~]` in progress · `[x]` done · `[!]` blocked.

Keep this file updated as work progresses. Commit and push frequently (small commits).

---

## Milestone 0 — Spikes (decision gates)

### M0.0 Repo scaffold
- [x] Create `go.mod` with module path `github.com/ngbcoder/Chroma`, go 1.27
- [x] Create directory tree per Design.md §13
- [x] Add embedded `app.manifest` (DPI awareness v2, supported OS GUIDs, no elevation)
- [x] Add `.gitignore` for Go (binaries, build artifacts, logs)
- [x] Add `Todo.md`
- [x] Commit + push scaffold

### M0.1 Layered window spike
- [x] `internal/ui/window.go`: `NewWindow` — create HWND with `WS_POPUP | WS_EX_LAYERED | WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE`
- [x] `internal/ui/window.go`: `Present` — `UpdateLayeredWindow` with premul 32-bit BGRA DIB
- [x] `internal/ui/window.go`: `WM_MOUSEACTIVATE` handler returning `MA_NOACTIVATE`
- [x] `internal/ui/window.go`: `ScaleForDPI` helper (device-independent → device px)
- [x] `internal/ui/input.go`: 60 Hz `GetCursorPos` cursor poll within hot-zone
- [x] `internal/ui/input.go`: dwell timer (150 ms expand, 400 ms collapse) state machine
- [ ] `internal/ui/input.go`: click-through verification (alpha-0 pixels pass input)
- [ ] Manual test: window visible, topmost, no taskbar/Alt+Tab entry, click-through works
- [x] Commit + push (input handling)

### M0.2 Spring animation spike
- [x] `internal/ui/physics.go`: `Spring` struct (x, v, target, k=280, c=22)
- [x] `internal/ui/physics.go`: `Step(dt float64)` semi-implicit Euler with 1/240 s substep accumulator
- [x] `internal/ui/physics.go`: `Retarget`/`SetTarget(newTarget)` preserving velocity
- [x] `internal/ui/physics.go`: `AtRest() bool` threshold 0.05 (distance + velocity)
- [x] `internal/ui/physics.go`: snap-to-target when at rest, stop requesting frames
- [x] `internal/ui/layout.go`: state sizes (collapsed 190×36, expanded 420×140, thin line h=6, edge blade, volume flyout)
- [x] Unit tests: spring integration, retarget smoothness, rest detection
- [x] Commit + push

### M0.3 Render backend spike
- [ ] `internal/render/backend.go`: `Backend` interface (`Begin`, `Clear`, `FillRoundedRect`, `DrawImage`, `DrawText`, `Present`)
- [ ] `internal/render/gdiplus/*.go`: GDI+ flat API via lazy DLL, `PixelFormat32bppPARGB`
- [ ] `internal/render/raster/*.go`: pure-Go rasterizer into DIB memory (for tests/golden images)
- [ ] Golden-image test harness comparing raster output to reference PNGs
- [ ] Commit + push

### M0.4 SMTC bridge spike (decision gate)
- [ ] `internal/media/winrt/*.go`: `RoGetActivationFactory` for `GlobalSystemMediaTransportControlsSessionManager`
- [ ] `internal/media/smtc.go`: enumerate sessions, track current session, cycle sessions
- [ ] `internal/media/smtc.go`: metadata read (title, artist, album, status, timeline)
- [ ] `internal/media/smtc.go`: controls (play/pause, next, prev, seek via `TryChangePlaybackPositionAsync`)
- [ ] `internal/media/smtc.go`: async polling via `IAsyncInfo.Status` (no delegates) OR delegate callbacks
- [ ] `internal/media/smtc.go`: event subscriptions or 4 Hz/1 Hz polling fallback
- [ ] `internal/media/artwork.go`: `IRandomAccessStream` → `IStream` via `CreateStreamOverRandomAccessStream`, decode via GDI+
- [ ] Decision gate check: track change < 300 ms; no crash/leak under 30-min session churn; vtable maintenance cost
- [ ] If gate fails: implement Tier 2 native shim under `native/` with build tag `nativeshim`
- [ ] Commit + push

### M0.5 Frame pacing + fullscreen detection spike
- [ ] `internal/system/*.go`: `DwmFlush` / waitable timer frame driver
- [ ] `internal/system/*.go`: `SHQueryUserNotificationState` hide/minimal mode
- [ ] `internal/system/*.go`: re-assert topmost on `WM_DISPLAYCHANGE`/`WM_SETTINGCHANGE`
- [ ] Commit + push

---

## Milestone 1 — Core

### M1.1 App lifecycle + state store
- [ ] `internal/app/*.go`: root context, worker goroutine management, shutdown sequence
- [ ] `internal/app/*.go`: state store single-writer, immutable snapshots, channel event bus
- [ ] `internal/app/*.go`: health states (ok/degraded/unavailable) per subsystem
- [ ] `internal/app/*.go`: panic recovery + restart with rate limit in workers
- [ ] Commit + push

### M1.2 Config store
- [ ] `internal/config/store.go`: load `%APPDATA%\Chroma\config.json`, validate, fill defaults
- [ ] `internal/config/store.go`: preserve unknown keys on save
- [ ] `internal/config/store.go`: atomic write (temp + rename)
- [ ] `internal/config/store.go`: file watcher for live reload
- [ ] Unit tests: validation, defaults, unknown-key preservation, atomic write
- [ ] Commit + push

### M1.3 Window + layout integration
- [ ] `internal/ui/layout.go`: dock modes (top pill, top notch, thin line, left/right edge blade)
- [ ] `internal/ui/layout.go`: per-mode geometry and animated transitions via springs
- [ ] `internal/ui/window.go`: fixed 640×260 canvas, position at chosen edge/monitor
- [ ] Commit + push

### M1.4 Media UI (expanded state)
- [ ] Render collapsed pill (190×36) with clock/idle fallback when no session
- [ ] Render expanded widget (420×140): controls, progress, artwork
- [ ] Play/pause/next/prev buttons hit-testing + commands
- [ ] Progress extrapolation from last known position + timestamp
- [ ] Session cycling UI
- [ ] Ambient artwork glow from dominant color (cache by track identity)
- [ ] Commit + push

### M1.5 System tray
- [ ] Tray icon with menu: quit, settings, pause overlay, network feature toggles
- [ ] Start-with-Windows via `HKCU\...\Run`
- [ ] Commit + push

### M1.6 Entry point
- [ ] `cmd/chroma/main.go`: init, manifest, message pump, tray, shutdown
- [ ] Cold start to visible < 500 ms check
- [ ] Commit + push

---

## Milestone 2 — Audio

### M2.1 Volume
- [ ] `internal/audio/volume.go`: `IAudioEndpointVolume` on default render endpoint (scalar + mute)
- [ ] `internal/audio/volume.go`: mouse wheel over pill adjusts volume in steps
- [ ] `internal/audio/volume.go`: volume change notifications → flyout trigger
- [ ] `internal/audio/volume.go`: `IMMNotificationClient` default-device change rebind
- [ ] Commit + push

### M2.2 Volume flyout UI
- [ ] Transient volume flyout state + render
- [ ] Commit + push

### M2.3 Loopback capture
- [ ] `internal/audio/loopback.go`: `AUDCLNT_STREAMFLAGS_LOOPBACK` on default render endpoint
- [ ] `internal/audio/loopback.go`: start only when expanded/visualizer visible and audio playing
- [ ] `internal/audio/loopback.go`: 1024-sample frames, Hann window
- [ ] `internal/audio/loopback.go`: device/format change restart
- [ ] Commit + push

### M2.4 FFT + visualizer
- [ ] `internal/audio/fft.go`: FFT, 6 log bands 60 Hz–12 kHz
- [ ] `internal/audio/fft.go`: fast attack / slow release smoothing
- [ ] `internal/audio/fft.go`: feed spring-smoothed bar heights
- [ ] Visualizer render in expanded state
- [ ] Unit tests: FFT binning
- [ ] Commit + push

---

## Milestone 3 — Extras

### M3.1 Synced lyrics
- [ ] `internal/lyrics/lrclib.go`: query LRCLIB by title/artist/album/duration (opt-in)
- [ ] `internal/lyrics/lrclib.go`: parse LRC timestamps
- [ ] `internal/lyrics/lrclib.go`: disk cache keyed by track identity
- [ ] `internal/lyrics/lrclib.go`: render active line using extrapolated position
- [ ] Short timeouts, never block UI, fail silently
- [ ] Unit tests: LRC parsing
- [ ] Commit + push

### M3.2 Bluetooth battery
- [ ] `internal/hardware/bluetooth.go`: read battery % via setupapi/cfgmgr32 + BT battery property
- [ ] `internal/hardware/bluetooth.go`: validate property key across Win10/11 + device types
- [ ] `internal/hardware/bluetooth.go`: 60 s poll, only when BT audio is active output; refresh on connect
- [ ] Graceful hide when no value
- [ ] Commit + push

### M3.3 Quick launchers
- [ ] 3 slots via `ShellExecuteW`
- [ ] Icons via `SHGetFileInfoW`, cached on disk
- [ ] Commit + push

### M3.4 Clock
- [ ] Live clock in idle state + optional clock while playing
- [ ] Commit + push

### M3.5 Companion pet (optional)
- [ ] Animated sprite loop, disabled by default
- [ ] Commit + push

### M3.6 Dock modes + multi-monitor
- [ ] Animated dock mode switching via springs
- [ ] Monitor selection by device ID, fallback to primary
- [ ] Commit + push

---

## Milestone 4 — Polish

### M4.1 Quiet Feed Mode
- [ ] Suppress auto-expand on rapid title changes (N changes/window)
- [ ] Configurable thresholds, process allowlist, manual override, easy disable
- [ ] Unit tests: heuristic
- [ ] Commit + push

### M4.2 Fullscreen handling
- [ ] Hide/minimal mode via `SHQueryUserNotificationState`
- [ ] Commit + push

### M4.3 Explorer restart / sleep / RDP resilience
- [ ] Re-create window + rebind audio/SMTC on Explorer restart, display change, sleep/resume, RDP
- [ ] Commit + push

### M4.4 Performance pass
- [ ] Debug overlay: frame time, goroutine count, heap size
- [ ] Verify budgets: <15 MB idle RAM, ~0% idle CPU, <3% playing+visualizer, <4 ms frame, <300 ms track change, <500 ms cold start
- [ ] Commit + push

---

## Milestone 5 — Release

### M5.1 Music Radar (opt-in, optional)
- [ ] Capture ~4 s rolling loopback window, encode WAV in memory
- [ ] Submit to recognition service (backend TBD — open question)
- [ ] Off by default, explicit user action each time
- [ ] Commit + push

### M5.2 Packaging
- [ ] Single static exe with embedded manifest
- [ ] GitHub Releases portable zip
- [ ] Optional MSI/MSIX + winget manifest
- [ ] Reproducible builds (`-trimpath`, pinned Go version)
- [ ] Code signing plan
- [ ] Commit + push

### M5.3 Docs
- [ ] README usage + privacy notes
- [ ] Contributing + license decision (open question)
- [ ] Commit + push

---

## Open questions (from Design.md §18)
- [ ] Music Radar recognition backend
- [ ] License choice (MIT/GPL/MPL)
- [ ] Settings UI: inside overlay vs separate window
