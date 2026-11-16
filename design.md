# Dynamic Edge (Open-Source Edition) — Design Document

Revision 2 · Status: design stage · Platform: Windows 10 / 11 · Language: Go

This revision keeps the original vision and updates the engineering approach: Go-first with a pragmatic native-interop policy instead of an absolute "zero CGo" rule.

## 1. Overview

Dynamic Edge is a lightweight, zero-telemetry desktop utility that brings a dynamic-island and edge-dock experience to Windows. It shows media controls, a live audio visualizer, volume feedback, Bluetooth battery status and synced lyrics in a small animated overlay docked to a screen edge.

### 1.1 Goals

- Memory footprint under 15 MB resident while idle, near-zero CPU when nothing is animating.
- Fluid animation: spring-driven transitions that track the monitor refresh rate (60 to 144 Hz).
- Full local control: no telemetry, no accounts, config stored locally, network calls only for features the user enables.
- Multi-session media control through the Windows System Media Transport Controls (SMTC).
- Never steals focus, never appears in the taskbar or Alt+Tab.

### 1.2 Non-goals

- Cross-platform support. The design leans on Win32, COM and WinRT.
- A general widget platform or plugin system in v1.
- Acting as a media player. Dynamic Edge controls existing players only.

## 2. Engineering Principles

1. **Go first.** Everything is written in Go using `golang.org/x/sys/windows`, `syscall` and lazy DLL loading. This keeps cross-compilation trivial and builds fast.
2. **CGo is an escape hatch, not a goal.** The default build is CGo-free. If a subsystem (most likely WinRT/SMTC) proves too brittle in pure Go, a small isolated native shim is allowed behind a build tag, with a pure-Go path kept as the default whenever feasible.
3. **Idle means idle.** No render loop, polling timer or audio capture runs unless something on screen needs it.
4. **Privacy by default.** Anything that leaves the machine (lyrics lookup, song recognition) is opt-in, documented and individually toggleable.
5. **Fail soft.** A broken subsystem (no Bluetooth, no SMTC session, lyrics server down) degrades its own widget, never the whole app.

## 3. Architecture

```text
┌────────────────────────────────────────────────────────────┐
│              UI thread: Win32 message pump                 │
│   window, input, hooks, UpdateLayeredWindow                │
└───────┬──────────────────┬───────────────────┬─────────────┘
        │                  │                   │
        ▼                  ▼                   ▼
┌──────────────┐   ┌──────────────┐   ┌─────────────────┐
│ Render       │   │ State store  │   │ Animation       │
│ backend      │◄──┤ (single      ├──►│ engine (springs)│
│ (pluggable)  │   │  writer)     │   │                 │
└──────────────┘   └──────▲───────┘   └─────────────────┘
                          │ events (channels)
   ┌──────────┬───────────┼───────────┬──────────┬─────────┐
   │          │           │           │          │         │
┌──┴───┐ ┌────┴────┐ ┌────┴────┐ ┌────┴───┐ ┌────┴───┐ ┌───┴────┐
│ SMTC │ │ Audio   │ │ Volume  │ │ BT     │ │ Lyrics │ │ Config │
│      │ │ loopback│ │ endpoint│ │ battery│ │ client │ │ store  │
└──────┘ └─────────┘ └─────────┘ └────────┘ └────────┘ └────────┘
```

### 3.1 Dependencies

| Concern | Choice | Notes |
| --- | --- | --- |
| Win32 calls | `golang.org/x/sys/windows` | Lazy DLL procs for anything not wrapped |
| COM | `go-ole/go-ole` | Pure Go, syscall based |
| WASAPI | `moutend/go-wca` | Built on go-ole |
| FFT | `mjibson/go-dsp/fft` | Pure Go; can be replaced by a hand-rolled radix-2 FFT to drop the dependency |
| WinRT | Hand-written vtable bindings, or generated bindings such as `saltosystems/winrt-go` | See section 4 |
| Rendering | GDI+ flat API, or a pure-Go rasterizer | See section 5 |

## 4. Native Interop Policy

### 4.1 Tiers

