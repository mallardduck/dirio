package jsonutil

import (
	"encoding/json"
	"io"
	"os"

	"github.com/go-git/go-billy/v5"
	"github.com/minio/madmin-go/v3"

	"github.com/mallardduck/dirio/internal/config"
)

// isDebugMode determines whether to use pretty-printed JSON output.
// It checks multiple sources in order of precedence:
// 1. DIRIO_DEBUG environment variable (for explicit override: 1 or true)
// 2. Application config settings:
//   - --debug flag
//   - --log-level=debug AND --verbosity=verbose (both required)
//
// Returns true for development/debug mode (pretty JSON), false for production (compact JSON).
func isDebugMode() bool {
	// Check environment variable first for explicit override
	if debug := os.Getenv("DIRIO_DEBUG"); debug != "" {
		return debug == "1" || debug == "true"
	}

	// Check application config settings if available
	cfg := config.GetConfig()
	if cfg != nil {
		// Pretty-print if any debug/verbose flag is set
		if cfg.Debug || (cfg.LogLevel == "debug" && cfg.Verbosity == "verbose") {
			return true
		}
	}

	// Default to compact (production mode) if not explicitly set
	return false
}

// Marshal encodes v to JSON, automatically selecting compact or pretty format
// based on the current debug mode. Debug mode is enabled by:
//  1. DIRIO_DEBUG environment variable (1 or true)
//  2. --debug flag
//  3. --log-level=debug AND --verbosity=verbose together
//
// In debug mode, uses indented format with 2 spaces.
// In production mode (default), uses compact format.
func Marshal(v any) ([]byte, error) {
	if isDebugMode() {
		return json.MarshalIndent(v, "", "  ")
	}
	return json.Marshal(v)
}

// MarshalToFile encodes v to JSON and writes it to the specified path on the given filesystem.
// Automatically selects compact or pretty format based on debug mode.
// Uses file permissions 0644 for the output file.
func MarshalToFile(fs billy.Filesystem, path string, v any) error {
	data, err := Marshal(v)
	if err != nil {
		return err
	}

	file, err := fs.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}

// Unmarshal is a convenience wrapper around json.Unmarshal for consistency.
// It decodes JSON data into v.
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// DecryptAndUnmarshal decrypts MinIO admin API encrypted data and unmarshals it into v.
// This is used for handling MinIO admin API requests that use encrypted payloads.
// The password is typically the admin user's secret key.
func DecryptAndUnmarshal(password string, data io.Reader, v any) error {
	decrypted, err := madmin.DecryptData(password, data)
	if err != nil {
		return err
	}
	return json.Unmarshal(decrypted, v)
}

// MarshalAndEncrypt marshals v to JSON and encrypts it using MinIO admin API encryption.
// This is used for returning encrypted responses to MinIO admin API clients.
// The password is typically the admin user's secret key.
func MarshalAndEncrypt(password string, v any) ([]byte, error) {
	jsonData, err := Marshal(v)
	if err != nil {
		return nil, err
	}
	return madmin.EncryptData(password, jsonData)
}
