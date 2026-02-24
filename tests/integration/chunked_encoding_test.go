package integration

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/http/auth"
)

// signStreamingRequest signs a request with STREAMING-AWS4-HMAC-SHA256-PAYLOAD
func signStreamingRequest(ts *TestServer, req *http.Request) {
	// Get current timestamp
	timestamp := time.Now().UTC()

	// For streaming/chunked encoding, use special marker
	payloadHash := consts.ContentSHA256Streaming

	// Set required headers
	req.Header.Set("X-Amz-Date", timestamp.Format("20060102T150405Z"))
	req.Header.Set(consts.HeaderContentSHA256, payloadHash)
	req.Header.Set("Host", req.Host)

	// Signed headers (must be sorted)
	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	sort.Strings(signedHeaders)

	// Build canonical request
	canonicalRequest := auth.BuildCanonicalRequest(req, signedHeaders, payloadHash)

	// Build string to sign
	region := "us-east-1"
	stringToSign := auth.BuildStringToSign(timestamp, region, canonicalRequest)

	// Compute signature
	signature := auth.ComputeSignature(ts.SecretKey, timestamp, region, stringToSign)

	// Build Authorization header
	dateStamp := timestamp.Format("20060102")
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		ts.AccessKey, credentialScope, strings.Join(signedHeaders, ";"), signature)

	req.Header.Set("Authorization", authHeader)
}

// TestPutObject_ChunkedEncoding verifies that AWS SigV4 chunked transfer encoding is properly decoded
// This test addresses bug #001 where chunked encoding markers were being saved to object files
func TestPutObject_ChunkedEncoding(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Prepare chunked encoded data simulating AWS SDK behavior
	// We want to upload "hello world" (11 bytes = 0xb)
	chunkedBody := "b;chunk-signature=abc123def456\r\nhello world\r\n0;chunk-signature=final123\r\n\r\n"

	// Create PUT request with chunked encoding
	req, err := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "chunked-test.txt"), strings.NewReader(chunkedBody))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "text/plain")
	// Sign request with streaming signature
	signStreamingRequest(ts, req)

	// Send request
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verify successful upload
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	// Now retrieve the object to verify content was decoded correctly
	getReq, err := http.NewRequest("GET", ts.ObjectURL("test-bucket", "chunked-test.txt"), http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(getReq, nil)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	// Read and verify content
	content, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	expected := "hello world"
	actual := string(content)

	// CRITICAL: Verify no chunked encoding markers in the content
	assert.NotContains(t, actual, "chunk-signature", "Content should not contain chunked encoding markers")
	assert.NotContains(t, actual, ";", "Content should not contain chunked format markers")

	// Verify exact content match
	assert.Equal(t, expected, actual, "Content should match exactly without encoding artifacts")
	assert.Len(t, actual, len(expected), "Content length should match exactly")
}

// TestPutObject_MultipleChunks verifies decoding of multiple chunks
func TestPutObject_MultipleChunks(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Create data with multiple chunks: "hello" + " " + "world"
	chunkedBody := "5;chunk-signature=abc\r\nhello\r\n1;chunk-signature=def\r\n \r\n5;chunk-signature=ghi\r\nworld\r\n0;chunk-signature=final\r\n\r\n"

	req, err := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "multi-chunk.txt"), strings.NewReader(chunkedBody))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "text/plain")
	signStreamingRequest(ts, req)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify content
	getReq, _ := http.NewRequest("GET", ts.ObjectURL("test-bucket", "multi-chunk.txt"), http.NoBody)
	ts.SignRequest(getReq, nil)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	content, _ := io.ReadAll(getResp.Body)
	expected := "hello world"

	assert.Equal(t, expected, string(content))
}

// TestPutObject_LargeChunkedData verifies large chunked uploads work correctly
func TestPutObject_LargeChunkedData(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Create 1KB of data
	data := strings.Repeat("x", 1024)
	chunkedBody := "400;chunk-signature=large\r\n" + data + "\r\n0;chunk-signature=final\r\n\r\n"

	req, err := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "large-chunk.txt"), strings.NewReader(chunkedBody))
	require.NoError(t, err)

	signStreamingRequest(ts, req)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify size matches expected (1024 bytes, not 1024 + chunk overhead)
	headReq, _ := http.NewRequest("HEAD", ts.ObjectURL("test-bucket", "large-chunk.txt"), http.NoBody)
	ts.SignRequest(headReq, nil)

	headResp, err := http.DefaultClient.Do(headReq)
	require.NoError(t, err)
	defer headResp.Body.Close()

	contentLength := headResp.Header.Get("Content-Length")
	assert.Equal(t, "1024", contentLength, "Content-Length should be 1024, not including chunk overhead")

	// Verify actual content
	getReq, _ := http.NewRequest("GET", ts.ObjectURL("test-bucket", "large-chunk.txt"), http.NoBody)
	ts.SignRequest(getReq, nil)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	content, _ := io.ReadAll(getResp.Body)
	assert.Equal(t, data, string(content), "Downloaded content should match original data exactly")
	assert.Len(t, content, 1024, "Downloaded size should be 1024 bytes")
}

// TestPutObject_NonChunkedStillWorks verifies normal (non-chunked) uploads still work
func TestPutObject_NonChunkedStillWorks(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Normal upload without chunked encoding (this is what existing tests do)
	content := "normal content"
	req, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "normal.txt"), strings.NewReader(content))
	bodyBytes := []byte(content)
	ts.SignRequest(req, bodyBytes)
	req.Header.Set("Content-Type", "text/plain")
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify content
	getReq, _ := http.NewRequest("GET", ts.ObjectURL("test-bucket", "normal.txt"), http.NoBody)
	ts.SignRequest(getReq, nil)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	retrieved, _ := io.ReadAll(getResp.Body)
	assert.Equal(t, content, string(retrieved))
}

// TestPutObject_EmptyChunkedUpload verifies handling of empty chunked uploads
func TestPutObject_EmptyChunkedUpload(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Empty upload with just final chunk
	chunkedBody := "0;chunk-signature=final\r\n\r\n"

	req, err := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "empty.txt"), strings.NewReader(chunkedBody))
	require.NoError(t, err)

	signStreamingRequest(ts, req)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify empty content
	getReq, _ := http.NewRequest("GET", ts.ObjectURL("test-bucket", "empty.txt"), http.NoBody)
	ts.SignRequest(getReq, nil)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	content, _ := io.ReadAll(getResp.Body)
	assert.Empty(t, content, "Empty chunked upload should create empty file")
}
