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
