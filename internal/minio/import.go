package minio

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"

	"github.com/mallardduck/dirio/internal/config/data"
)

// ImportResult contains the results of a MinIO import operation
type ImportResult struct {
	Users          map[string]*User
	Buckets        map[string]*BucketMetadata
	Policies       map[string]*Policy
	ObjectMetadata map[string]map[string]*ObjectMetadata // bucket -> object key -> metadata
	DataConfig     *data.ConfigData                      // Data directory configuration
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
	nodeUID, err := ValidateFormat(minioFS)
	if err != nil {
		return nil, fmt.Errorf("format validation failed: %w", err)
	}

	result := &ImportResult{
		Users:          make(map[string]*User),
		Buckets:        make(map[string]*BucketMetadata),
		Policies:       make(map[string]*Policy),
		ObjectMetadata: make(map[string]map[string]*ObjectMetadata),
	}

	// Import configuration
	dataConfig, err := ImportConfig(minioFS)
	if err != nil {
		return nil, fmt.Errorf("failed to import config: %w", err)
	}
	dataConfig.InstanceID = nodeUID
	result.DataConfig = dataConfig

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

	// Expand group memberships into user policy lists
	if err := importGroupMemberships(minioFS, result.Users); err != nil {
		return nil, fmt.Errorf("failed to import group memberships: %w", err)
	}

	// Import buckets
	if err := importBuckets(minioFS, result.Buckets); err != nil {
		return nil, fmt.Errorf("failed to import buckets: %w", err)
	}

	// Import per-object metadata
	if err := importObjectMetadata(minioFS, result.ObjectMetadata); err != nil {
		return nil, fmt.Errorf("failed to import object metadata: %w", err)
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

		// Try modern format first
		var identity UserIdentity
		var user *User
		if err := json.Unmarshal(data, &identity); err == nil && identity.Credentials.SecretKey != "" {
			// Modern format
			user = &User{
				AccessKey: identity.Credentials.AccessKey,
				SecretKey: identity.Credentials.SecretKey,
				Status:    identity.Credentials.Status,
				UpdatedAt: identity.UpdatedAt,
			}
			fmt.Printf("Found modern MinIO user: %s\n", username)
		} else {
			// Try legacy MinIO 2019 format
			var legacyIdentity LegacyUserIdentity
			if err := json.Unmarshal(data, &legacyIdentity); err != nil {
				fmt.Printf("Warning: failed to parse identity for user %s: %v\n", username, err)
				continue
			}
			// In 2019, username is the accessKey
			user = &User{
				AccessKey: username,
				SecretKey: legacyIdentity.SecretKey,
				Status:    legacyIdentity.Status,
				UpdatedAt: time.Now(), // 2019 format doesn't have UpdatedAt
			}
			fmt.Printf("Found MinIO 2019 user: %s (legacy format)\n", username)
		}

		// Also try reading policy.json from the user directory (MinIO 2019 format).
		// In 2019, per-user policy assignments were stored at
		// config/iam/users/<username>/policy.json as a bare JSON string
		// (e.g. "alpha-rw") rather than in policydb/users/.
		policyPath := filepath.Join(usersDir, username, "policy.json")
		if policyData, err := util.ReadFile(minioFS, policyPath); err == nil {
			var userPolicy string
			if err := json.Unmarshal(policyData, &userPolicy); err == nil && userPolicy != "" {
				user.AttachedPolicy = []string{userPolicy}
				fmt.Printf("Attached policy to user %s from user dir: %s\n", username, userPolicy)
			}
		}

		users[username] = user
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

		// Try modern format first (wrapped in MinIO metadata)
		var policyFile PolicyFile
		var policy *Policy
		if err := json.Unmarshal(data, &policyFile); err == nil && policyFile.Policy != nil {
			// Modern format - extract the Policy field
			policyJSON, err := json.Marshal(policyFile.Policy)
			if err != nil {
				fmt.Printf("Warning: failed to marshal policy %s: %v\n", policyName, err)
				continue
			}
			policy = &Policy{
				Name:       policyName,
				PolicyJSON: string(policyJSON),
				CreateDate: policyFile.CreateDate,
				UpdateDate: policyFile.UpdateDate,
			}
			fmt.Printf("Found MinIO policy: %s (modern format)\n", policyName)
		} else {
			// Legacy MinIO 2019 format - policy.json IS the IAM policy document
			// Just validate it's valid JSON by unmarshaling
			var iamPolicy map[string]any
			if err := json.Unmarshal(data, &iamPolicy); err != nil {
				fmt.Printf("Warning: failed to parse policy %s: %v\n", policyName, err)
				continue
			}
			policy = &Policy{
				Name:       policyName,
				PolicyJSON: string(data),
				CreateDate: time.Now(), // 2019 format doesn't have CreateDate
				UpdateDate: time.Now(),
			}
			fmt.Printf("Found MinIO 2019 policy: %s (legacy format)\n", policyName)
		}

		policies[policyName] = policy
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
			user.AttachedPolicy = []string(mapping.Policy)
			if len(user.AttachedPolicy) > 0 {
				fmt.Printf("Attached %d policy(ies) to user %s: %s\n", len(user.AttachedPolicy), username, mapping.Policy.String())
			}
		}
	}

	return nil
}

