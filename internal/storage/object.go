package storage

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/path"
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
func (s *Storage) GetObject(bucket, key string) (*Object, error) {
	// Check bucket exists
	if exists, err := s.BucketExists(bucket); err != nil {
		return nil, err
	} else if !exists {
		return nil, ErrNoSuchBucket
	}

	// Validate key for path safety
	if err := path.ValidatePathSafe(key); err != nil {
		return nil, fmt.Errorf("invalid key: %w", err)
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
	meta, err := s.metadata.GetObjectMetadata(bucket, key)
	if err != nil {
		// If no metadata, create basic metadata
		etag := s.calculateETag(bucketFS, objectPath)
		meta = &metadata.ObjectMetadata{
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
func (s *Storage) PutObject(bucket, key string, content io.Reader, contentType string, customMetadata map[string]string) (string, error) {
	// Check bucket exists
	if exists, err := s.BucketExists(bucket); err != nil {
		return "", err
	} else if !exists {
		return "", ErrNoSuchBucket
	}

	// Validate key for path safety
	if err := path.ValidatePathSafe(key); err != nil {
		return "", fmt.Errorf("invalid key: %w", err)
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

	// Create temporary file in the same directory for atomic rename
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
	defer bucketFS.Remove(tmpPath) // Cleanup on failure

	// Calculate MD5 hash while writing
	hash := md5.New()
	multiWriter := io.MultiWriter(tmpFile, hash)

	// Copy content to temp file
	size, err := io.Copy(multiWriter, content)
	if err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write object: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	// Calculate ETag (MD5 hash)
	etag := `"` + hex.EncodeToString(hash.Sum(nil)) + `"`

	// Atomically rename temp file to final location
	if err := bucketFS.Rename(tmpPath, objectPath); err != nil {
		return "", fmt.Errorf("failed to rename object: %w", err)
	}

	// Store object metadata
	meta := &metadata.ObjectMetadata{
		ContentType:    contentType,
		Size:           size,
		ETag:           etag,
		LastModified:   time.Now(),
		CustomMetadata: customMetadata,
	}
	if err := s.metadata.PutObjectMetadata(bucket, key, meta); err != nil {
		// Log error but don't fail the operation
		// Metadata can be regenerated if needed
		s.log.Warn("failed to save object metadata", "bucket", bucket, "key", key, "error", err)
	}

	return etag, nil
}

// GetObjectMetadata retrieves metadata for an object
func (s *Storage) GetObjectMetadata(bucket, key string) (*metadata.ObjectMetadata, error) {
	// Check bucket exists
	if exists, err := s.BucketExists(bucket); err != nil {
		return nil, err
	} else if !exists {
		return nil, ErrNoSuchBucket
	}

	// Validate key for path safety
	if err := path.ValidatePathSafe(key); err != nil {
		return nil, fmt.Errorf("invalid key: %w", err)
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
	meta, err := s.metadata.GetObjectMetadata(bucket, key)
	if err != nil {
		// If no metadata, create basic metadata
		etag := s.calculateETag(bucketFS, objectPath)
		meta = &metadata.ObjectMetadata{
			ContentType:  "application/octet-stream",
			Size:         info.Size(),
			LastModified: info.ModTime(),
			ETag:         etag,
		}
	}

	return meta, nil
}

// DeleteObject removes an object
func (s *Storage) DeleteObject(bucket, key string) error {
	// Check bucket exists
	if exists, err := s.BucketExists(bucket); err != nil {
		return err
	} else if !exists {
		return ErrNoSuchBucket
	}

	// Validate key for path safety
	if err := path.ValidatePathSafe(key); err != nil {
		return fmt.Errorf("invalid key: %w", err)
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
	s.metadata.DeleteObjectMetadata(bucket, key)

	// Clean up empty parent directories
	s.cleanupEmptyDirs(bucketFS, filepath.Dir(objectPath))

	return nil
}

// cleanupEmptyDirs removes empty directories up to the bucket root
func (s *Storage) cleanupEmptyDirs(bucketFS billy.Filesystem, dir string) {
	for dir != "." && dir != "" && dir != "/" {
		entries, err := bucketFS.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		bucketFS.Remove(dir)
		dir = filepath.Dir(dir)
	}
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
