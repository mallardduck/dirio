package integration

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/pkg/s3types"
)

// TestBucketNameValidation tests S3 bucket name validation
func TestBucketNameValidation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	tests := []struct {
		name           string
		bucketName     string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "bucket name too short",
			bucketName:     "ab",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "InvalidBucketName",
		},
		{
			name:           "bucket name too long",
			bucketName:     strings.Repeat("a", 64),
			expectedStatus: http.StatusBadRequest,
			expectedError:  "InvalidBucketName",
		},
		{
			name:           "bucket name with uppercase",
			bucketName:     "MyBucket",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "InvalidBucketName",
		},
		{
			name:           "bucket name starting with hyphen",
			bucketName:     "-mybucket",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "InvalidBucketName",
		},
		{
			name:           "bucket name ending with hyphen",
			bucketName:     "mybucket-",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "InvalidBucketName",
		},
		{
			name:           "bucket name with consecutive dots",
			bucketName:     "my..bucket",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "InvalidBucketName",
		},
		{
			name:           "bucket name formatted as IP address",
			bucketName:     "192.168.1.1",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "InvalidBucketName",
		},
		{
			name:           "bucket name with invalid characters",
			bucketName:     "my_bucket",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "InvalidBucketName",
		},
		{
			name:           "valid bucket name lowercase",
			bucketName:     "valid-bucket-123",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "valid bucket name with dots",
			bucketName:     "my.valid.bucket",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("PUT", ts.BucketURL(tt.bucketName), http.NoBody)
			ts.SignRequest(req, nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var errResp s3types.ErrorResponse
				err = xml.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Equal(t, tt.expectedError, errResp.Code)
			}
		})
	}
}

// TestObjectKeyValidation tests S3 object key validation
func TestObjectKeyValidation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// Create a valid bucket first
	ts.CreateBucket(t, "test-bucket")

	tests := []struct {
		name           string
		key            string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "key too long (>1024 bytes)",
			key:            strings.Repeat("a", 1025),
			expectedStatus: http.StatusBadRequest,
			expectedError:  "KeyTooLongError",
		},
		{
			name:           "valid key simple",
			key:            "test.txt",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "valid key with path",
			key:            "docs/readme.md",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "valid key with deep path",
			key:            "a/b/c/d/e/file.txt",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "valid key with special chars",
			key:            "file-name_2024.txt",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "valid key with spaces",
			key:            "my file.txt",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes := []byte("test content")
			req, err := http.NewRequest("PUT", ts.ObjectURL("test-bucket", tt.key), strings.NewReader("test content"))
			ts.SignRequest(req, bodyBytes)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var errResp s3types.ErrorResponse
				err = xml.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Equal(t, tt.expectedError, errResp.Code)
			}
		})
	}
}

// TestFilesystemIsolation verifies that the filesystem layer provides isolation
func TestFilesystemIsolation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	t.Run("objects are stored within bucket directory", func(t *testing.T) {
		// Create a bucket and add objects
		ts.CreateBucket(t, "test-bucket")
		ts.PutObject(t, "test-bucket", "file1.txt", "content1")
		ts.PutObject(t, "test-bucket", "subdir/file2.txt", "content2")

		// Verify objects can be retrieved
		req, err := http.NewRequest("GET", ts.ObjectURL("test-bucket", "file1.txt"), http.NoBody)
		require.NoError(t, err)
		ts.SignRequest(req, nil)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "content1", string(body))
	})

	t.Run("buckets are isolated from each other", func(t *testing.T) {
		// Create two buckets with same file names
		ts.CreateBucket(t, "bucket1")
		ts.CreateBucket(t, "bucket2")

		ts.PutObject(t, "bucket1", "test.txt", "content from bucket1")
		ts.PutObject(t, "bucket2", "test.txt", "content from bucket2")

		// Verify each bucket has its own content
		req1, _ := http.NewRequest("GET", ts.ObjectURL("bucket1", "test.txt"), http.NoBody)
		ts.SignRequest(req1, nil)
		resp1, err := http.DefaultClient.Do(req1)
		require.NoError(t, err)
		defer resp1.Body.Close()

		body1, _ := io.ReadAll(resp1.Body)
		assert.Equal(t, "content from bucket1", string(body1))

		req2, _ := http.NewRequest("GET", ts.ObjectURL("bucket2", "test.txt"), http.NoBody)
		ts.SignRequest(req2, nil)
		resp2, err := http.DefaultClient.Do(req2)
		require.NoError(t, err)
		defer resp2.Body.Close()

		body2, _ := io.ReadAll(resp2.Body)
		assert.Equal(t, "content from bucket2", string(body2))
	})
}

// TestHeadObjectKeyValidation tests HEAD requests work correctly
func TestHeadObjectKeyValidation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	t.Run("HEAD with nonexistent key returns 404", func(t *testing.T) {
		req, err := http.NewRequest("HEAD", ts.ObjectURL("test-bucket", "nonexistent.txt"), http.NoBody)
		ts.SignRequest(req, nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("HEAD with valid key works", func(t *testing.T) {
		// Put a valid object first
		ts.PutObject(t, "test-bucket", "valid.txt", "content")

		req, err := http.NewRequest("HEAD", ts.ObjectURL("test-bucket", "valid.txt"), http.NoBody)
		ts.SignRequest(req, nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "7", resp.Header.Get("Content-Length"))
	})
}
