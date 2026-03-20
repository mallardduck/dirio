package crypto

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mallardduck/dirio/internal/consts"
)

// testKeyB64 is a 32-byte key in the "base64:<encoded>" format used by DirIO.
const testKeyB64 = "base64:AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="

// altKeyB64 is a different 32-byte key, used for rotation tests.
const altKeyB64 = "base64:ISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0+P0A="

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	key, err := decodeKey(testKeyB64)
	if err != nil {
		t.Fatalf("decodeKey: %v", err)
	}
	return &Manager{key: key}
}

// --- Encrypt / Decrypt core ---

func TestEncryptDecryptRoundtrip(t *testing.T) {
	m := newTestManager(t)

	plaintext := "super-secret-password"
	ciphertext, err := m.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(ciphertext, encPrefix) {
		t.Fatalf("expected enc:v1: prefix, got %q", ciphertext)
	}

	got, err := m.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plaintext {
		t.Errorf("got %q, want %q", got, plaintext)
	}
}

func TestEncryptProducesUniqueCiphertexts(t *testing.T) {
	m := newTestManager(t)

	ct1, _ := m.Encrypt("same-input")
	ct2, _ := m.Encrypt("same-input")
	if ct1 == ct2 {
		t.Error("expected different ciphertexts for same plaintext (random nonce)")
	}
}

func TestDecryptPlaintextPassthrough(t *testing.T) {
	m := newTestManager(t)
	got, err := m.Decrypt("not-encrypted")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "not-encrypted" {
		t.Errorf("got %q, want %q", got, "not-encrypted")
	}
}

// --- No-op manager (no key configured) ---

func TestNoOpManagerEncrypt(t *testing.T) {
	m := &Manager{}
	got, err := m.Encrypt("hello")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if got != "hello" {
		t.Errorf("no-op manager should return plaintext unchanged, got %q", got)
	}
}

func TestNoOpManagerDecryptPlaintext(t *testing.T) {
	m := &Manager{}
	got, err := m.Decrypt("hello")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestNoOpManagerDecryptEncryptedErrors(t *testing.T) {
	ct, _ := newTestManager(t).Encrypt("secret")
	_, err := (&Manager{}).Decrypt(ct)
	if err == nil {
		t.Error("expected error when decrypting with no key configured")
	}
}

// --- Key rotation: previous key fallback ---

func TestDecryptFallsBackToPreviousKey(t *testing.T) {
	oldKey, _ := decodeKey(testKeyB64)
	newKey, _ := decodeKey(altKeyB64)

	// Value encrypted with old key.
	old := &Manager{key: oldKey}
	ct, err := old.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Manager rotated to new key, old key in fallback list.
	rotated := &Manager{key: newKey, previousKeys: [][]byte{oldKey}}
	got, err := rotated.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt after rotation: %v", err)
	}
	if got != "secret" {
		t.Errorf("got %q, want %q", got, "secret")
	}
}

func TestDecryptCurrentKeyUsedBeforePrevious(t *testing.T) {
	currentKey, _ := decodeKey(testKeyB64)
	oldKey, _ := decodeKey(altKeyB64)

	// Encrypt with the current key.
	m := &Manager{key: currentKey, previousKeys: [][]byte{oldKey}}
	ct, _ := m.Encrypt("value")

	// Should decrypt fine — uses current key, not fallback.
	got, err := m.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "value" {
		t.Errorf("got %q, want %q", got, "value")
	}
}

func TestDecryptFailsWhenNoKeyMatches(t *testing.T) {
	oldKey, _ := decodeKey(testKeyB64)
	unrelatedKey, _ := decodeKey(altKeyB64)

	ct, _ := (&Manager{key: oldKey}).Encrypt("secret")

	// Manager with neither the right current nor previous key.
	m := &Manager{key: unrelatedKey}
	_, err := m.Decrypt(ct)
	if err == nil {
		t.Error("expected error when no key matches")
	}
}

// --- decodeKey ---

