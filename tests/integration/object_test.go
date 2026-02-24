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
	bodyBytes := []byte(content)
	ts.SignRequest(req, bodyBytes)
	req.Header.Set("Content-Type", "text/plain")
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify ETag is returned
	etag := resp.Header.Get("ETag")
	assert.NotEmpty(t, etag)
}

func TestPutObjectInSubfolder(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	content := "Nested content"
	req, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "folder/subfolder/file.txt"), strings.NewReader(content))
	bodyBytes := []byte(content)
	ts.SignRequest(req, bodyBytes)
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPutObjectToNonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bodyBytes := []byte("content")
	req, _ := http.NewRequest("PUT", ts.ObjectURL("nonexistent", "file.txt"), strings.NewReader("content"))
	ts.SignRequest(req, bodyBytes)
	req.ContentLength = 7

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "NoSuchBucket")
}

func TestGetObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	content := "Hello, DirIO!"
	ts.PutObject(t, "test-bucket", "hello.txt", content)

	req, err := http.NewRequest("GET", ts.ObjectURL("test-bucket", "hello.txt"), http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, content, string(body))

	// Verify headers
	assert.NotEmpty(t, resp.Header.Get("ETag"))
	assert.Equal(t, "13", resp.Header.Get("Content-Length"))
	assert.NotEmpty(t, resp.Header.Get("Last-Modified"))
	assert.Equal(t, "bytes", resp.Header.Get("Accept-Ranges"))
}

func TestGetObjectNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	req, err := http.NewRequest("GET", ts.ObjectURL("test-bucket", "nonexistent.txt"), http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "NoSuchKey")
}

func TestGetObjectFromNonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, err := http.NewRequest("GET", ts.ObjectURL("nonexistent", "file.txt"), http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "NoSuchBucket")
}

func TestHeadObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.PutObject(t, "test-bucket", "hello.txt", "Hello, DirIO!")

	req, _ := http.NewRequest("HEAD", ts.ObjectURL("test-bucket", "hello.txt"), http.NoBody)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// HEAD should return headers but no body
	assert.Equal(t, "13", resp.Header.Get("Content-Length"))
	assert.NotEmpty(t, resp.Header.Get("ETag"))
	assert.NotEmpty(t, resp.Header.Get("Last-Modified"))
}

func TestHeadObjectNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	req, _ := http.NewRequest("HEAD", ts.ObjectURL("test-bucket", "nonexistent.txt"), http.NoBody)
	ts.SignRequest(req, nil)
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

	req, _ := http.NewRequest("DELETE", ts.ObjectURL("test-bucket", "hello.txt"), http.NoBody)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify object is gone
	req, err = http.NewRequest("GET", ts.ObjectURL("test-bucket", "hello.txt"), http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	getResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

func TestDeleteObjectNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// S3 returns 204 even when deleting non-existent object
	req, _ := http.NewRequest("DELETE", ts.ObjectURL("test-bucket", "nonexistent.txt"), http.NoBody)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestDeleteObjectFromNonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("DELETE", ts.ObjectURL("nonexistent", "file.txt"), http.NoBody)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "NoSuchBucket")
}

func TestPutAndGetLargeObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Create a 1MB object
	content := strings.Repeat("A", 1024*1024)
	req, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "large.bin"), strings.NewReader(content))
	bodyBytes := []byte(content)
	ts.SignRequest(req, bodyBytes)
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Retrieve and verify
	getReq, _ := http.NewRequest("GET", ts.ObjectURL("test-bucket", "large.bin"), http.NoBody)
	ts.SignRequest(getReq, nil)
	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	body, _ := io.ReadAll(getResp.Body)
	assert.Len(t, body, len(content))
}