- **Tier 0, pure Go (default):** syscalls, lazy DLLs, go-ole COM. Used for window management, WASAPI, GDI+, PnP and shell calls.
- **Tier 1, pure Go with generated or hand-written WinRT projections:** used for SMTC. Only the handful of interfaces actually needed are projected.
- **Tier 2, optional native shim (build tag `nativeshim`):** a minimal C or C++/WinRT helper compiled only if Tier 1 is unreliable. Its surface is a flat C API: get sessions, get metadata, send commands, get thumbnail bytes, subscribe to change callbacks.

### 4.2 WinRT specifics

- Activation goes through `RoGetActivationFactory` in `combase.dll` for `Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager`.
- Async operations (`RequestAsync`, `TryGetMediaPropertiesAsync`, `TryChangePlaybackPositionAsync`) need either a completion-handler delegate or polling.
  - Preferred: poll `IAsyncInfo.Status` on a short timer from a worker goroutine. It avoids implementing COM delegates in Go.
  - Alternative: implement the completion handler with `syscall.NewCallback`. Note the process-wide callback limit and the need to keep stdcall conventions exact.
- Event subscriptions (`CurrentSessionChanged`, `MediaPropertiesChanged`, `PlaybackInfoChanged`, `TimelinePropertiesChanged`) also require delegates. If delegates are avoided, fall back to a low-rate poll (for example 4 Hz while visible, 1 Hz while collapsed) of the current session.
- Thread apartment: initialize COM as MTA on dedicated OS-locked goroutines (`runtime.LockOSThread`) for WinRT work, separate from the UI thread.
- Artwork: use `CreateStreamOverRandomAccessStream` (`shcore.dll`) to turn the thumbnail's `IRandomAccessStream` into an `IStream`, then decode it with `GdipCreateBitmapFromStream` or a pure-Go image decoder. This avoids the WinRT `DataReader` path.

### 4.3 Decision gate

Build the SMTC bridge as the first spike. If after the spike any of the following is true, move SMTC to Tier 2:

- Delegate-free polling cannot meet responsiveness targets (track change reflected in under 300 ms).
- Callbacks crash or leak under session churn (opening and closing players repeatedly for 30 minutes).
- Maintaining hand-written vtables costs more than the shim would.

## 5. Window Shell and Rendering

### 5.1 Window

- Extended styles: `WS_EX_LAYERED | WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE`, style `WS_POPUP`.
- Per-monitor DPI awareness v2 declared in the application manifest. All geometry is stored in device-independent units and scaled at render time.
- A fixed canvas (default 640 × 260 px at 100% scale) positioned at the top center or the chosen edge of the target monitor. The window never resizes during animation; only what is drawn inside it changes.
- Per-pixel alpha via `UpdateLayeredWindow` with a premultiplied 32-bit BGRA DIB section. Pixels with alpha 0 are click-through.

### 5.2 Input handling

Because fully transparent pixels pass input through, the window receives no mouse messages outside the visible shape. Therefore:

- Hover detection for the dwell timer uses a low-level mouse hook (`WH_MOUSE_LL`) or a 60 Hz cursor poll (`GetCursorPos`) that runs only while the cursor is within a generous hot-zone around the docked shape.
- Clicks, wheel events and drags on the visible shape arrive normally as `WM_LBUTTONDOWN`, `WM_MOUSEWHEEL` and so on.
- A 150 ms dwell debounce prevents accidental expansion. Collapse uses a longer leave delay (default 400 ms) so brief mouse exits do not flicker.

### 5.3 Render backend interface

Rendering is hidden behind a small interface so backends can be swapped without touching UI logic:

- `Begin(w, h)`, `Clear()`, `FillRoundedRect`, `DrawImage`, `DrawText`, `Blur/Glow` (optional), `Present(hwnd)`.

Backends:

1. **GDI+ (default for v1):** flat API through lazy DLL calls, `PixelFormat32bppPARGB`, anti-aliased paths. Simple and dependency-free.
2. **Pure-Go rasterizer:** draws into the DIB memory directly. More control over glow and blur, no GDI+ quirks, and fully portable testing.
3. **Direct2D / DirectComposition (future):** GPU path if CPU rendering misses frame budget on high-refresh displays.

### 5.4 Frame pacing

