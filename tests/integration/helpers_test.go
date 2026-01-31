package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mallardduck/dirio/internal/auth"
	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/server"
)

// TestServer wraps a dirio server for integration testing
type TestServer struct {
	Server    *server.Server
	DataDir   string
	Port      int
	BaseURL   string
	AccessKey string
	SecretKey string
}

// NewTestServer creates and starts a new test server
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()

	// Create temp data directory
	dataDir, err := os.MkdirTemp("", "dirio-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Find an available port by letting the OS assign one
	port := findAvailablePort(t)

	config := &server.Config{
		DataDir:   dataDir,
		Port:      port,
		AccessKey: "testaccess",
		SecretKey: "testsecret",
	}

	srv, err := server.New(config)
	if err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("Failed to create server: %v", err)
	}

	ts := &TestServer{
		Server:    srv,
		DataDir:   dataDir,
		Port:      port,
		BaseURL:   fmt.Sprintf("http://localhost:%d", port),
		AccessKey: config.AccessKey,
		SecretKey: config.SecretKey,
	}

	// Start server in background
	go func() {
		srv.Start()
	}()

	// Wait for server to be ready
	if !ts.waitForReady(5 * time.Second) {
		ts.Cleanup()
		t.Fatalf("Server failed to start within timeout")
	}

	return ts
}

// findAvailablePort finds an available TCP port
func findAvailablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

// waitForReady polls the server until it responds or timeout
func (ts *TestServer) waitForReady(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 100 * time.Millisecond}

	for time.Now().Before(deadline) {
		resp, err := client.Get(ts.BaseURL + "/")
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// Cleanup removes the test data directory
func (ts *TestServer) Cleanup() {
	if ts.DataDir != "" {
		os.RemoveAll(ts.DataDir)
	}
}

// URL returns the full URL for a path
func (ts *TestServer) URL(path string) string {
	return ts.BaseURL + path
}

// BucketURL returns the URL for a bucket
func (ts *TestServer) BucketURL(bucket string) string {
	return fmt.Sprintf("%s/%s", ts.BaseURL, bucket)
}

// ObjectURL returns the URL for an object
func (ts *TestServer) ObjectURL(bucket, key string) string {
	return fmt.Sprintf("%s/%s/%s", ts.BaseURL, bucket, key)
}

// NewRequest creates a new signed HTTP request
func (ts *TestServer) NewRequest(method, url string, body []byte) (*http.Request, error) {
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
	ts.SignRequest(req, body)
	return req, nil
}

// SignRequest signs an HTTP request with AWS Signature V4
func (ts *TestServer) SignRequest(req *http.Request, body []byte) {
	// Get current timestamp
	timestamp := time.Now().UTC()

	// Calculate payload hash
	var payloadHash string
	if body != nil {
		h := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(h[:])
	} else {
		// Empty payload hash
		h := sha256.Sum256([]byte{})
		payloadHash = hex.EncodeToString(h[:])
	}

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

// CreateBucket creates a bucket and fails the test if it fails
func (ts *TestServer) CreateBucket(t *testing.T, bucket string) {
	t.Helper()
	req, _ := http.NewRequest("PUT", ts.BucketURL(bucket), nil)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create bucket %s: %v", bucket, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create bucket %s: status %d, body: %s", bucket, resp.StatusCode, body)
	}
}

// CreateTestObjects creates multiple test objects in a bucket
func (ts *TestServer) CreateTestObjects(t *testing.T, bucket string, objects map[string]string) {
	t.Helper()
	for key, content := range objects {
		ts.PutObject(t, bucket, key, content)
	}
}

// PutObject uploads an object
func (ts *TestServer) PutObject(t *testing.T, bucket, key, content string) {
	t.Helper()
	body := []byte(content)
	req, _ := http.NewRequest("PUT", ts.ObjectURL(bucket, key), strings.NewReader(content))
	req.ContentLength = int64(len(content))
	ts.SignRequest(req, body)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to put object %s/%s: %v", bucket, key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to put object %s/%s: status %d, body: %s", bucket, key, resp.StatusCode, respBody)
	}
}

// DataPath returns the full path to a file in the data directory
func (ts *TestServer) DataPath(parts ...string) string {
	return filepath.Join(append([]string{ts.DataDir}, parts...)...)
}
