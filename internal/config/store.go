package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Theme config.
type Theme struct {
	DarkMode     bool    `json:"dark_mode"`
	AccentColor  string  `json:"accent_color"`
	GlassOpacity float64 `json:"glass_opacity"`
	Snappiness   float64 `json:"snappiness"`
}

// Behavior config.
type Behavior struct {
	QuietFeedMode       bool `json:"quiet_feed_mode"`
	DoubleClickThinLine bool `json:"double_click_thin_line"`
	DwellDelayMs        int  `json:"dwell_delay_ms"`
	LeaveDelayMs        int  `json:"leave_delay_ms"`
	HideInFullscreen    bool `json:"hide_in_fullscreen"`
	Locked              bool `json:"locked"`
}

// Network config.
type Network struct {
	Lyrics     bool `json:"lyrics"`
	MusicRadar bool `json:"music_radar"`
}

// Launcher quick-launch slot.
type Launcher struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Config is the application configuration.
type Config struct {
	Version        int        `json:"version"`
	DockMode       string     `json:"dock_mode"`
	Monitor        string     `json:"monitor"`
	PresetLocation string     `json:"preset_location"`
	WindowX        int        `json:"window_x"`
	WindowY        int        `json:"window_y"`
	Theme          Theme      `json:"theme"`
	Behavior       Behavior   `json:"behavior"`
	Network        Network    `json:"network"`
	QuickLaunchers []Launcher `json:"quick_launchers"`

	extra map[string]json.RawMessage
	path  string
	mu    sync.Mutex
}

func defaults() Config {
	return Config{
		Version:        2,
		DockMode:       "top_island",
		Monitor:        "primary",
		PresetLocation: "top_center",
		Theme: Theme{
			DarkMode:     true,
			AccentColor:  "#0F0F12",
			GlassOpacity: 0.92,
			Snappiness:   0.66,
		},
		Behavior: Behavior{
			QuietFeedMode:       true,
			DoubleClickThinLine: true,
			DwellDelayMs:        150,
			LeaveDelayMs:        400,
			HideInFullscreen:    true,
		},
		QuickLaunchers: []Launcher{{Name: "Terminal", Path: "wt.exe"}},
	}
}

// ConfigPath returns the application config file path.
func ConfigPath() string {
	root := os.Getenv("APPDATA")
	if root == "" {
		root, _ = os.UserConfigDir()
	}
	if root == "" {
		root = "."
	}
	return filepath.Join(root, "Chroma", "config.json")
}

