package storage

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"

	contextInt "github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/persistence/path"
	"github.com/mallardduck/dirio/pkg/iam"
)

// Object represents an S3 object with content
type Object struct {
	Key            string
	Size           int64
	ContentType    string
	ETag           string
	LastModified   time.Time
	Content        io.ReadCloser
	CustomMetadata map[string]string // Custom headers like Cache-Control, Content-Disposition, x-amz-meta-*, etc.
}

// GetObject retrieves an object from storage
func (s *Storage) GetObject(ctx context.Context, bucket, key string) (*Object, error) {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	// Check bucket exists
	if exists, err := s.BucketExists(ctx, bucket); err != nil {
		return nil, err
	} else if !exists {
		return nil, ErrNoSuchBucket
	}

	// Validate key for path safety
	if err := path.ValidatePathSafe(key); err != nil {
		return nil, fmt.Errorf("invalid key: %w", err)
	}

	// Check context before proceeding
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled before opening: %w", err)
	}

	// Get bucket filesystem
	bucketFS, err := path.NewBucketFS(s.rootFS, bucket)
	if err != nil {
		return nil, err
	}

	// Convert S3 key to filesystem path
	objectPath := filepath.FromSlash(key)

	// Check if object exists
	info, err := bucketFS.Stat(objectPath)
	if err != nil {
		if isNotExist(err) {
			return nil, ErrNoSuchKey
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrNoSuchKey
	}

	// Open file for reading
	file, err := bucketFS.Open(objectPath)
	if err != nil {
		return nil, err
	}

	// Get object metadata
	meta, err := s.metadata.GetObjectMetadata(ctx, bucket, key)
	if err != nil {
		// If no metadata, create basic metadata
		etag := s.calculateETag(bucketFS, objectPath)
		meta = &metadata.ObjectMetadata{
			Version:      metadata.ObjectMetadataVersion,
			ContentType:  "application/octet-stream",
			Size:         info.Size(),
			LastModified: info.ModTime(),
			ETag:         etag,
		}
	}

	return &Object{
		Key:            key,
		Size:           info.Size(),
		ContentType:    meta.ContentType,
		ETag:           meta.ETag,
		LastModified:   info.ModTime(),
		Content:        file,
		CustomMetadata: meta.CustomMetadata,
	}, nil
}

// PutObject stores an object
func (s *Storage) PutObject(ctx context.Context, bucket, key string, content io.Reader, contentType string, customMetadata map[string]string) (string, error) {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context cancelled: %w", err)
	}

	// Check bucket exists
	if exists, err := s.BucketExists(ctx, bucket); err != nil {
		return "", err
	} else if !exists {
		return "", ErrNoSuchBucket
	}

	// Validate key for path safety
	if err := path.ValidatePathSafe(key); err != nil {
		return "", fmt.Errorf("invalid key: %w", err)
	}

	// Check context before proceeding
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context cancelled before writing: %w", err)
	}

	// Get bucket filesystem
	bucketFS, err := path.NewBucketFS(s.rootFS, bucket)
	if err != nil {
		return "", err
	}

	// Convert S3 key to filesystem path
	objectPath := filepath.FromSlash(key)

	// Create parent directories
	dir := filepath.Dir(objectPath)
	if dir != "." && dir != "" {
		if err := bucketFS.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Create a temporary file in the same directory for atomic rename
	// Use a generated name instead of TempFile to avoid path issues with scoped filesystems
	tmpName := fmt.Sprintf(".tmp-%d", time.Now().UnixNano())
	tmpPath := filepath.Join(dir, tmpName)
	if dir == "." || dir == "" {
		tmpPath = tmpName
	}

	tmpFile, err := bucketFS.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	// Calculate MD5 hash while writing
	hash := md5.New()
	multiWriter := io.MultiWriter(tmpFile, hash)

	// Create a context-aware reader that checks for cancellation
	type contextReader struct {
		ctx context.Context
		r   io.Reader
	}
	ctxReader := &contextReader{ctx: ctx, r: content}

	// Copy content to temp file with context checking
	size, err := io.Copy(multiWriter, io.LimitReader(
		readerFunc(func(p []byte) (int, error) {
			// Check for context cancellation during read
			if err := ctxReader.ctx.Err(); err != nil {
				return 0, fmt.Errorf("context cancelled during write: %w", err)
			}
			return ctxReader.r.Read(p)
		}),
		1<<63-1, // max int64
	))
	if err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write object: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	// Calculate ETag (MD5 hash)
	etag := hex.EncodeToString(hash.Sum(nil))

	// Check context before finalizing
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context cancelled before finalizing: %w", err)
	}

	// Atomically rename the temp file to the final location
	if err := bucketFS.Rename(tmpPath, objectPath); err != nil {
		return "", fmt.Errorf("failed to rename object: %w", err)
	}

	// Get user from context for ownership tracking
	// Golden Rule: Admin as creator → nil (implicit access), regular user → UUID (explicit ownership)
	var ownerUUID *uuid.UUID
	if user, err := contextInt.GetUser(ctx); err == nil && user != nil {
		// Only set owner UUID if NOT admin (admin access is implicit, not ownership)
		if user.UUID != iam.AdminUserUUID {
			ownerUUID = &user.UUID
		}
	}
	// Note: nil owner = admin-only access (if created by admin) or anonymous (if no user context)

	// Store object metadata
	meta := &metadata.ObjectMetadata{
		Version:        metadata.ObjectMetadataVersion,
		Owner:          ownerUUID, // nil for admin, UUID pointer for regular user
		ContentType:    contentType,
		Size:           size,
		ETag:           etag,
		LastModified:   time.Now(),
		CustomMetadata: customMetadata,
	}
	if err := s.metadata.PutObjectMetadata(ctx, bucket, key, meta); err != nil {
		// If custom metadata was provided, fail the operation since user's explicit request cannot be fulfilled
		// Basic metadata (ContentType, Size, ETag) can be regenerated if needed
		if len(customMetadata) > 0 {
			// Clean up the object file to maintain atomicity (S3 API contract: PutObject succeeds completely or not at all)
			// This prevents orphaned objects with missing custom metadata
			if removeErr := bucketFS.Remove(objectPath); removeErr != nil {
				s.log.Error("failed to remove object after metadata save failure - object may be orphaned",
					"bucket", bucket, "key", key, "metadata_error", err, "cleanup_error", removeErr)
				return "", fmt.Errorf("failed to save object metadata and failed to cleanup object file: metadata error: %w, cleanup error: %v", err, removeErr)
			}
			return "", fmt.Errorf("failed to save custom metadata (object upload rolled back to maintain consistency): %w", err)
		}
		// Just log warning for basic metadata (can be regenerated)
		s.log.Warn("failed to save object metadata", "bucket", bucket, "key", key, "error", err)
	}

	return etag, nil
}

