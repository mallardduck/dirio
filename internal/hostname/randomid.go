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
	path := filepath.Join(stateDirPath, "hostid")

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

// stateDirPath is set by SetStateDir (called by startup.Init).
var stateDirPath string

// SetStateDir configures the directory where hostname state files are stored.
// startup.Init calls this with the resolved data directory.
func SetStateDir(dir string) { stateDirPath = dir }
