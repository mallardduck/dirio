package integration

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mallardduck/dirio/pkg/s3types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListObjectsV2Empty(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?list-type=2", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, xml.Unmarshal(body, &result))

	assert.Equal(0, result.KeyCount)
	assert.Empty(result.Contents)
}

func TestListObjectsV2WithObjects(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"file1.txt":          "content1",
		"file2.txt":          "content2",
		"photos/photo1.jpg":  "photo1",
		"photos/photo2.jpg":  "photo2",
		"docs/readme.md":     "readme",
		"docs/sub/nested.md": "nested",
	})

	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?list-type=2", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, xml.Unmarshal(body, &result))

	assert := assert.New(t)
	assert.Equal(6, result.KeyCount)

	// Check that all keys are present
	keys := make(map[string]bool)
	for _, obj := range result.Contents {
		keys[obj.Key] = true
	}

	expectedKeys := []string{
		"file1.txt", "file2.txt",
		"photos/photo1.jpg", "photos/photo2.jpg",
		"docs/readme.md", "docs/sub/nested.md",
	}
	for _, key := range expectedKeys {
		assert.True(keys[key], "Expected key %s not found in results", key)
	}
}

func TestListObjectsV2WithPrefix(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"file1.txt":         "content1",
		"file2.txt":         "content2",
		"photos/photo1.jpg": "photo1",
		"photos/photo2.jpg": "photo2",
		"docs/readme.md":    "readme",
	})

	// Test prefix=photos/
	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?list-type=2&prefix=photos/", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, xml.Unmarshal(body, &result))

	assert := assert.New(t)
	assert.Equal(2, result.KeyCount)
	assert.Equal("photos/", result.Prefix)

	for _, obj := range result.Contents {
		assert.True(strings.HasPrefix(obj.Key, "photos/"))
	}
}

func TestListObjectsV2WithPrefixPartialMatch(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"file1.txt":  "content1",
		"file2.txt":  "content2",
		"filter.log": "log",
		"photos.zip": "zip",
	})

	// Test prefix=file (should match file1.txt, file2.txt, filter.log)
	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?list-type=2&prefix=file", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, xml.Unmarshal(body, &result))

	assert.Equal(t, 2, result.KeyCount)
}

func TestListObjectsV2NonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, err := http.NewRequest("GET", ts.BucketURL("nonexistent")+"?list-type=2", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "NoSuchBucket")
}

func TestListObjectsV1(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
	})

	// V1 is the default (no list-type param)
	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket"), nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	var result s3types.ListBucketResult
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, xml.Unmarshal(body, &result))

	assert.Len(result.Contents, 2)
	assert.Equal("test-bucket", result.Name)
}

func TestListObjectsV1WithPrefix(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"logs/app.log":    "app",
		"logs/error.log":  "error",
		"config/app.yaml": "config",
	})

	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?prefix=logs/", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result s3types.ListBucketResult
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, xml.Unmarshal(body, &result))

	assert := assert.New(t)
	assert.Len(result.Contents, 2)
	assert.Equal("logs/", result.Prefix)
}

func TestListObjectsV1NonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, err := http.NewRequest("GET", ts.BucketURL("nonexistent"), nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "NoSuchBucket")
}

func TestListObjectsResponseFields(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.PutObject(t, "test-bucket", "test.txt", "test content")

	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?list-type=2", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, xml.Unmarshal(body, &result))

	require.Len(t, result.Contents, 1)

	assert := assert.New(t)
	obj := result.Contents[0]
	assert.Equal("test.txt", obj.Key)
	assert.Equal(int64(12), obj.Size)
	assert.Equal("STANDARD", obj.StorageClass)
	assert.False(obj.LastModified.IsZero())
}

