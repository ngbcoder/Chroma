package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Theme config.
type Theme struct {
	DarkMode    bool    `json:"dark_mode"`
	AccentColor string  `json:"accent_color"`
	GlassOpacity float64 `json:"glass_opacity"`
	Snappiness  float64 `json:"snappiness"`
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

	// extra preserves unknown keys.
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
			Locked:              false,
		},
		Network: Network{
			Lyrics:     false,
			MusicRadar: false,
		},
		QuickLaunchers: []Launcher{
			{Name: "Terminal", Path: "wt.exe"},
		},
	}
}

// ConfigPath returns %APPDATA%\Chroma\config.json.
func ConfigPath() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		appdata = "."
	}
	return filepath.Join(appdata, "Chroma", "config.json")
}

// Load reads config from path (or default path).
func Load(path string) (*Config, error) {
	if path == "" {
		path = ConfigPath()
	}
	c := defaults()
	c.path = path
	c.extra = map[string]json.RawMessage{}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// write defaults
			if err := c.Save(); err != nil {
				return nil, err
			}
			return &c, nil
		}
		return nil, err
	}

	// unmarshal into map to preserve unknown keys
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// known fields
	if v, ok := raw["version"]; ok {
		json.Unmarshal(v, &c.Version)
	}
	if v, ok := raw["dock_mode"]; ok {
		json.Unmarshal(v, &c.DockMode)
	}
	if v, ok := raw["monitor"]; ok {
		json.Unmarshal(v, &c.Monitor)
	}
	if v, ok := raw["preset_location"]; ok {
		json.Unmarshal(v, &c.PresetLocation)
	}
	if v, ok := raw["window_x"]; ok {
		json.Unmarshal(v, &c.WindowX)
	}
	if v, ok := raw["window_y"]; ok {
		json.Unmarshal(v, &c.WindowY)
	}
	if v, ok := raw["theme"]; ok {
		json.Unmarshal(v, &c.Theme)
	}
	if v, ok := raw["behavior"]; ok {
		json.Unmarshal(v, &c.Behavior)
	}
	if v, ok := raw["network"]; ok {
		json.Unmarshal(v, &c.Network)
	}
	if v, ok := raw["quick_launchers"]; ok {
		json.Unmarshal(v, &c.QuickLaunchers)
	}

	// preserve unknown keys
	known := map[string]bool{
		"version": true, "dock_mode": true, "monitor": true,
		"preset_location": true, "window_x": true, "window_y": true,
		"theme": true, "behavior": true, "network": true, "quick_launchers": true,
	}
	for k, v := range raw {
		if !known[k] {
			c.extra[k] = v
		}
	}

	// validate / fill defaults for zero values
	if c.Version == 0 {
		c.Version = 2
	}
	if c.DockMode == "" {
		c.DockMode = "top_island"
	}
	if c.Monitor == "" {
		c.Monitor = "primary"
	}
	if c.PresetLocation == "" {
		c.PresetLocation = "top_center"
	}
	if c.Theme.GlassOpacity == 0 {
		c.Theme.GlassOpacity = 0.92
	}
	if c.Theme.Snappiness == 0 {
		c.Theme.Snappiness = 0.66
	}
	if c.Theme.AccentColor == "" {
		c.Theme.AccentColor = "#0F0F12"
	}
	if c.Behavior.DwellDelayMs == 0 {
		c.Behavior.DwellDelayMs = 150
	}
	if c.Behavior.LeaveDelayMs == 0 {
		c.Behavior.LeaveDelayMs = 400
	}

	return &c, nil
}

// Save writes config atomically (temp + rename).
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.path == "" {
		c.path = ConfigPath()
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0755); err != nil {
		return err
	}

	// build output map preserving unknown keys
	out := map[string]any{}
	for k, v := range c.extra {
		out[k] = v
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
		return err
	}

	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// Watch watches the config file for changes and calls onChange.
func (c *Config) Watch(onChange func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	dir := filepath.Dir(c.path)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return err
	}
	go func() {
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if ev.Name == c.path && (ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Create == fsnotify.Create) {
					if onChange != nil {
						onChange()
					}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}

// Path returns the config file path.
func (c *Config) Path() string { return c.path }
