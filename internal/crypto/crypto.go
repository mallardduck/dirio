// Package crypto provides credential encryption/decryption for DirIO.
//
// Encrypted values use the format "enc:v1:<base64(nonce||ciphertext)>" so they
// are self-describing and backward-compatible — plaintext values (no prefix)
// pass through Decrypt unchanged.
//
// # Key format
//
// All keys are expressed as "base64:<standard-base64-encoded-32-bytes>",
// e.g. DIRIO_ENCRYPTION_KEY="base64:AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
//
// # Key management (priority order)
//
//  1. DIRIO_ENCRYPTION_KEY env var. Recommended for production / container
//     deployments.
//
//  2. Keyring file at <dataDir>/.dirio/keyring — auto-created on first run
//     with a randomly generated key (permissions 0o600). Back this file up
//     separately from the data directory; losing it means encrypted credentials
//     cannot be recovered without key rotation.
//
// # Key rotation
//
// Zero-downtime rotation — new writes use the new key while old encrypted
// values remain readable via ordered fallback through previous keys:
//
//  1. Generate a new key (`dirio key generate`).
//  2. Move the current key to DIRIO_PREVIOUS_ENCRYPTION_KEYS (comma-separated)
//     or to subsequent lines in the keyring file.
//  3. Set the new key as DIRIO_ENCRYPTION_KEY / the first line of the keyring.
//  4. Restart — new writes use the new key, old values decrypt via fallback.
//  5. Optionally run `dirio rekey` to re-encrypt all values, then drop old keys.
//
// Call Init(dataDir) once at process startup before any encrypt/decrypt
// operations.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mallardduck/dirio/internal/consts"
)

const (
	// EnvKey is the environment variable name for the current 32-byte master key.
	EnvKey = "DIRIO_ENCRYPTION_KEY"

	// EnvPreviousKeys is a comma-separated list of previous keys used as
	// decryption fallbacks during key rotation.
	EnvPreviousKeys = "DIRIO_PREVIOUS_ENCRYPTION_KEYS"

	keyPrefix   = "base64:"
	encPrefix   = "enc:v1:"
	keyringFile = ".dirio/keyring"
)

// Manager handles AES-256-GCM encryption and decryption of credential strings.
// A zero-value Manager (nil key) is a safe no-op.
type Manager struct {
	key          []byte   // current key — always used for encryption
	previousKeys [][]byte // fallback keys tried during decryption, in order
}

// defaultManager is the package-level singleton initialised by Init.
var defaultManager = &Manager{}

// Init initialises the package-level encryption manager.
//
// Key selection priority:
//  1. DIRIO_ENCRYPTION_KEY env var (+ optional DIRIO_PREVIOUS_ENCRYPTION_KEYS)
//  2. <dataDir>/.dirio/keyring file — loaded if present, auto-generated if not
//
// Call this once at startup, before any data config or metadata operations.
func Init(dataDir string) error {
	// Priority 1: explicit env var — good for production / containers.
	if raw := os.Getenv(EnvKey); raw != "" {
		key, err := decodeKey(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", EnvKey, err)
		}
		prev, err := decodePreviousKeysFromEnv()
		if err != nil {
			return err
		}
		defaultManager = &Manager{key: key, previousKeys: prev}
		return nil
	}

	// Priority 2: keyring file (load existing or auto-generate on first run).
	key, previousKeys, err := loadOrCreateKeyring(dataDir)
	if err != nil {
		return fmt.Errorf("keyring: %w", err)
	}
	defaultManager = &Manager{key: key, previousKeys: previousKeys}
	return nil
}

// decodePreviousKeysFromEnv parses DIRIO_PREVIOUS_ENCRYPTION_KEYS.
func decodePreviousKeysFromEnv() ([][]byte, error) {
	raw := os.Getenv(EnvPreviousKeys)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	keys := make([][]byte, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key, err := decodeKey(p)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", EnvPreviousKeys, i, err)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// loadOrCreateKeyring loads keys from <dataDir>/.dirio/keyring.
// The file format is one "base64:<key>" per line: the first line is the
// current key, subsequent lines are previous keys (decryption fallbacks).
// If the file does not exist a fresh key is generated and the file is created.
func loadOrCreateKeyring(dataDir string) (current []byte, previous [][]byte, err error) {
	keyringPath := filepath.Join(dataDir, keyringFile)

	data, err := os.ReadFile(keyringPath)
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
			return nil, nil, fmt.Errorf("keyring file %s is empty", keyringPath)
		}

		current, err = decodeKey(strings.TrimSpace(lines[0]))
		if err != nil {
			return nil, nil, fmt.Errorf("load current key from %s: %w", keyringPath, err)
		}

		for i, line := range lines[1:] {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			pk, err := decodeKey(line)
			if err != nil {
				return nil, nil, fmt.Errorf("load previous key [%d] from %s: %w", i, keyringPath, err)
			}
			previous = append(previous, pk)
		}
		return current, previous, nil
	}

	if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("read %s: %w", keyringPath, err)
	}

	// First run — generate a fresh key.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	// Ensure .dirio directory exists (restrictive permissions).
	dirPath := filepath.Join(dataDir, consts.DirIOMetadataDir)
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create %s: %w", dirPath, err)
	}

	// Write keyring file — owner read/write only.
	encoded := encodeKey(key) + "\n"
	if err := os.WriteFile(keyringPath, []byte(encoded), 0o600); err != nil {
		return nil, nil, fmt.Errorf("write %s: %w", keyringPath, err)
	}

	slog.Info("encryption keyring generated",
		"path", keyringPath,
		"hint_backup", "back this file up — losing it means encrypted credentials cannot be recovered",
		"hint_production", "for production deployments, set "+EnvKey+" instead",
	)

	return key, nil, nil
}

