package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPutObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	content := "Hello, DirIO!"
	req, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "hello.txt"), strings.NewReader(content))
	req.Header.Set("Content-Type", "text/plain")
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	// Verify ETag is returned
	etag := resp.Header.Get("ETag")
	assert.NotEmpty(etag)
}

func TestPutObjectInSubfolder(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	content := "Nested content"
	req, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "folder/subfolder/file.txt"), strings.NewReader(content))
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPutObjectToNonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("PUT", ts.ObjectURL("nonexistent", "file.txt"), strings.NewReader("content"))
	req.ContentLength = 7

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "NoSuchBucket")
}

func TestGetObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	content := "Hello, DirIO!"
	ts.PutObject(t, "test-bucket", "hello.txt", content)

	resp, err := http.Get(ts.ObjectURL("test-bucket", "hello.txt"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(content, string(body))

	// Verify headers
	assert.NotEmpty(resp.Header.Get("ETag"))
	assert.Equal("13", resp.Header.Get("Content-Length"))
	assert.NotEmpty(resp.Header.Get("Last-Modified"))
	assert.Equal("bytes", resp.Header.Get("Accept-Ranges"))
}

func TestGetObjectNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	resp, err := http.Get(ts.ObjectURL("test-bucket", "nonexistent.txt"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "NoSuchKey")
}

func TestGetObjectFromNonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.ObjectURL("nonexistent", "file.txt"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "NoSuchBucket")
}

func TestHeadObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.PutObject(t, "test-bucket", "hello.txt", "Hello, DirIO!")

	req, _ := http.NewRequest("HEAD", ts.ObjectURL("test-bucket", "hello.txt"), nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	// HEAD should return headers but no body
	assert.Equal("13", resp.Header.Get("Content-Length"))
	assert.NotEmpty(resp.Header.Get("ETag"))
	assert.NotEmpty(resp.Header.Get("Last-Modified"))
}

func TestHeadObjectNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	req, _ := http.NewRequest("HEAD", ts.ObjectURL("test-bucket", "nonexistent.txt"), nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDeleteObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.PutObject(t, "test-bucket", "hello.txt", "Hello, DirIO!")

	req, _ := http.NewRequest("DELETE", ts.ObjectURL("test-bucket", "hello.txt"), nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify object is gone
	getResp, _ := http.Get(ts.ObjectURL("test-bucket", "hello.txt"))
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

func TestDeleteObjectNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// S3 returns 204 even when deleting non-existent object
	req, _ := http.NewRequest("DELETE", ts.ObjectURL("test-bucket", "nonexistent.txt"), nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestDeleteObjectFromNonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("DELETE", ts.ObjectURL("nonexistent", "file.txt"), nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "NoSuchBucket")
}

func TestPutAndGetLargeObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Create a 1MB object
	content := strings.Repeat("A", 1024*1024)
	req, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "large.bin"), strings.NewReader(content))
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	// Retrieve and verify
	getResp, err := http.Get(ts.ObjectURL("test-bucket", "large.bin"))
	require.NoError(t, err)
	defer getResp.Body.Close()

	body, _ := io.ReadAll(getResp.Body)
	assert.Len(body, len(content))
}