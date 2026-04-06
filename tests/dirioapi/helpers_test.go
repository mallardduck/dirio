package dirioapi

// helpers_test.go re-exports shared test infrastructure from internal/testutil
// and adds DirIO API-specific request helpers.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/mallardduck/dirio/internal/testutil"
)

// TestServer is an alias so test files need no direct testutil import.
type TestServer = testutil.TestServer

// NewTestServer starts a server with the standard test credentials.
func NewTestServer(t *testing.T) *testutil.TestServer {
	t.Helper()
	return testutil.New(t)
}

// DrainAndClose discards the response body and closes it.
func DrainAndClose(resp *http.Response) { testutil.DrainAndClose(resp) }

// DecodeJSON reads and JSON-decodes the response body into v, then closes it.
func DecodeJSON(t *testing.T, resp *http.Response, v any) {
	testutil.DecodeJSON(t, resp, v)
}

// SignRequestWithCredentials signs req in-place with the provided credentials.
func SignRequestWithCredentials(req *http.Request, body []byte, accessKey, secretKey string) {
	testutil.SignRequestWithCredentials(req, body, accessKey, secretKey)
}

// dirioURL returns the full URL for a DirIO API v1 path.
func dirioURL(ts *testutil.TestServer, path string) string {
	return ts.URL("/.dirio/api/v1" + path)
}

// newDirioRequest builds a signed DirIO API request using the server's admin credentials.
func newDirioRequest(t *testing.T, ts *testutil.TestServer, method, path string, body []byte) *http.Request {
	t.Helper()
	req, err := ts.NewRequest(method, dirioURL(ts, path), body)
	if err != nil {
		t.Fatalf("dirioapi: build request %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// newUnsignedDirioRequest builds an unsigned DirIO API request (no auth headers).
func newUnsignedDirioRequest(t *testing.T, ts *testutil.TestServer, method, path string, body []byte) *http.Request {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, dirioURL(ts, path), r)
	if err != nil {
		t.Fatalf("dirioapi: build unsigned request %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// newDirioRequestAs builds a signed DirIO API request using the given credentials.
func newDirioRequestAs(t *testing.T, ts *testutil.TestServer, method, path string, body []byte, accessKey, secretKey string) *http.Request {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, dirioURL(ts, path), r)
	if err != nil {
		t.Fatalf("dirioapi: build request as %s %s %s: %v", accessKey, method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	SignRequestWithCredentials(req, body, accessKey, secretKey)
	return req
}

// do executes req and returns the response.  Fails the test on network error.
func do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("dirioapi: execute %s %s: %v", req.Method, req.URL, err)
	}
	return resp
}

// decodeErrorCode reads the JSON error envelope and returns the code field.
func decodeErrorCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("dirioapi: decode error envelope: %v", err)
	}
	return envelope.Error.Code
}

// createUser creates a user via the MinIO admin API.  Fails the test on error.
func createUser(t *testing.T, ts *testutil.TestServer, accessKey, secretKey string) {
	t.Helper()
	body := map[string]string{"secretKey": secretKey, "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey="+accessKey, body)
	DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dirioapi: createUser %q: status %d", accessKey, resp.StatusCode)
	}
}
