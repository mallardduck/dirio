package hostname

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBase(t *testing.T) {
	// Save original environment
	originalEnv := os.Getenv(envOverride)
	defer func() {
		if originalEnv != "" {
			os.Setenv(envOverride, originalEnv)
		} else {
			os.Unsetenv(envOverride)
		}
	}()

	t.Run("returns env override when set", func(t *testing.T) {
		os.Setenv(envOverride, "custom-hostname")
		result := Base()
		assert.Equal(t, "custom-hostname", result)
	})

	t.Run("sanitizes env override", func(t *testing.T) {
		os.Setenv(envOverride, "Custom@Hostname!")
		result := Base()
		assert.Equal(t, "custom-hostname", result)
	})

	t.Run("returns OS hostname when env not set", func(t *testing.T) {
		os.Unsetenv(envOverride)
		result := Base()
		// Should return either the sanitized OS hostname or default
		assert.NotEmpty(t, result)
		// Result should be sanitized (lowercase, no special chars)
		assert.Equal(t, sanitize(result), result)
	})

	t.Run("returns default when env override is invalid", func(t *testing.T) {
		os.Setenv(envOverride, "@@@@@")
		result := Base()
		// Should fall back to OS hostname or default
		assert.NotEmpty(t, result)
	})

	t.Run("never returns empty string", func(t *testing.T) {
		os.Unsetenv(envOverride)
		result := Base()
		assert.NotEmpty(t, result)
	})
}

func TestBaseDefault(t *testing.T) {
	// This test might be brittle depending on the test environment
	// but ensures the default fallback works
	os.Unsetenv(envOverride)
	result := Base()
	assert.NotEmpty(t, result)
	assert.LessOrEqual(t, len(result), maxLabelLen)
}
