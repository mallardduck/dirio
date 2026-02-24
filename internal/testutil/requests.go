package testutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/minio/madmin-go/v3"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/http/auth"
)

// ---------------------------------------------------------------------------
// AWS Signature V4 helpers
// ---------------------------------------------------------------------------

// SignRequest signs req in-place using the server's own credentials.
func (ts *TestServer) SignRequest(req *http.Request, body []byte) {
	SignRequestWithCredentials(req, body, ts.AccessKey, ts.SecretKey)
}

// SignRequestWithCredentials signs req in-place using the provided credentials.
// Useful when a test needs to sign requests as a different user.
func SignRequestWithCredentials(req *http.Request, body []byte, accessKey, secretKey string) {
	timestamp := time.Now().UTC()

	var payloadHash string
	if len(body) > 0 {
		h := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(h[:])
	} else {
		h := sha256.Sum256([]byte{})
		payloadHash = hex.EncodeToString(h[:])
	}

	req.Header.Set("X-Amz-Date", timestamp.Format("20060102T150405Z"))
	req.Header.Set(consts.HeaderContentSHA256, payloadHash)
	req.Header.Set("Host", req.Host)

	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	sort.Strings(signedHeaders)

	canonicalRequest := auth.BuildCanonicalRequest(req, signedHeaders, payloadHash)
	region := "us-east-1"
	stringToSign := auth.BuildStringToSign(timestamp, region, canonicalRequest)
	signature := auth.ComputeSignature(secretKey, timestamp, region, stringToSign)

	dateStamp := timestamp.Format("20060102")
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, strings.Join(signedHeaders, ";"), signature)

	req.Header.Set("Authorization", authHeader)
}

// ---------------------------------------------------------------------------
// S3 API helpers
// ---------------------------------------------------------------------------

// NewRequest builds a signed HTTP request against the server.
func (ts *TestServer) NewRequest(method, url string, body []byte) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	ts.SignRequest(req, body)
	return req, nil
}

// CreateBucket creates a bucket via the S3 API and fails the test on error.
func (ts *TestServer) CreateBucket(t *testing.T, bucket string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, ts.BucketURL(bucket), nil)
	if err != nil {
		t.Fatalf("testutil: build CreateBucket request: %v", err)
	}
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("testutil: CreateBucket %q: %v", bucket, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("testutil: CreateBucket %q: status %d body: %s", bucket, resp.StatusCode, body)
	}
}

// PutObject uploads an object via the S3 API and fails the test on error.
func (ts *TestServer) PutObject(t *testing.T, bucket, key, content string) {
	t.Helper()
	body := []byte(content)
	req, err := http.NewRequest(http.MethodPut, ts.ObjectURL(bucket, key), strings.NewReader(content))
	if err != nil {
		t.Fatalf("testutil: build PutObject request: %v", err)
	}
	req.ContentLength = int64(len(content))
	ts.SignRequest(req, body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("testutil: PutObject %s/%s: %v", bucket, key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("testutil: PutObject %s/%s: status %d body: %s", bucket, key, resp.StatusCode, respBody)
	}
}

// CreateTestObjects uploads multiple objects from a map[key]content.
func (ts *TestServer) CreateTestObjects(t *testing.T, bucket string, objects map[string]string) {
	t.Helper()
	for key, content := range objects {
		ts.PutObject(t, bucket, key, content)
	}
}

// ---------------------------------------------------------------------------
// MinIO admin API helpers
// ---------------------------------------------------------------------------

// AdminRequest performs a signed request against the MinIO admin API.
// body is sent as-is (use nil for requests without a body).
func (ts *TestServer) AdminRequest(t *testing.T, method, path string, body []byte) *http.Response {
	t.Helper()
	url := ts.AdminURL + path

	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("testutil: build admin request: %v", err)
	}
	if body != nil {
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/json")
	}
	ts.SignRequest(req, body)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("testutil: admin request %s %s: %v", method, path, err)
	}
	return resp
}

// EncryptedAdminRequest JSON-marshals bodyData, encrypts it with the admin
// secret key using madmin, and sends it as an admin API request.
func (ts *TestServer) EncryptedAdminRequest(t *testing.T, method, path string, bodyData interface{}) *http.Response {
	t.Helper()

	jsonData, err := json.Marshal(bodyData)
	if err != nil {
		t.Fatalf("testutil: marshal admin request body: %v", err)
	}
	encrypted, err := madmin.EncryptData(ts.SecretKey, jsonData)
	if err != nil {
		t.Fatalf("testutil: encrypt admin request body: %v", err)
	}

	url := ts.AdminURL + path
	req, err := http.NewRequest(method, url, bytes.NewReader(encrypted))
	if err != nil {
		t.Fatalf("testutil: build encrypted admin request: %v", err)
	}
	req.ContentLength = int64(len(encrypted))
	req.Header.Set("Content-Type", "application/octet-stream")
	ts.SignRequest(req, encrypted)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("testutil: encrypted admin request %s %s: %v", method, path, err)
	}
	return resp
}

// SetBucketPolicy sets a bucket policy via the S3 API, failing the test on error.
func (ts *TestServer) SetBucketPolicy(t *testing.T, bucket, policyJSON string) {
	t.Helper()
	body := []byte(policyJSON)
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s?policy", ts.BaseURL, bucket), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("testutil: build SetBucketPolicy request: %v", err)
	}
	req.ContentLength = int64(len(body))
	ts.SignRequest(req, body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("testutil: SetBucketPolicy %q: %v", bucket, err)
	}
	DrainAndClose(resp)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("testutil: SetBucketPolicy %q: status %d", bucket, resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

// DecodeJSON reads and JSON-decodes the response body into v, then closes it.
func DecodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("testutil: decode JSON response: %v", err)
	}
}

// DecryptAndDecodeJSON decrypts a madmin-encrypted response body and JSON-decodes it into v.
func (ts *TestServer) DecryptAndDecodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("testutil: read encrypted response: %v", err)
	}
	decrypted, err := madmin.DecryptData(ts.SecretKey, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("testutil: decrypt response: %v", err)
	}
	if err := json.Unmarshal(decrypted, v); err != nil {
		t.Fatalf("testutil: decode decrypted JSON response: %v", err)
	}
}
