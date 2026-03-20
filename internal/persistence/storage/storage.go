package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"

	"github.com/go-git/go-billy/v5"

	"github.com/mallardduck/dirio/internal/consts"

	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/persistence/path"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// Use s3types errors for consistency across all layers
var (
	ErrBucketExists   = s3types.ErrBucketAlreadyExists
	ErrNoSuchBucket   = s3types.ErrBucketNotFound
	ErrBucketNotEmpty = s3types.ErrBucketNotEmpty
	ErrNoSuchKey      = s3types.ErrObjectNotFound
)

// errStopWalk is a sentinel returned by the walk callback to signal early termination.
// It is not a real error; listInternal treats it as a normal (non-error) completion.
var errStopWalk = errors.New("stop walk")

// Storage handles filesystem operations for buckets and objects
type Storage struct {
	rootFS          billy.Filesystem
	metadataManager *metadata.Manager
	log             *slog.Logger
	// keyMutexes provides per-key serialization for the atomic rename+metadata
	// step of PutObject.  On Windows, concurrent renames to the same destination
	// path race at the OS level; serializing only this fast final step keeps I/O
	// fully parallel while preventing "Access is denied" failures.
	keyMutexes sync.Map // map[string]*sync.Mutex
}

// New creates a new storage backend
func New(rootFS billy.Filesystem, metadataManager *metadata.Manager) (*Storage, error) {
	if rootFS == nil {
		return nil, fmt.Errorf("rootFS cannot be nil")
	}

	return &Storage{
		rootFS:          rootFS,
		metadataManager: metadataManager,
		log:             logging.Component("storage"),
	}, nil
}

// GetBucketFS returns a billy.Filesystem for the specified bucket
func (s *Storage) GetBucketFS(ctx context.Context, bucket string) (billy.Filesystem, error) {
	return path.NewBucketFS(s.rootFS, bucket)
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

		// Skip metadataManager directories
		name := entry.Name()
		if name == consts.MinioMetadataDir || name == consts.DirIOMetadataDir || name[0] == '.' {
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
	if err := s.rootFS.MkdirAll(bucket, 0o755); err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	// Create bucket metadataManager
	if err := s.metadataManager.CreateBucket(ctx, bucket); err != nil {
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

	// Remove bucket metadataManager
	return s.metadataManager.DeleteBucket(ctx, bucket)
}

// ListObjects returns objects in a bucket with optional prefix, marker, and delimiter (V1 API)
func (s *Storage) ListObjects(ctx context.Context, bucket, prefix, marker, delimiter string, maxKeys int) (InternalResult, error) {
	return s.listInternal(ctx, bucket, prefix, marker, delimiter, maxKeys, false)
}

// ListObjectsV2 returns objects in a bucket with optional prefix and delimiter (V2 API)
// The fetchOwner parameter determines whether to include owner information in each object.
// Per S3 spec, owner is NOT included by default - set fetchOwner=true to include it.
func (s *Storage) ListObjectsV2(ctx context.Context, bucket, prefix, continuationToken, startAfter, delimiter string, maxKeys int, fetchOwner bool) (InternalResult, error) {
	// V2 uses either continuation-token or start-after for pagination
	startAt := continuationToken
	if startAt == "" {
		startAt = startAfter
	}

	return s.listInternal(ctx, bucket, prefix, startAt, delimiter, maxKeys, fetchOwner)
}

// listInternal is the core listing implementation used by both V1 and V2
func (s *Storage) listInternal(ctx context.Context, bucket, prefix, startAt, delimiter string, maxKeys int, fetchOwner bool) (InternalResult, error) {
	if err := ctx.Err(); err != nil {
		return InternalResult{}, fmt.Errorf("context cancelled: %w", err)
	}

	if _, err := s.rootFS.Stat(bucket); err != nil {
		if isNotExist(err) {
			return InternalResult{}, ErrNoSuchBucket
		}
		return InternalResult{}, err
	}

	var bucketOwner *s3types.Owner
	if fetchOwner {
		bucketOwner = s.fetchBucketOwner(ctx, bucket)
	}

	allEntries, err := s.collectEntries(ctx, bucket, prefix, startAt, delimiter, maxKeys)
	if err != nil {
		return InternalResult{}, err
	}

	sortObjectEntries(allEntries)

	s.log.Debug("listInternal processing",
		"bucket", bucket,
		"prefix", prefix,
		"delimiter", delimiter,
		"startAt", startAt,
		"entryCount", len(allEntries))

	objects, commonPrefixMap := groupEntriesByDelimiter(allEntries, prefix, delimiter, fetchOwner, bucketOwner)
	objects = filterObjectsByStartAt(objects, startAt)
	commonPrefixes := filterAndSortCommonPrefixes(commonPrefixMap, startAt)

	s.log.Debug("listInternal after grouping and filtering",
		"objectCount", len(objects),
		"commonPrefixCount", len(commonPrefixes),
		"commonPrefixMapSize", len(commonPrefixMap))

	objects, commonPrefixes = applyMaxKeysLimit(objects, commonPrefixes, maxKeys)
	result := buildListResult(objects, commonPrefixes, maxKeys)

	s.log.Debug("listInternal returning results",
		"objectCount", len(result.Objects),
		"commonPrefixCount", len(result.CommonPrefixes),
		"isTruncated", result.IsTruncated,
		"nextMarker", result.NextMarker)

	return result, nil
}

// Type aliases for clarity
type (
	Prefix            string
	Delimiter         string
	Marker            string
	ContinuationToken string
)

// objectEntry represents an object during the listing process
type objectEntry struct {
	key  string
	info fs.FileInfo
	meta *metadata.ObjectMetadata
}

// InternalResult contains the unified listing result used by both V1 and V2
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

// getKeyMu returns the per-key mutex for bucket+key, creating it on first use.
// The mutex serializes the rename-and-metadata step inside PutObject so that
// concurrent writes to the same key do not race at the OS level.
func (s *Storage) getKeyMu(bucket, key string) *sync.Mutex {
	k := bucket + "\x00" + key
	v, _ := s.keyMutexes.LoadOrStore(k, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// isNotExist checks if an error is a "not exist" error
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// sortObjectEntries sorts object entries by key in lexicographic order
func sortObjectEntries(entries []objectEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})
}

// sortCommonPrefixes sorts common prefixes in lexicographic order
func sortCommonPrefixes(prefixes []s3types.CommonPrefix) {
	sort.Slice(prefixes, func(i, j int) bool {
		return prefixes[i].Prefix < prefixes[j].Prefix
	})
}
