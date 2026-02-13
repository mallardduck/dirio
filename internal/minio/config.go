package minio

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"

	configdata "github.com/mallardduck/dirio/internal/config/data"
)

// Config2019 represents MinIO 2019 config.json structure (format version 33)
type Config2019 struct {
	Version    string                `json:"version"`
	Credential Config2019Credential  `json:"credential"`
	Region     string                `json:"region"`
	WORM       string                `json:"worm"` // "on" or "off"
	Compress   Config2019Compression `json:"compress"`
}

type Config2019Credential struct {
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	Status    string `json:"status"`
}

type Config2019Compression struct {
	Enabled    bool     `json:"enabled"`
	Extensions []string `json:"extensions"`
	MIMETypes  []string `json:"mime-types"`
}

// Config2022 represents MinIO 2022 config.json structure (newer format)
// This is a flat key-value structure where each top-level key is a config section
type Config2022 struct {
	Credentials  Config2022Section `json:"credentials"`
	Region       Config2022Section `json:"region"`
	Compression  Config2022Section `json:"compression"`
	StorageClass Config2022Section `json:"storage_class"`
	// Add other sections as needed
}

type Config2022Section struct {
	Underscore []Config2022KV `json:"_"`
}

type Config2022KV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ImportConfig reads MinIO's config.json and converts it to DirIO ConfigData
func ImportConfig(minioFS billy.Filesystem) (*configdata.ConfigData, error) {
	configPath := "config/config.json"

	data, err := util.ReadFile(minioFS, configPath)
	if err != nil {
		if isNotExist(err) {
			// No config.json - return default config
			return configdata.DefaultDataConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config.json: %w", err)
	}

	// Try parsing as MinIO 2022 format first (flat key-value structure)
	var config2022 Config2022
	if err := json.Unmarshal(data, &config2022); err == nil && config2022.Credentials.Underscore != nil {
		fmt.Println("Detected MinIO 2022 config format")
		return convertConfig2022(config2022), nil
	}

	// Try parsing as MinIO 2019 format (nested structure)
	var config2019 Config2019
	if err := json.Unmarshal(data, &config2019); err == nil && config2019.Version != "" {
		fmt.Println("Detected MinIO 2019 config format")
		return convertConfig2019(config2019), nil
	}

	// Couldn't parse either format
	return nil, fmt.Errorf("unrecognized MinIO config.json format")
}

// convertConfig2019 converts MinIO 2019 config to DirIO ConfigData
func convertConfig2019(minioConfig Config2019) *configdata.ConfigData {
	config := configdata.DefaultDataConfig()

	// Credentials
	if minioConfig.Credential.AccessKey != "" {
		config.Credentials.AccessKey = minioConfig.Credential.AccessKey
	}
	if minioConfig.Credential.SecretKey != "" {
		config.Credentials.SecretKey = minioConfig.Credential.SecretKey
	}

	// Region
	config.Region = minioConfig.Region

	// Compression
	config.Compression.Enabled = minioConfig.Compress.Enabled
	if len(minioConfig.Compress.Extensions) > 0 {
		config.Compression.Extensions = minioConfig.Compress.Extensions
	}
	if len(minioConfig.Compress.MIMETypes) > 0 {
		config.Compression.MIMETypes = minioConfig.Compress.MIMETypes
	}

	// WORM mode
	config.WORMEnabled = strings.ToLower(minioConfig.WORM) == "on"

	fmt.Printf("Imported MinIO 2019 config: region=%s, worm=%v, compression=%v\n",
		config.Region, config.WORMEnabled, config.Compression.Enabled)

	return config
}

// convertConfig2022 converts MinIO 2022 config to DirIO ConfigData
func convertConfig2022(minioConfig Config2022) *configdata.ConfigData {
	config := configdata.DefaultDataConfig()

	// Credentials
	for _, kv := range minioConfig.Credentials.Underscore {
		switch kv.Key {
		case "access_key":
			config.Credentials.AccessKey = kv.Value
		case "secret_key":
			config.Credentials.SecretKey = kv.Value
		}
	}

	// Region
	for _, kv := range minioConfig.Region.Underscore {
		if kv.Key == "name" {
			config.Region = kv.Value
		}
	}

	// Compression
	for _, kv := range minioConfig.Compression.Underscore {
		switch kv.Key {
		case "enable":
			config.Compression.Enabled = kv.Value == "on"
		case "allow_encryption":
			config.Compression.AllowEncryption = kv.Value == "on"
		case "extensions":
			if kv.Value != "" {
				config.Compression.Extensions = strings.Split(kv.Value, ",")
			}
		case "mime_types":
			if kv.Value != "" {
				config.Compression.MIMETypes = strings.Split(kv.Value, ",")
			}
		}
	}

	// Note: MinIO 2022 doesn't have WORM in the main config anymore
	// It's now object lock configuration per bucket

	fmt.Printf("Imported MinIO 2022 config: region=%s, compression=%v\n",
		config.Region, config.Compression.Enabled)

	return config
}
