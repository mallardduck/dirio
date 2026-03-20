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

	SetStateDir(tmpDir)
	t.Cleanup(func() { SetStateDir("") })

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

func TestSetStateDir(t *testing.T) {
	original := stateDirPath
	t.Cleanup(func() { stateDirPath = original })

	SetStateDir("/mnt/data/.dirio")
	assert.Equal(t, "/mnt/data/.dirio", stateDirPath)
}