// readerFunc is a helper type to create an io.Reader from a function
type readerFunc func([]byte) (int, error)

func (rf readerFunc) Read(p []byte) (int, error) {
	return rf(p)
}

// GetObjectMetadata retrieves metadata for an object
func (s *Storage) GetObjectMetadata(ctx context.Context, bucket, key string) (*metadata.ObjectMetadata, error) {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	// Check bucket exists
	if exists, err := s.BucketExists(ctx, bucket); err != nil {
		return nil, err
	} else if !exists {
		return nil, ErrNoSuchBucket
	}

	// Validate key for path safety
	if err := path.ValidatePathSafe(key); err != nil {
		return nil, fmt.Errorf("invalid key: %w", err)
	}

	// Check context before proceeding
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled before reading metadata: %w", err)
	}

	// Get bucket filesystem
	bucketFS, err := path.NewBucketFS(s.rootFS, bucket)
	if err != nil {
		return nil, err
	}

	// Convert S3 key to filesystem path
	objectPath := filepath.FromSlash(key)

	// Check if object exists
	info, err := bucketFS.Stat(objectPath)
	if err != nil {
		if isNotExist(err) {
			return nil, ErrNoSuchKey
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrNoSuchKey
	}

	// Try to get metadata from metadata store
	meta, err := s.metadata.GetObjectMetadata(ctx, bucket, key)
	if err != nil {
		// If no metadata, create basic metadata
		etag := s.calculateETag(bucketFS, objectPath)
		meta = &metadata.ObjectMetadata{
			Version:      metadata.ObjectMetadataVersion,
			ContentType:  "application/octet-stream",
			Size:         info.Size(),
			LastModified: info.ModTime(),
			ETag:         etag,
		}
	}

	return meta, nil
}

// DeleteObject removes an object
func (s *Storage) DeleteObject(ctx context.Context, bucket, key string) error {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	// Check bucket exists
	if exists, err := s.BucketExists(ctx, bucket); err != nil {
		return err
	} else if !exists {
		return ErrNoSuchBucket
	}

	// Validate key for path safety
	if err := path.ValidatePathSafe(key); err != nil {
		return fmt.Errorf("invalid key: %w", err)
	}

	// Check context before proceeding
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled before deletion: %w", err)
	}

	// Get bucket filesystem
	bucketFS, err := path.NewBucketFS(s.rootFS, bucket)
	if err != nil {
		return err
	}

	// Convert S3 key to filesystem path
	objectPath := filepath.FromSlash(key)

	// Check if object exists
	info, err := bucketFS.Stat(objectPath)
	if err != nil {
		if isNotExist(err) {
			return ErrNoSuchKey
		}
		return err
	}
	if info.IsDir() {
		return ErrNoSuchKey
	}

	// Delete the file
	if err := bucketFS.Remove(objectPath); err != nil {
		return err
	}

	// Delete metadata
	if err := s.metadata.DeleteObjectMetadata(ctx, bucket, key); err != nil {
		s.log.Error("failed to delete object metadata", "bucket", bucket, "key", key, "error", err)
	}

	// Clean up empty parent directories
	return s.cleanupEmptyDirs(bucketFS, filepath.Dir(objectPath))
}

// cleanupEmptyDirs removes empty directories up to the bucket root
func (s *Storage) cleanupEmptyDirs(bucketFS billy.Filesystem, dir string) error {
	for dir != "." && dir != "" && dir != "/" {
		entries, err := bucketFS.ReadDir(dir)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			break
		}
		if err := bucketFS.Remove(dir); err != nil {
			return err
		}
		dir = filepath.Dir(dir)
	}

	return nil
}

// calculateETag computes the ETag for a file
func (s *Storage) calculateETag(bucketFS billy.Filesystem, path string) string {
	file, err := bucketFS.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}

	return `"` + hex.EncodeToString(hash.Sum(nil)) + `"`
}
