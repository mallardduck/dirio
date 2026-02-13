package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Public read policy - allows anonymous GetObject
const publicReadPolicy = `{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Principal": "*",
			"Action": "s3:GetObject",
			"Resource": "arn:aws:s3:::%s/*"
		}
	]
}`

// Public read + list policy - allows anonymous GetObject and ListBucket
const publicReadListPolicy = `{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Principal": "*",
			"Action": ["s3:GetObject", "s3:ListBucket"],
			"Resource": ["arn:aws:s3:::%s/*", "arn:aws:s3:::%s"]
		}
	]
}`

// SetBucketPolicy sets a bucket policy using the S3 API
func (ts *TestServer) SetBucketPolicy(t *testing.T, bucket, policy string) {
	t.Helper()
	body := []byte(policy)
	req, err := http.NewRequest("PUT", ts.BucketURL(bucket)+"?policy", strings.NewReader(policy))
	require.NoError(t, err)
	req.ContentLength = int64(len(body))
	ts.SignRequest(req, body)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to set bucket policy on %s: status %d, body: %s", bucket, resp.StatusCode, respBody)
	}
}

// AnonymousRequest makes an HTTP request without any authentication
func AnonymousRequest(method, url string, body []byte) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	return http.DefaultClient.Do(req)
}

func TestBucketPolicy_PrivateBucketDeniesAnonymousAccess(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// Create a private bucket (no policy = private by default)
	ts.CreateBucket(t, "private-bucket")
	ts.PutObject(t, "private-bucket", "secret.txt", "secret content")

	// Anonymous GET should be denied
	resp, err := AnonymousRequest("GET", ts.ObjectURL("private-bucket", "secret.txt"), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Anonymous GET on private bucket should be denied")

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "AccessDenied", "Response should contain AccessDenied error")
}

