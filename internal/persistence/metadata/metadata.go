package metadata

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
	"github.com/google/uuid"

	contextInt "github.com/mallardduck/dirio/internal/context"

	"github.com/mallardduck/dirio/internal/persistence/path"

	"github.com/mallardduck/dirio/internal/jsonutil"
	"github.com/mallardduck/dirio/pkg/iam"
)

// Metadata format versions
const (
	BucketMetadataVersion = "1.0.0"
	ObjectMetadataVersion = "1.0.0"
)

// Domain errors
var (
	ErrUserNotFound   = errors.New("user not found")
	ErrPolicyNotFound = errors.New("policy not found")
)

// Manager handles metadata storage and retrieval
type Manager struct {
	rootFS     billy.Filesystem
	metadataFS billy.Filesystem
}

// Type aliases for backward compatibility
type User = iam.User
type Policy = iam.Policy
type PolicyDocument = iam.PolicyDocument
type PolicyStatement = iam.Statement

// BucketMetadata represents bucket configuration
type BucketMetadata struct {
	Version      string          `json:"version"`                // DirIO metadata version
	Name         string          `json:"name"`                   // Bucket name
	Owner        *uuid.UUID      `json:"owner,omitempty"`        // User UUID; nil = admin-only (no specific owner)
	Created      time.Time       `json:"created"`                // Creation timestamp
	BucketPolicy *PolicyDocument `json:"bucketPolicy,omitempty"` // S3 bucket policy (resource-based)

	// Extended MinIO metadata (imported but may not be actively used yet)
	NotificationConfigXML       string    `json:"notificationConfig,omitempty"`
	LifecycleConfigXML          string    `json:"lifecycleConfig,omitempty"`
	ObjectLockConfigXML         string    `json:"objectLockConfig,omitempty"`
	VersioningConfigXML         string    `json:"versioningConfig,omitempty"`
	EncryptionConfigXML         string    `json:"encryptionConfig,omitempty"`
	TaggingConfigXML            string    `json:"taggingConfig,omitempty"`
	QuotaConfigJSON             string    `json:"quotaConfig,omitempty"`
	ReplicationConfigXML        string    `json:"replicationConfig,omitempty"`
	BucketTargetsConfigJSON     string    `json:"bucketTargetsConfig,omitempty"`
	BucketTargetsConfigMetaJSON string    `json:"bucketTargetsConfigMeta,omitempty"`
	PolicyConfigUpdatedAt       time.Time `json:"policyConfigUpdatedAt,omitempty"`
	ObjectLockConfigUpdatedAt   time.Time `json:"objectLockConfigUpdatedAt,omitempty"`
	EncryptionConfigUpdatedAt   time.Time `json:"encryptionConfigUpdatedAt,omitempty"`
	TaggingConfigUpdatedAt      time.Time `json:"taggingConfigUpdatedAt,omitempty"`
	QuotaConfigUpdatedAt        time.Time `json:"quotaConfigUpdatedAt,omitempty"`
	ReplicationConfigUpdatedAt  time.Time `json:"replicationConfigUpdatedAt,omitempty"`
	VersioningConfigUpdatedAt   time.Time `json:"versioningConfigUpdatedAt,omitempty"`
}

// ObjectMetadata represents object metadata
type ObjectMetadata struct {
	Version        string            `json:"version"`                  // DirIO metadata version
	Owner          *uuid.UUID        `json:"owner,omitempty"`          // User UUID; nil = admin-only (no specific owner)
	ContentType    string            `json:"contentType"`              // MIME type
	Size           int64             `json:"size"`                     // Object size in bytes
	ETag           string            `json:"etag"`                     // Entity tag (MD5 hash)
	LastModified   time.Time         `json:"lastModified"`             // Last modification timestamp
	CustomMetadata map[string]string `json:"customMetadata,omitempty"` // Custom headers like Cache-Control, Content-Disposition, x-amz-meta-*, etc.
	Tags           map[string]string `json:"tags,omitempty"`           // Object tags (key-value pairs)
}

