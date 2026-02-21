package metadata

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
	"github.com/google/uuid"

	contextInt "github.com/mallardduck/dirio/internal/context"

	"github.com/mallardduck/dirio/internal/crypto"
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
	ErrUserNotFound           = errors.New("user not found")
	ErrPolicyNotFound         = errors.New("policy not found")
	ErrGroupNotFound          = errors.New("group not found")
	ErrGroupAlreadyExists     = errors.New("group already exists")
	ErrServiceAccountNotFound = errors.New("service account not found")
)

// Manager handles metadata storage and retrieval
type Manager struct {
	rootFS     billy.Filesystem
	metadataFS billy.Filesystem

	// userUUIDIdx provides O(1) lookup of user access key by UUID.
	// Populated at startup and kept in sync via CreateOrUpdateUser / DeleteUser.
	userUUIDMu  sync.RWMutex
	userUUIDIdx map[uuid.UUID]string // UUID → access key (username)
}

// Type aliases for backward compatibility
type User = iam.User
type Policy = iam.Policy
type PolicyDocument = iam.PolicyDocument
type PolicyStatement = iam.Statement
type Group = iam.Group
type ServiceAccount = iam.ServiceAccount

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
	if err := metadataFS.MkdirAll("iam/groups", 0755); err != nil {
		return nil, fmt.Errorf("failed to create IAM groups directory: %w", err)
	}
	if err := metadataFS.MkdirAll("iam/service-accounts", 0755); err != nil {
		return nil, fmt.Errorf("failed to create IAM service-accounts directory: %w", err)
	}

	mgr := &Manager{
		rootFS:      rootFS,
		metadataFS:  metadataFS,
		userUUIDIdx: make(map[uuid.UUID]string),
	}

	// Build UUID → access key index from existing user files.
	// Fail-soft: log and skip users that cannot be loaded.
	mgr.buildUserUUIDIndex(context.Background())

	return mgr, nil
}

// buildUserUUIDIndex populates the in-memory UUID index from disk.
// Called once at startup; individual errors are logged and skipped.
func (m *Manager) buildUserUUIDIndex(ctx context.Context) {
	usernames, err := m.ListUsers(ctx)
	if err != nil {
		fmt.Printf("Warning: failed to list users while building UUID index: %v\n", err)
		return
	}

	m.userUUIDMu.Lock()
	defer m.userUUIDMu.Unlock()

	for _, username := range usernames {
		user, err := m.GetUser(ctx, username)
		if err != nil {
			fmt.Printf("Warning: failed to load user %s while building UUID index: %v\n", username, err)
			continue
		}
		if user != nil && user.UUID != uuid.Nil {
			// Index maps UUID → username (the file key used by GetUser).
			m.userUUIDIdx[user.UUID] = username
		}
	}
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

	// Decrypt secret key if stored encrypted.
	decrypted, err := crypto.Decrypt(user.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret key for user %s: %w", username, err)
	}
	user.SecretKey = decrypted

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

	if err := m.SaveUser(ctx, user.Username, user); err != nil {
		return err
	}

	// Keep UUID index in sync. Store Username (the file key) not AccessKey.
	m.userUUIDMu.Lock()
	m.userUUIDIdx[user.UUID] = user.Username
	m.userUUIDMu.Unlock()

	return nil
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

// SaveUser saves a single user (atomic operation).
// The SecretKey is encrypted before writing; the in-memory user is unchanged.
func (m *Manager) SaveUser(ctx context.Context, username string, user *User) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	// Encrypt secret key before persisting.
	encryptedSecret, err := crypto.Encrypt(user.SecretKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret key for user %s: %w", username, err)
	}

	// Work on a shallow copy so the in-memory user keeps the plaintext value.
	toSave := *user
	toSave.SecretKey = encryptedSecret

	userPath := filepath.Join("iam", "users", username+".json")

	return jsonutil.MarshalToFile(m.metadataFS, userPath, &toSave)
}