func TestListObjectsV2WithDelimiter(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"test.txt":          "content",
		"folder1/file1.txt": "f1",
		"folder1/file2.txt": "f2",
		"folder2/file3.txt": "f3",
		"root.txt":          "root",
	})

	// Test with delimiter="/"
	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?list-type=2&delimiter=/", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, xml.Unmarshal(body, &result))

	assert := assert.New(t)

	// Should have 2 root-level files
	assert.Len(result.Contents, 2, "Should have 2 root-level objects")
	objectKeys := make(map[string]bool)
	for _, obj := range result.Contents {
		objectKeys[obj.Key] = true
	}
	assert.True(objectKeys["test.txt"], "Should include test.txt")
	assert.True(objectKeys["root.txt"], "Should include root.txt")

	// Should have 2 common prefixes (folder1/ and folder2/)
	assert.Len(result.CommonPrefixes, 2, "Should have 2 common prefixes")
	prefixKeys := make(map[string]bool)
	for _, prefix := range result.CommonPrefixes {
		prefixKeys[prefix.Prefix] = true
	}
	assert.True(prefixKeys["folder1/"], "Should include folder1/ prefix")
	assert.True(prefixKeys["folder2/"], "Should include folder2/ prefix")

	// KeyCount should include both objects and common prefixes
	assert.Equal(4, result.KeyCount, "KeyCount should be 2 objects + 2 common prefixes = 4")
	assert.Equal("/", result.Delimiter, "Delimiter should be /")
}

func TestListObjectsV2WithDelimiterAndPrefix(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"folder1/file1.txt":      "f1",
		"folder1/file2.txt":      "f2",
		"folder1/sub/nested.txt": "nested",
		"folder2/file3.txt":      "f3",
	})

	// Test with prefix="folder1/" and delimiter="/"
	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?list-type=2&prefix=folder1/&delimiter=/", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, xml.Unmarshal(body, &result))

	assert := assert.New(t)

	// Should have 2 files in folder1/ root (file1.txt, file2.txt)
	assert.Len(result.Contents, 2, "Should have 2 files in folder1/ root")

	// Should have 1 common prefix (folder1/sub/)
	assert.Len(result.CommonPrefixes, 1, "Should have 1 common prefix")
	assert.Equal("folder1/sub/", result.CommonPrefixes[0].Prefix, "Should have folder1/sub/ prefix")

	// KeyCount should include both
	assert.Equal(3, result.KeyCount, "KeyCount should be 2 objects + 1 common prefix = 3")
}

func TestListObjectsV2WithMaxKeys(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"file1.txt":        "1",
		"file2.txt":        "2",
		"folder1/file.txt": "f1",
		"folder2/file.txt": "f2",
		"root.txt":         "root",
	})

	// Test with delimiter="/" and max-keys=2
	// Should return first 2 items in lexicographic order (mixing objects and prefixes)
	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?list-type=2&delimiter=/&max-keys=2", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, xml.Unmarshal(body, &result))

	assert := assert.New(t)

	// Should return exactly 2 items total (objects + common prefixes)
	totalItems := len(result.Contents) + len(result.CommonPrefixes)
	assert.Equal(2, totalItems, "Should return exactly 2 items total")
	assert.Equal(2, result.KeyCount, "KeyCount should be 2")
	assert.True(result.IsTruncated, "Should be truncated")
	assert.Equal(2, result.MaxKeys, "MaxKeys should be 2")
}

// TestListObjectsV2Boto3Scenario replicates the exact boto3 test scenario
func TestListObjectsV2Boto3Scenario(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Create objects in the same order as boto3 test
	ts.PutObject(t, "test-bucket", "test.txt", "test content") // Line 76 in boto3.py
	ts.PutObject(t, "test-bucket", "folder1/file1.txt", "f1")  // Line 114
	ts.PutObject(t, "test-bucket", "folder1/file2.txt", "f2")  // Line 115
	ts.PutObject(t, "test-bucket", "folder2/file3.txt", "f3")  // Line 116
	ts.PutObject(t, "test-bucket", "root.txt", "root")         // Line 117

	// Test with delimiter="/" (same as boto3 line 136)
	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?list-type=2&delimiter=/", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	t.Logf("Response body: %s", string(body))
	require.NoError(t, xml.Unmarshal(body, &result))

	assert := assert.New(t)

	// Should have 2 root-level files (test.txt, root.txt)
	t.Logf("Contents count: %d", len(result.Contents))
	for i, obj := range result.Contents {
		t.Logf("  Contents[%d]: %s", i, obj.Key)
	}

	// Should have 2 common prefixes (folder1/, folder2/)
	t.Logf("CommonPrefixes count: %d", len(result.CommonPrefixes))
	for i, prefix := range result.CommonPrefixes {
		t.Logf("  CommonPrefixes[%d]: %s", i, prefix.Prefix)
	}

	assert.Len(result.Contents, 2, "Should have 2 root-level objects")
	assert.Len(result.CommonPrefixes, 2, "Should have 2 common prefixes (folder1/, folder2/)")
	assert.Equal(4, result.KeyCount, "KeyCount should be 2 objects + 2 prefixes = 4")
}
