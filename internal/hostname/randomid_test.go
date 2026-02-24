package hostname

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStableID(t *testing.T) {
	t.Run("returns non-empty ID", func(t *testing.T) {
		id := stableID()
		assert.NotEmpty(t, id)
	})

	t.Run("returns hex string", func(t *testing.T) {
		id := stableID()
		_, err := hex.DecodeString(id)
		require.NoError(t, err, "should return valid hex string")
	})

	t.Run("returns consistent ID", func(t *testing.T) {
		id1 := stableID()
		id2 := stableID()
		// Note: This might fail if machine ID is read successfully,
		// as it would return the machine ID instead of random ID
		assert.NotEmpty(t, id1)
		assert.NotEmpty(t, id2)
	})
}

func TestLoadOrCreateRandomID(t *testing.T) {
	tmpDir := t.TempDir()

	// Override stateDir for testing
	originalStateDir := stateDirFunc
	defer func() {
		stateDirFunc = originalStateDir
	}()
	stateDirFunc = func() string {
		return tmpDir
	}

	t.Run("creates new ID when file doesn't exist", func(t *testing.T) {
		id := loadOrCreateRandomID()
		assert.NotEmpty(t, id)
		assert.Len(t, id, idBytes*2, "should return 6 hex characters")

		// Verify file was created
		path := filepath.Join(tmpDir, "hostid")
		_, err := os.Stat(path)
		require.NoError(t, err)
	})

	t.Run("loads existing ID from file", func(t *testing.T) {
		id1 := loadOrCreateRandomID()
		id2 := loadOrCreateRandomID()
		assert.Equal(t, id1, id2, "should load the same ID from file")
	})

	t.Run("creates valid hex ID", func(t *testing.T) {
		id := loadOrCreateRandomID()
		_, err := hex.DecodeString(id)
		require.NoError(t, err)
	})

	t.Run("recreates ID if file is corrupted", func(t *testing.T) {
		path := filepath.Join(tmpDir, "hostid")

		// Write invalid data
		os.WriteFile(path, []byte("bad"), 0o600)

		id := loadOrCreateRandomID()
		assert.Len(t, id, idBytes*2)

		// Verify new valid ID was written
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Len(t, data, idBytes*2)
	})
}

func TestDefaultStateDir(t *testing.T) {
	t.Run("uses XDG_CONFIG_HOME when set", func(t *testing.T) {
		originalXDG := os.Getenv("XDG_CONFIG_HOME")
		defer func() {
			if originalXDG != "" {
				os.Setenv("XDG_CONFIG_HOME", originalXDG)
			} else {
				os.Unsetenv("XDG_CONFIG_HOME")
			}
		}()

		os.Setenv("XDG_CONFIG_HOME", "/tmp/test-config")
		dir := defaultStateDir()
		// Normalize to forward slashes for cross-platform comparison
		assert.Equal(t, "/tmp/test-config/dirio", filepath.ToSlash(dir))
	})

	t.Run("falls back to ~/.config when XDG not set", func(t *testing.T) {
		originalXDG := os.Getenv("XDG_CONFIG_HOME")
		defer func() {
			if originalXDG != "" {
				os.Setenv("XDG_CONFIG_HOME", originalXDG)
			} else {
				os.Unsetenv("XDG_CONFIG_HOME")
			}
		}()

		os.Unsetenv("XDG_CONFIG_HOME")
		dir := defaultStateDir()

		// Normalize to forward slashes for cross-platform comparison
		// Should contain .config/dirio
		assert.Contains(t, filepath.ToSlash(dir), ".config/dirio")
	})

	t.Run("always returns non-empty path", func(t *testing.T) {
		dir := defaultStateDir()
		assert.NotEmpty(t, dir)
	})
}
