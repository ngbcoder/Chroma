package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaultsOnMissing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 2 {
		t.Fatalf("version %d", c.Version)
	}
	if c.DockMode != "top_island" {
		t.Fatalf("dock_mode %s", c.DockMode)
	}
	// file created
	if _, err := os.Stat(p); err != nil {
		t.Fatal("config file not created")
	}
}

func TestPreserveUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	// write config with unknown key
	data := []byte(`{"version":2,"foo":"bar","theme":{"dark_mode":false}}`)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme.DarkMode {
		t.Fatal("dark_mode should be false")
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["foo"]; !ok {
		t.Fatal("unknown key 'foo' not preserved")
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	c.DockMode = "thin_line"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["dock_mode"] != "thin_line" {
		t.Fatalf("got %v", m["dock_mode"])
	}
	c.DockMode = "edge_right"
	if err := c.Save(); err != nil {
		t.Fatalf("replace existing config: %v", err)
	}
}

func TestLoadPartialConfigKeepsDefaultsAndHonorsExplicitFalse(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{"theme":{"dark_mode":false},"behavior":{"locked":true}}`), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme.DarkMode || !c.Behavior.Locked {
		t.Fatalf("explicit booleans not loaded: theme=%+v behavior=%+v", c.Theme, c.Behavior)
	}
	if c.Theme.AccentColor != "#0F0F12" || c.Behavior.DwellDelayMs != 150 || !c.Behavior.QuietFeedMode {
		t.Fatalf("omitted fields did not retain defaults: theme=%+v behavior=%+v", c.Theme, c.Behavior)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "invalid field type", data: `{"window_x":"left"}`},
		{name: "unknown dock mode", data: `{"dock_mode":"floating"}`},
		{name: "unsupported version", data: `{"version":3}`},
		{name: "invalid opacity", data: `{"theme":{"glass_opacity":1.5}}`},
		{name: "invalid json root", data: `[]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(p, []byte(tt.data), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(p); err == nil {
				t.Fatal("expected invalid config error")
			}
		})
	}
}

func TestWatchNotifiesOnConfigChangeAndCloses(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	changed := make(chan struct{}, 1)
	watcher, err := c.Watch(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	c.DockMode = "top_notch"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for config change")
	}
	if err := watcher.Close(); err != nil {
		t.Fatal(err)
	}
}
