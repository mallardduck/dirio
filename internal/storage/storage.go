package storage

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/pkg/s3types"
)

var (
	ErrBucketExists   = errors.New("bucket already exists")
	ErrNoSuchBucket   = errors.New("no such bucket")
	ErrBucketNotEmpty = errors.New("bucket not empty")
	ErrNoSuchKey      = errors.New("no such key")
)

// Storage handles filesystem operations for buckets and objects
type Storage struct {
	dataDir  string
	metadata *metadata.Manager
	log      *slog.Logger
}

// New creates a new storage backend
func New(dataDir string, metadata *metadata.Manager) (*Storage, error) {
	bucketsDir := filepath.Join(dataDir, "buckets")
	if err := os.MkdirAll(bucketsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create buckets directory: %w", err)
	}

	return &Storage{
		dataDir:  dataDir,
		metadata: metadata,
		log:      logging.Component("storage"),
	}, nil
}

// ListBuckets returns all buckets
func (s *Storage) ListBuckets() ([]s3types.Bucket, error) {
	bucketsDir := filepath.Join(s.dataDir, "buckets")

	entries, err := os.ReadDir(bucketsDir)
	if err != nil {
		return nil, err
	}

	buckets := make([]s3types.Bucket, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == ".minio.sys" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		buckets = append(buckets, s3types.Bucket{
			Name:         entry.Name(),
			CreationDate: info.ModTime(),
		})
	}

	return buckets, nil
}

// CreateBucket creates a new bucket
func (s *Storage) CreateBucket(bucket string) error {
	bucketPath := s.bucketPath(bucket)

	// Check if bucket exists
	if _, err := os.Stat(bucketPath); err == nil {
		return ErrBucketExists
	}

	// Create bucket directory
	if err := os.MkdirAll(bucketPath, 0755); err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	// Create bucket metadata
	if err := s.metadata.CreateBucket(bucket); err != nil {
		os.RemoveAll(bucketPath) // Cleanup on failure
		return err
	}

	return nil
}

// BucketExists checks if a bucket exists
func (s *Storage) BucketExists(bucket string) (bool, error) {
	bucketPath := s.bucketPath(bucket)
	_, err := os.Stat(bucketPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// DeleteBucket deletes a bucket (must be empty)
func (s *Storage) DeleteBucket(bucket string) error {
	bucketPath := s.bucketPath(bucket)

	// Check if bucket exists
	info, err := os.Stat(bucketPath)
	if os.IsNotExist(err) {
		return ErrNoSuchBucket
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ErrNoSuchBucket
	}

	// Check if bucket is empty
	entries, err := os.ReadDir(bucketPath)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return ErrBucketNotEmpty
	}

	// Remove bucket directory
	if err := os.Remove(bucketPath); err != nil {
		return err
	}

	// Remove bucket metadata
	s.metadata.DeleteBucket(bucket)

	return nil
}

// ListObjects returns objects in a bucket with optional prefix and delimiter
func (s *Storage) ListObjects(bucket, prefix, delimiter string, maxKeys int) ([]s3types.Object, error) {
	bucketPath := s.bucketPath(bucket)

	// Check if bucket exists
	if _, err := os.Stat(bucketPath); os.IsNotExist(err) {
		return nil, ErrNoSuchBucket
	}

	objects := make([]s3types.Object, 0)

	// Walk the bucket directory
	err := filepath.Walk(bucketPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path from bucket root
		relPath, err := filepath.Rel(bucketPath, path)
		if err != nil {
			return err
		}

		// Convert to forward slashes for S3 compatibility
		key := filepath.ToSlash(relPath)

		// Apply prefix filter
		if prefix != "" && !hasPrefix(key, prefix) {
			return nil
		}

		// Get object metadata
		meta, err := s.metadata.GetObjectMetadata(bucket, key)
		if err != nil {
			// If no metadata, create basic metadata
			meta = &metadata.ObjectMetadata{
				ContentType:  "application/octet-stream",
				Size:         info.Size(),
				LastModified: info.ModTime(),
			}
		}

		objects = append(objects, s3types.Object{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime(),
			ETag:         meta.ETag,
			StorageClass: "STANDARD",
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return objects, nil
}

// bucketPath returns the filesystem path for a bucket
func (s *Storage) bucketPath(bucket string) string {
	return filepath.Join(s.dataDir, "buckets", bucket)
}

// objectPath returns the filesystem path for an object
func (s *Storage) objectPath(bucket, key string) string {
	return filepath.Join(s.bucketPath(bucket), filepath.FromSlash(key))
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
