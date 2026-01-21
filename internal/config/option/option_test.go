package option

import (
	"os"
	"testing"
)

func TestNewOption(t *testing.T) {
	// Clear any existing options for clean test
	options = make(map[string]RegisteredOption)

	t.Run("creates option with defaults", func(t *testing.T) {
		opt := NewOption("test-option", "default-value")

		if opt.GetName() != "test-option" {
			t.Errorf("expected name 'test-option', got '%s'", opt.GetName())
		}
		if opt.GetDefaultAsString() != "default-value" {
			t.Errorf("expected default 'default-value', got '%s'", opt.GetDefaultAsString())
		}
		if opt.GetEnvKey() != "DIRIO_TEST_OPTION" {
			t.Errorf("expected env key 'DIRIO_TEST_OPTION', got '%s'", opt.GetEnvKey())
		}
		if opt.GetFlagKey() != "test-option" {
			t.Errorf("expected flag key 'test-option', got '%s'", opt.GetFlagKey())
		}
		if opt.GetViperKey() != "test_option" {
			t.Errorf("expected viper key 'test_option', got '%s'", opt.GetViperKey())
		}
		if !opt.AllowsEnv() {
			t.Error("expected AllowsEnv() to be true")
		}
		if !opt.AllowsFlag() {
			t.Error("expected AllowsFlag() to be true")
		}
		if !opt.AllowsViper() {
			t.Error("expected AllowsViper() to be true")
		}
	})

	t.Run("registers option globally", func(t *testing.T) {
		options = make(map[string]RegisteredOption) // reset
		opt := NewOption("registered-opt", "value")

		allOpts := AllOptions()
		if allOpts["registered-opt"] != opt {
			t.Error("option was not registered globally")
		}
	})

	t.Run("custom keys via functional options", func(t *testing.T) {
		options = make(map[string]RegisteredOption) // reset
		opt := NewOption("custom-opt", "value",
			WithEnvKey("CUSTOM_ENV_KEY"),
			WithFlagKey("custom-flag"),
			WithViperKey("custom_viper"),
		)

		if opt.GetEnvKey() != "CUSTOM_ENV_KEY" {
			t.Errorf("expected custom env key, got '%s'", opt.GetEnvKey())
		}
		if opt.GetFlagKey() != "custom-flag" {
			t.Errorf("expected custom flag key, got '%s'", opt.GetFlagKey())
		}
		if opt.GetViperKey() != "custom_viper" {
			t.Errorf("expected custom viper key, got '%s'", opt.GetViperKey())
		}
	})

	t.Run("WithoutEnv disables env", func(t *testing.T) {
		options = make(map[string]RegisteredOption) // reset
		opt := NewOption("no-env-opt", "value", WithoutEnv)

		if opt.AllowsEnv() {
			t.Error("expected AllowsEnv() to be false")
		}
		if opt.GetEnvKey() != "" {
			t.Errorf("expected empty env key, got '%s'", opt.GetEnvKey())
		}
	})

	t.Run("WithoutFlag disables flag", func(t *testing.T) {
		options = make(map[string]RegisteredOption) // reset
		opt := NewOption("no-flag-opt", "value", WithoutFlag)

		if opt.AllowsFlag() {
			t.Error("expected AllowsFlag() to be false")
		}
		if opt.GetFlagKey() != "" {
			t.Errorf("expected empty flag key, got '%s'", opt.GetFlagKey())
		}
	})
}

func TestGetEnv(t *testing.T) {
	options = make(map[string]RegisteredOption) // reset
	opt := NewOption("env-test", "default")

	t.Run("returns empty when env not set", func(t *testing.T) {
		val := opt.GetEnv()
		if val != "" {
			t.Errorf("expected empty string, got '%s'", val)
		}
	})

	t.Run("returns env value when set", func(t *testing.T) {
		os.Setenv("DIRIO_ENV_TEST", "from-env")
		defer os.Unsetenv("DIRIO_ENV_TEST")

		val := opt.GetEnv()
		if val != "from-env" {
			t.Errorf("expected 'from-env', got '%s'", val)
		}
	})

	t.Run("returns empty when env disabled", func(t *testing.T) {
		os.Setenv("DIRIO_ENV_TEST", "from-env")
		defer os.Unsetenv("DIRIO_ENV_TEST")

		opt.SetAllowFromEnv(false)
		val := opt.GetEnv()
		if val != "" {
			t.Errorf("expected empty when env disabled, got '%s'", val)
		}
	})
}

func TestGetDefaultAsString(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"float", 3.14, "3.14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options = make(map[string]RegisteredOption) // reset
			var result string

			switch v := tt.value.(type) {
			case string:
				opt := NewOption("test", v)
				result = opt.GetDefaultAsString()
			case int:
				opt := NewOption("test", v)
				result = opt.GetDefaultAsString()
			case bool:
				opt := NewOption("test", v)
				result = opt.GetDefaultAsString()
			case float64:
				opt := NewOption("test", v)
				result = opt.GetDefaultAsString()
			}

			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestAllEnvValues(t *testing.T) {
	options = make(map[string]RegisteredOption) // reset

	NewOption("opt-one", "default1")
	NewOption("opt-two", "default2")
	NewOption("opt-three", "default3", WithoutEnv)

	os.Setenv("DIRIO_OPT_ONE", "value1")
	os.Setenv("DIRIO_OPT_TWO", "value2")
	defer os.Unsetenv("DIRIO_OPT_ONE")
	defer os.Unsetenv("DIRIO_OPT_TWO")

	t.Run("AllEnvValues returns all env-enabled options", func(t *testing.T) {
		envVals := AllEnvValues()

		if len(envVals) != 2 { // opt-three is excluded
			t.Errorf("expected 2 env values, got %d", len(envVals))
		}
		if envVals["DIRIO_OPT_ONE"] != "value1" {
			t.Errorf("expected 'value1', got '%s'", envVals["DIRIO_OPT_ONE"])
		}
		if envVals["DIRIO_OPT_TWO"] != "value2" {
			t.Errorf("expected 'value2', got '%s'", envVals["DIRIO_OPT_TWO"])
		}
	})

	t.Run("ConfiguredEnvValues returns only set values", func(t *testing.T) {
		os.Unsetenv("DIRIO_OPT_TWO") // unset one

		envVals := ConfiguredEnvValues()
		if len(envVals) != 1 {
			t.Errorf("expected 1 configured env value, got %d", len(envVals))
		}
		if envVals["DIRIO_OPT_ONE"] != "value1" {
			t.Errorf("expected 'value1', got '%s'", envVals["DIRIO_OPT_ONE"])
		}
	})
}