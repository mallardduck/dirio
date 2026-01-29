package config

import (
	"fmt"
	"sync"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Settings represents all configuration values that dirio relies on to run.
// These values are resolved from: 1. Environment variables, 2. CLI flags, 3. Config file (YAML)
type Settings struct {
	// Server settings
	DataDir   string
	Port      int
	AccessKey string
	SecretKey string

	// Logging settings
	LogLevel  string
	LogFormat string
	Verbosity string
	Debug     bool

	// mDNS settings
	MDNSEnabled     bool
	MDNSName        string
	MDNSHostname    string
	MDNSMode        string
	CanonicalDomain string
}

// Validate checks that the configured settings are valid
func (s *Settings) Validate() error {
	if s.DataDir == "" {
		return fmt.Errorf("data directory must be set")
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if s.AccessKey == "" {
		return fmt.Errorf("access key must be set")
	}
	if s.SecretKey == "" {
		return fmt.Errorf("secret key must be set")
	}

	// Validate log level (slog levels)
	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLogLevels[s.LogLevel] {
		return fmt.Errorf("invalid log level: %s (valid: debug, info, warn, error)", s.LogLevel)
	}

	// Validate log format
	if s.LogFormat != "text" && s.LogFormat != "json" {
		return fmt.Errorf("invalid log format: %s (valid: text, json)", s.LogFormat)
	}

	// Validate verbosity
	validVerbosities := map[string]bool{
		"quiet": true, "normal": true, "verbose": true,
	}
	if !validVerbosities[s.Verbosity] {
		return fmt.Errorf("invalid verbosity: %s (valid: quiet, normal, verbose)", s.Verbosity)
	}

	// Validate mDNS mode
	validMDNSModes := map[string]bool{
		"auto": true, "guest": true, "master": true,
	}
	if !validMDNSModes[s.MDNSMode] {
		return fmt.Errorf("invalid mdns-mode: %s (valid: auto, guest, master)", s.MDNSMode)
	}

	return nil
}

// Global configuration state
var (
	currentConfig *Settings
	mu            sync.RWMutex
)

// LoadConfig creates a Settings struct by resolving values from all sources.
// This should be called during application startup after viper and flags are initialized.
func LoadConfig(flags *pflag.FlagSet, v *viper.Viper) (*Settings, error) {
	resolver := NewValueResolver(flags, v)

	settings := &Settings{
		// Server settings
		DataDir:   resolver.Get(DataDir),
		Port:      resolver.GetInt(Port),
		AccessKey: resolver.Get(AccessKey),
		SecretKey: resolver.Get(SecretKey),

		// Logging settings
		LogLevel:  resolver.Get(LogLevel),
		LogFormat: resolver.Get(LogFormat),
		Verbosity: resolver.Get(Verbosity),
		Debug:     resolver.GetBool(Debug),

		// mDNS settings
		MDNSEnabled:     resolver.GetBool(MDNSEnabled),
		MDNSName:        resolver.Get(MDNSName),
		MDNSHostname:    resolver.Get(MDNSHostname),
		MDNSMode:        resolver.Get(MDNSMode),
		CanonicalDomain: resolver.Get(CanonicalDomain),
	}

	// Debug flag overrides log level
	if settings.Debug {
		settings.LogLevel = "debug"
	}

	// Store as global config
	mu.Lock()
	currentConfig = settings
	mu.Unlock()

	return settings, nil
}

// GetConfig returns the current global configuration.
// Returns nil if LoadConfig has not been called.
func GetConfig() *Settings {
	mu.RLock()
	defer mu.RUnlock()
	return currentConfig
}

// MustGetConfig returns the current global configuration or panics if not loaded.
func MustGetConfig() *Settings {
	cfg := GetConfig()
	if cfg == nil {
		panic("config: MustGetConfig called before LoadConfig")
	}
	return cfg
}
