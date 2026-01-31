package storage

import (
	"context"
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
func (s *Storage) ListBuckets(ctx context.Context) ([]s3types.Bucket, error) {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	entries, err := s.rootFS.ReadDir(".")
	if err != nil {
		return nil, err
	}

	buckets := make([]s3types.Bucket, 0, len(entries))
	for _, entry := range entries {
		// Check for cancellation during iteration
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled during listing: %w", err)
		}

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
func (s *Storage) CreateBucket(ctx context.Context, bucket string) error {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	// Validate bucket name for path safety
	if err := path.ValidatePathSafe(bucket); err != nil {
		return fmt.Errorf("invalid bucket name: %w", err)
	}

	// Check if bucket exists
	if _, err := s.rootFS.Stat(bucket); err == nil {
		return ErrBucketExists
	}

	// Check context before creating
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled before creation: %w", err)
	}

	// Create bucket directory
	if err := s.rootFS.MkdirAll(bucket, 0755); err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	// Create bucket metadata
	if err := s.metadata.CreateBucket(ctx, bucket); err != nil {
		// Cleanup on failure
		if err := s.rootFS.Remove(bucket); err != nil {
			s.log.Error("failed to cleanup bucket directory", "error", err)
		}
		return err
	}

	return nil
}

// BucketExists checks if a bucket exists
func (s *Storage) BucketExists(ctx context.Context, bucket string) (bool, error) {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}

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
func (s *Storage) DeleteBucket(ctx context.Context, bucket string) error {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

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

	// Check context before checking contents
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled before checking contents: %w", err)
	}

	// Check if bucket is empty
	entries, err := s.rootFS.ReadDir(bucket)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return ErrBucketNotEmpty
	}

	// Check context before deletion
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled before deletion: %w", err)
	}

	// Remove bucket directory
	if err := s.rootFS.Remove(bucket); err != nil {
		return err
	}

	// Remove bucket metadata
	return s.metadata.DeleteBucket(ctx, bucket)
}

// ListObjects returns objects in a bucket with optional prefix and delimiter
func (s *Storage) ListObjects(ctx context.Context, bucket, prefix, delimiter string, maxKeys int) ([]s3types.Object, error) {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	// Check if bucket exists
	if _, err := s.rootFS.Stat(bucket); err != nil {
		if isNotExist(err) {
			return nil, ErrNoSuchBucket
		}
		return nil, err
	}

	objects := make([]s3types.Object, 0)

	// Walk the bucket directory recursively
	err := s.walkDir(ctx, bucket, "", func(key string, info fs.FileInfo) error {
		// Check for context cancellation during walk
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled during walk: %w", err)
		}

		// Apply prefix filter
		if prefix != "" && !hasPrefix(key, prefix) {
			return nil
		}

		// Get object metadata
		meta, err := s.metadata.GetObjectMetadata(ctx, bucket, key)
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

// ListObjectsV2 returns objects in a bucket with optional prefix and delimiter
func (s *Storage) ListObjectsV2(ctx context.Context, bucket, prefix, continuationToken, startAfter, delimiter string, maxKeys int, fetchOwner bool) (InternalResult, error) {
	// TODO v2
	return InternalResult{}, nil
}

func (s *Storage) listInternal(ctx context.Context, bucket, prefix, startAt, delimiter string, maxKeys int, fetchOwner bool) (InternalResult, error) {
	// 1. Validate the bucket exists first
	// if !s.bucketExists(bucket) {
	// 	return InternalResult{}, ErrNoSuchBucket
	// }

	// 2. Formulate the scan range
	// Your DB 'Seek' will likely look for: bucket + "/" + startAt

	// 3. Execute the scan...
	// 4. Return the unified result
	return InternalResult{}, nil
}

type InternalResult struct {
	Objects        []s3types.Object
	CommonPrefixes []s3types.CommonPrefix
	IsTruncated    bool
	NextMarker     string // Use this as NextMarker (V1) or NextContinuationToken (V2)
}

// walkDir recursively walks a directory in the filesystem
func (s *Storage) walkDir(ctx context.Context, bucket, dir string, fn func(key string, info fs.FileInfo) error) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled during walk: %w", err)
	}

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
		// Check for context cancellation in each iteration
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled during walk iteration: %w", err)
		}

		var entryPath string
		if dir == "" {
			entryPath = entry.Name()
		} else {
			entryPath = filepath.Join(dir, entry.Name())
		}

		if entry.IsDir() {
			// Recursively walk subdirectory
			if err := s.walkDir(ctx, bucket, entryPath, fn); err != nil {
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