// DeleteUser removes a user
func (m *Manager) DeleteUser(ctx context.Context, username string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	// Load user first to get their UUID for index cleanup.
	user, _ := m.GetUser(ctx, username)

	userPath := filepath.Join("iam", "users", username+".json")
	err := m.metadataFS.Remove(userPath)
	if err != nil && !isNotExist(err) {
		return err
	}

	// Remove from UUID index if we had the user's UUID.
	if user != nil && user.UUID != uuid.Nil {
		m.userUUIDMu.Lock()
		delete(m.userUUIDIdx, user.UUID)
		m.userUUIDMu.Unlock()
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

// GetUserByUUID retrieves a user by their UUID using the in-memory index (O(1)).
func (m *Manager) GetUserByUUID(ctx context.Context, userUUID uuid.UUID) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	m.userUUIDMu.RLock()
	accessKey, ok := m.userUUIDIdx[userUUID]
	m.userUUIDMu.RUnlock()

	if !ok {
		return nil, ErrUserNotFound
	}

	return m.GetUser(ctx, accessKey)
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

// ListBucketMetadatas returns the full metadata for every bucket.
func (m *Manager) ListBucketMetadatas(ctx context.Context) ([]*BucketMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	entries, err := m.metadataFS.ReadDir("buckets")
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	out := make([]*BucketMetadata, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled during bucket listing: %w", err)
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		bucketName := entry.Name()[:len(entry.Name())-5]
		meta, err := m.GetBucketMetadata(ctx, bucketName)
		if err != nil {
			continue
		}
		out = append(out, meta)
	}

	return out, nil
}

// SetBucketOwner updates the owner UUID of an existing bucket.
// Pass nil to clear ownership (admin-only bucket).
func (m *Manager) SetBucketOwner(ctx context.Context, bucket string, ownerUUID *uuid.UUID) error {
	meta, err := m.GetBucketMetadata(ctx, bucket)
	if err != nil {
		return err
	}
	meta.Owner = ownerUUID
	return m.saveBucketMetadata(ctx, bucket, meta)
}

// ============================================================================
// Group Operations
// ============================================================================

// CreateGroup creates a new empty group. Returns ErrGroupAlreadyExists if it already exists.
func (m *Manager) CreateGroup(ctx context.Context, groupName string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	groupPath := filepath.Join("iam", "groups", groupName+".json")

	// Check if already exists
	if _, err := m.metadataFS.Stat(groupPath); err == nil {
		return ErrGroupAlreadyExists
	}

	now := time.Now()
	g := &iam.Group{
		Version:   iam.GroupMetadataVersion,
		Name:      groupName,
		Status:    iam.GroupStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return jsonutil.MarshalToFile(m.metadataFS, groupPath, g)
}

// GetGroup retrieves a group by name.
func (m *Manager) GetGroup(ctx context.Context, groupName string) (*Group, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	groupPath := filepath.Join("iam", "groups", groupName+".json")
	data, err := util.ReadFile(m.metadataFS, groupPath)
	if err != nil {
		if isNotExist(err) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}

	var g Group
	if err := jsonutil.Unmarshal(data, &g); err != nil {
		return nil, err
	}

	return &g, nil
}

// SaveGroup atomically saves a group.
func (m *Manager) SaveGroup(ctx context.Context, g *Group) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	groupPath := filepath.Join("iam", "groups", g.Name+".json")
	return jsonutil.MarshalToFile(m.metadataFS, groupPath, g)
}

// DeleteGroup removes a group.
func (m *Manager) DeleteGroup(ctx context.Context, groupName string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	groupPath := filepath.Join("iam", "groups", groupName+".json")
	err := m.metadataFS.Remove(groupPath)
	if err != nil && !isNotExist(err) {
		return err
	}
	return nil
}

// ListGroupNames returns all group names.
func (m *Manager) ListGroupNames(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	groupsDir := filepath.Join("iam", "groups")
	entries, err := m.metadataFS.ReadDir(groupsDir)
	if err != nil {
		if isNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		names = append(names, entry.Name()[:len(entry.Name())-5])
	}

	return names, nil
}

// GetGroups loads all groups.
func (m *Manager) GetGroups(ctx context.Context) (map[string]*Group, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	names, err := m.ListGroupNames(ctx)
	if err != nil {
		return nil, err
	}

	groups := make(map[string]*Group, len(names))
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled during group loading: %w", err)
		}
		g, err := m.GetGroup(ctx, name)
		if err != nil {
			fmt.Printf("Warning: failed to load group %s: %v\n", name, err)
			continue
		}
		groups[name] = g
	}

	return groups, nil
}

// AddUserToGroup adds an access key to a group's member list (idempotent).
func (m *Manager) AddUserToGroup(ctx context.Context, groupName, accessKey string) error {
	g, err := m.GetGroup(ctx, groupName)
	if err != nil {
		return err
	}

	for _, member := range g.Members {
		if member == accessKey {
			return nil // already a member
		}
	}

	g.Members = append(g.Members, accessKey)
	g.UpdatedAt = time.Now()

	return m.SaveGroup(ctx, g)
}

// RemoveUserFromGroup removes an access key from a group's member list.
func (m *Manager) RemoveUserFromGroup(ctx context.Context, groupName, accessKey string) error {
	g, err := m.GetGroup(ctx, groupName)
	if err != nil {
		return err
	}

	filtered := g.Members[:0]
	for _, member := range g.Members {
		if member != accessKey {
			filtered = append(filtered, member)
		}
	}
	g.Members = filtered
	g.UpdatedAt = time.Now()

	return m.SaveGroup(ctx, g)
}

