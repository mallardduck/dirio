package data

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/crypto"
)

// ConfigDataVersion represents the version of the configuration data format
const (
	ConfigDataVersion = "1.0.0"
)

// ConfigData represents configuration stored in the data directory (.dirio/config.json)
// These settings control how DirIO must behave when working with this specific data directory.
// Unlike application config (CLI flags), these settings travel with the data and take precedence.
type ConfigData struct {
	// Version of this config format for future migrations
	Version string `json:"version"`

	InstanceID uuid.UUID `json:"instance_id"`

	// Root credentials for this data directory
	Credentials CredentialsConfig `json:"credentials"`

	// Region is the geographic/logical region for this data
	Region string `json:"region,omitempty"`

	// Compression settings control how data is stored
	Compression CompressionConfig `json:"compression"`

	// StorageClass defines storage tier configuration
	StorageClass StorageClassConfig `json:"storageClass"`

	// TODO: Add API rate limits (per data directory)
	// TODO: Add storage path configurations if needed per data directory
	// TODO: Consider other MinIO settings that should be data-bound

	// Metadata timestamps
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CredentialsConfig contains root access credentials for this data directory.
// Both fields are optional — an unconfigured CredentialsConfig means the server
// falls back to CLI/env credentials. Use "dirio init" or "dirio credentials set"
// to configure them explicitly.
type CredentialsConfig struct {
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
}

// IsConfigured reports whether both access key and secret key are set.
func (c CredentialsConfig) IsConfigured() bool {
	return c.AccessKey != "" && c.SecretKey != ""
}

// CompressionConfig defines compression behavior
type CompressionConfig struct {
	// Enabled controls whether server-side compression is active
	Enabled bool `json:"enabled"`

	// AllowEncryption allows compression of encrypted objects
	AllowEncryption bool `json:"allowEncryption"`

	// Extensions lists file extensions that should be compressed (e.g., [".txt", ".log", ".json"])
	Extensions []string `json:"extensions,omitempty"`

	// MIMETypes lists MIME types that should be compressed (e.g., ["text/*", "application/json"])
	MIMETypes []string `json:"mimeTypes,omitempty"`
}

// StorageClassConfig defines storage tier settings
type StorageClassConfig struct {
	// Standard storage class configuration
	Standard string `json:"standard,omitempty"`

	// RRS (Reduced Redundancy Storage) configuration
	RRS string `json:"rrs,omitempty"`
}

// DefaultDataConfig returns a ConfigData with sensible defaults.
// Credentials are intentionally left empty — set them explicitly via
// "dirio init" or "dirio credentials set" rather than baking in defaults.
func DefaultDataConfig() *ConfigData {
	return &ConfigData{
		Version:     ConfigDataVersion,
		InstanceID:  uuid.New(),
		Credentials: CredentialsConfig{}, // empty — must be configured explicitly
		Region:      "us-east-1",         // AWS-style region for consistency
		Compression: CompressionConfig{
			Enabled:         false,
			AllowEncryption: false,
			Extensions:      []string{".txt", ".log", ".csv", ".json"},
			MIMETypes:       []string{"text/*", "application/json"},
		},
		StorageClass: StorageClassConfig{
			Standard: "",
			RRS:      "",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// Validate checks if the ConfigData is valid.
// Credentials are optional — either both fields must be set, or neither.
func (dc *ConfigData) Validate() error {
	if dc.Version == "" {
		return fmt.Errorf("version is required")
	}
	// Credentials are optional, but must be a consistent pair.
	if (dc.Credentials.AccessKey == "") != (dc.Credentials.SecretKey == "") {
		return fmt.Errorf("credentials: accessKey and secretKey must both be set or both be empty")
	}
	return nil
}

// LoadDataConfig loads the data config from .dirio/config.json
func LoadDataConfig(rootFS billy.Filesystem) (*ConfigData, error) {
	configPath := ".dirio/config.json"

	data, err := util.ReadFile(rootFS, configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read data config: %w", err)
	}

	var config ConfigData
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse data config: %w", err)
	}

	// Decrypt a secret key if stored encrypted (skip when credentials are not configured).
	if config.Credentials.SecretKey != "" {
		decrypted, err := crypto.Decrypt(config.Credentials.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
		}
		config.Credentials.SecretKey = decrypted
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid data config: %w", err)
	}

	return &config, nil
}

// SaveDataConfig saves the data config to .dirio/config.json
func SaveDataConfig(rootFS billy.Filesystem, config *ConfigData) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid data config: %w", err)
	}

	// Update the timestamp
	config.UpdatedAt = time.Now()

	// Ensure .dirio directory exists
	if err := rootFS.MkdirAll(consts.DirIOMetadataDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .dirio directory: %w", err)
	}

	configPath := ".dirio/config.json"

	// Work on a shallow copy so the in-memory config keeps the plaintext value.
	toSave := *config

	// Encrypt secret key before persisting (skip when credentials are not configured).
	if config.Credentials.SecretKey != "" {
		encryptedSecret, err := crypto.Encrypt(config.Credentials.SecretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt credentials: %w", err)
		}
		toSave.Credentials.SecretKey = encryptedSecret
	}

	// Marshal with indentation for readability
	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data config: %w", err)
	}

	if err := util.WriteFile(rootFS, configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write data config: %w", err)
	}

	return nil
}

// ConfigDataExists checks if a data config file exists
func ConfigDataExists(rootFS billy.Filesystem) bool {
	configPath := ".dirio/config.json"
	_, err := rootFS.Stat(configPath)
	return err == nil
}
