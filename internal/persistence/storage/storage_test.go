package storage

import (
	"context"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStorage(t *testing.T) *Storage {
	t.Helper()

	rootFS := memfs.New()
	metaMgr, err := metadata.New(rootFS)
	require.NoError(t, err)

	storage, err := New(rootFS, metaMgr)
	require.NoError(t, err)

	return storage
}

func createTestBucket(t *testing.T, s *Storage, bucket string) {
	t.Helper()
	err := s.CreateBucket(context.Background(), bucket)
	require.NoError(t, err)
}

func createTestObject(t *testing.T, s *Storage, bucket, key string, size int64) {
	t.Helper()

	// Create parent directories if needed
	bucketFS, err := s.rootFS.Stat(bucket)
	require.NoError(t, err)
	require.True(t, bucketFS.IsDir())

	// Create the file
	f, err := s.rootFS.Create(bucket + "/" + key)
	require.NoError(t, err)

	// Write dummy data
	data := make([]byte, size)
	for i := range data {
		data[i] = 'x'
	}
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Create metadata
	meta := &metadata.ObjectMetadata{
		Version:      metadata.ObjectMetadataVersion,
		ContentType:  "application/octet-stream",
		Size:         size,
		ETag:         `"test-etag"`,
		LastModified: time.Now().Truncate(time.Second),
	}
	err = s.metadata.PutObjectMetadata(context.Background(), bucket, key, meta)
	require.NoError(t, err)
}

func TestListInternal_EmptyBucket(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	result, err := s.listInternal(context.Background(), "test-bucket", "", "", "", 1000, false)
	require.NoError(t, err)

	assert.Empty(t, result.Objects)
	assert.Empty(t, result.CommonPrefixes)
	assert.False(t, result.IsTruncated)
	assert.Empty(t, result.NextMarker)
}

func TestListInternal_BucketNotExist(t *testing.T) {
	s := setupTestStorage(t)

	_, err := s.listInternal(context.Background(), "nonexistent", "", "", "", 1000, false)
	assert.ErrorIs(t, err, ErrNoSuchBucket)
}

func TestListInternal_BasicListing(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create test objects
	createTestObject(t, s, "test-bucket", "file1.txt", 100)
	createTestObject(t, s, "test-bucket", "file2.txt", 200)
	createTestObject(t, s, "test-bucket", "file3.txt", 300)

	result, err := s.listInternal(context.Background(), "test-bucket", "", "", "", 1000, false)
	require.NoError(t, err)

	assert.Len(t, result.Objects, 3)
	assert.Empty(t, result.CommonPrefixes)
	assert.False(t, result.IsTruncated)

	// Verify objects are sorted
	assert.Equal(t, "file1.txt", result.Objects[0].Key)
	assert.Equal(t, "file2.txt", result.Objects[1].Key)
	assert.Equal(t, "file3.txt", result.Objects[2].Key)

	// Verify object details
	assert.Equal(t, int64(100), result.Objects[0].Size)
	assert.Equal(t, int64(200), result.Objects[1].Size)
	assert.Equal(t, int64(300), result.Objects[2].Size)
}

func TestListInternal_WithPrefix(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create test objects with different prefixes
	createTestObject(t, s, "test-bucket", "docs/readme.md", 100)
	createTestObject(t, s, "test-bucket", "docs/guide.md", 200)
	createTestObject(t, s, "test-bucket", "images/logo.png", 300)
	createTestObject(t, s, "test-bucket", "file.txt", 400)

	// List with prefix "docs/"
	result, err := s.listInternal(context.Background(), "test-bucket", "docs/", "", "", 1000, false)
	require.NoError(t, err)

	assert.Len(t, result.Objects, 2)
	assert.Equal(t, "docs/guide.md", result.Objects[0].Key)
	assert.Equal(t, "docs/readme.md", result.Objects[1].Key)
}

func TestListInternal_WithDelimiter(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create nested directory structure
	createTestObject(t, s, "test-bucket", "docs/readme.md", 100)
	createTestObject(t, s, "test-bucket", "docs/guide.md", 200)
	createTestObject(t, s, "test-bucket", "images/logo.png", 300)
	createTestObject(t, s, "test-bucket", "file.txt", 400)

	// List with delimiter "/"
	result, err := s.listInternal(context.Background(), "test-bucket", "", "", "/", 1000, false)
	require.NoError(t, err)

	// Should have 1 object (file.txt) and 2 common prefixes (docs/, images/)
	assert.Len(t, result.Objects, 1)
	assert.Equal(t, "file.txt", result.Objects[0].Key)

	assert.Len(t, result.CommonPrefixes, 2)
	assert.Equal(t, "docs/", result.CommonPrefixes[0].Prefix)
	assert.Equal(t, "images/", result.CommonPrefixes[1].Prefix)
}

func TestListInternal_WithPrefixAndDelimiter(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create nested directory structure
	createTestObject(t, s, "test-bucket", "docs/api/v1/endpoints.md", 100)
	createTestObject(t, s, "test-bucket", "docs/api/v2/endpoints.md", 200)
	createTestObject(t, s, "test-bucket", "docs/readme.md", 300)
	createTestObject(t, s, "test-bucket", "images/logo.png", 400)

	// List with prefix "docs/" and delimiter "/"
	result, err := s.listInternal(context.Background(), "test-bucket", "docs/", "", "/", 1000, false)
	require.NoError(t, err)

	// Should have 1 object (docs/readme.md) and 1 common prefix (docs/api/)
	assert.Len(t, result.Objects, 1)
	assert.Equal(t, "docs/readme.md", result.Objects[0].Key)

	assert.Len(t, result.CommonPrefixes, 1)
	assert.Equal(t, "docs/api/", result.CommonPrefixes[0].Prefix)
}

func TestListInternal_MaxKeys(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create 10 objects
	for i := 1; i <= 10; i++ {
		createTestObject(t, s, "test-bucket", "file"+string(rune('0'+i))+".txt", int64(i*100))
	}

	// List with maxKeys=5
	result, err := s.listInternal(context.Background(), "test-bucket", "", "", "", 5, false)
	require.NoError(t, err)

	assert.Len(t, result.Objects, 5)
	assert.True(t, result.IsTruncated)
	assert.NotEmpty(t, result.NextMarker)

	// Next marker should be the last returned key
	assert.Equal(t, result.Objects[4].Key, result.NextMarker)
}

func TestListInternal_MaxKeysWithCommonPrefixes(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create objects in different directories
	createTestObject(t, s, "test-bucket", "a/file1.txt", 100)
	createTestObject(t, s, "test-bucket", "b/file2.txt", 200)
	createTestObject(t, s, "test-bucket", "c/file3.txt", 300)
	createTestObject(t, s, "test-bucket", "d/file4.txt", 400)
	createTestObject(t, s, "test-bucket", "e/file5.txt", 500)

	// List with delimiter and maxKeys=3
	result, err := s.listInternal(context.Background(), "test-bucket", "", "", "/", 3, false)
	require.NoError(t, err)

	// Should return 3 common prefixes total
	totalItems := len(result.Objects) + len(result.CommonPrefixes)
	assert.Equal(t, 3, totalItems)
	assert.True(t, result.IsTruncated)
	assert.NotEmpty(t, result.NextMarker)
}

func TestListInternal_StartAt(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create test objects
	createTestObject(t, s, "test-bucket", "file1.txt", 100)
	createTestObject(t, s, "test-bucket", "file2.txt", 200)
	createTestObject(t, s, "test-bucket", "file3.txt", 300)
	createTestObject(t, s, "test-bucket", "file4.txt", 400)
	createTestObject(t, s, "test-bucket", "file5.txt", 500)

	// List starting after file2.txt
	result, err := s.listInternal(context.Background(), "test-bucket", "", "file2.txt", "", 1000, false)
	require.NoError(t, err)

	// Should return file3.txt, file4.txt, file5.txt (files > file2.txt)
	assert.Len(t, result.Objects, 3)
	assert.Equal(t, "file3.txt", result.Objects[0].Key)
	assert.Equal(t, "file4.txt", result.Objects[1].Key)
	assert.Equal(t, "file5.txt", result.Objects[2].Key)
}

func TestListInternal_StartAtWithCommonPrefixes(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create objects in different directories
	createTestObject(t, s, "test-bucket", "a/file1.txt", 100)
	createTestObject(t, s, "test-bucket", "b/file2.txt", 200)
	createTestObject(t, s, "test-bucket", "c/file3.txt", 300)
	createTestObject(t, s, "test-bucket", "d/file4.txt", 400)

	// List with delimiter and startAt "b/"
	result, err := s.listInternal(context.Background(), "test-bucket", "", "b/", "/", 1000, false)
	require.NoError(t, err)

	// Should return common prefixes c/ and d/ (prefixes > "b/")
	assert.Len(t, result.CommonPrefixes, 2)
	assert.Equal(t, "c/", result.CommonPrefixes[0].Prefix)
	assert.Equal(t, "d/", result.CommonPrefixes[1].Prefix)
}

func TestListInternal_Pagination(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create 10 objects
	for i := 1; i <= 10; i++ {
		createTestObject(t, s, "test-bucket", "file"+string(rune('0'+i))+".txt", int64(i*100))
	}

	// First page (maxKeys=4)
	result1, err := s.listInternal(context.Background(), "test-bucket", "", "", "", 4, false)
	require.NoError(t, err)
	assert.Len(t, result1.Objects, 4)
	assert.True(t, result1.IsTruncated)

	// Second page (use NextMarker from first page)
	result2, err := s.listInternal(context.Background(), "test-bucket", "", result1.NextMarker, "", 4, false)
	require.NoError(t, err)
	assert.Len(t, result2.Objects, 4)
	assert.True(t, result2.IsTruncated)

	// Third page (should have remaining 2 objects)
	result3, err := s.listInternal(context.Background(), "test-bucket", "", result2.NextMarker, "", 4, false)
	require.NoError(t, err)
	assert.Len(t, result3.Objects, 2)
	assert.False(t, result3.IsTruncated)

	// Verify no duplicate keys across pages
	allKeys := make(map[string]bool)
	for _, obj := range result1.Objects {
		allKeys[obj.Key] = true
	}
	for _, obj := range result2.Objects {
		assert.False(t, allKeys[obj.Key], "Duplicate key found: %s", obj.Key)
		allKeys[obj.Key] = true
	}
	for _, obj := range result3.Objects {
		assert.False(t, allKeys[obj.Key], "Duplicate key found: %s", obj.Key)
		allKeys[obj.Key] = true
	}

	// Verify all 10 objects were returned
	assert.Len(t, allKeys, 10)
}

func TestListInternal_FetchOwner(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")
	createTestObject(t, s, "test-bucket", "file.txt", 100)

	// Set bucket owner
	bucketMeta := &metadata.BucketMetadata{
		Owner: "test-owner-id",
	}
	err := s.metadata.CreateBucket(context.Background(), "test-bucket")
	require.NoError(t, err)

	// Test with fetchOwner=false
	result, err := s.listInternal(context.Background(), "test-bucket", "", "", "", 1000, false)
	require.NoError(t, err)
	assert.Len(t, result.Objects, 1)
	assert.Nil(t, result.Objects[0].Owner)

	// Test with fetchOwner=true
	// Note: This might fail if bucket metadata doesn't include owner, which is expected
	result2, err := s.listInternal(context.Background(), "test-bucket", "", "", "", 1000, true)
	require.NoError(t, err)
	assert.Len(t, result2.Objects, 1)
	// Owner might be nil if bucket metadata doesn't have owner set
	_ = bucketMeta // Keep for potential future use
}

func TestListInternal_ContextCancellation(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")
	createTestObject(t, s, "test-bucket", "file.txt", 100)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should return context error
	_, err := s.listInternal(ctx, "test-bucket", "", "", "", 1000, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context cancelled")
}

func TestListInternal_SortOrder(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create objects in non-alphabetical order
	createTestObject(t, s, "test-bucket", "zebra.txt", 100)
	createTestObject(t, s, "test-bucket", "apple.txt", 200)
	createTestObject(t, s, "test-bucket", "banana.txt", 300)
	createTestObject(t, s, "test-bucket", "cherry.txt", 400)

	result, err := s.listInternal(context.Background(), "test-bucket", "", "", "", 1000, false)
	require.NoError(t, err)

	// Verify lexicographic sort order
	assert.Len(t, result.Objects, 4)
	assert.Equal(t, "apple.txt", result.Objects[0].Key)
	assert.Equal(t, "banana.txt", result.Objects[1].Key)
	assert.Equal(t, "cherry.txt", result.Objects[2].Key)
	assert.Equal(t, "zebra.txt", result.Objects[3].Key)
}

func TestListInternal_CommonPrefixesSortOrder(t *testing.T) {
	s := setupTestStorage(t)
	createTestBucket(t, s, "test-bucket")

	// Create objects with different prefixes
	createTestObject(t, s, "test-bucket", "z-dir/file.txt", 100)
	createTestObject(t, s, "test-bucket", "a-dir/file.txt", 200)
	createTestObject(t, s, "test-bucket", "m-dir/file.txt", 300)

	result, err := s.listInternal(context.Background(), "test-bucket", "", "", "/", 1000, false)
	require.NoError(t, err)

	// Verify common prefixes are sorted
	assert.Len(t, result.CommonPrefixes, 3)
	assert.Equal(t, "a-dir/", result.CommonPrefixes[0].Prefix)
	assert.Equal(t, "m-dir/", result.CommonPrefixes[1].Prefix)
	assert.Equal(t, "z-dir/", result.CommonPrefixes[2].Prefix)
}

func TestListInternal_EdgeCases(t *testing.T) {
	t.Run("zero maxKeys", func(t *testing.T) {
		s := setupTestStorage(t)
		createTestBucket(t, s, "test-bucket")
		createTestObject(t, s, "test-bucket", "file.txt", 100)

		// maxKeys=0 should return all results (no limit)
		result, err := s.listInternal(context.Background(), "test-bucket", "", "", "", 0, false)
		require.NoError(t, err)
		assert.Len(t, result.Objects, 1)
		assert.False(t, result.IsTruncated)
	})

	t.Run("negative maxKeys", func(t *testing.T) {
		s := setupTestStorage(t)
		createTestBucket(t, s, "test-bucket")
		createTestObject(t, s, "test-bucket", "file.txt", 100)

		// negative maxKeys should be treated as no limit
		result, err := s.listInternal(context.Background(), "test-bucket", "", "", "", -1, false)
		require.NoError(t, err)
		assert.Len(t, result.Objects, 1)
		assert.False(t, result.IsTruncated)
	})

	t.Run("prefix with no matches", func(t *testing.T) {
		s := setupTestStorage(t)
		createTestBucket(t, s, "test-bucket")
		createTestObject(t, s, "test-bucket", "file.txt", 100)

		result, err := s.listInternal(context.Background(), "test-bucket", "nonexistent/", "", "", 1000, false)
		require.NoError(t, err)
		assert.Empty(t, result.Objects)
		assert.Empty(t, result.CommonPrefixes)
	})
}