// importBuckets reads MinIO bucket metadata
func importBuckets(minioFS billy.Filesystem, buckets map[string]*BucketMetadata) error {
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

		// Skip special MinIO system directories that aren't actual buckets
		// These directories may exist in .minio.sys/buckets/ but aren't buckets
		if isSpecialMinIODirectory(bucketName) {
			fmt.Printf("Skipping special MinIO directory: %s\n", bucketName)
			continue
		}

		// Helper function to read files from MinIO filesystem
		readFileFunc := func(path string) ([]byte, error) {
			return util.ReadFile(minioFS, path)
		}

		// Always try to read legacy config files (policy.json, lifecycle.xml, etc.)
		// This supports both pure 2019 data and hybrid configurations
		minioMeta, err := readLegacyBucketMetadata(bucketName, readFileFunc)
		if err != nil {
			fmt.Printf("Warning: failed to read legacy config for bucket %s: %v\n", bucketName, err)
			// Create basic bucket info as fallback
			buckets[bucketName] = &BucketMetadata{
				Name:    bucketName,
				Created: time.Now(),
			}
			fmt.Printf("Found MinIO bucket (no metadata): %s\n", bucketName)
			continue
		}

		// Check if .metadata.bin also exists (hybrid or modern MinIO 2022+)
		// Parse it and merge with legacy config (legacy takes precedence)
		metadataPath := filepath.Join(bucketsMetaDir, bucketName, ".metadata.bin")
		if _, statErr := minioFS.Stat(metadataPath); statErr == nil {
			// Read and parse .metadata.bin
			binData, readErr := util.ReadFile(minioFS, metadataPath)
			if readErr != nil {
				fmt.Printf("Warning: failed to read .metadata.bin for %s: %v\n", bucketName, readErr)
				fmt.Printf("Found MinIO bucket: %s (with .metadata.bin but failed to read)\n", bucketName)
			} else {
				binMeta, parseErr := parseBucketMetadataBin(binData)
				if parseErr != nil {
					fmt.Printf("Warning: failed to parse .metadata.bin for %s: %v\n", bucketName, parseErr)
					fmt.Printf("Found MinIO bucket: %s (with .metadata.bin but failed to parse)\n", bucketName)
				} else {
					// Merge binary metadata with legacy config
					// Legacy config takes precedence, binary fills in gaps
					mergeBucketMetadata(minioMeta, binMeta)
					fmt.Printf("Found MinIO bucket: %s (merged .metadata.bin + legacy config)\n", bucketName)
				}
			}
		} else {
			fmt.Printf("Found MinIO bucket: %s (legacy config only)\n", bucketName)
		}

		buckets[bucketName] = minioMeta
	}

	return nil
}

// importObjectMetadata reads per-object metadata from fs.json files
func importObjectMetadata(minioFS billy.Filesystem, objectMetadata map[string]map[string]*ObjectMetadata) error {
	bucketsDir := "buckets"

	if _, err := minioFS.Stat(bucketsDir); err != nil {
		if isNotExist(err) {
			return nil // No buckets to import
		}
		return err
	}

	bucketEntries, err := minioFS.ReadDir(bucketsDir)
	if err != nil {
		return err
	}

	for _, bucketEntry := range bucketEntries {
		if !bucketEntry.IsDir() {
			continue
		}
		if bucketEntry.Name() == ".minio.sys" {
			continue
		}

		bucketName := bucketEntry.Name()

		// Skip special MinIO system directories that aren't actual buckets
		if isSpecialMinIODirectory(bucketName) {
			fmt.Printf("Skipping special MinIO directory: %s\n", bucketName)
			continue
		}

		bucketPath := filepath.Join(bucketsDir, bucketName)

		// Initialize map for this bucket's object metadata
		objectMetadata[bucketName] = make(map[string]*ObjectMetadata)

		// Walk the bucket's metadata directory to find all fs.json files
		if err := walkBucketMetadata(minioFS, bucketPath, bucketName, objectMetadata[bucketName]); err != nil {
			fmt.Printf("Warning: failed to walk bucket %s metadata: %v\n", bucketName, err)
			continue
		}
	}

	return nil
}

