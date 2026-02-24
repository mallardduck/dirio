package minio

import (
	_ "embed"
	"encoding/json"
	"testing"
	"time"
)

//go:embed testdata/beta.metadata.bin
var betaMetadataBin []byte

//go:embed testdata/gamma.metadata.bin
var gammaMetadataBin []byte

func TestParseBucketMetadataBin_Beta(t *testing.T) {
	meta, err := parseBucketMetadataBin(betaMetadataBin)
	if err != nil {
		t.Fatalf("Failed to parse beta metadata: %v", err)
	}

	// Verify basic fields
	if meta.Name != "beta" {
		t.Errorf("Expected Name='beta', got '%s'", meta.Name)
	}

	if meta.LockEnabled {
		t.Errorf("Expected LockEnabled=false, got true")
	}

	// Verify PolicyConfigJSON
	if len(meta.PolicyConfigJSON) == 0 {
		t.Fatal("Expected PolicyConfigJSON to be present, got empty")
	}

	// Parse policy JSON to verify it's valid
	var policy map[string]any
	if err := json.Unmarshal(meta.PolicyConfigJSON, &policy); err != nil {
		t.Errorf("PolicyConfigJSON is not valid JSON: %v", err)
	}

	// Check policy structure
	if version, ok := policy["Version"].(string); !ok || version != "2012-10-17" {
		t.Errorf("Expected policy Version='2012-10-17', got %v", policy["Version"])
	}

	statements, ok := policy["Statement"].([]any)
	if !ok {
		t.Fatal("Expected policy to have Statement array")
	}

	if len(statements) != 2 {
		t.Errorf("Expected 2 policy statements, got %d", len(statements))
	}

	// Verify PolicyConfigUpdatedAt timestamp
	if meta.PolicyConfigUpdatedAt.IsZero() {
		t.Error("Expected PolicyConfigUpdatedAt to be set, got zero time")
	}

	// Verify other timestamps are zero (no configs)
	if !meta.ObjectLockConfigUpdatedAt.IsZero() {
		t.Error("Expected ObjectLockConfigUpdatedAt to be zero, got non-zero")
	}

	// Verify empty configs
	if len(meta.NotificationConfigXML) != 0 {
		t.Errorf("Expected NotificationConfigXML to be empty, got %d bytes", len(meta.NotificationConfigXML))
	}

	if len(meta.LifecycleConfigXML) != 0 {
		t.Errorf("Expected LifecycleConfigXML to be empty, got %d bytes", len(meta.LifecycleConfigXML))
	}
}

func TestParseBucketMetadataBin_Gamma(t *testing.T) {
	meta, err := parseBucketMetadataBin(gammaMetadataBin)
	if err != nil {
		t.Fatalf("Failed to parse gamma metadata: %v", err)
	}

	// Verify basic fields
	if meta.Name != "gamma" {
		t.Errorf("Expected Name='gamma', got '%s'", meta.Name)
	}

	// Verify PolicyConfigJSON
	if len(meta.PolicyConfigJSON) == 0 {
		t.Fatal("Expected PolicyConfigJSON to be present, got empty")
	}

	// Parse policy JSON
	var policy map[string]any
	if err := json.Unmarshal(meta.PolicyConfigJSON, &policy); err != nil {
		t.Errorf("PolicyConfigJSON is not valid JSON: %v", err)
	}

	// Verify timestamp
	if meta.PolicyConfigUpdatedAt.IsZero() {
		t.Error("Expected PolicyConfigUpdatedAt to be set, got zero time")
	}
}

func TestParseBucketMetadataBin_TooSmall(t *testing.T) {
	// Test with data that's too small
	data := []byte{0x01, 0x00, 0x01}
	_, err := parseBucketMetadataBin(data)
	if err == nil {
		t.Error("Expected error for data too small, got nil")
	}
}

func TestParseBucketMetadataBin_InvalidFormat(t *testing.T) {
	// Test with unsupported format version
	data := make([]byte, 100)
	data[0] = 0x99 // Invalid format
	data[1] = 0x00
	data[2] = 0x01 // Valid version
	data[3] = 0x00

	_, err := parseBucketMetadataBin(data)
	if err == nil {
		t.Error("Expected error for invalid format, got nil")
	}
	if err != nil && err.Error() != "unsupported metadata format 153 (expected 1)" {
		t.Errorf("Expected unsupported format error, got: %v", err)
	}
}

func TestParseBucketMetadataBin_PolicySize(t *testing.T) {
	// Verify policy sizes match what we observed in manual testing
	betaMeta, err := parseBucketMetadataBin(betaMetadataBin)
	if err != nil {
		t.Fatalf("Failed to parse beta: %v", err)
	}

	// Beta policy is 272 bytes
	if len(betaMeta.PolicyConfigJSON) != 272 {
		t.Errorf("Expected beta policy to be 272 bytes, got %d", len(betaMeta.PolicyConfigJSON))
	}

	gammaMeta, err := parseBucketMetadataBin(gammaMetadataBin)
	if err != nil {
		t.Fatalf("Failed to parse gamma: %v", err)
	}

	// Gamma policy is 274 bytes
	if len(gammaMeta.PolicyConfigJSON) != 274 {
		t.Errorf("Expected gamma policy to be 274 bytes, got %d", len(gammaMeta.PolicyConfigJSON))
	}
}

func TestParseBucketMetadataBin_TimestampComparison(t *testing.T) {
	// Both buckets should have the same PolicyConfigUpdatedAt
	betaMeta, _ := parseBucketMetadataBin(betaMetadataBin)
	gammaMeta, _ := parseBucketMetadataBin(gammaMetadataBin)

	betaTime := betaMeta.PolicyConfigUpdatedAt
	gammaTime := gammaMeta.PolicyConfigUpdatedAt

	// They should be within seconds of each other (created at same time)
	diff := betaTime.Sub(gammaTime)
	if diff < 0 {
		diff = -diff
	}

	if diff > time.Minute {
		t.Errorf("Expected beta and gamma timestamps to be close, got diff of %v", diff)
	}
}
