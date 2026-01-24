package integration

import (
	"encoding/xml"
	"io"
	"net/http"
	"testing"

	"github.com/mallardduck/dirio/pkg/s3types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListBucketsEmpty(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, err := http.NewRequest("GET", ts.URL("/"), nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "<Buckets></Buckets>")
}

func TestCreateBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("PUT", ts.BucketURL("test-bucket"), nil)
	ts.SignRequest(req, nil)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCreateBucketDuplicate(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// Create first bucket
	ts.CreateBucket(t, "test-bucket")

	// Try to create duplicate
	req, _ := http.NewRequest("PUT", ts.BucketURL("test-bucket"), nil)
	ts.SignRequest(req, nil)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusConflict, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "BucketAlreadyExists")
}

func TestListBucketsAfterCreate(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// Create two buckets
	ts.CreateBucket(t, "bucket-alpha")
	ts.CreateBucket(t, "bucket-beta")

	req, err := http.NewRequest("GET", ts.URL("/"), nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	assert := assert.New(t)
	assert.Contains(bodyStr, "<Name>bucket-alpha</Name>")
	assert.Contains(bodyStr, "<Name>bucket-beta</Name>")
}

func TestHeadBucketExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	req, _ := http.NewRequest("HEAD", ts.BucketURL("test-bucket"), nil)
	ts.SignRequest(req, nil)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)
	// Verify bucket region header is present (AWS best practice)
	assert.Equal("us-east-1", resp.Header.Get("x-amz-bucket-region"))
}

func TestHeadBucketNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("HEAD", ts.BucketURL("nonexistent"), nil)
	ts.SignRequest(req, nil)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetBucketLocation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	req, err := http.NewRequest("GET", ts.BucketURL("test-bucket")+"?location", nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "us-east-1")
}

func TestDeleteBucketEmpty(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	req, _ := http.NewRequest("DELETE", ts.BucketURL("test-bucket"), nil)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify bucket is gone
	headReq, _ := http.NewRequest("HEAD", ts.BucketURL("test-bucket"), nil)
	ts.SignRequest(headReq, nil)
	headResp, _ := http.DefaultClient.Do(headReq)
	defer headResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, headResp.StatusCode)
}

func TestDeleteBucketNotEmpty(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.PutObject(t, "test-bucket", "file.txt", "content")

	req, _ := http.NewRequest("DELETE", ts.BucketURL("test-bucket"), nil)
	ts.SignRequest(req, nil)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusConflict, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "BucketNotEmpty")
}

func TestDeleteBucketNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("DELETE", ts.BucketURL("nonexistent"), nil)
	ts.SignRequest(req, nil)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp s3types.ErrorResponse
	body, _ := io.ReadAll(resp.Body)
	xml.Unmarshal(body, &errResp)

	assert.Equal(t, "NoSuchBucket", errResp.Code)
}

func TestCreateBucket_ReturnsLocationHeader(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("PUT", ts.BucketURL("test-bucket"), nil)
	ts.SignRequest(req, nil)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	location := resp.Header.Get("Location")
	assert.NotEmpty(location, "Location header should be present")
	assert.Contains(location, "/test-bucket", "Location should contain bucket name")
	assert.Contains(location, "localhost", "Location should contain host")
}

func TestCreateBucket_LocationWithCustomHost(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("PUT", ts.BucketURL("test-bucket"), nil)
	req.Host = "dirio-s3.local:9000"
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	location := resp.Header.Get("Location")
	assert.Contains(location, "dirio-s3.local:9000/test-bucket", "Location should use custom Host header")
}

func TestCreateBucket_LocationWithXForwardedProto(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("PUT", ts.BucketURL("test-bucket"), nil)
	ts.SignRequest(req, nil)
	ts.SignRequest(req, nil)
	req.Header.Set("X-Forwarded-Proto", "https")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert := assert.New(t)
	assert.Equal(http.StatusOK, resp.StatusCode)

	location := resp.Header.Get("Location")
	assert.NotEmpty(location, "Location header should be present")
	assert.True(
		assert.Contains(location, "https://") || assert.Contains(location, "http://"),
		"Location should have a scheme",
	)
}
