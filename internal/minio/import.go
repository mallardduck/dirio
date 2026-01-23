package minio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

// ImportResult contains the results of a MinIO import operation
type ImportResult struct {
	Users   map[string]*User
	Buckets map[string]*Bucket
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
		Users:   make(map[string]*User),
		Buckets: make(map[string]*Bucket),
	}

	// Import users
	if err := importUsers(dataDir, result.Users); err != nil {
		return nil, fmt.Errorf("failed to import users: %w", err)
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
		data, err := os.ReadFile(metadataPath)
		if err != nil {
			fmt.Printf("Warning: failed to read metadata for bucket %s: %v\n", bucketName, err)
			continue
		}

		var minioMeta BucketMetadata
		if err := msgpack.Unmarshal(data, &minioMeta); err != nil {
			fmt.Printf("Warning: failed to parse metadata for bucket %s: %v\n", bucketName, err)
			continue
		}

		// Convert to our format
		buckets[bucketName] = &Bucket{
			Name:    minioMeta.Name,
			Owner:   "root", // MinIO doesn't store owner in bucket metadata
			Created: time.Now(), // TODO: Parse Created timestamp from msgpack
			Policy:  minioMeta.PolicyConfigJSON,
		}

		fmt.Printf("Found MinIO bucket: %s\n", bucketName)
	}

	return nil
}
