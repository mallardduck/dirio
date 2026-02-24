package hostname

import (
	"encoding/hex"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadMachineID(t *testing.T) {
	t.Run("returns empty or valid hex string", func(t *testing.T) {
		id := readMachineID()

		// If it returns something, it should be valid hex
		if id != "" {
			_, err := hex.DecodeString(id)
			require.NoError(t, err, "machine ID should be valid hex")
			assert.Len(t, id, idBytes*2, "machine ID should be 6 hex characters")
		}
	})

	t.Run("is deterministic", func(t *testing.T) {
		id1 := readMachineID()
		id2 := readMachineID()
		assert.Equal(t, id1, id2, "should return same ID on multiple calls")
	})
}

func TestReadLinuxMachineID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test")
	}

	t.Run("returns valid hex or empty", func(t *testing.T) {
		id := readLinuxMachineID()

		if id != "" {
			_, err := hex.DecodeString(id)
			require.NoError(t, err)
			assert.Len(t, id, idBytes*2)
		}
	})
}

func TestReadDarwinMachineID(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

	t.Run("returns valid hex or empty", func(t *testing.T) {
		id := readDarwinMachineID()

		if id != "" {
			_, err := hex.DecodeString(id)
			require.NoError(t, err)
			assert.Len(t, id, idBytes*2)
		}
	})
}

func TestReadWindowsMachineID(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	t.Run("returns valid hex or empty", func(t *testing.T) {
		id := readWindowsMachineID()

		if id != "" {
			_, err := hex.DecodeString(id)
			require.NoError(t, err)
			assert.Len(t, id, idBytes*2)
		}
	})
}

func TestHashAndTruncate(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "short string",
			input: "test",
		},
		{
			name:  "long string",
			input: "this-is-a-very-long-string-that-needs-to-be-hashed",
		},
		{
			name:  "UUID format",
			input: "12345678-1234-1234-1234-123456789012",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashAndTruncate(tt.input)

			assert.NotEmpty(t, result)
			assert.Len(t, result, idBytes*2, "should return 6 hex characters")

			// Verify it's valid hex
			_, err := hex.DecodeString(result)
			require.NoError(t, err)
		})
	}

	t.Run("same input produces same output", func(t *testing.T) {
		input := "test-string"
		result1 := hashAndTruncate(input)
		result2 := hashAndTruncate(input)
		assert.Equal(t, result1, result2)
	})

	t.Run("different inputs produce different outputs", func(t *testing.T) {
		result1 := hashAndTruncate("input1")
		result2 := hashAndTruncate("input2")
		assert.NotEqual(t, result1, result2)
	})
}
