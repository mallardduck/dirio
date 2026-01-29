package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"

	"github.com/go-git/go-billy/v5"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/path"
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
	rootFS   billy.Filesystem
	metadata *metadata.Manager
	log      *slog.Logger
}

// New creates a new storage backend
func New(rootFS billy.Filesystem, metadata *metadata.Manager) (*Storage, error) {
	if rootFS == nil {
		return nil, fmt.Errorf("rootFS cannot be nil")
	}

	return &Storage{
		rootFS:   rootFS,
		metadata: metadata,
		log:      logging.Component("storage"),
	}, nil
}

// ListBuckets returns all buckets
func (s *Storage) ListBuckets() ([]s3types.Bucket, error) {
	entries, err := s.rootFS.ReadDir(".")
	if err != nil {
		return nil, err
	}

	buckets := make([]s3types.Bucket, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip metadata directories
		name := entry.Name()
		if name == ".minio.sys" || name == ".dirio" || name[0] == '.' {
			continue
		}

		buckets = append(buckets, s3types.Bucket{
			Name:         name,
			CreationDate: entry.ModTime(),
		})
	}

	return buckets, nil
}

// CreateBucket creates a new bucket
func (s *Storage) CreateBucket(bucket string) error {
	// Validate bucket name for path safety
	if err := path.ValidatePathSafe(bucket); err != nil {
		return fmt.Errorf("invalid bucket name: %w", err)
	}

	// Check if bucket exists
	if _, err := s.rootFS.Stat(bucket); err == nil {
		return ErrBucketExists
	}

	// Create bucket directory
	if err := s.rootFS.MkdirAll(bucket, 0755); err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	// Create bucket metadata
	if err := s.metadata.CreateBucket(bucket); err != nil {
		// Cleanup on failure
		if err := s.rootFS.Remove(bucket); err != nil {
			s.log.Error("failed to cleanup bucket directory", "error", err)
		}
		return err
	}

	return nil
}

// BucketExists checks if a bucket exists
func (s *Storage) BucketExists(bucket string) (bool, error) {
	_, err := s.rootFS.Stat(bucket)
	if err != nil {
		if isNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeleteBucket deletes a bucket (must be empty)
func (s *Storage) DeleteBucket(bucket string) error {
	// Check if bucket exists
	info, err := s.rootFS.Stat(bucket)
	if err != nil {
		if isNotExist(err) {
			return ErrNoSuchBucket
		}
		return err
	}
	if !info.IsDir() {
		return ErrNoSuchBucket
	}

	// Check if bucket is empty
	entries, err := s.rootFS.ReadDir(bucket)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return ErrBucketNotEmpty
	}

	// Remove bucket directory
	if err := s.rootFS.Remove(bucket); err != nil {
		return err
	}

	// Remove bucket metadata
	s.metadata.DeleteBucket(bucket)

	return nil
}

// ListObjects returns objects in a bucket with optional prefix and delimiter
func (s *Storage) ListObjects(bucket, prefix, delimiter string, maxKeys int) ([]s3types.Object, error) {
	// Check if bucket exists
	if _, err := s.rootFS.Stat(bucket); err != nil {
		if isNotExist(err) {
			return nil, ErrNoSuchBucket
		}
		return nil, err
	}

	objects := make([]s3types.Object, 0)

	// Walk the bucket directory recursively
	err := s.walkDir(bucket, "", func(key string, info fs.FileInfo) error {
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

// walkDir recursively walks a directory in the filesystem
func (s *Storage) walkDir(bucket, dir string, fn func(key string, info fs.FileInfo) error) error {
	// Get bucket filesystem
	bucketFS, err := path.NewBucketFS(s.rootFS, bucket)
	if err != nil {
		return err
	}

	// Read directory entries
	entries, err := bucketFS.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		var entryPath string
		if dir == "" {
			entryPath = entry.Name()
		} else {
			entryPath = filepath.Join(dir, entry.Name())
		}

		if entry.IsDir() {
			// Recursively walk subdirectory
			if err := s.walkDir(bucket, entryPath, fn); err != nil {
				return err
			}
		} else {
			// Convert to S3 key format (forward slashes)
			key := filepath.ToSlash(entryPath)
			if err := fn(key, entry); err != nil {
				return err
			}
		}
	}

	return nil
}

// isNotExist checks if an error is a "not exist" error
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
