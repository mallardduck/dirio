package admin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/minio/madmin-go/v3"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/http/server"
)

// TestServer wraps a dirio server for admin integration testing
type TestServer struct {
	Server    *server.Server
	DataDir   string
	Port      int
	BaseURL   string
	AdminURL  string
	AccessKey string
	SecretKey string
	cancel    context.CancelFunc
	done      chan struct{}
}

// NewTestServer creates and starts a new test server with a fresh data directory
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()

	dataDir, err := os.MkdirTemp("", "dirio-admin-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	port := findAvailablePort(t)
	config := &server.Config{
		DataDir:   dataDir,
		Port:      port,
		AccessKey: "testaccess",
		SecretKey: "testsecretkey123",
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
		AdminURL:  fmt.Sprintf("http://localhost:%d/minio/admin/v3", port),
		AccessKey: config.AccessKey,
		SecretKey: config.SecretKey,
	}

	go func() { _ = srv.Start(context.Background()) }()

	if !ts.waitForReady(5 * time.Second) {
		ts.Cleanup()
		t.Fatalf("Server failed to start within timeout")
	}

	t.Cleanup(ts.Cleanup)
	return ts
}

// NewTestServerWithDataDir creates a test server using an existing data directory.
// Used for MinIO import tests where the data dir is pre-populated.
// Call Stop() to shut the server down without removing the data directory.
func NewTestServerWithDataDir(t *testing.T, dataDir string) *TestServer {
	t.Helper()

	port := findAvailablePort(t)
	config := &server.Config{
		DataDir:   dataDir,
		Port:      port,
		AccessKey: "testaccess",
		SecretKey: "testsecretkey123",
	}

	srv, err := server.New(config)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	ts := &TestServer{
		Server:    srv,
		DataDir:   dataDir,
		Port:      port,
		BaseURL:   fmt.Sprintf("http://localhost:%d", port),
		AdminURL:  fmt.Sprintf("http://localhost:%d/minio/admin/v3", port),
		AccessKey: config.AccessKey,
		SecretKey: config.SecretKey,
		cancel:    cancel,
		done:      done,
	}

	go func() {
		defer close(done)
		_ = srv.Start(ctx)
	}()

	if !ts.waitForReady(5 * time.Second) {
		ts.Stop()
		t.Fatalf("Server failed to start within timeout")
	}

	return ts
}

// Stop shuts down the server gracefully without removing the data directory.
// Blocks until the server has fully stopped (and the bolt DB is closed).
func (ts *TestServer) Stop() {
	if ts.cancel != nil {
		ts.cancel()
	}
	if ts.done != nil {
		<-ts.done
	}
}

// Cleanup removes the test data directory
func (ts *TestServer) Cleanup() {
	if ts.DataDir != "" {
		os.RemoveAll(ts.DataDir)
	}
}

// waitForReady polls the server until it responds or times out
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

// AdminRequest performs a signed admin API request without an encrypted body
func (ts *TestServer) AdminRequest(t *testing.T, method, path string, body []byte) *http.Response {
	t.Helper()
	url := ts.AdminURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	if body != nil {
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/json")
	}

	ts.signRequest(req, body)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// EncryptedAdminRequest performs a signed admin API request with a madmin-encrypted body.
// The body is JSON-marshaled then encrypted with the admin secret key.
func (ts *TestServer) EncryptedAdminRequest(t *testing.T, method, path string, bodyData interface{}) *http.Response {
	t.Helper()

	jsonData, err := json.Marshal(bodyData)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	encrypted, err := madmin.EncryptData(ts.SecretKey, jsonData)
	if err != nil {
		t.Fatalf("Failed to encrypt request body: %v", err)
	}

	url := ts.AdminURL + path
	req, err := http.NewRequest(method, url, bytes.NewReader(encrypted))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.ContentLength = int64(len(encrypted))
	req.Header.Set("Content-Type", "application/octet-stream")

	ts.signRequest(req, encrypted)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// signRequest signs an HTTP request with AWS Signature V4
func (ts *TestServer) signRequest(req *http.Request, body []byte) {
	timestamp := time.Now().UTC()

	var payloadHash string
	if body != nil {
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
	signature := auth.ComputeSignature(ts.SecretKey, timestamp, region, stringToSign)

	dateStamp := timestamp.Format("20060102")
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		ts.AccessKey, credentialScope, strings.Join(signedHeaders, ";"), signature)

	req.Header.Set("Authorization", authHeader)
}

// DecodeJSON reads and decodes the JSON body of an HTTP response
func DecodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}
}

// DrainAndClose reads and discards the response body and closes it
func DrainAndClose(resp *http.Response) {
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

// samplePolicyDocument returns a valid IAM policy document JSON for testing
func samplePolicyDocument(bucket string) []byte {
	doc := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": ["s3:GetObject", "s3:PutObject"],
				"Resource": ["arn:aws:s3:::%s/*"]
			}
		]
	}`, bucket)
	return []byte(doc)
}
