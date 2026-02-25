package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// fetchBucketOwner retrieves owner information for a bucket.
// Returns nil if owner info is unavailable or cannot be fetched.
func (s *Storage) fetchBucketOwner(ctx context.Context, bucket string) *s3types.Owner {
	bucketMeta, err := s.metadataManager.GetBucketMetadata(ctx, bucket)
	if err != nil || bucketMeta.Owner == nil {
		return nil
	}

	ownerStr := bucketMeta.Owner.String()
	displayName := ownerStr

	user, err := s.metadataManager.GetUser(ctx, *bucketMeta.Owner)
	if err == nil && user != nil && user.Username != "" {
		displayName = user.Username
	}

	return &s3types.Owner{
		ID:          ownerStr,
		DisplayName: displayName,
	}
}

// collectEntries walks the bucket directory and returns all matching object entries.
func (s *Storage) collectEntries(ctx context.Context, bucket, prefix, startAt, delimiter string, maxKeys int) ([]objectEntry, error) {
	var allEntries []objectEntry
	var walked int

	err := s.walkDir(ctx, bucket, "", func(key string, info fs.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled during walk: %w", err)
		}

		if prefix != "" && !hasPrefix(key, prefix) {
			return nil
		}

		if delimiter == "" && maxKeys > 0 {
			if startAt == "" || key > startAt {
				walked++
				if walked > maxKeys+1 {
					return errStopWalk
				}
			}
		}

		meta, err := s.metadataManager.GetObjectMetadata(ctx, bucket, key)
		if err != nil {
			meta = &metadata.ObjectMetadata{
				ContentType:  "application/octet-stream",
				Size:         info.Size(),
				LastModified: info.ModTime(),
			}
		}

		allEntries = append(allEntries, objectEntry{key: key, info: info, meta: meta})
		return nil
	})

	if err != nil && !errors.Is(err, errStopWalk) {
		return nil, err
	}

	return allEntries, nil
}

// groupEntriesByDelimiter classifies entries into regular objects and a common prefix map.
func groupEntriesByDelimiter(entries []objectEntry, prefix, delimiter string, fetchOwner bool, bucketOwner *s3types.Owner) (objectList []s3types.Object, prefixMap map[string]bool) {
	objects := make([]s3types.Object, 0, len(entries))
	commonPrefixMap := make(map[string]bool)

	for _, entry := range entries {
		if delimiter != "" {
			keyAfterPrefix := entry.key
			if prefix != "" {
				keyAfterPrefix = entry.key[len(prefix):]
			}

			if delimiterPos := strings.Index(keyAfterPrefix, delimiter); delimiterPos >= 0 {
				commonPrefixMap[prefix+keyAfterPrefix[:delimiterPos+len(delimiter)]] = true
				continue
			}
		}

		obj := s3types.Object{
			Key:          entry.key,
			Size:         entry.info.Size(),
			LastModified: entry.info.ModTime(),
			ETag:         entry.meta.ETag,
			StorageClass: "STANDARD",
		}
		if fetchOwner && bucketOwner != nil {
			obj.Owner = bucketOwner
		}

		objects = append(objects, obj)
	}

	return objects, commonPrefixMap
}

// filterObjectsByStartAt removes objects not lexicographically greater than startAt.
func filterObjectsByStartAt(objects []s3types.Object, startAt string) []s3types.Object {
	if startAt == "" {
		return objects
	}

	filtered := make([]s3types.Object, 0, len(objects))
	for _, obj := range objects {
		if obj.Key > startAt {
			filtered = append(filtered, obj)
		}
	}

	return filtered
}

// filterAndSortCommonPrefixes converts a common prefix map to a sorted slice, filtered by startAt.
func filterAndSortCommonPrefixes(commonPrefixMap map[string]bool, startAt string) []s3types.CommonPrefix {
	commonPrefixes := make([]s3types.CommonPrefix, 0, len(commonPrefixMap))
	for prefixKey := range commonPrefixMap {
		if startAt != "" && prefixKey <= startAt {
			continue
		}
		commonPrefixes = append(commonPrefixes, s3types.CommonPrefix{Prefix: prefixKey})
	}

	sortCommonPrefixes(commonPrefixes)

	return commonPrefixes
}

// applyMaxKeysLimit trims the combined objects and common prefixes to at most maxKeys entries,
// preserving lexicographic order across both slices.
func applyMaxKeysLimit(objects []s3types.Object, commonPrefixes []s3types.CommonPrefix, maxKeys int) ([]s3types.Object, []s3types.CommonPrefix) {
	if maxKeys <= 0 {
		return objects, commonPrefixes
	}

	type resultItem struct {
		key      string
		isPrefix bool
		index    int
	}

	allItems := make([]resultItem, 0, len(objects)+len(commonPrefixes))
	for i, obj := range objects {
		allItems = append(allItems, resultItem{key: obj.Key, isPrefix: false, index: i})
	}
	for i, cp := range commonPrefixes {
		allItems = append(allItems, resultItem{key: cp.Prefix, isPrefix: true, index: i})
	}

	sort.Slice(allItems, func(i, j int) bool {
		return allItems[i].key < allItems[j].key
	})

	if len(allItems) <= maxKeys {
		return objects, commonPrefixes
	}

	allItems = allItems[:maxKeys]
	newObjects := make([]s3types.Object, 0, maxKeys)
	newCommonPrefixes := make([]s3types.CommonPrefix, 0, maxKeys)

	for _, item := range allItems {
		if item.isPrefix {
			newCommonPrefixes = append(newCommonPrefixes, commonPrefixes[item.index])
		} else {
			newObjects = append(newObjects, objects[item.index])
		}
	}

	return newObjects, newCommonPrefixes
}

// buildListResult assembles the final InternalResult with truncation detection and next marker.
func buildListResult(objects []s3types.Object, commonPrefixes []s3types.CommonPrefix, maxKeys int) InternalResult {
	totalResults := len(objects) + len(commonPrefixes)
	isTruncated := maxKeys > 0 && totalResults >= maxKeys

	var nextMarker string
	if isTruncated {
		lastObjectKey := ""
		if len(objects) > 0 {
			lastObjectKey = objects[len(objects)-1].Key
		}
		lastPrefixKey := ""
		if len(commonPrefixes) > 0 {
			lastPrefixKey = commonPrefixes[len(commonPrefixes)-1].Prefix
		}
		nextMarker = max(lastObjectKey, lastPrefixKey)
	}

	return InternalResult{
		Objects:        objects,
		CommonPrefixes: commonPrefixes,
		IsTruncated:    isTruncated,
		NextMarker:     nextMarker,
	}
}
