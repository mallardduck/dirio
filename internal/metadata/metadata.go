package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
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
	Version        string    `json:"version"` // DirIO metadata version
	AccessKey      string    `json:"accessKey"`
	SecretKey      string    `json:"secretKey"`
	Status         string    `json:"status"`
	UpdatedAt      time.Time `json:"updatedAt"`
	AttachedPolicy string    `json:"attachedPolicy,omitempty"` // Name of attached IAM policy
}

// BucketMetadata represents bucket configuration
type BucketMetadata struct {
	Version string    `json:"version"` // DirIO metadata version
	Name    string    `json:"name"`
	Owner   string    `json:"owner"`
	Created time.Time `json:"created"`
	Policy  string    `json:"policy,omitempty"` // S3 bucket policy JSON

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

// Policy represents an IAM policy
type Policy struct {
	Version    string    `json:"version"` // DirIO metadata version
	Name       string    `json:"name"`
	PolicyJSON string    `json:"policyJson"` // IAM policy document (S3 format)
	CreateDate time.Time `json:"createDate"`
	UpdateDate time.Time `json:"updateDate"`
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
	if err := metadataFS.MkdirAll("policies", 0755); err != nil {
		return nil, fmt.Errorf("failed to create policies directory: %w", err)
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
func (m *Manager) CreateBucket(bucket string) error {
	meta := BucketMetadata{
		Version: BucketMetadataVersion,
		Name:    bucket,
		Owner:   "root", // TODO: Get from auth context
		Created: time.Now(),
	}

	return m.saveBucketMetadata(bucket, &meta)
}

// DeleteBucket removes bucket metadata
func (m *Manager) DeleteBucket(bucket string) error {
	metaPath := filepath.Join("buckets", bucket+".json")
	return m.metadataFS.Remove(metaPath)
}

// GetBucketMetadata retrieves bucket metadata
func (m *Manager) GetBucketMetadata(bucket string) (*BucketMetadata, error) {
	metaPath := filepath.Join("buckets", bucket+".json")

	data, err := util.ReadFile(m.metadataFS, metaPath)
	if err != nil {
		return nil, err
	}

	var meta BucketMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// saveBucketMetadata saves bucket metadata to disk
func (m *Manager) saveBucketMetadata(bucket string, meta *BucketMetadata) error {
	metaPath := filepath.Join("buckets", bucket+".json")

	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	return util.WriteFile(m.metadataFS, metaPath, data, 0644)
}

// GetObjectMetadata retrieves object metadata
func (m *Manager) GetObjectMetadata(bucket, key string) (*ObjectMetadata, error) {
	metaPath := filepath.Join("objects", bucket, key+".json")

	data, err := util.ReadFile(m.metadataFS, metaPath)
	if err != nil {
		if isNotExist(err) {
			return nil, fmt.Errorf("object metadata not found")
		}
		return nil, err
	}

	var meta ObjectMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// PutObjectMetadata stores object metadata
func (m *Manager) PutObjectMetadata(bucket, key string, meta *ObjectMetadata) error {
	metaPath := filepath.Join("objects", bucket, key+".json")

	// Create parent directories
	dir := filepath.Dir(metaPath)
	if err := m.metadataFS.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	return util.WriteFile(m.metadataFS, metaPath, data, 0644)
}

// DeleteObjectMetadata removes object metadata
func (m *Manager) DeleteObjectMetadata(bucket, key string) error {
	metaPath := filepath.Join("objects", bucket, key+".json")

	err := m.metadataFS.Remove(metaPath)
	if err != nil && !isNotExist(err) {
		return err
	}

	// Clean up empty parent directories
	dir := filepath.Dir(metaPath)
	for dir != "." && dir != "" && dir != "/" && dir != "objects" {
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

// GetUsers retrieves all users
func (m *Manager) GetUsers() (map[string]*User, error) {
	data, err := util.ReadFile(m.metadataFS, "users.json")
	if err != nil {
		if isNotExist(err) {
			return make(map[string]*User), nil
		}
		return nil, err
	}

	var users map[string]*User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, err
	}

	return users, nil
}

// SaveUsers saves all users
func (m *Manager) SaveUsers(users map[string]*User) error {
	data, err := json.Marshal(users)
	if err != nil {
		return err
	}

	return util.WriteFile(m.metadataFS, "users.json", data, 0644)
}

// SavePolicy saves a single policy
func (m *Manager) SavePolicy(policy *Policy) error {
	policyPath := filepath.Join("policies", policy.Name+".json")

	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}

	return util.WriteFile(m.metadataFS, policyPath, data, 0644)
}

// GetPolicy retrieves a policy by name
func (m *Manager) GetPolicy(name string) (*Policy, error) {
	policyPath := filepath.Join("policies", name+".json")

	data, err := util.ReadFile(m.metadataFS, policyPath)
	if err != nil {
		return nil, err
	}

	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, err
	}

	return &policy, nil
}

// GetPolicies retrieves all policies
func (m *Manager) GetPolicies() (map[string]*Policy, error) {
	entries, err := m.metadataFS.ReadDir("policies")
	if err != nil {
		if isNotExist(err) {
			return make(map[string]*Policy), nil
		}
		return nil, err
	}

	policies := make(map[string]*Policy)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		policyName := entry.Name()[:len(entry.Name())-5] // Remove .json
		policy, err := m.GetPolicy(policyName)
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
