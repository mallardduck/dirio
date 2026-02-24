package integration

// This file re-exports the shared test server helpers from internal/testutil so
// that the integration test files need no import changes.  All construction now
// goes through startup.Init + startup.Prepare, fixing the "rootFS cannot be nil"
// class of failures.

import (
	"net/http"
	"testing"

	"github.com/mallardduck/dirio/internal/testutil"
)

// TestServer is an alias so integration test files can use the type without
// importing testutil directly.
type TestServer = testutil.TestServer

// NewTestServer starts a server with the standard test credentials.
func NewTestServer(t *testing.T) *testutil.TestServer {
	t.Helper()
	return testutil.New(t)
}

// NewTestServerWithExplicitCredentials starts a server with the given CLI
// credentials marked as explicitly set.
func NewTestServerWithExplicitCredentials(t *testing.T, accessKey, secretKey string) *testutil.TestServer {
	t.Helper()
	return testutil.NewWithCredentials(t, accessKey, secretKey, true)
}

// NewTestServerWithDefaults starts a server whose CLI credentials are NOT
// marked as explicitly set (simulates first-run / no-flag scenario).
func NewTestServerWithDefaults(t *testing.T) *testutil.TestServer {
	t.Helper()
	return testutil.NewWithCredentials(t, testutil.DefaultAccessKey, "testsecret", false)
}

// NewTestServerWithExplicitCredentialsAndDataConfig starts a server in dual-
// admin mode: both CLI credentials and a separate data-config admin.
func NewTestServerWithExplicitCredentialsAndDataConfig(t *testing.T, cliAccessKey, cliSecretKey, dataAccessKey, dataSecretKey string) *testutil.TestServer {
	t.Helper()
	return testutil.NewDualAdmin(t, cliAccessKey, cliSecretKey, dataAccessKey, dataSecretKey)
}

// NewTestServerWithDefaultsAndDataConfig starts a server where the admin
// credentials come solely from a data config (CLI credentials not explicit).
func NewTestServerWithDefaultsAndDataConfig(t *testing.T, dataAccessKey, dataSecretKey string) *testutil.TestServer {
	t.Helper()
	return testutil.NewDataConfigOnly(t, dataAccessKey, dataSecretKey)
}

// CreateDataConfigWithCredentials writes a data config into the running
// server's data directory.  Used to test live credential reload.
func CreateDataConfigWithCredentials(ts *testutil.TestServer, accessKey, secretKey string) {
	testutil.CreateDataConfigWithCredentials(ts, accessKey, secretKey)
}

// SignRequestWithCredentials signs req with the provided credentials.
func SignRequestWithCredentials(req *http.Request, body []byte, accessKey, secretKey string) {
	testutil.SignRequestWithCredentials(req, body, accessKey, secretKey)
}

// FindFreePort finds an available TCP port.
func FindFreePort(t *testing.T) int { return testutil.FindFreePort(t) }

// DrainAndClose discards the response body and closes it.
func DrainAndClose(resp *http.Response) { testutil.DrainAndClose(resp) }
