package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDualAdminAccess(t *testing.T) {
	t.Run("both credentials work when CLI explicitly set", func(t *testing.T) {
		// Create server with custom CLI credentials (explicitly set) and data config
		ts := NewTestServerWithExplicitCredentialsAndDataConfig(t,
			"cli-admin", "cli-secret",
			"data-admin", "data-secret")
		defer ts.Cleanup()

		// Test CLI credentials work
		t.Run("CLI credentials authenticate", func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL("/"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "cli-admin", "cli-secret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Should not get 403 Forbidden (which indicates auth failure)
			assert.NotEqual(t, http.StatusForbidden, resp.StatusCode,
				"CLI credentials should work when explicitly set")
		})

		// Test data config credentials work
		t.Run("data config credentials authenticate", func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL("/"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "data-admin", "data-secret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Should not get 403 Forbidden
			assert.NotEqual(t, http.StatusForbidden, resp.StatusCode,
				"Data config credentials should work")
		})

		// Test wrong credentials fail
		t.Run("wrong credentials fail", func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL("/"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "wrong-key", "wrong-secret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Should get 403 Forbidden
			assert.Equal(t, http.StatusForbidden, resp.StatusCode,
				"Wrong credentials should fail")
		})
	})

	t.Run("only data config works when CLI not explicitly set", func(t *testing.T) {
		// Create server with default CLI credentials (not explicitly set) and data config
		ts := NewTestServerWithDefaultsAndDataConfig(t, "data-admin", "data-secret")
		defer ts.Cleanup()

		// Test data config credentials work
		t.Run("data config credentials authenticate", func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL("/"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "data-admin", "data-secret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Should not get 403 Forbidden
			assert.NotEqual(t, http.StatusForbidden, resp.StatusCode,
				"Data config credentials should work")
		})

		// Test default CLI credentials do NOT work (security feature)
		t.Run("default CLI credentials blocked", func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL("/"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "testaccess", "testsecret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Should get 403 Forbidden when default credentials are not explicitly set
			// This is the security feature we're implementing
			assert.Equal(t, http.StatusForbidden, resp.StatusCode,
				"Default CLI credentials should NOT work when not explicitly set and data config exists")
		})
	})

	t.Run("CLI defaults work for initial setup without data config", func(t *testing.T) {
		// Create server with defaults, no data config
		ts := NewTestServerWithDefaults(t)
		defer ts.Cleanup()

		// Do NOT create data config - testing initial setup

		// Test default credentials work (needed for first run)
		t.Run("default credentials work initially", func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL("/"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "testaccess", "testsecret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Should not get 403 Forbidden
			assert.NotEqual(t, http.StatusForbidden, resp.StatusCode,
				"Default credentials should work for initial setup")
		})

		// Verify we can create a bucket with default credentials
		t.Run("can create bucket with defaults", func(t *testing.T) {
			req, err := http.NewRequest("PUT", ts.BucketURL("test-bucket"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "testaccess", "testsecret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"Should be able to create bucket with default credentials during initial setup")
		})
	})

	t.Run("both admins can perform operations independently", func(t *testing.T) {
		// Create server with explicit CLI credentials and data config
		ts := NewTestServerWithExplicitCredentialsAndDataConfig(t,
			"cli-admin", "cli-secret",
			"data-admin", "data-secret")
		defer ts.Cleanup()

		// CLI admin creates a bucket
		t.Run("CLI admin creates bucket", func(t *testing.T) {
			req, err := http.NewRequest("PUT", ts.BucketURL("cli-bucket"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "cli-admin", "cli-secret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		// Data admin creates a bucket
		t.Run("data admin creates bucket", func(t *testing.T) {
			req, err := http.NewRequest("PUT", ts.BucketURL("data-bucket"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "data-admin", "data-secret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		// Both can list buckets
		t.Run("CLI admin lists buckets", func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL("/"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "cli-admin", "cli-secret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)
			assert.Contains(t, bodyStr, "cli-bucket")
			assert.Contains(t, bodyStr, "data-bucket")
		})

		t.Run("data admin lists buckets", func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL("/"), http.NoBody)
			require.NoError(t, err)
			SignRequestWithCredentials(req, nil, "data-admin", "data-secret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)
			assert.Contains(t, bodyStr, "cli-bucket")
			assert.Contains(t, bodyStr, "data-bucket")
		})

		// Both can upload objects
		t.Run("CLI admin uploads object", func(t *testing.T) {
			content := "Hello from CLI admin"
			body := []byte(content)
			req, err := http.NewRequest("PUT", ts.ObjectURL("cli-bucket", "test.txt"),
				strings.NewReader(content))
			require.NoError(t, err)
			req.ContentLength = int64(len(content))
			SignRequestWithCredentials(req, body, "cli-admin", "cli-secret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("data admin uploads object", func(t *testing.T) {
			content := "Hello from data admin"
			body := []byte(content)
			req, err := http.NewRequest("PUT", ts.ObjectURL("data-bucket", "test.txt"),
				strings.NewReader(content))
			require.NoError(t, err)
			req.ContentLength = int64(len(content))
			SignRequestWithCredentials(req, body, "data-admin", "data-secret")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	})
}
