package console_test

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
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/minio/madmin-go/v3"

	"github.com/mallardduck/dirio/console"
	consolewire "github.com/mallardduck/dirio/internal/console"
	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/http/server"
	"github.com/mallardduck/dirio/internal/service"
	"github.com/mallardduck/dirio/pkg/iam"
)

const (
	consolePrefix = "/dirio/ui"
	testAccessKey = "testaccess"
	testSecretKey = "testsecretkey123"
)

// TestServer wraps a dirio server with the console wired in for stopgap feature testing.
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

// NewTestServer creates and starts a test server with the console enabled.
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()

	dataDir, err := os.MkdirTemp("", "dirio-console-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	port := findAvailablePort(t)
	config := &server.Config{
		DataDir:   dataDir,
		Port:      port,
		AccessKey: testAccessKey,
		SecretKey: testSecretKey,
	}

	srv, err := server.New(config)
	if err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("Failed to create server: %v", err)
	}

	// Wire the console (mirrors cmd/server/cmd/wire_console.go).
	factory := service.NewServiceFactory(srv.Storage(), srv.Metadata(), srv.PolicyEngine())
	adapter := consolewire.NewAdapter(factory)
	handler := console.New(adapter, srv.Router(), &testAdminAuth{authenticator: srv.Auth()})
	srv.SetConsole(handler, 0) // 0 = same port, mounted at /dirio/ui/

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
		ts.Cleanup()
		t.Fatalf("Server failed to start within timeout")
	}

	t.Cleanup(ts.Cleanup)
	return ts
}

// Stop shuts down the server gracefully without removing the data directory.
func (ts *TestServer) Stop() {
	if ts.cancel != nil {
		ts.cancel()
	}
	if ts.done != nil {
		<-ts.done
	}
}

// Cleanup stops the server and removes the temp data directory.
func (ts *TestServer) Cleanup() {
	ts.Stop()
	if ts.DataDir != "" {
		os.RemoveAll(ts.DataDir)
	}
}

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

// ConsoleURL returns the full URL for a console path (e.g. "/login").
func (ts *TestServer) ConsoleURL(path string) string {
	return ts.BaseURL + consolePrefix + path
}

// noRedirectClient returns an HTTP client that never follows redirects.
var noRedirectClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// Login performs a console login and returns the session cookie.
// Fails the test if login does not succeed.
func (ts *TestServer) Login(t *testing.T) *http.Cookie {
	t.Helper()
	return ts.LoginAs(t, ts.AccessKey, ts.SecretKey)
}

// LoginAs performs a console login with the given credentials.
// Returns the session cookie on success, or fails the test.
func (ts *TestServer) LoginAs(t *testing.T, accessKey, secretKey string) *http.Cookie {
	t.Helper()

	form := url.Values{
		"access_key": {accessKey},
		"secret_key": {secretKey},
	}
	resp, err := noRedirectClient.PostForm(ts.ConsoleURL("/login"), form)
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("Expected 303 redirect after login, got %d", resp.StatusCode)
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "dirio_console_session" {
			return cookie
		}
	}
	t.Fatalf("No session cookie in login response")
	return nil
}

// ConsoleGet makes a GET request to the console with the given session cookie.
func (ts *TestServer) ConsoleGet(t *testing.T, path string, session *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.ConsoleURL(path), nil)
	if err != nil {
		t.Fatalf("Failed to create GET request: %v", err)
	}
	if session != nil {
		req.AddCookie(session)
	}
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	return resp
}

// ConsolePost makes a POST form request to the console with the given session cookie.
func (ts *TestServer) ConsolePost(t *testing.T, path string, form url.Values, session *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.ConsoleURL(path), strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("Failed to create POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if session != nil {
		req.AddCookie(session)
	}
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	return resp
}

// ReadBody reads and returns the full response body as a string, and closes the body.
func ReadBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	return string(b)
}

// DrainAndClose discards the response body and closes it.
func DrainAndClose(resp *http.Response) {
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

// --- S3 / Admin setup helpers ------------------------------------------------

// CreateBucket creates a bucket via the S3 API.
func (ts *TestServer) CreateBucket(t *testing.T, bucket string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", ts.BaseURL, bucket), nil)
	if err != nil {
		t.Fatalf("Failed to create bucket request: %v", err)
	}
	ts.signRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("CreateBucket failed: %v", err)
	}
	DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateBucket %q returned %d", bucket, resp.StatusCode)
	}
}

// SetBucketPolicy sets a bucket policy via the S3 API.
func (ts *TestServer) SetBucketPolicy(t *testing.T, bucket string, policyJSON string) {
	t.Helper()
	body := []byte(policyJSON)
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s?policy", ts.BaseURL, bucket), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create set-policy request: %v", err)
	}
	req.ContentLength = int64(len(body))
	ts.signRequest(req, body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SetBucketPolicy failed: %v", err)
	}
	DrainAndClose(resp)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("SetBucketPolicy %q returned %d", bucket, resp.StatusCode)
	}
}

// CreateUser creates an IAM user via the MinIO admin API.
func (ts *TestServer) CreateUser(t *testing.T, accessKey, secretKey string) {
	t.Helper()

	body := map[string]string{
		"secretKey": secretKey,
		"status":    "enabled",
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal user body: %v", err)
	}
	encrypted, err := madmin.EncryptData(ts.SecretKey, jsonData)
	if err != nil {
		t.Fatalf("Failed to encrypt user body: %v", err)
	}

	path := fmt.Sprintf("%s/add-user?accessKey=%s", ts.AdminURL, url.QueryEscape(accessKey))
	req, err := http.NewRequest(http.MethodPut, path, bytes.NewReader(encrypted))
	if err != nil {
		t.Fatalf("Failed to create add-user request: %v", err)
	}
	req.ContentLength = int64(len(encrypted))
	req.Header.Set("Content-Type", "application/octet-stream")
	ts.signRequest(req, encrypted)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateUser %q returned %d", accessKey, resp.StatusCode)
	}
}

// signRequest signs an HTTP request with AWS Signature V4 using the admin credentials.
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

// findAvailablePort finds an available TCP port.
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

// testAdminAuth implements consoleauth.AdminAuth, matching the logic in wire_console.go.
type testAdminAuth struct {
	authenticator *auth.Authenticator
}

func (a *testAdminAuth) AuthenticateAdmin(ctx context.Context, accessKey, secretKey string) bool {
	if !a.authenticator.ValidateCredentials(ctx, accessKey, secretKey) {
		return false
	}
	user, err := a.authenticator.GetUserForAccessKey(ctx, accessKey)
	if err != nil || user == nil {
		return false
	}
	return user.UUID == iam.AdminUserUUID
}

// publicReadPolicy returns a bucket policy that allows anonymous GetObject.
func publicReadPolicy(bucket string) string {
	return fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": "*",
			"Action": ["s3:GetObject"],
			"Resource": ["arn:aws:s3:::%s/*"]
		}]
	}`, bucket)
}

// allowAllPolicy returns a bucket policy that allows all S3 actions for any authenticated user.
// Using Principal "*" ensures the policy evaluates correctly for any IAM user in simulator tests.
func allowAllPolicy(bucket, _ string) string {
	return fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": "*",
			"Action": ["s3:*"],
			"Resource": ["arn:aws:s3:::%s", "arn:aws:s3:::%s/*"]
		}]
	}`, bucket, bucket)
}
