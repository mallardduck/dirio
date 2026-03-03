package config

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/mallardduck/dirio/internal/config/data"
)

// Settings represents all configuration values that dirio relies on to run.
// These values are resolved from: 1. Environment variables, 2. CLI flags, 3. Config file (YAML)
//
// Note: Some settings (credentials, region) may also come from data directory config (.dirio/config.json)
// which takes precedence over CLI/app config for those values.
type Settings struct {
	// Server settings
	DataDir   string
	Port      int
	Region    string // CLI region (informational if data config exists)
	AccessKey string // CLI admin credentials (coexists with data config credentials)
	SecretKey string // CLI admin credentials (coexists with data config credentials)

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

	// Data directory configuration (loaded from .dirio/config.json if exists)
	DataConfig *data.ConfigData

	// CLICredentialsExplicitlySet tracks whether access_key/secret_key were
	// explicitly provided (via env, flag, or config) vs using defaults
	CLICredentialsExplicitlySet bool

	// CLIRegionExplicitlySet tracks whether region was explicitly provided
	CLIRegionExplicitlySet bool

	// Console settings
	ConsoleEnabled       bool
	ConsoleDedicatedPort bool
	ConsolePort          int

	// Lifecycle settings
	ShutdownTimeout time.Duration

	// Telemetry / OTLP settings
	OTLPMetricsEnabled  bool
	OTLPMetricsEndpoint string
	OTLPMetricsInterval time.Duration
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
		Region:    resolver.Get(Region),
		AccessKey: resolver.Get(AccessKey), // CLI admin credentials
		SecretKey: resolver.Get(SecretKey), // CLI admin credentials

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

		// Track if credentials were explicitly set
		CLICredentialsExplicitlySet: resolver.WasExplicitlySet(AccessKey) ||
			resolver.WasExplicitlySet(SecretKey),

		// Track if region was explicitly set
		CLIRegionExplicitlySet: resolver.WasExplicitlySet(Region),

		// Console settings
		ConsoleEnabled:       resolver.GetBool(ConsoleEnabled),
		ConsoleDedicatedPort: resolver.GetBool(ConsoleDedicatedPort),
		ConsolePort:          resolver.GetInt(ConsolePort),

		// Lifecycle settings
		ShutdownTimeout: time.Duration(resolver.GetInt(ShutdownTimeout)) * time.Second,

		// Telemetry / OTLP settings
		OTLPMetricsEnabled:  resolver.GetBool(OTLPMetricsEnabled),
		OTLPMetricsEndpoint: resolver.Get(OTLPMetricsEndpoint),
		OTLPMetricsInterval: time.Duration(resolver.GetInt(OTLPMetricsInterval)) * time.Second,
	}

	// Debug flag overrides log level
	if settings.Debug {
		settings.LogLevel = "debug"
	}

	// Try to load data config from data directory
	if err := loadDataConfig(settings); err != nil {
		return nil, fmt.Errorf("failed to load data config: %w", err)
	}

	// Store as global config
	mu.Lock()
	currentConfig = settings
	mu.Unlock()

	return settings, nil
}

// loadDataConfig attempts to load data config from .dirio/config.json
// If it exists, it populates settings.ConfigData
func loadDataConfig(settings *Settings) error {
	// Create filesystem for data directory
	fs := osfs.New(settings.DataDir)

	// Check if data config exists
	if !data.ConfigDataExists(fs) {
		slog.Debug("No data config found, will use CLI/app config values")
		return nil
	}

	// Load data config
	dc, err := data.LoadDataConfig(fs)
	if err != nil {
		return fmt.Errorf("data config exists but failed to load: %w", err)
	}

	settings.DataConfig = dc
	slog.Info("Loaded data config from .dirio/config.json",
		"region", dc.Region,
		"compression", dc.Compression.Enabled,
		"data_admin", dc.Credentials.AccessKey)

	// Warn if CLI region differs from data config region
	if settings.CLIRegionExplicitlySet && settings.Region != dc.Region {
		slog.Warn("CLI region flag ignored - data config takes precedence",
			"cli_region", settings.Region,
			"data_config_region", dc.Region,
			"hint", "To update region use: dirio config set region <value>")
	}

	// Note: We do NOT warn about credentials - both CLI and data config credentials coexist
	// CLI credentials (settings.AccessKey/SecretKey) provide temporary/alternative admin access
	// Data config credentials (dc.Credentials) are the "official" admin for this data directory

	return nil
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
