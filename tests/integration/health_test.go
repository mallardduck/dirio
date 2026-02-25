package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// healthBody is a partial unmarshal of the /.dirio/health JSON response.
type healthBody struct {
	Status     string                     `json:"status"`
	Uptime     string                     `json:"uptime"`
	Components map[string]componentHealth `json:"components"`
}

type componentHealth struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// TestHealthEndpoint verifies /.dirio/health returns 200 with valid JSON when
// the server is healthy.
func TestHealthEndpoint(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.URL("/.dirio/health"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body healthBody
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.Equal(t, "ok", body.Status)
	assert.NotEmpty(t, body.Uptime, "uptime should be present")

	dbComp, ok := body.Components["metadata_db"]
	require.True(t, ok, "metadata_db component should be present")
	assert.Equal(t, "ok", dbComp.Status)

	storComp, ok := body.Components["storage"]
	require.True(t, ok, "storage component should be present")
	assert.Equal(t, "ok", storComp.Status)
}

// TestHealthEndpointResponseShape verifies the exact JSON keys are present and
// the response body is valid JSON regardless of component values.
func TestHealthEndpointResponseShape(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.URL("/.dirio/health"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var raw map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))

	assert.Contains(t, raw, "status")
	assert.Contains(t, raw, "uptime")
	assert.Contains(t, raw, "components")
}

// TestHealthReadyEndpoint verifies /.dirio/health/ready returns 200 when healthy.
func TestHealthReadyEndpoint(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.URL("/.dirio/health/ready"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestHealthLiveEndpoint verifies /.dirio/health/live always returns 200.
func TestHealthLiveEndpoint(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.URL("/.dirio/health/live"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestHealthzLegacyAlias verifies /.dirio/healthz still returns 200 for
// Docker HEALTHCHECK directives that may reference it.
func TestHealthzLegacyAlias(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.URL("/.dirio/healthz"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestMinIOCompatHealthEndpoints verifies /minio/health/live and
// /minio/health/ready exist and return 200, matching the MinIO health router
// that mc and other MinIO-compatible clients expect.
func TestMinIOCompatHealthEndpoints(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	for _, ep := range []string{"/minio/health/live", "/minio/health/ready"} {
		t.Run(strings.TrimPrefix(ep, "/"), func(t *testing.T) {
			resp, err := http.Get(ts.URL(ep))
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

// TestHealthEndpointNoAuthRequired verifies all health endpoints are
// accessible without authentication.
func TestHealthEndpointNoAuthRequired(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	endpoints := []string{
		"/.dirio/health",
		"/.dirio/health/ready",
		"/.dirio/health/live",
		"/.dirio/healthz",
		"/minio/health/live",
		"/minio/health/ready",
	}
	for _, ep := range endpoints {
		t.Run(strings.TrimPrefix(ep, "/"), func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, ts.URL(ep), nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"endpoint %s should be accessible without auth", ep)
		})
	}
}

// TestHealthUptimeIncreases verifies uptime is non-empty and contains a
// recognisable Go duration suffix.
func TestHealthUptimeFormat(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.URL("/.dirio/health"))
	require.NoError(t, err)
	defer resp.Body.Close()

	var body healthBody
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	uptime := body.Uptime
	assert.NotEmpty(t, uptime)
	hasSuffix := strings.HasSuffix(uptime, "s") ||
		strings.HasSuffix(uptime, "m") ||
		strings.HasSuffix(uptime, "h") ||
		strings.HasSuffix(uptime, "ms") ||
		strings.HasSuffix(uptime, "us")
	assert.True(t, hasSuffix, "uptime %q should end with a duration unit", uptime)
}
