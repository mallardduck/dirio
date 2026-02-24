package hostname

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

func stableID() string {
	if id := readMachineID(); id != "" {
		return id
	}
	return loadOrCreateRandomID()
}

func loadOrCreateRandomID() string {
	path := filepath.Join(stateDir(), "hostid")

	if b, err := os.ReadFile(path); err == nil {
		if len(b) == idBytes*2 {
			return string(b)
		}
	}

	b := make([]byte, idBytes)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)

	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte(id), 0o600)

	return id
}

// stateDirFunc is the function used to determine the state directory.
// Can be overridden in tests.
var stateDirFunc = defaultStateDir

// stateDir returns the directory path where hostname state files are stored.
func stateDir() string {
	return stateDirFunc()
}

// defaultStateDir returns the directory path where hostname state files are stored.
// This uses XDG conventions and returns ~/.config/dirio on Unix-like systems.
// Future enhancement: integrate with cobra+viper config system for centralized
// configuration management including write support for persisted state.
func defaultStateDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "dirio")
	}

	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "dirio")
	}

	// Fallback to current directory if home can't be determined
	return ".dirio"
}