func TestDecodeKeyBase64(t *testing.T) {
	key, err := decodeKey(testKeyB64)
	if err != nil {
		t.Fatalf("decodeKey: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(key))
	}
}

func TestDecodeKeyRejectsUnprefixed(t *testing.T) {
	_, err := decodeKey("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=")
	if err == nil {
		t.Error("expected error for key without base64: prefix")
	}
}

func TestDecodeKeyRejectsWrongSize(t *testing.T) {
	// base64 of 16 bytes (too short).
	_, err := decodeKey("base64:AAAAAAAAAAAAAAAAAAAAAA==")
	if err == nil {
		t.Error("expected error for key that is not 32 bytes")
	}
}

// --- IsEncrypted ---

func TestIsEncrypted(t *testing.T) {
	ct, _ := newTestManager(t).Encrypt("x")
	if !IsEncrypted(ct) {
		t.Error("expected IsEncrypted=true for ciphertext")
	}
	if IsEncrypted("plaintext") {
		t.Error("expected IsEncrypted=false for plaintext")
	}
}

// --- Keyring / Init ---

func TestInitEnvVarTakesPriority(t *testing.T) {
	t.Setenv(EnvKey, testKeyB64)
	dir := t.TempDir()
	t.Cleanup(func() { defaultManager = &Manager{} })

	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !Enabled() {
		t.Error("expected encryption to be enabled after Init with env var")
	}

	// Keyring file should NOT have been created since env var was used.
	if _, err := os.Stat(filepath.Join(dir, consts.DirIOMetadataDir, "keyring")); err == nil {
		t.Error("keyring file should not be created when env var is set")
	}
}

func TestInitCreatesKeyringOnFirstRun(t *testing.T) {
	t.Setenv(EnvKey, "")
	dir := t.TempDir()
	t.Cleanup(func() { defaultManager = &Manager{} })

	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !Enabled() {
		t.Error("expected encryption to be enabled after keyring auto-generation")
	}

	keyringPath := filepath.Join(dir, consts.DirIOMetadataDir, "keyring")
	data, err := os.ReadFile(keyringPath)
	if err != nil {
		t.Fatalf("keyring file not created: %v", err)
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, keyPrefix) {
		t.Errorf("expected keyring to start with %q, got %q", keyPrefix, line)
	}

	// Permission check is Unix-only; Windows uses ACLs and ignores mode bits.
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(keyringPath)
		if info.Mode().Perm() != 0o600 {
			t.Errorf("expected keyring permissions 0o600, got %v", info.Mode().Perm())
		}
	}
}

func TestInitLoadsExistingKeyring(t *testing.T) {
	t.Setenv(EnvKey, "")
	dir := t.TempDir()
	t.Cleanup(func() { defaultManager = &Manager{} })

	// Write a known key to the keyring.
	keyringDir := filepath.Join(dir, consts.DirIOMetadataDir)
	if err := os.MkdirAll(keyringDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keyringDir, "keyring"), []byte(testKeyB64+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Verify the loaded key matches testKeyB64 by cross-decrypting.
	ct, err := Encrypt("hello")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := newTestManager(t).Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt with reference manager: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestInitLoadsKeyringWithPreviousKeys(t *testing.T) {
	t.Setenv(EnvKey, "")
	dir := t.TempDir()
	t.Cleanup(func() { defaultManager = &Manager{} })

	// Keyring with current + one previous key.
	keyringDir := filepath.Join(dir, consts.DirIOMetadataDir)
	if err := os.MkdirAll(keyringDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := altKeyB64 + "\n" + testKeyB64 + "\n"
	if err := os.WriteFile(filepath.Join(keyringDir, "keyring"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Value encrypted with the old key (testKeyB64) should still decrypt.
	oldManager := newTestManager(t) // uses testKeyB64
	ct, _ := oldManager.Encrypt("legacy-secret")

	got, err := Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt legacy value: %v", err)
	}
	if got != "legacy-secret" {
		t.Errorf("got %q, want %q", got, "legacy-secret")
	}
}

func TestInitPreviousKeysFromEnv(t *testing.T) {
	t.Setenv(EnvKey, altKeyB64)
	t.Setenv(EnvPreviousKeys, testKeyB64)
	dir := t.TempDir()
	t.Cleanup(func() { defaultManager = &Manager{} })

	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Value encrypted with old key (testKeyB64) should decrypt via fallback.
	ct, _ := newTestManager(t).Encrypt("old-value")
	got, err := Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "old-value" {
		t.Errorf("got %q, want %q", got, "old-value")
	}
}