- Animation frames are driven by a waitable timer or `DwmFlush`, aligned to the monitor's refresh rate.
- The render loop starts when a spring is moving, a visualizer is active or lyrics are scrolling, and stops when everything is at rest.
- Dirty-region tracking is a later optimization. The canvas is small enough that full-canvas redraws are acceptable at first; measure before optimizing.

### 5.5 Fullscreen and focus behavior

- Query `SHQueryUserNotificationState` and hide the overlay (or switch to a minimal mode) when an exclusive-fullscreen app or presentation is active.
- Re-assert topmost position occasionally and on `WM_DISPLAYCHANGE` / `WM_SETTINGCHANGE`.
- The window must never take activation. All interactions use `WM_MOUSEACTIVATE` returning `MA_NOACTIVATE`.

## 6. Animation Engine

### 6.1 Spring model

Each animated property (width, height, corner radius, opacity, content offset) is a 1D damped harmonic oscillator with unit mass:

```latex
a = -k\,(x - x_{\text{target}}) - c\,v
```

Defaults: stiffness k = 280, damping c = 22. The damping ratio is about 0.66, which gives a small, intentional overshoot. Critical damping would be c ≈ 33.5; this is exposed as a "snappiness" setting.

### 6.2 Integration

- Semi-implicit Euler with a fixed internal substep (1/240 s) and an accumulator, so behavior is identical at 60, 120 and 144 Hz.
- A property is considered at rest when its distance to target and its velocity are both below 0.05, at which point it snaps to target and stops requesting frames.
- Retargeting mid-flight preserves velocity, which keeps interrupted transitions smooth.

### 6.3 States

| State | Size (w × h) | Notes |
| --- | --- | --- |
| Collapsed pill | 190 × 36 | Default resting state |
| Expanded widget | 420 × 140 | Media controls, progress, artwork |
| Thin line | height 6 | Minimal presence, double-click toggle |
| Edge blade (left/right) | vertical variant | Reveals on edge hover |
| Volume flyout | transient | Shown on wheel or hardware volume change |

All sizes are defaults and configurable per theme.

## 7. Media Subsystem (SMTC)

- **Session list:** enumerate sessions from the manager; track current session and let the user cycle between them (Spotify, browsers, VLC, and so on). Remember the last manually selected session until it disappears.
- **Metadata:** title, artist, album, thumbnail, playback status, and timeline (position, start, end).
- **Controls:** play/pause, next, previous, and seek through `TryChangePlaybackPositionAsync`.
- **Progress:** between timeline events, extrapolate position locally from the last known position and timestamp rather than polling.
- **Ambient artwork:** decode the thumbnail, downscale, and derive a dominant color for the background glow. Cache by track identity to avoid redecoding.
- **Fallbacks:** if no session exists, the pill shows the clock or idle state.

## 8. Audio Subsystem

### 8.1 Volume

- `IAudioEndpointVolume` on the default render endpoint for scalar volume and mute.
- Mouse wheel over the pill adjusts volume in configurable steps. Register for volume change notifications so hardware keys also trigger the flyout.
- Handle default-device changes (`IMMNotificationClient`) by rebinding the endpoint.

### 8.2 Visualizer

- Loopback capture (`AUDCLNT_STREAMFLAGS_LOOPBACK`) on the default render endpoint, started only while the expanded or visualizer-enabled state is visible and audio is actually playing.
- 1024-sample frames, Hann window, FFT, magnitudes binned into 6 logarithmic bands from 60 Hz to 12 kHz.
- Smoothing: fast attack, slower release per band; the result feeds spring-smoothed bar heights.
- Device changes and format changes (sample rate, channel count) must restart the capture cleanly.

## 9. Advanced Features

### 9.1 Synced lyrics

- On track change (and only if enabled), query LRCLIB by title, artist, album and duration.
- Parse LRC timestamps, cache results on disk keyed by track identity, and render the active line using the extrapolated playback position.
- Network requests use short timeouts and never block the UI. Failure shows nothing rather than an error.

### 9.2 Bluetooth battery

- Read battery percentage from the PnP device tree through `setupapi.dll` and `cfgmgr32.dll`, using the Bluetooth battery device property.
- Treat this as an undocumented-ish property: verify the property key and behavior across Windows 10 and 11 and across device types, and handle devices that do not report a value.
- Poll slowly (for example every 60 s) and only when a Bluetooth audio device is the active output; refresh on connect events.

