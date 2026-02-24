package admin

// This file re-exports the shared test server helpers from internal/testutil.
// Admin-specific helpers (samplePolicyDocument) remain here.

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mallardduck/dirio/internal/testutil"
)

// TestServer is an alias so admin test files need no import changes.
type TestServer = testutil.TestServer

// NewTestServer starts a server with the standard test credentials.
func NewTestServer(t *testing.T) *testutil.TestServer {
	t.Helper()
	return testutil.New(t)
}

// NewTestServerWithDataDir starts a server against an existing data directory.
// Used by MinIO import tests.  Call Stop() to shut down without removing data.
func NewTestServerWithDataDir(t *testing.T, dataDir string) *testutil.TestServer {
	t.Helper()
	return testutil.NewWithDataDir(t, dataDir)
}

// FindFreePort finds an available TCP port.
func FindFreePort(t *testing.T) int { return testutil.FindFreePort(t) }

// DrainAndClose discards the response body and closes it.
func DrainAndClose(resp *http.Response) { testutil.DrainAndClose(resp) }

// DecodeJSON reads and decodes the JSON body of an HTTP response.
func DecodeJSON(t *testing.T, resp *http.Response, v any) {
	testutil.DecodeJSON(t, resp, v)
}

// samplePolicyDocument returns a valid IAM policy JSON for testing.
func samplePolicyDocument(bucket string) []byte {
	return fmt.Appendf(nil, `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": ["s3:GetObject", "s3:PutObject"],
				"Resource": ["arn:aws:s3:::%s/*"]
			}
		]
	}`, bucket)
}