// New creates a new metadata manager
func New(rootFS billy.Filesystem) (*Manager, error) {
	if rootFS == nil {
		return nil, fmt.Errorf("rootFS cannot be nil")
	}

	// Create metadata filesystem
	metadataFS, err := path.NewMetadataFS(rootFS)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata filesystem: %w", err)
	}

	// Create subdirectories
	if err := metadataFS.MkdirAll("buckets", 0755); err != nil {
		return nil, fmt.Errorf("failed to create buckets directory: %w", err)
	}
	if err := metadataFS.MkdirAll("iam/users", 0755); err != nil {
		return nil, fmt.Errorf("failed to create IAM users directory: %w", err)
	}
	if err := metadataFS.MkdirAll("iam/policies", 0755); err != nil {
		return nil, fmt.Errorf("failed to create IAM policies directory: %w", err)
	}
	if err := metadataFS.MkdirAll("objects", 0755); err != nil {
		return nil, fmt.Errorf("failed to create objects directory: %w", err)
	}

	return &Manager{
		rootFS:     rootFS,
		metadataFS: metadataFS,
	}, nil
}

// CreateBucket creates metadata for a new bucket
func (m *Manager) CreateBucket(ctx context.Context, bucket string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	user, err := contextInt.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Determine owner: admin as creator → nil (implicit), regular user → UUID (explicit)
	var ownerUUID *uuid.UUID
	if user.UUID != iam.AdminUserUUID {
		ownerUUID = &user.UUID // Regular user gets explicit ownership
	}
	// Admin leaves ownerUUID nil, which omits field with omitempty

	meta := BucketMetadata{
		Version: BucketMetadataVersion,
		Name:    bucket,
		Owner:   ownerUUID, // nil for admin creator, UUID pointer for user creator
		Created: time.Now(),
	}

	return m.saveBucketMetadata(ctx, bucket, &meta)
}

// DeleteBucket removes bucket metadata
func (m *Manager) DeleteBucket(ctx context.Context, bucket string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	metaPath := filepath.Join("buckets", bucket+".json")
	return m.metadataFS.Remove(metaPath)
}

// GetBucketMetadata retrieves bucket metadata
func (m *Manager) GetBucketMetadata(ctx context.Context, bucket string) (*BucketMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	metaPath := filepath.Join("buckets", bucket+".json")

	data, err := util.ReadFile(m.metadataFS, metaPath)
	if err != nil {
		return nil, err
	}

	var meta BucketMetadata
	if err := jsonutil.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// saveBucketMetadata saves bucket metadata to disk
func (m *Manager) saveBucketMetadata(ctx context.Context, bucket string, meta *BucketMetadata) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	metaPath := filepath.Join("buckets", bucket+".json")
	return jsonutil.MarshalToFile(m.metadataFS, metaPath, meta)
}

// GetObjectMetadata retrieves object metadata
func (m *Manager) GetObjectMetadata(ctx context.Context, bucket, key string) (*ObjectMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	metaPath := filepath.Join("objects", bucket, key+".json")

	data, err := util.ReadFile(m.metadataFS, metaPath)
	if err != nil {
		if isNotExist(err) {
			return nil, fmt.Errorf("object metadata not found")
		}
		return nil, err
	}

	var meta ObjectMetadata
	if err := jsonutil.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// PutObjectMetadata stores object metadata
func (m *Manager) PutObjectMetadata(ctx context.Context, bucket, key string, meta *ObjectMetadata) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	metaPath := filepath.Join("objects", bucket, key+".json")

	// Create parent directories
	dir := filepath.Dir(metaPath)
	if err := m.metadataFS.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	return jsonutil.MarshalToFile(m.metadataFS, metaPath, meta)
}

// DeleteObjectMetadata removes object metadata
func (m *Manager) DeleteObjectMetadata(ctx context.Context, bucket, key string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	metaPath := filepath.Join("objects", bucket, key+".json")

	err := m.metadataFS.Remove(metaPath)
	if err != nil && !isNotExist(err) {
		return err
	}

	// Clean up empty parent directories
	dir := filepath.Dir(metaPath)
	for dir != "." && dir != "" && dir != "/" && dir != "objects" {
		// Check for cancellation during cleanup
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled during cleanup: %w", err)
		}

		entries, err := m.metadataFS.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := m.metadataFS.Remove(dir); err != nil {
			return err
		}
		dir = filepath.Dir(dir)
	}

	return nil
}

