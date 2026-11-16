package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
	os.WriteFile(p, data, 0644)
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
	out, _ := os.ReadFile(p)
	var m map[string]json.RawMessage
	json.Unmarshal(out, &m)
	if _, ok := m["foo"]; !ok {
		t.Fatal("unknown key 'foo' not preserved")
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	c, _ := Load(p)
	c.DockMode = "thin_line"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	var m map[string]any
	json.Unmarshal(data, &m)
	if m["dock_mode"] != "thin_line" {
		t.Fatalf("got %v", m["dock_mode"])
	}
}
