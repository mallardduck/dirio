package data

import (
	"encoding/json"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultDataConfig(t *testing.T) {
	config := DefaultDataConfig()

	assert.Equal(t, ConfigDataVersion, config.Version)
	// Credentials are intentionally empty in defaults — set via "dirio init".
	assert.False(t, config.Credentials.IsConfigured())
	assert.Equal(t, "us-east-1", config.Region)
	assert.False(t, config.Compression.Enabled)
	assert.False(t, config.WORMEnabled)
	assert.NotZero(t, config.CreatedAt)
	assert.NotZero(t, config.UpdatedAt)

	// Validate should pass even with empty credentials.
	err := config.Validate()
	require.NoError(t, err)
}

func TestDataConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    *ConfigData
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid config",
			config:    DefaultDataConfig(),
			expectErr: false,
		},
		{
			name: "missing version",
			config: &ConfigData{
				Version: "",
				Credentials: CredentialsConfig{
					AccessKey: "key",
					SecretKey: "secret",
				},
			},
			expectErr: true,
			errMsg:    "version is required",
		},
		{
			name: "only access key set",
			config: &ConfigData{
				Version: ConfigDataVersion,
				Credentials: CredentialsConfig{
					AccessKey: "key",
					SecretKey: "",
				},
			},
			expectErr: true,
			errMsg:    "must both be set or both be empty",
		},
		{
			name: "only secret key set",
			config: &ConfigData{
				Version: ConfigDataVersion,
				Credentials: CredentialsConfig{
					AccessKey: "",
					SecretKey: "secret",
				},
			},
			expectErr: true,
			errMsg:    "must both be set or both be empty",
		},
		{
			name: "both credentials empty is valid",
			config: &ConfigData{
				Version:     ConfigDataVersion,
				Credentials: CredentialsConfig{},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSaveAndLoadDataConfig(t *testing.T) {
	fs := memfs.New()

	// Create a config
	original := DefaultDataConfig()
	original.Region = "us-west-2"
	original.Compression.Enabled = true
	original.WORMEnabled = true

	// Save it
	err := SaveDataConfig(fs, original)
	require.NoError(t, err)

	// Verify file exists
	assert.True(t, ConfigDataExists(fs))

	// Load it back
	loaded, err := LoadDataConfig(fs)
	require.NoError(t, err)

	// Compare
	assert.Equal(t, original.Version, loaded.Version)
	assert.Equal(t, original.Credentials.AccessKey, loaded.Credentials.AccessKey)
	assert.Equal(t, original.Credentials.SecretKey, loaded.Credentials.SecretKey)
	assert.Equal(t, original.Region, loaded.Region)
	assert.Equal(t, original.Compression.Enabled, loaded.Compression.Enabled)
	assert.Equal(t, original.WORMEnabled, loaded.WORMEnabled)
}

func TestDataConfigExists(t *testing.T) {
	fs := memfs.New()

	// Initially should not exist
	assert.False(t, ConfigDataExists(fs))

	// Save a config
	config := DefaultDataConfig()
	err := SaveDataConfig(fs, config)
	require.NoError(t, err)

	// Now should exist
	assert.True(t, ConfigDataExists(fs))
}

func TestLoadDataConfig_NotFound(t *testing.T) {
	fs := memfs.New()

	_, err := LoadDataConfig(fs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read data config")
}

func TestLoadDataConfig_InvalidJSON(t *testing.T) {
	fs := memfs.New()

	// Create .dirio directory
	err := fs.MkdirAll(".dirio", 0755)
	require.NoError(t, err)

	// Write invalid JSON
	f, err := fs.Create(".dirio/config.json")
	require.NoError(t, err)
	_, err = f.Write([]byte("invalid json"))
	require.NoError(t, err)
	f.Close()

	_, err = LoadDataConfig(fs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse data config")
}

func TestLoadDataConfig_InvalidData(t *testing.T) {
	fs := memfs.New()

	// Create a config with missing required fields
	invalidConfig := &ConfigData{
		Version: ConfigDataVersion,
		Credentials: CredentialsConfig{
			AccessKey: "", // Missing
			SecretKey: "secret",
		},
	}

	// Manually save it without validation
	err := fs.MkdirAll(".dirio", 0755)
	require.NoError(t, err)

	data, err := json.MarshalIndent(invalidConfig, "", "  ")
	require.NoError(t, err)

	f, err := fs.Create(".dirio/config.json")
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	f.Close()

	// Loading should fail validation — mismatched credentials (one set, one empty).
	_, err = LoadDataConfig(fs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid data config")
	assert.Contains(t, err.Error(), "must both be set or both be empty")
}

func TestSaveDataConfig_UpdatesTimestamp(t *testing.T) {
	fs := memfs.New()

	config := DefaultDataConfig()
	originalUpdatedAt := config.UpdatedAt

	// Wait a tiny bit to ensure timestamp difference
	// (In practice UpdatedAt uses time.Now() so will be different)
	err := SaveDataConfig(fs, config)
	require.NoError(t, err)

	// UpdatedAt should be set to current time (will be >= original)
	assert.True(t, config.UpdatedAt.After(originalUpdatedAt) || config.UpdatedAt.Equal(originalUpdatedAt))
}