// GetUser retrieves a single user by username
func (m *Manager) GetUser(ctx context.Context, username string) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	userPath := filepath.Join("iam", "users", username+".json")
	data, err := util.ReadFile(m.metadataFS, userPath)
	if err != nil {
		if isNotExist(err) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	var user User
	if err := jsonutil.Unmarshal(data, &user); err != nil {
		return nil, err
	}

	// Backwards compatibility: populate Username from filename if not set
	if user.Username == "" {
		user.Username = username
	}

	return &user, nil
}

// CreateOrUpdateUser creates a new user or updates an existing one
func (m *Manager) CreateOrUpdateUser(ctx context.Context, user *User) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if user.AccessKey == "" {
		return fmt.Errorf("accessKey is required")
	}

	if user.Username == "" {
		return fmt.Errorf("username is required")
	}

	// Generate UUID for new users or preserve existing UUID
	if user.UUID == uuid.Nil {
		// Check if user already exists
		existing, err := m.GetUser(ctx, user.Username)
		if err == nil && existing != nil && existing.UUID != uuid.Nil {
			// Preserve existing UUID on update
			user.UUID = existing.UUID
		} else {
			// Generate new UUID for new user
			user.UUID = uuid.New()
		}
	}

	// Set metadata fields
	user.Version = iam.UserMetadataVersion
	user.UpdatedAt = time.Now()

	return m.SaveUser(ctx, user.Username, user)
}

// UpdateUser updates an existing user's mutable fields
func (m *Manager) UpdateUser(ctx context.Context, username string, updates *User) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	// Get existing user
	existing, err := m.GetUser(ctx, username)
	if err != nil {
		return fmt.Errorf("failed to get existing user: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("user not found: %s", username)
	}

	// Update mutable fields
	if updates.SecretKey != "" {
		existing.SecretKey = updates.SecretKey
	}
	if updates.Status != "" {
		existing.Status = updates.Status
	}
	if updates.AttachedPolicies != nil {
		existing.AttachedPolicies = updates.AttachedPolicies
	}

	// Update timestamp and version
	existing.UpdatedAt = time.Now()
	existing.Version = iam.UserMetadataVersion

	return m.SaveUser(ctx, username, existing)
}

// SaveUser saves a single user (atomic operation)
func (m *Manager) SaveUser(ctx context.Context, username string, user *User) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	userPath := filepath.Join("iam", "users", username+".json")

	return jsonutil.MarshalToFile(m.metadataFS, userPath, user)
}

// DeleteUser removes a user
func (m *Manager) DeleteUser(ctx context.Context, username string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	userPath := filepath.Join("iam", "users", username+".json")
	err := m.metadataFS.Remove(userPath)
	if err != nil && !isNotExist(err) {
		return err
	}
	return nil
}

// ListUsers returns a list of all usernames
func (m *Manager) ListUsers(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	usersDir := filepath.Join("iam", "users")
	entries, err := m.metadataFS.ReadDir(usersDir)
	if err != nil {
		if isNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	usernames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		username := entry.Name()[:len(entry.Name())-5] // Remove .json
		usernames = append(usernames, username)
	}

	return usernames, nil
}

// GetUsers retrieves all users (convenience method, loads all user files)
func (m *Manager) GetUsers(ctx context.Context) (map[string]*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	usernames, err := m.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	users := make(map[string]*User)
	for _, username := range usernames {
		user, err := m.GetUser(ctx, username)
		if err != nil {
			fmt.Printf("Warning: failed to load user %s: %v\n", username, err)
			continue
		}
		if user != nil {
			users[username] = user
		}
	}

	return users, nil
}

// SavePolicy saves a single policy
func (m *Manager) SavePolicy(ctx context.Context, policy *Policy) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	policyPath := filepath.Join("iam", "policies", policy.Name+".json")

	return jsonutil.MarshalToFile(m.metadataFS, policyPath, policy)
}

// GetPolicy retrieves a policy by name
func (m *Manager) GetPolicy(ctx context.Context, name string) (*Policy, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	policyPath := filepath.Join("iam", "policies", name+".json")

	data, err := util.ReadFile(m.metadataFS, policyPath)
	if err != nil {
		if isNotExist(err) {
			return nil, ErrPolicyNotFound
		}
		return nil, err
	}

	var policy Policy
	if err := jsonutil.Unmarshal(data, &policy); err != nil {
		return nil, err
	}

	return &policy, nil
}