### 9.3 Music Radar (opt-in)

- On user action, capture a short rolling window (about 4 seconds) of loopback audio, encode as WAV in memory, and submit to a recognition service.
- This is the only feature that sends audio off the machine. It must be off by default, clearly labeled, and require an explicit user action each time.
- Open question: which recognition backend is acceptable. Options include a fingerprint service with a free or self-hostable tier, or a local fingerprint (Chromaprint-style) against a user-supplied database. Pick one before building this feature.

### 9.4 Quiet Feed Mode

- Suppress auto-expansion when a browser session changes title rapidly (for example more than N changes in a short window), which is typical of short-form video feeds.
- Heuristic and configurable: thresholds, process allowlist, and a manual override. Make it easy to disable if it misfires.

## 10. Customization and Utilities

- **Dock modes:** top pill, top notch, thin line, left/right edge blade. Mode changes are animated through the same spring system.
- **Multi-monitor:** choose the target monitor; remember it by device ID and fall back to primary if it is unplugged.
- **Quick launchers:** three slots using `ShellExecuteW`; icons through `SHGetFileInfoW`, cached on disk.
- **Clock:** live clock in idle state and an optional clock while playing.
- **Companion pet:** animated sprite loop, disabled by default and fully optional.
- **System tray icon:** quit, settings, pause overlay, and a quick toggle for each network feature.

## 11. Concurrency Model

- **UI goroutine** (locked to an OS thread): owns the window, message pump, input hooks and the render backend. Nothing else touches Win32 window state.
- **Subsystem goroutines:** one each for SMTC, audio capture, volume, Bluetooth and lyrics. They communicate through typed events on channels.
- **State store:** a single writer applies events and publishes immutable snapshots that the UI reads each frame. This avoids locks on the hot path.
- **COM apartments:** each COM-using goroutine locks its OS thread and calls `CoInitializeEx` once; no COM objects cross threads except through marshaled proxies or by recreating them on the owning thread.
- **Shutdown:** a root context cancels all workers; each releases its COM objects and stops capture before the process exits.

## 12. Configuration

Stored at `%APPDATA%\DynamicEdge\config.json`, validated on load with defaults filled in for missing keys. Unknown keys are preserved on save. Writes are atomic (write temp file, then rename).

```json
{
  "version": 2,
  "dock_mode": "top_island",
  "monitor": "primary",
  "theme": {
    "dark_mode": true,
    "accent_color": "#0F0F12",
    "glass_opacity": 0.92,
    "snappiness": 0.66
  },
  "behavior": {
    "quiet_feed_mode": true,
    "double_click_thin_line": true,
    "dwell_delay_ms": 150,
    "leave_delay_ms": 400,
    "hide_in_fullscreen": true
  },
  "network": {
    "lyrics": false,
    "music_radar": false
  },
  "quick_launchers": [
    { "name": "Terminal", "path": "wt.exe" },
    { "name": "Browser", "path": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe" },
    { "name": "VS Code", "path": "code" }
  ]
}
```

The config file is watched for changes so edits apply live without a restart.

## 13. Directory Structure

```text
dynamic-edge/
├── cmd/dynamic-edge/main.go        # entry point, message pump, tray
├── internal/
│   ├── app/                        # lifecycle, state store, event bus
│   ├── ui/
│   │   ├── window.go               # layered HWND, UpdateLayeredWindow
│   │   ├── input.go                # hooks, dwell logic, hit testing
│   │   ├── layout.go               # states and per-mode geometry
│   │   └── physics.go              # spring engine
│   ├── render/
│   │   ├── backend.go              # renderer interface
│   │   ├── gdiplus/                # default backend
│   │   └── raster/                 # pure-Go backend
│   ├── audio/
│   │   ├── volume.go               # endpoint volume, notifications
│   │   ├── loopback.go             # capture
│   │   └── fft.go                  # 6-band analyzer
│   ├── media/
│   │   ├── smtc.go                 # session manager, controls
│   │   ├── winrt/                  # minimal WinRT bindings
│   │   ├── artwork.go              # decode, dominant color, cache
│   │   └── radar.go                # opt-in audio capture and lookup
│   ├── hardware/bluetooth.go
│   ├── lyrics/lrclib.go
│   ├── system/                     # fullscreen detection, display changes
│   └── config/store.go
├── native/                         # optional shim, build tag nativeshim
├── assets/
├── docs/
└── go.mod
```

