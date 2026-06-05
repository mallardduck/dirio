// Package profile manages dio client profiles stored in ~/.dirio/client.yaml.
package profile

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultConfigFile = ".dirio/client.yaml"

// Profile holds connection parameters for a single DirIO server.
type Profile struct {
	Endpoint   string `yaml:"endpoint"`
	AccessKey  string `yaml:"access_key"`
	SecretKey  string `yaml:"secret_key"`
	Region     string `yaml:"region,omitempty"`
	ServerType string `yaml:"server_type,omitempty"`
}

// Config is the top-level structure of ~/.dirio/client.yaml.
type Config struct {
	DefaultProfile string             `yaml:"default_profile"`
	Profiles       map[string]Profile `yaml:"profiles"`
}

// DefaultConfig returns a Config with a single "local" profile pointing at
// localhost:9000. Used as the seed value for `dio config init`.
func DefaultConfig() Config {
	return Config{
		DefaultProfile: "local",
		Profiles: map[string]Profile{
			"local": {
				Endpoint:  "http://localhost:9000",
				AccessKey: "dirio-admin",
				SecretKey: "dirio-admin-secret",
				Region:    "us-east-1",
			},
		},
	}
}

// ConfigPath returns the path to the client config file (~/.dirio/client.yaml).
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("profile: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, defaultConfigFile), nil
}

// Load reads the config file and returns the parsed Config. If the file does
// not exist, an empty Config (no profiles) is returned without error.
func Load() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{Profiles: make(map[string]Profile)}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("profile: read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("profile: parse %s: %w", path, err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]Profile)
	}
	return cfg, nil
}

// Save writes cfg to ~/.dirio/client.yaml, creating the directory if needed.
func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("profile: create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("profile: marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("profile: write %s: %w", path, err)
	}
	return nil
}
