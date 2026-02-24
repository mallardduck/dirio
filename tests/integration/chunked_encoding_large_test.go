package integration

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPutObject_LargeChunkedUpload tests chunked encoding with a larger file (1MB)
// to simulate real-world AWS SDK behavior for large files
func TestPutObject_LargeChunkedUpload(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Create 1MB of random data
	originalData := make([]byte, 1*1024*1024)
	_, err := rand.Read(originalData)
	require.NoError(t, err)

	// Encode as chunked data with 256KB chunks (simulating AWS SDK)
	chunkSize := 256 * 1024
	var chunkedBody bytes.Buffer

	for offset := 0; offset < len(originalData); offset += chunkSize {
		end := min(offset+chunkSize, len(originalData))
		chunk := originalData[offset:end]

		// Write chunk header: size;chunk-signature=xxx\r\n
		fmt.Fprintf(&chunkedBody, "%x;chunk-signature=testsig%d\r\n", len(chunk), offset/chunkSize)
		chunkedBody.Write(chunk)
		chunkedBody.WriteString("\r\n")
	}

	// Final chunk
	chunkedBody.WriteString("0;chunk-signature=finalsig\r\n\r\n")

	t.Logf("Original data size: %d bytes", len(originalData))
	t.Logf("Chunked body size: %d bytes", chunkedBody.Len())

	// Create PUT request with chunked encoding
	req, err := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "large-chunked.dat"), bytes.NewReader(chunkedBody.Bytes()))
	require.NoError(t, err)

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

	t.Logf("Upload successful")

	// Download and verify content
	getReq, err := http.NewRequest("GET", ts.ObjectURL("test-bucket", "large-chunked.dat"), nil)
	require.NoError(t, err)
	ts.SignRequest(getReq, nil)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	// Read downloaded content
	downloaded, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	t.Logf("Downloaded size: %d bytes", len(downloaded))

	// CRITICAL: Verify no chunked encoding markers in the content
	assert.NotContains(t, string(downloaded), "chunk-signature", "Content should not contain chunked encoding markers")

	// Verify exact size match
	assert.Len(t, downloaded, len(originalData), "Downloaded size should match original data size")

	// Verify exact content match
	assert.True(t, bytes.Equal(originalData, downloaded), "Downloaded content should match original data exactly")

	if bytes.Equal(originalData, downloaded) {
		t.Log("✓ Content integrity verified - chunked encoding decoded correctly for large file!")
	}
}
