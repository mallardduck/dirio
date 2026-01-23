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

// Manager handles metadata storage and retrieval
type Manager struct {
	rootFS     billy.Filesystem
	metadataFS billy.Filesystem
}

// User represents a user with credentials
type User struct {
	AccessKey      string    `json:"accessKey"`
	SecretKey      string    `json:"secretKey"`
	Status         string    `json:"status"`
	UpdatedAt      time.Time `json:"updatedAt"`
	AttachedPolicy string    `json:"attachedPolicy,omitempty"` // Name of attached IAM policy
}

// BucketMetadata represents bucket configuration
type BucketMetadata struct {
	Name    string    `json:"name"`
	Owner   string    `json:"owner"`
	Created time.Time `json:"created"`
	Policy  string    `json:"policy,omitempty"` // S3 bucket policy JSON
}

// Policy represents an IAM policy
type Policy struct {
	Name       string    `json:"name"`
	PolicyJSON string    `json:"policyJson"` // IAM policy document (S3 format)
	CreateDate time.Time `json:"createDate"`
	UpdateDate time.Time `json:"updateDate"`
}

// ObjectMetadata represents object metadata
type ObjectMetadata struct {
	ContentType  string    `json:"contentType"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag"`
	LastModified time.Time `json:"lastModified"`
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

	return &Manager{
		rootFS:     rootFS,
		metadataFS: metadataFS,
	}, nil
}

// CreateBucket creates metadata for a new bucket
func (m *Manager) CreateBucket(bucket string) error {
	meta := BucketMetadata{
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

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return util.WriteFile(m.metadataFS, metaPath, data, 0644)
}

// GetObjectMetadata retrieves object metadata
func (m *Manager) GetObjectMetadata(bucket, key string) (*ObjectMetadata, error) {
	// For now, we don't store per-object metadata separately
	// We can add this later if needed
	return nil, fmt.Errorf("object metadata not found")
}

// PutObjectMetadata stores object metadata
func (m *Manager) PutObjectMetadata(bucket, key string, meta *ObjectMetadata) error {
	// For now, we don't store per-object metadata separately
	// We can add this later if needed
	return nil
}

// DeleteObjectMetadata removes object metadata
func (m *Manager) DeleteObjectMetadata(bucket, key string) error {
	// For now, we don't store per-object metadata separately
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
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}

	return util.WriteFile(m.metadataFS, "users.json", data, 0644)
}

// SavePolicy saves a single policy
func (m *Manager) SavePolicy(policy *Policy) error {
	policyPath := filepath.Join("policies", policy.Name+".json")

	data, err := json.MarshalIndent(policy, "", "  ")
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
