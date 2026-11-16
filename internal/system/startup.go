package system

import (
	"fmt"
	"os"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

const (
	startupKeyName   = `Software\Microsoft\Windows\CurrentVersion\Run`
	startupValueName = "Chroma"
)

// StartupEnabled reports whether Chroma is registered to start for this user.
func StartupEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, startupKeyName, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, fmt.Errorf("open startup registry key: %w", err)
	}
	defer key.Close()
	_, _, err = key.GetStringValue(startupValueName)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read startup registry value: %w", err)
	}
	return true, nil
}

// SetStartupEnabled registers or removes the current executable in HKCU Run.
func SetStartupEnabled(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, startupKeyName, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open startup registry key: %w", err)
	}
	defer key.Close()
	if !enabled {
		err := key.DeleteValue(startupValueName)
		if err == registry.ErrNotExist {
			return nil
		}
		if err != nil {
			return fmt.Errorf("remove startup registry value: %w", err)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	if err := key.SetStringValue(startupValueName, strconv.Quote(exe)); err != nil {
		return fmt.Errorf("write startup registry value: %w", err)
	}
	return nil
}
