package option

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewOption(t *testing.T) {
	// Clear any existing options for clean test
	options = make(map[string]RegisteredOption)

	t.Run("creates option with defaults", func(t *testing.T) {
		assert := assert.New(t)
		opt := NewOption("test-option", "default-value")

		assert.Equal("test-option", opt.GetName())
		assert.Equal("default-value", opt.GetDefaultAsString())
		assert.Equal("DIRIO_TEST_OPTION", opt.GetEnvKey())
		assert.Equal("test-option", opt.GetFlagKey())
		assert.Equal("test_option", opt.GetViperKey())
		assert.True(opt.AllowsEnv())
		assert.True(opt.AllowsFlag())
		assert.True(opt.AllowsViper())
	})

	t.Run("registers option globally", func(t *testing.T) {
		options = make(map[string]RegisteredOption) // reset
		opt := NewOption("registered-opt", "value")

		allOpts := AllOptions()
		assert.Equal(t, opt, allOpts["registered-opt"])
	})

	t.Run("custom keys via functional options", func(t *testing.T) {
		assert := assert.New(t)
		options = make(map[string]RegisteredOption) // reset
		opt := NewOption("custom-opt", "value",
			WithEnvKey("CUSTOM_ENV_KEY"),
			WithFlagKey("custom-flag"),
			WithViperKey("custom_viper"),
		)

		assert.Equal("CUSTOM_ENV_KEY", opt.GetEnvKey())
		assert.Equal("custom-flag", opt.GetFlagKey())
		assert.Equal("custom_viper", opt.GetViperKey())
	})

	t.Run("WithoutEnv disables env", func(t *testing.T) {
		assert := assert.New(t)
		options = make(map[string]RegisteredOption) // reset
		opt := NewOption("no-env-opt", "value", WithoutEnv)

		assert.False(opt.AllowsEnv())
		assert.Empty(opt.GetEnvKey())
	})

	t.Run("WithoutFlag disables flag", func(t *testing.T) {
		assert := assert.New(t)
		options = make(map[string]RegisteredOption) // reset
		opt := NewOption("no-flag-opt", "value", WithoutFlag)

		assert.False(opt.AllowsFlag())
		assert.Empty(opt.GetFlagKey())
	})
}

func TestGetEnv(t *testing.T) {
	options = make(map[string]RegisteredOption) // reset
	opt := NewOption("env-test", "default")

	t.Run("returns empty when env not set", func(t *testing.T) {
		val := opt.GetEnv()
		assert.Empty(t, val)
	})

	t.Run("returns env value when set", func(t *testing.T) {
		os.Setenv("DIRIO_ENV_TEST", "from-env")
		defer os.Unsetenv("DIRIO_ENV_TEST")

		val := opt.GetEnv()
		assert.Equal(t, "from-env", val)
	})

	t.Run("returns empty when env disabled", func(t *testing.T) {
		os.Setenv("DIRIO_ENV_TEST", "from-env")
		defer os.Unsetenv("DIRIO_ENV_TEST")

		opt.SetAllowFromEnv(false)
		val := opt.GetEnv()
		assert.Empty(t, val)
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

			assert.Equal(t, tt.expected, result)
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
		assert := assert.New(t)
		envVals := AllEnvValues()

		assert.Len(envVals, 2) // opt-three is excluded
		assert.Equal("value1", envVals["DIRIO_OPT_ONE"])
		assert.Equal("value2", envVals["DIRIO_OPT_TWO"])
	})

	t.Run("ConfiguredEnvValues returns only set values", func(t *testing.T) {
		assert := assert.New(t)
		os.Unsetenv("DIRIO_OPT_TWO") // unset one

		envVals := ConfiguredEnvValues()
		assert.Len(envVals, 1)
		assert.Equal("value1", envVals["DIRIO_OPT_ONE"])
	})
}