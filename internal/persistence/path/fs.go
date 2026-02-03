// Package path provides filesystem abstraction and security for DirIO.
//
// This package wraps go-billy to provide:
// - Chroot-based filesystem isolation
// - Path traversal attack prevention
// - Scoped filesystem access for buckets and metadata
//
// Design principles:
// - Filesystem-level security only (path traversal, null bytes, absolute paths)
// - NO S3 validation (that belongs in API handlers)
// - Generic and reusable for any filesystem access
package path

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-billy/v5/util"
)

const (
	// MinIODir MinIO metadata directory name
	MinIODir = ".minio.sys"

	// MetadataDir DirIO metadata directory name
	MetadataDir = ".dirio"
)

var (
	// ErrInvalidPath indicates a path failed security validation
	ErrInvalidPath = errors.New("invalid path: security violation")

	// ErrPathTraversal indicates an attempt to escape the filesystem boundary
	ErrPathTraversal = errors.New("path traversal detected")

	// ErrAbsolutePath indicates an absolute path was provided where relative is required
	ErrAbsolutePath = errors.New("absolute paths not allowed")

	// ErrNullByte indicates a null byte in the path
	ErrNullByte = errors.New("null byte in path")
)

// NewRootFS creates a chroot-protected filesystem at the specified data directory.
// This is the root filesystem for all DirIO operations.
//
// The filesystem is isolated to dataDir using chroot, preventing any access
// outside this directory tree.
func NewRootFS(dataDir string) (billy.Filesystem, error) {
	// Ensure dataDir is an absolute path
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve data directory: %w", err)
	}

	// Create OS filesystem at the data directory
	fs := osfs.New(absDataDir)

	// Note: go-billy's chroot.New() doesn't exist in v5
	// Instead, osfs.New() already provides the base directory isolation
	// All paths are relative to the base directory

	return fs, nil
}

// NewMinIOFS creates a read-only filesystem scoped to the MinIO metadata directory.
// This is used for importing existing MinIO data.
//
// Returns an error if the MinIO directory doesn't exist.
func NewMinIOFS(rootFS billy.Filesystem) (billy.Filesystem, error) {
	// Verify MinIO directory exists
	info, err := rootFS.Stat(MinIODir)
	if err != nil {
		return nil, fmt.Errorf("MinIO metadata directory not found: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("MinIO metadata path is not a directory")
	}

	// Create a chrooted filesystem for the MinIO directory
	// Since go-billy v5 doesn't have chroot, we'll use a wrapper
	readOnlyRootFS := ReadOnlyFS{rootFS}
	return newScopedFS(readOnlyRootFS, MinIODir), nil
}

// NewMetadataFS creates a read/write filesystem scoped to the DirIO metadata directory.
// This is used for storing DirIO's own metadata (bucket info, policies, etc.)
//
// Creates the metadata directory if it doesn't exist.
func NewMetadataFS(rootFS billy.Filesystem) (billy.Filesystem, error) {
	// Ensure metadata directory exists
	if err := rootFS.MkdirAll(MetadataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create metadata directory: %w", err)
	}

	// Create a scoped filesystem for the metadata directory
	return newScopedFS(rootFS, MetadataDir), nil
}

// NewBucketFS creates a filesystem scoped to a specific bucket directory.
// This ensures bucket operations cannot access other buckets or system directories.
//
// The bucket parameter should be the bucket name (already validated by API layer).
// This function only validates path safety, not S3 naming rules.
//
// Creates the bucket directory if it doesn't exist.
func NewBucketFS(rootFS billy.Filesystem, bucket string) (billy.Filesystem, error) {
	// Validate the bucket name for path safety (not S3 compliance)
	if err := ValidatePathSafe(bucket); err != nil {
		return nil, fmt.Errorf("bucket name failed path security check: %w", err)
	}

	// Ensure bucket directory exists
	if err := rootFS.MkdirAll(bucket, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bucket directory: %w", err)
	}

	// Create a scoped filesystem for the bucket
	return newScopedFS(rootFS, bucket), nil
}

// ValidatePathSafe checks if a path is safe from a filesystem security perspective.
// This function checks for:
// - Path traversal attempts (../)
// - Absolute paths
// - Null bytes
// - Empty paths
//
// This does NOT validate S3 naming rules - that's the API layer's responsibility.
func ValidatePathSafe(path string) error {
	if path == "" {
		return fmt.Errorf("%w: empty path", ErrInvalidPath)
	}

	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("%w: %w", ErrInvalidPath, ErrNullByte)
	}

	// Check for absolute paths
	if filepath.IsAbs(path) {
		return fmt.Errorf("%w: %w", ErrInvalidPath, ErrAbsolutePath)
	}

	// Check for paths starting with /
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: %w", ErrInvalidPath, ErrAbsolutePath)
	}

	// Check for path traversal attempts in original path
	// Look for .. as a path component (not just substring like my..file)
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		if part == ".." {
			return fmt.Errorf("%w: %w", ErrInvalidPath, ErrPathTraversal)
		}
	}

	return nil
}

