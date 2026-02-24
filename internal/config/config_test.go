package config

import (
	"bytes"
	"log/slog"
	"os"
	"testing"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/internal/config/data"
)

func TestValueResolverPriority(t *testing.T) {
	// Reset viper for clean test
	v := viper.New()

	// Create a flag set
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("data-dir", "/default", "test flag")
	flags.Int("port", 9000, "test port")

	// Test 1: Default value when nothing is set
	t.Run("default value", func(t *testing.T) {
		resolver := NewValueResolver(flags, v)
		val := resolver.Get(DataDir)
		assert.Equal(t, "/data", val) // From options.go default (single assertion)
	})

	// Test 2: Viper (config file) overrides default
	t.Run("viper overrides default", func(t *testing.T) {
		v.Set("data_dir", "/from/config")
		resolver := NewValueResolver(flags, v)
		val := resolver.Get(DataDir)
		assert.Equal(t, "/from/config", val)
		v.Set("data_dir", nil) // reset
	})

	// Test 3: Flag overrides viper
	t.Run("flag overrides viper", func(t *testing.T) {
		v.Set("data_dir", "/from/config")
		flags.Parse([]string{"--data-dir=/from/flag"})

		resolver := NewValueResolver(flags, v)
		val := resolver.Get(DataDir)
		assert.Equal(t, "/from/flag", val)
		v.Set("data_dir", nil) // reset
	})

	// Test 4: Env overrides flag
	t.Run("env overrides flag", func(t *testing.T) {
		os.Setenv("DIRIO_DATA_DIR", "/from/env")
		defer os.Unsetenv("DIRIO_DATA_DIR")

		// Need a new resolver to pick up the env change
		resolver := NewValueResolver(flags, v)
		val := resolver.Get(DataDir)
		assert.Equal(t, "/from/env", val)
	})
}