func TestBucketPolicy_PrivateBucketDeniesAnonymousHead(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// Create a private bucket
	ts.CreateBucket(t, "private-bucket")
	ts.PutObject(t, "private-bucket", "secret.txt", "secret content")

	// Anonymous HEAD should be denied
	resp, err := AnonymousRequest("HEAD", ts.ObjectURL("private-bucket", "secret.txt"), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Anonymous HEAD on private bucket should be denied")
}

func TestBucketPolicy_PrivateBucketDeniesAnonymousList(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// Create a private bucket
	ts.CreateBucket(t, "private-bucket")
	ts.PutObject(t, "private-bucket", "secret.txt", "secret content")

	// Anonymous ListObjects should be denied
	resp, err := AnonymousRequest("GET", ts.BucketURL("private-bucket"), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Anonymous ListObjects on private bucket should be denied")
}

func TestBucketPolicy_PublicReadAllowsAnonymousGet(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "public-bucket"

	// Create bucket and upload object as admin
	ts.CreateBucket(t, bucket)
	ts.PutObject(t, bucket, "public-file.txt", "hello world")

	// Set public read policy
	policy := strings.Replace(publicReadPolicy, "%s", bucket, 1)
	ts.SetBucketPolicy(t, bucket, policy)

	// Anonymous GET should succeed
	resp, err := AnonymousRequest("GET", ts.ObjectURL(bucket, "public-file.txt"), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Anonymous GET on public bucket should succeed")

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "hello world", string(body), "Response body should match uploaded content")
}

func TestBucketPolicy_PublicReadAllowsAnonymousHead(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "public-bucket"

	// Create bucket and upload object as admin
	ts.CreateBucket(t, bucket)
	ts.PutObject(t, bucket, "public-file.txt", "hello world")

	// Set public read policy
	policy := strings.Replace(publicReadPolicy, "%s", bucket, 1)
	ts.SetBucketPolicy(t, bucket, policy)

	// Anonymous HEAD should succeed (HeadObject requires s3:GetObject permission)
	resp, err := AnonymousRequest("HEAD", ts.ObjectURL(bucket, "public-file.txt"), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Anonymous HEAD on public bucket should succeed")
	assert.Equal(t, "11", resp.Header.Get("Content-Length"), "Content-Length should be set")
}

func TestBucketPolicy_PublicReadDeniesAnonymousPut(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "public-bucket"

	// Create bucket as admin
	ts.CreateBucket(t, bucket)

	// Set public read policy (read-only, no write)
	policy := strings.Replace(publicReadPolicy, "%s", bucket, 1)
	ts.SetBucketPolicy(t, bucket, policy)

	// Anonymous PUT should be denied
	resp, err := AnonymousRequest("PUT", ts.ObjectURL(bucket, "new-file.txt"), []byte("should fail"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Anonymous PUT on public-read bucket should be denied")

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "AccessDenied", "Response should contain AccessDenied error")
}

func TestBucketPolicy_PublicReadDeniesAnonymousDelete(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "public-bucket"

	// Create bucket and upload object as admin
	ts.CreateBucket(t, bucket)
	ts.PutObject(t, bucket, "public-file.txt", "hello world")

	// Set public read policy
	policy := strings.Replace(publicReadPolicy, "%s", bucket, 1)
	ts.SetBucketPolicy(t, bucket, policy)

	// Anonymous DELETE should be denied
	resp, err := AnonymousRequest("DELETE", ts.ObjectURL(bucket, "public-file.txt"), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Anonymous DELETE on public-read bucket should be denied")
}

func TestBucketPolicy_PublicReadDeniesAnonymousList(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "public-bucket"

	// Create bucket and upload object as admin
	ts.CreateBucket(t, bucket)
	ts.PutObject(t, bucket, "file.txt", "content")

	// Set public read policy (GetObject only, no ListBucket)
	policy := strings.Replace(publicReadPolicy, "%s", bucket, 1)
	ts.SetBucketPolicy(t, bucket, policy)

	// Anonymous ListObjects should be denied (policy only allows GetObject)
	resp, err := AnonymousRequest("GET", ts.BucketURL(bucket), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Anonymous ListObjects should be denied when policy only allows GetObject")
}

func TestBucketPolicy_PublicReadListAllowsAnonymousList(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "public-list-bucket"

	// Create bucket and upload object as admin
	ts.CreateBucket(t, bucket)
	ts.PutObject(t, bucket, "file1.txt", "content1")
	ts.PutObject(t, bucket, "file2.txt", "content2")

	// Set public read + list policy
	policy := strings.ReplaceAll(publicReadListPolicy, "%s", bucket) // Replace all occurrences
	ts.SetBucketPolicy(t, bucket, policy)

	// Anonymous ListObjects should succeed
	resp, err := AnonymousRequest("GET", ts.BucketURL(bucket), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Anonymous ListObjects should succeed with ListBucket permission")

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "file1.txt", "Response should contain file1.txt")
	assert.Contains(t, bodyStr, "file2.txt", "Response should contain file2.txt")
}

func TestBucketPolicy_MixedPublicAndPrivateBuckets(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// Create two buckets
	ts.CreateBucket(t, "private-bucket")
	ts.CreateBucket(t, "public-bucket")

	// Upload objects to both
	ts.PutObject(t, "private-bucket", "secret.txt", "secret data")
	ts.PutObject(t, "public-bucket", "public.txt", "public data")

	// Set public read policy on public bucket only
	policy := strings.Replace(publicReadPolicy, "%s", "public-bucket", 1)
	ts.SetBucketPolicy(t, "public-bucket", policy)

	// Anonymous access to private bucket should fail
	privateResp, err := AnonymousRequest("GET", ts.ObjectURL("private-bucket", "secret.txt"), nil)
	require.NoError(t, err)
	defer privateResp.Body.Close()
	assert.Equal(t, http.StatusForbidden, privateResp.StatusCode, "Private bucket should deny anonymous access")

	// Anonymous access to public bucket should succeed
	publicResp, err := AnonymousRequest("GET", ts.ObjectURL("public-bucket", "public.txt"), nil)
	require.NoError(t, err)
	defer publicResp.Body.Close()
	assert.Equal(t, http.StatusOK, publicResp.StatusCode, "Public bucket should allow anonymous access")

	body, _ := io.ReadAll(publicResp.Body)
	assert.Equal(t, "public data", string(body))
}

func TestBucketPolicy_AuthenticatedUserStillWorksOnPublicBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "public-bucket"

	// Create bucket and upload object as admin
	ts.CreateBucket(t, bucket)
	ts.PutObject(t, bucket, "file.txt", "content")

	// Set public read policy
	policy := strings.Replace(publicReadPolicy, "%s", bucket, 1)
	ts.SetBucketPolicy(t, bucket, policy)

	// Authenticated GET should also work (admin bypass)
	req, err := http.NewRequest("GET", ts.ObjectURL(bucket, "file.txt"), nil)
	require.NoError(t, err)
	ts.SignRequest(req, nil)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Authenticated GET should still work on public bucket")
}

func TestBucketPolicy_AdminCanStillWriteToPublicReadBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "public-bucket"

	// Create bucket as admin
	ts.CreateBucket(t, bucket)

	// Set public read policy (read-only for anonymous)
	policy := strings.Replace(publicReadPolicy, "%s", bucket, 1)
	ts.SetBucketPolicy(t, bucket, policy)

	// Admin PUT should still work (admin bypass)
	ts.PutObject(t, bucket, "admin-file.txt", "admin wrote this")

	// Verify the file exists via anonymous read
	resp, err := AnonymousRequest("GET", ts.ObjectURL(bucket, "admin-file.txt"), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "admin wrote this", string(body))
}

func TestBucketPolicy_NonExistentObjectReturns404NotForbidden(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "public-bucket"

	// Create bucket (no objects)
	ts.CreateBucket(t, bucket)

	// Set public read policy
	policy := strings.Replace(publicReadPolicy, "%s", bucket, 1)
	ts.SetBucketPolicy(t, bucket, policy)

	// Anonymous GET for non-existent object should return 404, not 403
	// (permission is granted, object just doesn't exist)
	resp, err := AnonymousRequest("GET", ts.ObjectURL(bucket, "does-not-exist.txt"), nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"Non-existent object in public bucket should return 404, not 403")
}