// DeletePolicy removes a policy by name
func (m *Manager) DeletePolicy(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	policyPath := filepath.Join("iam", "policies", name+".json")
	err := m.metadataFS.Remove(policyPath)
	if err != nil && !isNotExist(err) {
		return err
	}
	return nil
}

// ListPolicyNames returns all policy names
func (m *Manager) ListPolicyNames(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	policiesDir := filepath.Join("iam", "policies")
	entries, err := m.metadataFS.ReadDir(policiesDir)
	if err != nil {
		if isNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	policyNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		policyName := entry.Name()[:len(entry.Name())-5] // Remove .json
		policyNames = append(policyNames, policyName)
	}

	return policyNames, nil
}

// GetPolicies retrieves all policies
func (m *Manager) GetPolicies(ctx context.Context) (map[string]*Policy, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	policiesDir := filepath.Join("iam", "policies")
	entries, err := m.metadataFS.ReadDir(policiesDir)
	if err != nil {
		if isNotExist(err) {
			return make(map[string]*Policy), nil
		}
		return nil, err
	}

	policies := make(map[string]*Policy)
	for _, entry := range entries {
		// Check for cancellation during iteration
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled during policy loading: %w", err)
		}

		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		policyName := entry.Name()[:len(entry.Name())-5] // Remove .json
		policy, err := m.GetPolicy(ctx, policyName)
		if err != nil {
			fmt.Printf("Warning: failed to load policy %s: %v\n", policyName, err)
			continue
		}
		policies[policyName] = policy
	}

	return policies, nil
}

// isNotExist checks if an error is a "not exist" error
func isNotExist(err error) bool {
	return err != nil && fs.ErrNotExist != nil && (errors.Is(err, fs.ErrNotExist) || err.Error() == "file does not exist")
}

// ============================================================================
// Bucket Policy Operations
// ============================================================================

// SetBucketPolicy sets or updates the bucket policy
func (m *Manager) SetBucketPolicy(ctx context.Context, bucket string, policy *PolicyDocument) error {
	// Load existing bucket metadata
	meta, err := m.GetBucketMetadata(ctx, bucket)
	if err != nil {
		return err
	}

	// Update the policy
	meta.BucketPolicy = policy
	meta.PolicyConfigUpdatedAt = time.Now().UTC()

	// Save the updated metadata
	bucketPath := filepath.Join("buckets", bucket+".json")
	return jsonutil.MarshalToFile(m.metadataFS, bucketPath, meta)
}

// GetBucketPolicy retrieves the bucket policy
func (m *Manager) GetBucketPolicy(ctx context.Context, bucket string) (*PolicyDocument, error) {
	meta, err := m.GetBucketMetadata(ctx, bucket)
	if err != nil {
		return nil, err
	}

	return meta.BucketPolicy, nil
}

// DeleteBucketPolicy removes the bucket policy
func (m *Manager) DeleteBucketPolicy(ctx context.Context, bucket string) error {
	// Load existing bucket metadata
	meta, err := m.GetBucketMetadata(ctx, bucket)
	if err != nil {
		return err
	}

	// Clear the policy
	meta.BucketPolicy = nil
	meta.PolicyConfigUpdatedAt = time.Time{} // Zero time

	// Save the updated metadata
	bucketPath := filepath.Join("buckets", bucket+".json")
	return jsonutil.MarshalToFile(m.metadataFS, bucketPath, meta)
}

// GetAllBucketPolicies retrieves all bucket policies for loading into the policy engine.
// Returns a map from bucket name to policy document (nil policies are excluded).
func (m *Manager) GetAllBucketPolicies(ctx context.Context) (map[string]*PolicyDocument, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	bucketsDir := "buckets"
	entries, err := m.metadataFS.ReadDir(bucketsDir)
	if err != nil {
		if isNotExist(err) {
			return make(map[string]*PolicyDocument), nil
		}
		return nil, err
	}

	policies := make(map[string]*PolicyDocument)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled during policy loading: %w", err)
		}

		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		bucketName := entry.Name()[:len(entry.Name())-5] // Remove .json
		meta, err := m.GetBucketMetadata(ctx, bucketName)
		if err != nil {
			continue // Skip buckets we can't read
		}

		if meta.BucketPolicy != nil {
			policies[bucketName] = meta.BucketPolicy
		}
	}

	return policies, nil
}