// AttachPolicyToGroup adds a policy name to a group's attached policies (idempotent).
func (m *Manager) AttachPolicyToGroup(ctx context.Context, groupName, policyName string) error {
	g, err := m.GetGroup(ctx, groupName)
	if err != nil {
		return err
	}

	for _, p := range g.AttachedPolicies {
		if p == policyName {
			return nil // already attached
		}
	}

	g.AttachedPolicies = append(g.AttachedPolicies, policyName)
	g.UpdatedAt = time.Now()

	return m.SaveGroup(ctx, g)
}

// DetachPolicyFromGroup removes a policy name from a group's attached policies.
func (m *Manager) DetachPolicyFromGroup(ctx context.Context, groupName, policyName string) error {
	g, err := m.GetGroup(ctx, groupName)
	if err != nil {
		return err
	}

	filtered := g.AttachedPolicies[:0]
	for _, p := range g.AttachedPolicies {
		if p != policyName {
			filtered = append(filtered, p)
		}
	}
	g.AttachedPolicies = filtered
	g.UpdatedAt = time.Now()

	return m.SaveGroup(ctx, g)
}

// SetGroupStatus updates a group's status.
func (m *Manager) SetGroupStatus(ctx context.Context, groupName string, status iam.GroupStatus) error {
	g, err := m.GetGroup(ctx, groupName)
	if err != nil {
		return err
	}

	g.Status = status
	g.UpdatedAt = time.Now()

	return m.SaveGroup(ctx, g)
}

// ============================================================================
// Service Account Operations
// ============================================================================

// CreateServiceAccount saves a new service account. Returns error if key already exists.
func (m *Manager) CreateServiceAccount(ctx context.Context, sa *ServiceAccount) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	saPath := filepath.Join("iam", "service-accounts", sa.AccessKey+".json")

	// Check if already exists
	if _, err := m.metadataFS.Stat(saPath); err == nil {
		return fmt.Errorf("service account %q already exists", sa.AccessKey)
	}

	// Encrypt secret key before persisting
	encryptedSecret, err := crypto.Encrypt(sa.SecretKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret key for service account %s: %w", sa.AccessKey, err)
	}
	toSave := *sa
	toSave.SecretKey = encryptedSecret

	return jsonutil.MarshalToFile(m.metadataFS, saPath, &toSave)
}

// GetServiceAccount retrieves a service account by access key.
func (m *Manager) GetServiceAccount(ctx context.Context, accessKey string) (*ServiceAccount, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	saPath := filepath.Join("iam", "service-accounts", accessKey+".json")
	data, err := util.ReadFile(m.metadataFS, saPath)
	if err != nil {
		if isNotExist(err) {
			return nil, ErrServiceAccountNotFound
		}
		return nil, err
	}

	var sa ServiceAccount
	if err := jsonutil.Unmarshal(data, &sa); err != nil {
		return nil, err
	}

	// Decrypt secret key
	decrypted, err := crypto.Decrypt(sa.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret key for service account %s: %w", accessKey, err)
	}
	sa.SecretKey = decrypted

	return &sa, nil
}

// SaveServiceAccount atomically saves a service account (encrypts secret key).
func (m *Manager) SaveServiceAccount(ctx context.Context, sa *ServiceAccount) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	encryptedSecret, err := crypto.Encrypt(sa.SecretKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret key for service account %s: %w", sa.AccessKey, err)
	}
	toSave := *sa
	toSave.SecretKey = encryptedSecret

	saPath := filepath.Join("iam", "service-accounts", sa.AccessKey+".json")
	return jsonutil.MarshalToFile(m.metadataFS, saPath, &toSave)
}

// DeleteServiceAccount removes a service account.
func (m *Manager) DeleteServiceAccount(ctx context.Context, accessKey string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	saPath := filepath.Join("iam", "service-accounts", accessKey+".json")
	err := m.metadataFS.Remove(saPath)
	if err != nil && !isNotExist(err) {
		return err
	}
	return nil
}

// ListServiceAccountKeys returns all service account access keys.
func (m *Manager) ListServiceAccountKeys(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	saDir := filepath.Join("iam", "service-accounts")
	entries, err := m.metadataFS.ReadDir(saDir)
	if err != nil {
		if isNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		keys = append(keys, entry.Name()[:len(entry.Name())-5])
	}

	return keys, nil
}

// GetServiceAccounts loads all service accounts.
func (m *Manager) GetServiceAccounts(ctx context.Context) (map[string]*ServiceAccount, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	keys, err := m.ListServiceAccountKeys(ctx)
	if err != nil {
		return nil, err
	}

	accounts := make(map[string]*ServiceAccount, len(keys))
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled during SA loading: %w", err)
		}
		sa, err := m.GetServiceAccount(ctx, key)
		if err != nil {
			fmt.Printf("Warning: failed to load service account %s: %v\n", key, err)
			continue
		}
		accounts[key] = sa
	}

	return accounts, nil
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
