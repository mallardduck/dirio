package minio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ImportResult contains the results of a MinIO import operation
type ImportResult struct {
	Users    map[string]*User
	Buckets  map[string]*Bucket
	Policies map[string]*Policy
}

// Import reads MinIO data from the specified directory and returns parsed data.
// This is a read-only operation - it does not modify DirIO metadata.
//
// The import process:
// 1. Validates format.json (must be single-node FS mode)
// 2. Reads users from .minio.sys/config/iam/users/
// 3. Reads bucket metadata from .minio.sys/buckets/
func Import(dataDir string) (*ImportResult, error) {
	// First check: validate format
	if err := ValidateFormat(dataDir); err != nil {
		return nil, fmt.Errorf("format validation failed: %w", err)
	}

	result := &ImportResult{
		Users:    make(map[string]*User),
		Buckets:  make(map[string]*Bucket),
		Policies: make(map[string]*Policy),
	}

	// Import policies first
	if err := importPolicies(dataDir, result.Policies); err != nil {
		return nil, fmt.Errorf("failed to import policies: %w", err)
	}

	// Import users
	if err := importUsers(dataDir, result.Users); err != nil {
		return nil, fmt.Errorf("failed to import users: %w", err)
	}

	// Attach policies to users
	if err := importUserPolicyMappings(dataDir, result.Users); err != nil {
		return nil, fmt.Errorf("failed to import user-policy mappings: %w", err)
	}

	// Import buckets
	if err := importBuckets(dataDir, result.Buckets); err != nil {
		return nil, fmt.Errorf("failed to import buckets: %w", err)
	}

	return result, nil
}

// importUsers reads MinIO IAM users
func importUsers(dataDir string, users map[string]*User) error {
	usersDir := filepath.Join(dataDir, ".minio.sys", "config", "iam", "users")

	if _, err := os.Stat(usersDir); os.IsNotExist(err) {
		return nil // No users to import
	}

	entries, err := os.ReadDir(usersDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		username := entry.Name()
		identityPath := filepath.Join(usersDir, username, "identity.json")

		// Read identity.json
		data, err := os.ReadFile(identityPath)
		if err != nil {
			fmt.Printf("Warning: failed to read identity for user %s: %v\n", username, err)
			continue
		}

		var identity UserIdentity
		if err := json.Unmarshal(data, &identity); err != nil {
			fmt.Printf("Warning: failed to parse identity for user %s: %v\n", username, err)
			continue
		}

		// Import user
		users[username] = &User{
			AccessKey: identity.Credentials.AccessKey,
			SecretKey: identity.Credentials.SecretKey,
			Status:    identity.Credentials.Status,
			UpdatedAt: identity.UpdatedAt,
		}

		fmt.Printf("Found MinIO user: %s\n", username)
	}

	return nil
}

// importPolicies reads MinIO IAM policies
func importPolicies(dataDir string, policies map[string]*Policy) error {
	policiesDir := filepath.Join(dataDir, ".minio.sys", "config", "iam", "policies")

	if _, err := os.Stat(policiesDir); os.IsNotExist(err) {
		return nil // No policies to import
	}

	entries, err := os.ReadDir(policiesDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		policyName := entry.Name()
		policyPath := filepath.Join(policiesDir, policyName, "policy.json")

		// Read policy.json
		data, err := os.ReadFile(policyPath)
		if err != nil {
			fmt.Printf("Warning: failed to read policy %s: %v\n", policyName, err)
			continue
		}

		var policyFile PolicyFile
		if err := json.Unmarshal(data, &policyFile); err != nil {
			fmt.Printf("Warning: failed to parse policy %s: %v\n", policyName, err)
			continue
		}

		// Convert policy document back to JSON string
		policyJSON, err := json.Marshal(policyFile.Policy)
		if err != nil {
			fmt.Printf("Warning: failed to marshal policy %s: %v\n", policyName, err)
			continue
		}

		policies[policyName] = &Policy{
			Name:       policyName,
			PolicyJSON: string(policyJSON),
			CreateDate: policyFile.CreateDate,
			UpdateDate: policyFile.UpdateDate,
		}

		fmt.Printf("Found MinIO policy: %s\n", policyName)
	}

	return nil
}

// importUserPolicyMappings reads user-policy mappings and attaches them to users
func importUserPolicyMappings(dataDir string, users map[string]*User) error {
	policydbDir := filepath.Join(dataDir, ".minio.sys", "config", "iam", "policydb", "users")

	if _, err := os.Stat(policydbDir); os.IsNotExist(err) {
		return nil // No user-policy mappings
	}

	entries, err := os.ReadDir(policydbDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if filepath.Ext(filename) != ".json" {
			continue
		}

		username := filename[:len(filename)-5] // Remove .json extension
		mappingPath := filepath.Join(policydbDir, filename)

		// Read user policy mapping
		data, err := os.ReadFile(mappingPath)
		if err != nil {
			fmt.Printf("Warning: failed to read policy mapping for %s: %v\n", username, err)
			continue
		}

		var mapping UserPolicyMapping
		if err := json.Unmarshal(data, &mapping); err != nil {
			fmt.Printf("Warning: failed to parse policy mapping for %s: %v\n", username, err)
			continue
		}

		// Attach policy to user if user exists
		if user, exists := users[username]; exists {
			user.AttachedPolicy = mapping.Policy
			fmt.Printf("Attached policy %s to user %s\n", mapping.Policy, username)
		}
	}

	return nil
}

// importBuckets reads MinIO bucket metadata
func importBuckets(dataDir string, buckets map[string]*Bucket) error {
	bucketsMetaDir := filepath.Join(dataDir, ".minio.sys", "buckets")

	if _, err := os.Stat(bucketsMetaDir); os.IsNotExist(err) {
		return nil // No buckets to import
	}

	entries, err := os.ReadDir(bucketsMetaDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == ".minio.sys" {
			continue
		}

		bucketName := entry.Name()
		metadataPath := filepath.Join(bucketsMetaDir, bucketName, ".metadata.bin")

		// Check if metadata file exists
		if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
			// Create basic bucket info
			buckets[bucketName] = &Bucket{
				Name:    bucketName,
				Owner:   "root",
				Created: time.Now(),
			}
			fmt.Printf("Found MinIO bucket (no metadata): %s\n", bucketName)
			continue
		}

		// Read and parse MinIO metadata
		var minioMeta *BucketMetadata
		if minioMeta, err = readLegacyBucketMetadata(bucketName, os.ReadFile); err != nil {
			fmt.Printf("Warning: failed to parse metadata for bucket %s: %v\n", bucketName, err)
			continue
		}

		// Convert to our format
		buckets[bucketName] = &Bucket{
			Name:    minioMeta.Name,
			Owner:   "root", // MinIO doesn't store owner in bucket metadata
			Created: minioMeta.Created,
			Policy:  string(minioMeta.PolicyConfigJSON),
		}

		fmt.Printf("Found MinIO bucket: %s\n", bucketName)
	}

	return nil
}
