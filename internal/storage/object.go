package storage

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mallardduck/dirio/internal/metadata"
)

// Object represents an S3 object with content
type Object struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
	Content      io.ReadCloser
}

// GetObject retrieves an object from storage
func (s *Storage) GetObject(bucket, key string) (*Object, error) {
	// Check bucket exists
	if exists, err := s.BucketExists(bucket); err != nil {
		return nil, err
	} else if !exists {
		return nil, ErrNoSuchBucket
	}

	objectPath := s.objectPath(bucket, key)

	// Check if object exists
	info, err := os.Stat(objectPath)
	if os.IsNotExist(err) {
		return nil, ErrNoSuchKey
	}
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrNoSuchKey
	}

	// Open file for reading
	file, err := os.Open(objectPath)
	if err != nil {
		return nil, err
	}

	// Get object metadata
	meta, err := s.metadata.GetObjectMetadata(bucket, key)
	if err != nil {
		// If no metadata, create basic metadata
		meta = &metadata.ObjectMetadata{
			ContentType:  "application/octet-stream",
			Size:         info.Size(),
			LastModified: info.ModTime(),
			ETag:         calculateETag(objectPath),
		}
	}

	return &Object{
		Key:          key,
		Size:         info.Size(),
		ContentType:  meta.ContentType,
		ETag:         meta.ETag,
		LastModified: info.ModTime(),
		Content:      file,
	}, nil
}

// PutObject stores an object
func (s *Storage) PutObject(bucket, key string, content io.Reader, contentType string) (string, error) {
	// Check bucket exists
	if exists, err := s.BucketExists(bucket); err != nil {
		return "", err
	} else if !exists {
		return "", ErrNoSuchBucket
	}

	objectPath := s.objectPath(bucket, key)

	// Create parent directories
	dir := filepath.Dir(objectPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // Cleanup on failure

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
	if err := os.Rename(tmpPath, objectPath); err != nil {
		return "", fmt.Errorf("failed to rename object: %w", err)
	}

	// Store object metadata
	meta := &metadata.ObjectMetadata{
		ContentType:  contentType,
		Size:         size,
		ETag:         etag,
		LastModified: time.Now(),
	}
	if err := s.metadata.PutObjectMetadata(bucket, key, meta); err != nil {
		// Log error but don't fail the operation
		// Metadata can be regenerated if needed
		fmt.Printf("Warning: failed to save object metadata: %v\n", err)
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

	objectPath := s.objectPath(bucket, key)

	// Check if object exists
	info, err := os.Stat(objectPath)
	if os.IsNotExist(err) {
		return nil, ErrNoSuchKey
	}
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrNoSuchKey
	}

	// Try to get metadata from metadata store
	meta, err := s.metadata.GetObjectMetadata(bucket, key)
	if err != nil {
		// If no metadata, create basic metadata
		meta = &metadata.ObjectMetadata{
			ContentType:  "application/octet-stream",
			Size:         info.Size(),
			LastModified: info.ModTime(),
			ETag:         calculateETag(objectPath),
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

	objectPath := s.objectPath(bucket, key)

	// Check if object exists
	info, err := os.Stat(objectPath)
	if os.IsNotExist(err) {
		return ErrNoSuchKey
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return ErrNoSuchKey
	}

	// Delete the file
	if err := os.Remove(objectPath); err != nil {
		return err
	}

	// Delete metadata
	s.metadata.DeleteObjectMetadata(bucket, key)

	// Clean up empty parent directories
	s.cleanupEmptyDirs(filepath.Dir(objectPath), s.bucketPath(bucket))

	return nil
}

// cleanupEmptyDirs removes empty directories up to the bucket root
func (s *Storage) cleanupEmptyDirs(dir, stopAt string) {
	for dir != stopAt {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

// calculateETag computes the ETag for a file
func calculateETag(path string) string {
	file, err := os.Open(path)
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