## 14. Error Handling and Resilience

- Each subsystem reports a health state (ok, degraded, unavailable) to the state store; the UI hides or dims the affected widget.
- COM calls return HRESULTs that are mapped to typed errors; transient failures retry with backoff, permanent ones disable the subsystem until the next relevant event.
- Panics in worker goroutines are recovered and logged, and the subsystem is restarted with a limit on restart frequency.
- Logging is local only, to a size-capped rolling file in `%APPDATA%\DynamicEdge\logs`.
- Explorer restarts, display changes, sleep/resume and RDP sessions must re-create the window and rebind audio and SMTC state.

## 15. Performance Budgets

| Metric | Target |
| --- | --- |
| Resident memory, idle | under 15 MB |
| CPU, idle (collapsed, nothing playing) | effectively 0% |
| CPU, playing with visualizer | under 3% on a mid-range laptop |
| Frame time while animating | under 4 ms (fits 144 Hz) |
| Track change to UI update | under 300 ms |
| Cold start to visible | under 500 ms |

Measure these continuously: add a hidden debug overlay showing frame time, goroutine count and heap size.

## 16. Packaging and Distribution

- Single static executable with an embedded manifest (DPI awareness, supported OS GUIDs, no admin elevation).
- Distribution through GitHub Releases as a portable zip; optional MSI/MSIX and `winget` manifest later.
- Start-with-Windows via the `HKCU\...\Run` key, toggled from the tray.
- Reproducible builds with pinned Go version and `-trimpath`; release artifacts signed if a certificate is available.
- Code signing matters: unsigned executables that hook mouse input and capture audio are likely to be flagged by SmartScreen and antivirus, so plan for this early.

## 17. Testing Strategy

- **Unit tests:** spring integration, LRC parsing, FFT binning, config validation, quiet-feed heuristic.
- **Golden-image tests:** render each state through the pure-Go backend and compare against reference images.
- **Integration tests on Windows CI:** window creation flags, click-through behavior, SMTC against a scripted test media session.
- **Manual matrix:** Windows 10 and 11, mixed-DPI multi-monitor, 60/144 Hz displays, Bluetooth earbuds, exclusive-fullscreen games, long-running soak test (24 h, watch for handle and memory leaks).

## 18. Risks and Open Questions

| Risk | Impact | Mitigation |
| --- | --- | --- |
| WinRT bindings in pure Go are brittle | SMTC unreliable | Early spike, polling fallback, Tier 2 shim behind build tag |
| GDI+ CPU cost at high refresh rates | Dropped frames | Pure-Go or Direct2D backend behind the render interface |
| Bluetooth battery property is not officially documented | Missing or wrong values | Per-device validation, graceful hide |
| Music Radar needs an external service | Conflicts with privacy goals | Opt-in only; pick a backend deliberately; may ship later |
| Mouse hook and audio capture trigger antivirus heuristics | Install friction | Code signing, minimal hook usage, clear documentation |
| Quiet Feed heuristic misfires | Annoying behavior | Tunable thresholds, easy disable |

Open questions:

1. Which recognition backend (if any) for Music Radar?
2. Is a license choice (MIT, GPL, MPL) needed before accepting contributions?
3. Is a settings UI drawn inside the overlay, or a separate small window?

## 19. Roadmap

1. **Milestone 0, spikes:** layered window with click-through and spring animation; SMTC bridge decision gate.
2. **Milestone 1, core:** collapsed and expanded states, media metadata and controls, config, tray.
3. **Milestone 2, audio:** volume flyout, loopback visualizer, device change handling.
4. **Milestone 3, extras:** lyrics, Bluetooth battery, quick launchers, dock modes.
5. **Milestone 4, polish:** quiet feed mode, fullscreen handling, multi-monitor, performance pass.
6. **Milestone 5, release:** signing, packaging, documentation, optional Music Radar.
