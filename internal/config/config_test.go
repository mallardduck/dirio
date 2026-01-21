package config

import (
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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
		if val != "/data" { // From options.go default
			t.Errorf("expected default '/data', got '%s'", val)
		}
	})

	// Test 2: Viper (config file) overrides default
	t.Run("viper overrides default", func(t *testing.T) {
		v.Set("data_dir", "/from/config")
		resolver := NewValueResolver(flags, v)
		val := resolver.Get(DataDir)
		if val != "/from/config" {
			t.Errorf("expected '/from/config', got '%s'", val)
		}
		v.Set("data_dir", nil) // reset
	})

	// Test 3: Flag overrides viper
	t.Run("flag overrides viper", func(t *testing.T) {
		v.Set("data_dir", "/from/config")
		flags.Parse([]string{"--data-dir=/from/flag"})

		resolver := NewValueResolver(flags, v)
		val := resolver.Get(DataDir)
		if val != "/from/flag" {
			t.Errorf("expected '/from/flag', got '%s'", val)
		}
		v.Set("data_dir", nil) // reset
	})

	// Test 4: Env overrides flag
	t.Run("env overrides flag", func(t *testing.T) {
		os.Setenv("DIRIO_DATA_DIR", "/from/env")
		defer os.Unsetenv("DIRIO_DATA_DIR")

		// Need a new resolver to pick up the env change
		resolver := NewValueResolver(flags, v)
		val := resolver.Get(DataDir)
		if val != "/from/env" {
			t.Errorf("expected '/from/env', got '%s'", val)
		}
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
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.settings.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
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
		if val != 9000 {
			t.Errorf("expected 9000, got %d", val)
		}
	})

	t.Run("GetBool from default", func(t *testing.T) {
		resolver := NewValueResolver(flags, v)
		val := resolver.GetBool(Debug)
		if val != false {
			t.Errorf("expected false, got %v", val)
		}
	})

	t.Run("GetInt from viper", func(t *testing.T) {
		v.Set("port", 8080)
		resolver := NewValueResolver(flags, v)
		val := resolver.GetInt(Port)
		if val != 8080 {
			t.Errorf("expected 8080, got %d", val)
		}
		v.Set("port", nil)
	})

	t.Run("GetBool from env", func(t *testing.T) {
		os.Setenv("DIRIO_DEBUG", "true")
		defer os.Unsetenv("DIRIO_DEBUG")

		resolver := NewValueResolver(flags, v)
		val := resolver.GetBool(Debug)
		if val != true {
			t.Errorf("expected true, got %v", val)
		}
	})
}