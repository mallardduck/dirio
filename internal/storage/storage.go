package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

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

// ListObjects returns objects in a bucket with optional prefix and delimiter (V1 API)
func (s *Storage) ListObjects(ctx context.Context, bucket, prefix, delimiter string, maxKeys int) ([]s3types.Object, error) {
	result, err := s.listInternal(ctx, bucket, prefix, "", delimiter, maxKeys, false)
	if err != nil {
		return nil, err
	}
	return result.Objects, nil
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
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return InternalResult{}, fmt.Errorf("context cancelled: %w", err)
	}

	// Check if bucket exists
	if _, err := s.rootFS.Stat(bucket); err != nil {
		if isNotExist(err) {
			return InternalResult{}, ErrNoSuchBucket
		}
		return InternalResult{}, err
	}

	// Get bucket owner if fetchOwner is requested
	// Per S3 behavior: Owner info is only included when explicitly requested via fetchOwner=true
	// This is used by ListObjectsV2 but not ListObjects (V1 always omits owner)
	var bucketOwner *s3types.Owner
	if fetchOwner {
		bucketMeta, err := s.metadata.GetBucketMetadata(ctx, bucket)
		if err == nil && bucketMeta.Owner != "" {
			bucketOwner = &s3types.Owner{
				ID:          bucketMeta.Owner,
				DisplayName: bucketMeta.Owner,
			}
		}
		// If we can't get bucket metadata, that's ok - owner will be nil
	}

	// Collect all matching objects
	var allEntries []objectEntry

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

		allEntries = append(allEntries, objectEntry{
			key:  key,
			info: info,
			meta: meta,
		})

		return nil
	})

	if err != nil {
		return InternalResult{}, err
	}

	// Sort entries by key (S3 returns results in lexicographic order)
	sortObjectEntries(allEntries)

	// Process delimiter to create common prefixes
	// Step 1: Group all entries into either objects or common prefixes
	var objects []s3types.Object
	commonPrefixMap := make(map[string]bool)

	s.log.Debug("listInternal processing",
		"bucket", bucket,
		"prefix", prefix,
		"delimiter", delimiter,
		"startAt", startAt,
		"entryCount", len(allEntries))

	for _, entry := range allEntries {
		// Handle delimiter logic first
		if delimiter != "" {
			// Find delimiter position after the prefix
			keyAfterPrefix := entry.key
			if prefix != "" {
				keyAfterPrefix = entry.key[len(prefix):]
			}

			delimiterPos := strings.Index(keyAfterPrefix, delimiter)
			s.log.Debug("checking entry for delimiter",
				"key", entry.key,
				"keyAfterPrefix", keyAfterPrefix,
				"delimiter", delimiter,
				"delimiterPos", delimiterPos)
			if delimiterPos >= 0 {
				// This key contains delimiter - add to common prefixes
				commonPrefix := prefix + keyAfterPrefix[:delimiterPos+len(delimiter)]
				s.log.Debug("adding to common prefixes",
					"key", entry.key,
					"commonPrefix", commonPrefix)
				commonPrefixMap[commonPrefix] = true
				continue
			}
		}

		// Add as regular object (no delimiter found or no delimiter specified)
		obj := s3types.Object{
			Key:          entry.key,
			Size:         entry.info.Size(),
			LastModified: entry.info.ModTime(),
			ETag:         entry.meta.ETag,
			StorageClass: "STANDARD",
		}

		// Include owner if fetchOwner is true
		if fetchOwner && bucketOwner != nil {
			obj.Owner = bucketOwner
		}

		objects = append(objects, obj)
	}

	// Step 2: Filter objects by startAt marker
	// Per S3 spec: skip objects not lexicographically greater than StartAfter
	if startAt != "" {
		filteredObjects := make([]s3types.Object, 0, len(objects))
		for _, obj := range objects {
			if obj.Key > startAt {
				filteredObjects = append(filteredObjects, obj)
			}
		}
		objects = filteredObjects
	}

	// Step 3: Convert common prefix map to sorted slice and filter by startAt
	// Per S3 spec: CommonPrefixes is filtered out if not lexicographically greater than StartAfter
	var commonPrefixes []s3types.CommonPrefix
	for prefixKey := range commonPrefixMap {
		if startAt != "" && prefixKey <= startAt {
			continue
		}
		commonPrefixes = append(commonPrefixes, s3types.CommonPrefix{
			Prefix: prefixKey,
		})
	}
	sortCommonPrefixes(commonPrefixes)

	s.log.Debug("listInternal after grouping and filtering",
		"objectCount", len(objects),
		"commonPrefixCount", len(commonPrefixes),
		"commonPrefixMapSize", len(commonPrefixMap))

	// Step 4: Apply maxKeys limit across both objects and common prefixes
	// Per S3 spec: "each common prefix counts as a single return when calculating the number of returns"
	if maxKeys > 0 {
		// Merge objects and prefixes into a single sorted list for limiting
		type resultItem struct {
			key      string
			isPrefix bool
			index    int // index in original slice
		}

		allItems := make([]resultItem, 0, len(objects)+len(commonPrefixes))
		for i, obj := range objects {
			allItems = append(allItems, resultItem{key: obj.Key, isPrefix: false, index: i})
		}
		for i, cp := range commonPrefixes {
			allItems = append(allItems, resultItem{key: cp.Prefix, isPrefix: true, index: i})
		}

		// Sort by key to maintain lexicographic order
		sort.Slice(allItems, func(i, j int) bool {
			return allItems[i].key < allItems[j].key
		})

		// Limit to maxKeys items
		if len(allItems) > maxKeys {
			allItems = allItems[:maxKeys]

			// Rebuild objects and commonPrefixes from limited set
			newObjects := make([]s3types.Object, 0, maxKeys)
			newCommonPrefixes := make([]s3types.CommonPrefix, 0, maxKeys)

			for _, item := range allItems {
				if item.isPrefix {
					newCommonPrefixes = append(newCommonPrefixes, commonPrefixes[item.index])
				} else {
					newObjects = append(newObjects, objects[item.index])
				}
			}

			objects = newObjects
			commonPrefixes = newCommonPrefixes
		}
	}

	// Determine if results are truncated
	totalResults := len(objects) + len(commonPrefixes)
	isTruncated := maxKeys > 0 && totalResults >= maxKeys

	// Determine next marker (last key returned, could be object or prefix)
	var nextMarker string
	if isTruncated {
		// Find the last item lexicographically (could be an object or common prefix)
		lastObjectKey := ""
		if len(objects) > 0 {
			lastObjectKey = objects[len(objects)-1].Key
		}
		lastPrefixKey := ""
		if len(commonPrefixes) > 0 {
			lastPrefixKey = commonPrefixes[len(commonPrefixes)-1].Prefix
		}

		// Use the lexicographically greater of the two as the next marker
		if lastObjectKey > lastPrefixKey {
			nextMarker = lastObjectKey
		} else {
			nextMarker = lastPrefixKey
		}
	}

	s.log.Debug("listInternal returning results",
		"objectCount", len(objects),
		"commonPrefixCount", len(commonPrefixes),
		"isTruncated", isTruncated,
		"nextMarker", nextMarker)

	return InternalResult{
		Objects:        objects,
		CommonPrefixes: commonPrefixes,
		IsTruncated:    isTruncated,
		NextMarker:     nextMarker,
	}, nil
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

// isNotExist checks if an error is a "not exist" error
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// indexOf returns the index of the first occurrence of substr in s, or -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
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
