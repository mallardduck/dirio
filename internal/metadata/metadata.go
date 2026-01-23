package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manager handles metadata storage and retrieval
type Manager struct {
	dataDir      string
	metadataDir  string
	bucketsDir   string
	policiesDir  string
	minioSysDir  string
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
	Name      string    `json:"name"`
	Owner     string    `json:"owner"`
	Created   time.Time `json:"created"`
	Policy    string    `json:"policy,omitempty"`    // S3 bucket policy JSON
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
func New(dataDir string) (*Manager, error) {
	metadataDir := filepath.Join(dataDir, ".metadata")
	bucketsDir := filepath.Join(metadataDir, "buckets")
	policiesDir := filepath.Join(metadataDir, "policies")
	minioSysDir := filepath.Join(dataDir, ".minio.sys")

	// Create metadata directories
	if err := os.MkdirAll(bucketsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create buckets directory: %w", err)
	}
	if err := os.MkdirAll(policiesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create policies directory: %w", err)
	}

	return &Manager{
		dataDir:     dataDir,
		metadataDir: metadataDir,
		bucketsDir:  bucketsDir,
		policiesDir: policiesDir,
		minioSysDir: minioSysDir,
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
	metaPath := filepath.Join(m.bucketsDir, bucket+".json")
	return os.Remove(metaPath)
}

// GetBucketMetadata retrieves bucket metadata
func (m *Manager) GetBucketMetadata(bucket string) (*BucketMetadata, error) {
	metaPath := filepath.Join(m.bucketsDir, bucket+".json")
	
	data, err := os.ReadFile(metaPath)
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
	metaPath := filepath.Join(m.bucketsDir, bucket+".json")
	
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0644)
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
	usersPath := filepath.Join(m.metadataDir, "users.json")
	
	data, err := os.ReadFile(usersPath)
	if os.IsNotExist(err) {
		return make(map[string]*User), nil
	}
	if err != nil {
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
	usersPath := filepath.Join(m.metadataDir, "users.json")

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(usersPath, data, 0644)
}

// SavePolicy saves a single policy
func (m *Manager) SavePolicy(policy *Policy) error {
	policyPath := filepath.Join(m.policiesDir, policy.Name+".json")

	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(policyPath, data, 0644)
}

// GetPolicy retrieves a policy by name
func (m *Manager) GetPolicy(name string) (*Policy, error) {
	policyPath := filepath.Join(m.policiesDir, name+".json")

	data, err := os.ReadFile(policyPath)
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
	entries, err := os.ReadDir(m.policiesDir)
	if err != nil {
		if os.IsNotExist(err) {
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