// walkBucketMetadata recursively walks a bucket's metadata directory and reads fs.json files
func walkBucketMetadata(minioFS billy.Filesystem, currentPath, bucketName string, metadata map[string]*ObjectMetadata) error {
	entries, err := minioFS.ReadDir(currentPath)
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(currentPath, entry.Name())

		if entry.IsDir() {
			// Recursively walk subdirectories
			if err := walkBucketMetadata(minioFS, entryPath, bucketName, metadata); err != nil {
				return err
			}
		} else if entry.Name() == "fs.json" {
			// Found an fs.json file - parse it
			objectKey := filepath.Dir(strings.TrimPrefix(entryPath, filepath.Join("buckets", bucketName)+string(filepath.Separator)))

			// Read and parse fs.json
			data, err := util.ReadFile(minioFS, entryPath)
			if err != nil {
				fmt.Printf("Warning: failed to read %s: %v\n", entryPath, err)
				continue
			}

			var objMeta ObjectMetadata
			if err := json.Unmarshal(data, &objMeta); err != nil {
				fmt.Printf("Warning: failed to parse %s: %v\n", entryPath, err)
				continue
			}

			// Store the metadata with the object key
			metadata[objectKey] = &objMeta
			fmt.Printf("Imported metadata for %s/%s\n", bucketName, objectKey)
		}
	}

	return nil
}

// importGroupMemberships reads MinIO group definitions and expands each group's
// policy into the AttachedPolicy list of every member user.
//
// MinIO 2019 only supports one direct policy per user; groups are the native
// mechanism for granting a user multiple policies. This function flattens that
// indirection so DirIO's user model ends up with the full effective policy set.
//
// Paths read:
//
//	config/iam/groups/<name>/members.json  → {"version":1,"status":"enabled","members":[...]}
//	config/iam/policydb/groups/<name>.json  → {"version":1,"policy":"<name>"}
func importGroupMemberships(minioFS billy.Filesystem, users map[string]*User) error {
	groupsDir := filepath.Join("config", "iam", "groups")

	if _, err := minioFS.Stat(groupsDir); err != nil {
		if isNotExist(err) {
			return nil // No groups defined
		}
		return err
	}

	entries, err := minioFS.ReadDir(groupsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		groupUserDirName := entry.Name()

		// Read group identity (members list)
		identityPath := filepath.Join(groupsDir, groupUserDirName, "members.json")
		identityData, err := util.ReadFile(minioFS, identityPath)
		if err != nil {
			fmt.Printf("Warning: failed to read group identity for %s: %v\n", groupUserDirName, err)
			continue
		}

		var identity GroupIdentity
		if err := json.Unmarshal(identityData, &identity); err != nil {
			fmt.Printf("Warning: failed to parse group identity for %s: %v\n", groupUserDirName, err)
			continue
		}

		if identity.Status != "enabled" || len(identity.Members) == 0 {
			continue
		}

		// Read group's policy assignment from policydb
		policyPath := filepath.Join("config", "iam", "policydb", "groups", groupUserDirName+".json")
		policyData, err := util.ReadFile(minioFS, policyPath)
		if err != nil {
			fmt.Printf("Warning: failed to read policy mapping for group %s: %v\n", groupUserDirName, err)
			continue
		}

		var mapping GroupPolicyMapping
		if err := json.Unmarshal(policyData, &mapping); err != nil {
			fmt.Printf("Warning: failed to parse policy mapping for group %s: %v\n", groupUserDirName, err)
			continue
		}

		groupPolicies := []string(mapping.Policy)
		if len(groupPolicies) == 0 {
			continue
		}

		// Expand group policies to each member user
		for _, member := range identity.Members {
			user, exists := users[member]
			if !exists {
				continue
			}
			for _, gp := range groupPolicies {
				alreadyHas := slices.Contains(user.AttachedPolicy, gp)
				if !alreadyHas {
					user.AttachedPolicy = append(user.AttachedPolicy, gp)
				}
			}
			fmt.Printf("Expanded group '%s' → user %s gains policy(ies): %v\n", groupUserDirName, member, groupPolicies)
		}
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
		strings.Contains(errMsg, "does not exist") ||
		strings.Contains(errMsg, "cannot find the file") ||
		strings.Contains(errMsg, "cannot find the path")
}

// isSpecialMinIODirectory checks if a directory name is a special MinIO system directory
// that should not be treated as a bucket.
// These directories can exist in .minio.sys/buckets/ but are not actual buckets.
func isSpecialMinIODirectory(name string) bool {
	specialDirs := []string{
		"replication", // Contains replication stats (.stats files), not bucket data
	}
	return slices.Contains(specialDirs, name)
}