// GenerateKey generates a new random 32-byte key and returns it in
// "base64:<encoded>" format, ready to paste into a keyring file or env var.
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return encodeKey(key), nil
}

// RotateKeyring generates a new key and prepends it to the keyring file,
// shifting the current key down as the first previous-key fallback.
// Returns the new key string so callers can display it.
// Safe to call even if no keyring file exists yet.
func RotateKeyring(dataDir string) (string, error) {
	keyringPath := filepath.Join(dataDir, keyringFile)

	// Read existing keyring content (may not exist yet).
	existing := ""
	data, err := os.ReadFile(keyringPath)
	if err == nil {
		existing = strings.TrimSpace(string(data))
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read keyring: %w", err)
	}

	newKeyStr, err := GenerateKey()
	if err != nil {
		return "", err
	}

	// New key on top; old keys preserved below as fallbacks.
	content := newKeyStr + "\n"
	if existing != "" {
		content += existing + "\n"
	}

	dirPath := filepath.Join(dataDir, consts.DirIOMetadataDir)
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", dirPath, err)
	}
	if err := os.WriteFile(keyringPath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write keyring: %w", err)
	}

	return newKeyStr, nil
}

// Enabled reports whether the package-level manager has an active key.
func Enabled() bool { return defaultManager.Enabled() }

// Encrypt encrypts plaintext using the package-level manager.
func Encrypt(plaintext string) (string, error) { return defaultManager.Encrypt(plaintext) }

// Decrypt decrypts s using the package-level manager.
func Decrypt(s string) (string, error) { return defaultManager.Decrypt(s) }

// IsEncrypted reports whether s carries the enc:v1: prefix.
func IsEncrypted(s string) bool { return strings.HasPrefix(s, encPrefix) }

// encodeKey encodes a raw key as "base64:<standard-base64>".
func encodeKey(key []byte) string {
	return keyPrefix + base64.StdEncoding.EncodeToString(key)
}

// decodeKey decodes a "base64:<standard-base64>" key string into raw bytes.
// The key must decode to exactly 32 bytes.
func decodeKey(s string) ([]byte, error) {
	if !strings.HasPrefix(s, keyPrefix) {
		return nil, fmt.Errorf("key must start with %q", keyPrefix)
	}
	key, err := base64.StdEncoding.DecodeString(s[len(keyPrefix):])
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be exactly 32 bytes, got %d", len(key))
	}
	return key, nil
}

// Enabled reports whether the manager has an active encryption key.
func (m *Manager) Enabled() bool {
	return m != nil && len(m.key) == 32
}

// Encrypt encrypts plaintext with AES-256-GCM and returns "enc:v1:<base64>".
// Always uses the current key. Returns plaintext unchanged when not enabled.
func (m *Manager) Encrypt(plaintext string) (string, error) {
	if !m.Enabled() {
		return plaintext, nil
	}

	blob, err := m.gcmSeal(m.key, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return encPrefix + base64.StdEncoding.EncodeToString(blob), nil
}

// Decrypt decrypts an "enc:v1:<base64>" value.
// Tries the current key first, then each previous key in order — allowing
// seamless key rotation without re-encrypting existing values immediately.
// Values without the prefix are returned as-is (plaintext passthrough).
func (m *Manager) Decrypt(s string) (string, error) {
	if !strings.HasPrefix(s, encPrefix) {
		return s, nil // plaintext passthrough
	}

	if !m.Enabled() {
		return "", fmt.Errorf("crypto: value is encrypted but %s is not set", EnvKey)
	}

	blob, err := base64.StdEncoding.DecodeString(s[len(encPrefix):])
	if err != nil {
		return "", fmt.Errorf("crypto: decode encrypted value: %w", err)
	}

	// Try current key first.
	if pt, err := m.gcmOpen(m.key, blob); err == nil {
		return string(pt), nil
	}

	// Fall back through previous keys in order.
	for _, pk := range m.previousKeys {
		if pt, err := m.gcmOpen(pk, blob); err == nil {
			return string(pt), nil
		}
	}

	return "", fmt.Errorf("crypto: decrypt failed with current key and %d previous key(s)", len(m.previousKeys))
}

// gcmSeal encrypts plaintext with the given key using AES-256-GCM.
// Returns nonce||ciphertext as a single blob.
func (m *Manager) gcmSeal(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// gcmOpen decrypts a nonce||ciphertext blob with the given key using AES-256-GCM.
func (m *Manager) gcmOpen(key, blob []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(blob) < nonceSize {
		return nil, fmt.Errorf("blob too short")
	}
	return gcm.Open(nil, blob[:nonceSize], blob[nonceSize:], nil)
}
