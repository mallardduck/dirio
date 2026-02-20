package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListUsers_Empty verifies ListUsers returns an empty list on a fresh server
func TestListUsers_Empty(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/list-users", nil)
	defer DrainAndClose(resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var users []string
	DecodeJSON(t, resp, &users)
	assert.Empty(t, users)
}

// TestCreateUser_Success creates a user and verifies it exists
func TestCreateUser_Success(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{
		"secretKey": "alicesecretkey123",
		"status":    "enabled",
	}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=alice", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "add-user should succeed")

	// Verify alice now appears in list-users
	resp2 := ts.AdminRequest(t, http.MethodGet, "/list-users", nil)
	var users []string
	DecodeJSON(t, resp2, &users)
	assert.Contains(t, users, "alice")
}

// TestCreateUser_AlreadyExists returns 409 when creating a duplicate user
func TestCreateUser_AlreadyExists(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{"secretKey": "alicesecretkey123", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=alice", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Second create should return 409
	resp2 := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=alice", body)
	DrainAndClose(resp2)
	assert.Equal(t, http.StatusConflict, resp2.StatusCode)
}

// TestCreateUser_MissingAccessKey returns 400 when accessKey query param is absent
func TestCreateUser_MissingAccessKey(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{"secretKey": "somesecretkey123", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user", body)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestGetUserInfo_Success creates a user and reads back its info
func TestGetUserInfo_Success(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{"secretKey": "bobsecretkey1234", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=bob", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=bob", nil)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var info map[string]interface{}
	DecodeJSON(t, resp2, &info)
	assert.Equal(t, "bob", info["accessKey"])
}

// TestGetUserInfo_NotFound returns 404 for an unknown user
func TestGetUserInfo_NotFound(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=nobody", nil)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestGetUserInfo_MissingKey returns 400 when the accessKey param is absent
func TestGetUserInfo_MissingKey(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/user-info", nil)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestDeleteUser_Success creates and then deletes a user
func TestDeleteUser_Success(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{"secretKey": "charliesecret12", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=charlie", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.AdminRequest(t, http.MethodPost, "/remove-user?accessKey=charlie", nil)
	DrainAndClose(resp2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Confirm charlie is gone
	resp3 := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=charlie", nil)
	DrainAndClose(resp3)
	assert.Equal(t, http.StatusNotFound, resp3.StatusCode)
}

// TestDeleteUser_NotFound returns 404 when deleting a non-existent user
func TestDeleteUser_NotFound(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodPost, "/remove-user?accessKey=ghost", nil)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestSetUserStatus_Disable creates a user, disables it, and confirms the status change
func TestSetUserStatus_Disable(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{"secretKey": "davesecretkey123", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=dave", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.AdminRequest(t, http.MethodPost, "/set-user-status?accessKey=dave&status=disabled", nil)
	DrainAndClose(resp2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	resp3 := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=dave", nil)
	require.Equal(t, http.StatusOK, resp3.StatusCode)

	var info map[string]interface{}
	DecodeJSON(t, resp3, &info)
	assert.Equal(t, "off", info["status"])
}

// TestSetUserStatus_Enable creates a disabled user and enables it
func TestSetUserStatus_Enable(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{"secretKey": "evesecretkey1234", "status": "disabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=eve", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.AdminRequest(t, http.MethodPost, "/set-user-status?accessKey=eve&status=enabled", nil)
	DrainAndClose(resp2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	resp3 := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=eve", nil)
	require.Equal(t, http.StatusOK, resp3.StatusCode)

	var info map[string]interface{}
	DecodeJSON(t, resp3, &info)
	assert.Equal(t, "on", info["status"])
}

// TestSetUserStatus_InvalidStatus returns 400 for an unknown status value
func TestSetUserStatus_InvalidStatus(t *testing.T) {
	ts := NewTestServer(t)

	body := map[string]string{"secretKey": "franksecretkey12", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=frank", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.AdminRequest(t, http.MethodPost, "/set-user-status?accessKey=frank&status=suspended", nil)
	DrainAndClose(resp2)
	assert.Equal(t, http.StatusBadRequest, resp2.StatusCode)
}

// TestSetUserStatus_NotFound returns 404 for an unknown user
func TestSetUserStatus_NotFound(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodPost, "/set-user-status?accessKey=nobody&status=enabled", nil)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestListUsers_MultipleUsers creates several users and verifies all appear in the list
func TestListUsers_MultipleUsers(t *testing.T) {
	ts := NewTestServer(t)

	users := []string{"alice", "bob", "charlie"}
	for _, name := range users {
		body := map[string]string{"secretKey": name + "secretkey1", "status": "enabled"}
		resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey="+name, body)
		DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode, "creating user %s", name)
	}

	resp := ts.AdminRequest(t, http.MethodGet, "/list-users", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listed []string
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, json.Unmarshal(body, &listed))

	for _, name := range users {
		assert.Contains(t, listed, name)
	}
}
