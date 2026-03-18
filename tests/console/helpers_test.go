package console_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/mallardduck/dirio/console"
	consolewire "github.com/mallardduck/dirio/internal/console"
	"github.com/mallardduck/dirio/internal/http/auth"
	internalserver "github.com/mallardduck/dirio/internal/http/server"
	"github.com/mallardduck/dirio/internal/service"
	"github.com/mallardduck/dirio/internal/testutil"
	"github.com/mallardduck/dirio/pkg/iam"
)

const consolePrefix = "/dirio/ui"

// TestServer extends testutil.TestServer with console-specific methods.
// Embedding the pointer promotes all base methods (CreateBucket, SignRequest,
// SetBucketPolicy, etc.) so console_test.go files need no changes.
type TestServer struct {
	*testutil.TestServer
}

// NewTestServer creates a server with the console wired in before it starts.
// testutil.NewWithPreStartHook calls the hook after server.New but before
// srv.Start, so SetConsole takes effect when buildHandler() runs inside Start.
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()

	ts := testutil.NewWithPreStartHook(t, func(srv *internalserver.Server) {
		// Wire the console (mirrors cmd/server/cmd/wire_console.go).
		factory := service.NewServiceFactory(srv.Storage(), srv.Metadata(), srv.PolicyEngine(), srv.Auth())
		adapter := consolewire.NewAdapter(factory)
		handler := console.New(adapter, srv.Router(), &testAdminAuth{authenticator: srv.Auth()}, "test")
		srv.SetConsole(handler, 0) // 0 = same port, mounted at /dirio/ui/
	})

	return &TestServer{TestServer: ts}
}

// noRedirectClient never follows redirects.
var noRedirectClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// ConsoleURL returns the full URL for a console-relative path (e.g. "/login").
func (ts *TestServer) ConsoleURL(path string) string {
	return ts.BaseURL + consolePrefix + path
}

// ConsoleGet makes a GET request to the console with the given session cookie.
func (ts *TestServer) ConsoleGet(t *testing.T, path string, session *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.ConsoleURL(path), http.NoBody)
	if err != nil {
		t.Fatalf("build console GET request: %v", err)
	}
	if session != nil {
		req.AddCookie(session)
	}
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("console GET %s: %v", path, err)
	}
	return resp
}

// ConsolePost makes a POST form request to the console with the given session cookie.
func (ts *TestServer) ConsolePost(t *testing.T, path string, form url.Values, session *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.ConsoleURL(path), strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build console POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if session != nil {
		req.AddCookie(session)
	}
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("console POST %s: %v", path, err)
	}
	return resp
}

// Login performs a console login and returns the session cookie.
func (ts *TestServer) Login(t *testing.T) *http.Cookie {
	t.Helper()
	return ts.LoginAs(t, ts.AccessKey, ts.SecretKey)
}

// LoginAs performs a console login with the given credentials.
func (ts *TestServer) LoginAs(t *testing.T, accessKey, secretKey string) *http.Cookie {
	t.Helper()
	form := url.Values{
		"access_key": {accessKey},
		"secret_key": {secretKey},
	}
	resp, err := noRedirectClient.PostForm(ts.ConsoleURL("/login"), form)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 after login, got %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "dirio_console_session" {
			return c
		}
	}
	t.Fatalf("no session cookie in login response")
	return nil
}

// CreateUser creates an IAM user via the MinIO admin API.
func (ts *TestServer) CreateUser(t *testing.T, accessKey, secretKey string) {
	t.Helper()
	body := map[string]string{"secretKey": secretKey, "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey="+url.QueryEscape(accessKey), body)
	testutil.DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateUser %q: status %d", accessKey, resp.StatusCode)
	}
}

// ReadBody reads and returns the full response body as a string, then closes it.
func ReadBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(b)
}

// DrainAndClose discards the response body and closes it.
func DrainAndClose(resp *http.Response) { testutil.DrainAndClose(resp) }

// testAdminAuth implements consoleauth.AdminAuth.
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

// allowAllPolicy returns a bucket policy allowing all S3 actions for any principal.
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
