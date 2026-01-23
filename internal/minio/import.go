package minio

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
)

// ImportResult contains the results of a MinIO import operation
type ImportResult struct {
	Users    map[string]*User
	Buckets  map[string]*Bucket
	Policies map[string]*Policy
}

// Import reads MinIO data from the specified filesystem and returns parsed data.
// This is a read-only operation - it does not modify DirIO metadata.
//
// The import process:
// 1. Validates format.json (must be single-node FS mode)
// 2. Reads users from config/iam/users/
// 3. Reads bucket metadata from buckets/
func Import(minioFS billy.Filesystem) (*ImportResult, error) {
	// First check: validate format
	if err := ValidateFormat(minioFS); err != nil {
		return nil, fmt.Errorf("format validation failed: %w", err)
	}

	result := &ImportResult{
		Users:    make(map[string]*User),
		Buckets:  make(map[string]*Bucket),
		Policies: make(map[string]*Policy),
	}

	// Import policies first
	if err := importPolicies(minioFS, result.Policies); err != nil {
		return nil, fmt.Errorf("failed to import policies: %w", err)
	}

	// Import users
	if err := importUsers(minioFS, result.Users); err != nil {
		return nil, fmt.Errorf("failed to import users: %w", err)
	}

	// Attach policies to users
	if err := importUserPolicyMappings(minioFS, result.Users); err != nil {
		return nil, fmt.Errorf("failed to import user-policy mappings: %w", err)
	}

	// Import buckets
	if err := importBuckets(minioFS, result.Buckets); err != nil {
		return nil, fmt.Errorf("failed to import buckets: %w", err)
	}

	return result, nil
}

// importUsers reads MinIO IAM users
func importUsers(minioFS billy.Filesystem, users map[string]*User) error {
	usersDir := filepath.Join("config", "iam", "users")

	if _, err := minioFS.Stat(usersDir); err != nil {
		if isNotExist(err) {
			return nil // No users to import
		}
		return err
	}

	entries, err := minioFS.ReadDir(usersDir)
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
		data, err := util.ReadFile(minioFS, identityPath)
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
func importPolicies(minioFS billy.Filesystem, policies map[string]*Policy) error {
	policiesDir := filepath.Join("config", "iam", "policies")

	if _, err := minioFS.Stat(policiesDir); err != nil {
		if isNotExist(err) {
			return nil // No policies to import
		}
		return err
	}

	entries, err := minioFS.ReadDir(policiesDir)
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
		data, err := util.ReadFile(minioFS, policyPath)
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
func importUserPolicyMappings(minioFS billy.Filesystem, users map[string]*User) error {
	policydbDir := filepath.Join("config", "iam", "policydb", "users")

	if _, err := minioFS.Stat(policydbDir); err != nil {
		if isNotExist(err) {
			return nil // No user-policy mappings
		}
		return err
	}

	entries, err := minioFS.ReadDir(policydbDir)
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
		data, err := util.ReadFile(minioFS, mappingPath)
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
func importBuckets(minioFS billy.Filesystem, buckets map[string]*Bucket) error {
	bucketsMetaDir := "buckets"

	if _, err := minioFS.Stat(bucketsMetaDir); err != nil {
		if isNotExist(err) {
			return nil // No buckets to import
		}
		return err
	}

	entries, err := minioFS.ReadDir(bucketsMetaDir)
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
		if _, err := minioFS.Stat(metadataPath); err != nil {
			if isNotExist(err) {
				// Create basic bucket info
				buckets[bucketName] = &Bucket{
					Name:    bucketName,
					Owner:   "root",
					Created: time.Now(),
				}
				fmt.Printf("Found MinIO bucket (no metadata): %s\n", bucketName)
				continue
			}
			return err
		}

		// Read and parse MinIO metadata
		readFileFunc := func(path string) ([]byte, error) {
			return util.ReadFile(minioFS, path)
		}

		var minioMeta *BucketMetadata
		if minioMeta, err = readLegacyBucketMetadata(bucketName, readFileFunc); err != nil {
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

// isNotExist checks if an error is a "not exist" error
func isNotExist(err error) bool {
	if err == nil {
		return false
	}
	if err == fs.ErrNotExist {
		return true
	}
	// Check for common "not exist" error messages from different filesystem implementations
	errMsg := err.Error()
	return errMsg == "file does not exist" ||
		strings.Contains(errMsg, "no such file or directory") ||
		strings.Contains(errMsg, "does not exist")
}
