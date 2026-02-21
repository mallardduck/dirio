package admin

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListServiceAccounts_Empty verifies an empty list on a fresh server.
func TestListServiceAccounts_Empty(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/list-service-accounts", nil)
	defer DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	DecodeJSON(t, resp, &result)
	accounts, ok := result["accounts"].([]interface{})
	require.True(t, ok, "response should have 'accounts' field")
	assert.Empty(t, accounts)
}

// TestAddServiceAccount_Success creates a service account and verifies it appears in the list.
func TestAddServiceAccount_Success(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{
		"accessKey": "svcaccount1",
		"secretKey": "svcpassword123",
	}
	resp := ts.EncryptedAdminRequest(t, http.MethodPost, "/add-service-account", body)
	defer DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify it appears in the list
	resp2 := ts.AdminRequest(t, http.MethodGet, "/list-service-accounts", nil)
	defer DrainAndClose(resp2)
	var result map[string]interface{}
	DecodeJSON(t, resp2, &result)
	accounts := result["accounts"].([]interface{})
	assert.Len(t, accounts, 1)
	assert.Equal(t, "svcaccount1", accounts[0])
}

// TestAddServiceAccount_AlreadyExists verifies 409 on duplicate access key.
func TestAddServiceAccount_AlreadyExists(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{"accessKey": "svcaccount1", "secretKey": "svcpassword123"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPost, "/add-service-account", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.EncryptedAdminRequest(t, http.MethodPost, "/add-service-account", body)
	defer DrainAndClose(resp2)
	assert.Equal(t, http.StatusConflict, resp2.StatusCode)
}

// TestAddServiceAccount_ConflictsWithUser verifies 409 when using an access key
// already claimed by a regular user.
func TestAddServiceAccount_ConflictsWithUser(t *testing.T) {
	ts := NewTestServer(t)

	// Create a regular user first
	createUser(t, ts, "alice")

	// Try to create a service account with the same key
	body := map[string]string{"accessKey": "alice", "secretKey": "svcpassword123"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPost, "/add-service-account", body)
	defer DrainAndClose(resp)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

// TestAddServiceAccount_MissingKey verifies 400 when accessKey is absent.
func TestAddServiceAccount_MissingKey(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{"secretKey": "svcpassword123"} // no accessKey
	resp := ts.EncryptedAdminRequest(t, http.MethodPost, "/add-service-account", body)
	defer DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestDeleteServiceAccount_Success creates and deletes a service account.
func TestDeleteServiceAccount_Success(t *testing.T) {
	ts := NewTestServer(t)

	// Create
	body := map[string]string{"accessKey": "svcaccount1", "secretKey": "svcpassword123"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPost, "/add-service-account", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Delete
	resp2 := ts.AdminRequest(t, http.MethodPost, "/delete-service-account?accessKey=svcaccount1", nil)
	defer DrainAndClose(resp2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Verify it's gone
	resp3 := ts.AdminRequest(t, http.MethodGet, "/list-service-accounts", nil)
	defer DrainAndClose(resp3)
	var result map[string]interface{}
	DecodeJSON(t, resp3, &result)
	accounts := result["accounts"].([]interface{})
	assert.Empty(t, accounts)
}

// TestDeleteServiceAccount_NotFound verifies 404 for unknown service accounts.
func TestDeleteServiceAccount_NotFound(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodPost, "/delete-service-account?accessKey=ghost", nil)
	defer DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestInfoServiceAccount_Success verifies service account info retrieval.
func TestInfoServiceAccount_Success(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{"accessKey": "svcaccount1", "secretKey": "svcpassword123"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPost, "/add-service-account", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.AdminRequest(t, http.MethodGet, "/info-service-account?accessKey=svcaccount1", nil)
	defer DrainAndClose(resp2)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var info map[string]interface{}
	DecodeJSON(t, resp2, &info)
	assert.Equal(t, "svcaccount1", info["accessKey"])
	assert.Equal(t, "on", info["status"])
}

// TestInfoServiceAccount_NotFound verifies 404 for unknown service accounts.
func TestInfoServiceAccount_NotFound(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/info-service-account?accessKey=ghost", nil)
	defer DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestInfoServiceAccount_MissingParam verifies 400 when accessKey param is absent.
func TestInfoServiceAccount_MissingParam(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/info-service-account", nil)
	defer DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestUpdateServiceAccount_Status verifies status update for a service account.
func TestUpdateServiceAccount_Status(t *testing.T) {
	ts := NewTestServer(t)

	// Create
	body := map[string]string{"accessKey": "svcaccount1", "secretKey": "svcpassword123"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPost, "/add-service-account", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Disable
	updateBody := map[string]string{"newStatus": "off"}
	resp2 := ts.EncryptedAdminRequest(t, http.MethodPost, "/update-service-account?accessKey=svcaccount1", updateBody)
	defer DrainAndClose(resp2)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	// Verify status changed
	resp3 := ts.AdminRequest(t, http.MethodGet, "/info-service-account?accessKey=svcaccount1", nil)
	defer DrainAndClose(resp3)
	var info map[string]interface{}
	DecodeJSON(t, resp3, &info)
	assert.Equal(t, "off", info["status"])
}

// TestServiceAccountCanAuthenticate verifies that a service account can sign requests
// that are accepted by the authentication layer. We test this via the admin API, which
// requires a valid signed request but does not enforce extra admin-only restrictions.
// A request signed with bad credentials returns 403 with "InvalidAccessKeyId".
// A request signed with valid SA credentials returns 200 (the admin API just needs auth).
func TestServiceAccountCanAuthenticate(t *testing.T) {
	ts := NewTestServer(t)

	// Create the service account
	body := map[string]string{"accessKey": "svcaccount1", "secretKey": "svcpassword123"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPost, "/add-service-account", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Build a TestServer view that uses the SA credentials for signing
	saTS := &TestServer{
		Server:    ts.Server,
		DataDir:   ts.DataDir,
		Port:      ts.Port,
		BaseURL:   ts.BaseURL,
		AdminURL:  ts.AdminURL,
		AccessKey: "svcaccount1",
		SecretKey: "svcpassword123",
	}

	// The admin list-groups endpoint requires a valid signed request.
	// With valid SA credentials the auth middleware accepts the request → 200.
	// With invalid credentials the middleware would return 403 with InvalidAccessKeyId.
	resp2 := saTS.AdminRequest(t, http.MethodGet, "/groups", nil)
	defer DrainAndClose(resp2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode, "SA should authenticate successfully against admin API")
}
