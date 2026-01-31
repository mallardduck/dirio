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
	"github.com/mallardduck/dirio/internal/jsonutil"
	"github.com/mallardduck/dirio/internal/path"
)

// Metadata format versions
const (
	UserMetadataVersion   = "1.0.0"
	BucketMetadataVersion = "1.0.0"
	PolicyMetadataVersion = "1.0.0"
	ObjectMetadataVersion = "1.0.0"
)

// Manager handles metadata storage and retrieval
type Manager struct {
	rootFS     billy.Filesystem
	metadataFS billy.Filesystem
}

// User represents a user with credentials
type User struct {
	Version          string    `json:"version"` // DirIO metadata version
	AccessKey        string    `json:"accessKey"`
	SecretKey        string    `json:"secretKey"`
	Status           string    `json:"status"`
	UpdatedAt        time.Time `json:"updatedAt"`
	AttachedPolicies []string  `json:"attachedPolicies,omitempty"` // Names of attached IAM policies (supports multiple)
}

// BucketMetadata represents bucket configuration
type BucketMetadata struct {
	Version      string          `json:"version"` // DirIO metadata version
	Name         string          `json:"name"`
	Owner        string          `json:"owner"`
	Created      time.Time       `json:"created"`
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

// PolicyDocument represents an AWS IAM Policy Document (used by both IAM policies and bucket policies)
// See: https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements.html
type PolicyDocument struct {
	Version   string            `json:"Version"`      // Policy language version (usually "2012-10-17")
	Id        string            `json:"Id,omitempty"` // Optional policy ID
	Statement []PolicyStatement `json:"Statement"`    // List of policy statements
}

// PolicyStatement represents a single statement in a policy document
type PolicyStatement struct {
	Sid       string                 `json:"Sid,omitempty"`       // Optional statement ID
	Effect    string                 `json:"Effect"`              // "Allow" or "Deny"
	Principal interface{}            `json:"Principal,omitempty"` // Who (can be string, map, or array)
	Action    interface{}            `json:"Action"`              // What actions (string or []string)
	Resource  interface{}            `json:"Resource,omitempty"`  // What resources (string or []string)
	Condition map[string]interface{} `json:"Condition,omitempty"` // Optional conditions
}

// Policy represents an IAM policy (attached to users/roles)
type Policy struct {
	Version        string          `json:"version"`        // DirIO metadata version
	Name           string          `json:"name"`           // Policy name
	PolicyDocument *PolicyDocument `json:"policyDocument"` // The actual IAM policy
	CreateDate     time.Time       `json:"createDate"`
	UpdateDate     time.Time       `json:"updateDate"`
}

// ObjectMetadata represents object metadata
type ObjectMetadata struct {
	Version        string            `json:"version"` // DirIO metadata version
	ContentType    string            `json:"contentType"`
	Size           int64             `json:"size"`
	ETag           string            `json:"etag"`
	LastModified   time.Time         `json:"lastModified"`
	CustomMetadata map[string]string `json:"customMetadata,omitempty"` // Custom headers like Cache-Control, Content-Disposition, x-amz-meta-*, etc.
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

	meta := BucketMetadata{
		Version: BucketMetadataVersion,
		Name:    bucket,
		Owner:   "root", // TODO: Get from auth context
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
			return nil, nil // User doesn't exist
		}
		return nil, err
	}

	var user User
	if err := jsonutil.Unmarshal(data, &user); err != nil {
		return nil, err
	}

	return &user, nil
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

	var usernames []string
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
		return nil, err
	}

	var policy Policy
	if err := jsonutil.Unmarshal(data, &policy); err != nil {
		return nil, err
	}

	return &policy, nil
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