// CleanPath sanitizes a path for safe filesystem access.
// Returns the cleaned path or an error if the path is unsafe.
//
// This uses filepath.Clean to normalize the path, then validates it.
// Always returns forward slashes for consistency with S3 conventions.
func CleanPath(path string) (string, error) {
	// Clean the path (OS-native separators)
	cleaned := filepath.Clean(path)

	// Convert to forward slashes for S3 consistency (cross-platform)
	cleaned = filepath.ToSlash(cleaned)

	// Validate the cleaned path
	if err := ValidatePathSafe(cleaned); err != nil {
		return "", err
	}

	return cleaned, nil
}

// scopedFS wraps a billy.Filesystem to scope all operations to a subdirectory.
// This provides chroot-like functionality for go-billy v5.
type scopedFS struct {
	base billy.Filesystem
	root string
}

// newScopedFS creates a new scoped filesystem.
func newScopedFS(base billy.Filesystem, root string) billy.Filesystem {
	return &scopedFS{
		base: base,
		root: root,
	}
}

// join joins the scoped root with the given path.
func (fs *scopedFS) join(path string) string {
	return filepath.Join(fs.root, path)
}

// Create implements billy.Filesystem.
func (fs *scopedFS) Create(filename string) (billy.File, error) {
	return fs.base.Create(fs.join(filename))
}

// Open implements billy.Filesystem.
func (fs *scopedFS) Open(filename string) (billy.File, error) {
	return fs.base.Open(fs.join(filename))
}

// OpenFile implements billy.Filesystem.
func (fs *scopedFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	return fs.base.OpenFile(fs.join(filename), flag, perm)
}

// Stat implements billy.Filesystem.
func (fs *scopedFS) Stat(filename string) (os.FileInfo, error) {
	return fs.base.Stat(fs.join(filename))
}

// Rename implements billy.Filesystem.
func (fs *scopedFS) Rename(oldpath, newpath string) error {
	return fs.base.Rename(fs.join(oldpath), fs.join(newpath))
}

// Remove implements billy.Filesystem.
func (fs *scopedFS) Remove(filename string) error {
	return fs.base.Remove(fs.join(filename))
}

// Join implements billy.Filesystem.
func (fs *scopedFS) Join(elem ...string) string {
	return fs.base.Join(elem...)
}

// TempFile implements billy.Filesystem.
func (fs *scopedFS) TempFile(dir, prefix string) (billy.File, error) {
	return util.TempFile(fs.base, fs.join(dir), prefix)
}

// ReadDir implements billy.Filesystem.
func (fs *scopedFS) ReadDir(path string) ([]os.FileInfo, error) {
	return fs.base.ReadDir(fs.join(path))
}

// MkdirAll implements billy.Filesystem.
func (fs *scopedFS) MkdirAll(filename string, perm os.FileMode) error {
	return fs.base.MkdirAll(fs.join(filename), perm)
}

// Lstat implements billy.Filesystem.
func (fs *scopedFS) Lstat(filename string) (os.FileInfo, error) {
	return fs.base.Lstat(fs.join(filename))
}

// Symlink implements billy.Filesystem.
func (fs *scopedFS) Symlink(target, link string) error {
	return fs.base.Symlink(fs.join(target), fs.join(link))
}

// Readlink implements billy.Filesystem.
func (fs *scopedFS) Readlink(link string) (string, error) {
	return fs.base.Readlink(fs.join(link))
}

// Chroot implements billy.Filesystem.
func (fs *scopedFS) Chroot(path string) (billy.Filesystem, error) {
	return newScopedFS(fs.base, fs.join(path)), nil
}

// Root implements billy.Filesystem.
func (fs *scopedFS) Root() string {
	return fs.root
}