func TestSettingsValidation(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		wantErr  bool
	}{
		{
			name: "valid settings",
			settings: Settings{
				DataDir:   "/data",
				Port:      9000,
				AccessKey: "key",
				SecretKey: "secret",
				LogLevel:  "info",
				LogFormat: "text",
				Verbosity: "normal",
				MDNSMode:  "auto",
			},
			wantErr: false,
		},
		{
			name: "empty data dir",
			settings: Settings{
				DataDir:   "",
				Port:      9000,
				AccessKey: "key",
				SecretKey: "secret",
				LogLevel:  "info",
				LogFormat: "text",
				Verbosity: "normal",
				MDNSMode:  "auto",
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			settings: Settings{
				DataDir:   "/data",
				Port:      0,
				AccessKey: "key",
				SecretKey: "secret",
				LogLevel:  "info",
				LogFormat: "text",
				Verbosity: "normal",
				MDNSMode:  "auto",
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			settings: Settings{
				DataDir:   "/data",
				Port:      9000,
				AccessKey: "key",
				SecretKey: "secret",
				LogLevel:  "invalid",
				LogFormat: "text",
				Verbosity: "normal",
				MDNSMode:  "auto",
			},
			wantErr: true,
		},
		{
			name: "invalid log format",
			settings: Settings{
				DataDir:   "/data",
				Port:      9000,
				AccessKey: "key",
				SecretKey: "secret",
				LogLevel:  "info",
				LogFormat: "invalid",
				Verbosity: "normal",
				MDNSMode:  "auto",
			},
			wantErr: true,
		},
		{
			name: "invalid verbosity",
			settings: Settings{
				DataDir:   "/data",
				Port:      9000,
				AccessKey: "key",
				SecretKey: "secret",
				LogLevel:  "info",
				LogFormat: "text",
				Verbosity: "invalid",
				MDNSMode:  "auto",
			},
			wantErr: true,
		},
		{
			name: "quiet verbosity",
			settings: Settings{
				DataDir:   "/data",
				Port:      9000,
				AccessKey: "key",
				SecretKey: "secret",
				LogLevel:  "info",
				LogFormat: "text",
				Verbosity: "quiet",
				MDNSMode:  "auto",
			},
			wantErr: false,
		},
		{
			name: "verbose verbosity",
			settings: Settings{
				DataDir:   "/data",
				Port:      9000,
				AccessKey: "key",
				SecretKey: "secret",
				LogLevel:  "info",
				LogFormat: "text",
				Verbosity: "verbose",
				MDNSMode:  "auto",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.settings.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetIntAndBool(t *testing.T) {
	v := viper.New()
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Int("port", 9000, "test port")
	flags.Bool("debug", false, "debug mode")

	t.Run("GetInt from default", func(t *testing.T) {
		resolver := NewValueResolver(flags, v)
		val := resolver.GetInt(Port)
		assert.Equal(t, 9000, val)
	})

	t.Run("GetBool from default", func(t *testing.T) {
		resolver := NewValueResolver(flags, v)
		val := resolver.GetBool(Debug)
		assert.False(t, val)
	})

	t.Run("GetInt from viper", func(t *testing.T) {
		v.Set("port", 8080)
		resolver := NewValueResolver(flags, v)
		val := resolver.GetInt(Port)
		assert.Equal(t, 8080, val)
		v.Set("port", nil)
	})

	t.Run("GetBool from env", func(t *testing.T) {
		os.Setenv("DIRIO_DEBUG", "true")
		defer os.Unsetenv("DIRIO_DEBUG")

		resolver := NewValueResolver(flags, v)
		val := resolver.GetBool(Debug)
		assert.True(t, val)
	})
}

func TestRegionWarning(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a data config with region us-east-1
	fs := osfs.New(tmpDir)
	dc := data.DefaultDataConfig()
	dc.Region = "us-east-1"
	dc.Credentials.AccessKey = "test-admin"
	dc.Credentials.SecretKey = "test-secret"

	err := data.SaveDataConfig(fs, dc)
	require.NoError(t, err)

	// Create flags with region us-west-2
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("data-dir", tmpDir, "test data dir")
	flags.Int("port", 9000, "test port")
	flags.String("region", "us-west-2", "test region")
	flags.String("access-key", "cli-admin", "test access key")
	flags.String("secret-key", "cli-secret", "test secret key")
	flags.String("log-level", "info", "test log level")
	flags.String("log-format", "text", "test log format")
	flags.String("verbosity", "normal", "test verbosity")
	flags.Bool("debug", false, "test debug")
	flags.Bool("mdns-enabled", false, "test mdns")
	flags.String("mdns-name", "dirio-s3", "test mdns name")
	flags.String("mdns-hostname", "", "test mdns hostname")
	flags.String("mdns-mode", "auto", "test mdns mode")
	flags.String("canonical-domain", "", "test canonical domain")

	// Parse to mark region as explicitly set and set data-dir
	err = flags.Parse([]string{"--data-dir=" + tmpDir, "--region=us-west-2"})
	require.NoError(t, err)

	// Capture log output to verify warning
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// Load config - should trigger warning
	settings, err := LoadConfig(flags, nil)
	require.NoError(t, err)

	// Verify data config was loaded
	assert.NotNil(t, settings.DataConfig)
	assert.Equal(t, "us-east-1", settings.DataConfig.Region)
	assert.Equal(t, "us-west-2", settings.Region) // CLI region still in settings
	assert.True(t, settings.CLIRegionExplicitlySet)

	// Verify warning was logged
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "CLI region flag ignored")
	assert.Contains(t, logOutput, "us-west-2") // CLI region
	assert.Contains(t, logOutput, "us-east-1") // Data config region
}

func TestRegionNoWarningWhenNotExplicitlySet(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a data config with region us-east-1
	fs := osfs.New(tmpDir)
	dc := data.DefaultDataConfig()
	dc.Region = "us-east-1"
	dc.Credentials.AccessKey = "test-admin"
	dc.Credentials.SecretKey = "test-secret"

	err := data.SaveDataConfig(fs, dc)
	require.NoError(t, err)

	// Create flags without setting region (will use default)
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("data-dir", tmpDir, "test data dir")
	flags.Int("port", 9000, "test port")
	flags.String("region", "us-east-1", "test region") // default value
	flags.String("access-key", "cli-admin", "test access key")
	flags.String("secret-key", "cli-secret", "test secret key")
	flags.String("log-level", "info", "test log level")
	flags.String("log-format", "text", "test log format")
	flags.String("verbosity", "normal", "test verbosity")
	flags.Bool("debug", false, "test debug")
	flags.Bool("mdns-enabled", false, "test mdns")
	flags.String("mdns-name", "dirio-s3", "test mdns name")
	flags.String("mdns-hostname", "", "test mdns hostname")
	flags.String("mdns-mode", "auto", "test mdns mode")
	flags.String("canonical-domain", "", "test canonical domain")

	// Parse data-dir but not region - region not explicitly set
	err = flags.Parse([]string{"--data-dir=" + tmpDir})
	require.NoError(t, err)

	// Capture log output to verify NO warning
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// Load config - should NOT trigger warning
	settings, err := LoadConfig(flags, nil)
	require.NoError(t, err)

	// Verify data config was loaded
	assert.NotNil(t, settings.DataConfig)
	assert.Equal(t, "us-east-1", settings.DataConfig.Region)
	assert.False(t, settings.CLIRegionExplicitlySet)

	// Verify warning was NOT logged
	logOutput := logBuf.String()
	assert.NotContains(t, logOutput, "CLI region flag ignored")
}