// Load reads, validates, and fills defaults for the config at path.
func Load(path string) (*Config, error) {
	if path == "" {
		path = ConfigPath()
	}
	c := defaults()
	c.path = path
	c.extra = map[string]json.RawMessage{}

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
		if err := c.Save(); err != nil {
			return nil, err
		}
		return &c, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if raw == nil {
		return nil, errors.New("invalid config: expected a JSON object")
	}

	fields := map[string]any{
		"version":         &c.Version,
		"dock_mode":       &c.DockMode,
		"monitor":         &c.Monitor,
		"preset_location": &c.PresetLocation,
		"window_x":        &c.WindowX,
		"window_y":        &c.WindowY,
		"theme":           &c.Theme,
		"behavior":        &c.Behavior,
		"network":         &c.Network,
		"quick_launchers": &c.QuickLaunchers,
	}
	for key, dst := range fields {
		if value, ok := raw[key]; ok {
			if err := json.Unmarshal(value, dst); err != nil {
				return nil, fmt.Errorf("invalid config field %q: %w", key, err)
			}
		}
	}

	for key, value := range raw {
		if _, known := fields[key]; !known {
			c.extra[key] = value
		}
	}

	if err := c.validateAndFillDefaults(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validateAndFillDefaults() error {
	if c.Version == 0 {
		c.Version = 2
	}
	if c.Version != 2 {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if c.DockMode == "" {
		c.DockMode = "top_island"
	}
	switch c.DockMode {
	case "top_island", "top_notch", "thin_line", "edge_left", "edge_right":
	default:
		return fmt.Errorf("invalid dock_mode %q", c.DockMode)
	}
	if c.Monitor == "" {
		c.Monitor = "primary"
	}
	if c.PresetLocation == "" {
		c.PresetLocation = "top_center"
	}
	switch c.PresetLocation {
	case "top_center", "top_left", "top_right", "bottom_center", "bottom_left", "bottom_right":
	default:
		return fmt.Errorf("invalid preset_location %q", c.PresetLocation)
	}
	if c.Theme.AccentColor == "" {
		c.Theme.AccentColor = "#0F0F12"
	}
	if len(c.Theme.AccentColor) != 7 || c.Theme.AccentColor[0] != '#' {
		return fmt.Errorf("invalid theme.accent_color %q: expected #RRGGBB", c.Theme.AccentColor)
	}
	for _, ch := range c.Theme.AccentColor[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", ch) {
			return fmt.Errorf("invalid theme.accent_color %q: expected #RRGGBB", c.Theme.AccentColor)
		}
	}
	if c.Theme.GlassOpacity == 0 {
		c.Theme.GlassOpacity = 0.92
	}
	if c.Theme.GlassOpacity < 0 || c.Theme.GlassOpacity > 1 {
		return fmt.Errorf("theme.glass_opacity must be between 0 and 1")
	}
	if c.Theme.Snappiness == 0 {
		c.Theme.Snappiness = 0.66
	}
	if c.Theme.Snappiness < 0 || c.Theme.Snappiness > 1 {
		return fmt.Errorf("theme.snappiness must be between 0 and 1")
	}
	if c.Behavior.DwellDelayMs == 0 {
		c.Behavior.DwellDelayMs = 150
	}
	if c.Behavior.LeaveDelayMs == 0 {
		c.Behavior.LeaveDelayMs = 400
	}
	if c.Behavior.DwellDelayMs < 0 || c.Behavior.LeaveDelayMs < 0 {
		return errors.New("behavior dwell and leave delays cannot be negative")
	}
	return nil
}

// Save writes config atomically, retaining unrecognized top-level keys.
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.path == "" {
		c.path = ConfigPath()
	}
	if err := c.validateAndFillDefaults(); err != nil {
		return err
	}
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	out := map[string]any{}
	for key, value := range c.extra {
		out[key] = value
	}
	out["version"] = c.Version
	out["dock_mode"] = c.DockMode
	out["monitor"] = c.Monitor
	out["preset_location"] = c.PresetLocation
	out["window_x"] = c.WindowX
	out["window_y"] = c.WindowY
	out["theme"] = c.Theme
	out["behavior"] = c.Behavior
	out["network"] = c.Network
	out["quick_launchers"] = c.QuickLaunchers

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write config temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync config temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close config temp file: %w", err)
	}
	if err := os.Rename(tmpPath, c.path); err != nil {
		return fmt.Errorf("replace config file: %w", err)
	}
	return nil
}

// ConfigWatcher owns a config directory watcher and its event goroutine.
type ConfigWatcher struct {
	watcher *fsnotify.Watcher
	stop    chan struct{}
	done    chan struct{}
	errors  chan error
	once    sync.Once
}

// Watch watches for config replacement or edits and invokes onChange after a
// short debounce. Close the returned watcher to release its resources.
func (c *Config) Watch(onChange func()) (*ConfigWatcher, error) {
	path := c.Path()
	if path == "" {
		path = ConfigPath()
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create config watcher: %w", err)
	}
	if err := watcher.Add(filepath.Dir(absPath)); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("watch config directory: %w", err)
	}
	cw := &ConfigWatcher{
		watcher: watcher,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		errors:  make(chan error, 1),
	}
	go cw.run(absPath, onChange)
	return cw, nil
}

func (w *ConfigWatcher) run(configPath string, onChange func()) {
	defer close(w.done)
	defer close(w.errors)
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	var timerC <-chan time.Time
	for {
		select {
		case ev, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			eventPath, err := filepath.Abs(ev.Name)
			if err != nil || !samePath(eventPath, configPath) {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
				continue
			}
			if timer == nil {
				timer = time.NewTimer(100 * time.Millisecond)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(100 * time.Millisecond)
			}
			timerC = timer.C
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			select {
			case w.errors <- err:
			default:
			}
		case <-timerC:
			timerC = nil
			if onChange != nil {
				onChange()
			}
		case <-w.stop:
			return
		}
	}
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if os.PathSeparator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Errors reports the first watcher error, if any.
func (w *ConfigWatcher) Errors() <-chan error { return w.errors }

// Close stops the watcher and waits for its event goroutine to exit.
func (w *ConfigWatcher) Close() error {
	var err error
	w.once.Do(func() {
		close(w.stop)
		err = w.watcher.Close()
		<-w.done
	})
	return err
}

// Path returns the config file path.
func (c *Config) Path() string { return c.path }
