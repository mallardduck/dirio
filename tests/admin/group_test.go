package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Helper ----------------------------------------------------------------

// createUser creates a test user with a fixed secret key, failing the test on error.
func createUser(t *testing.T, ts *TestServer, accessKey string) {
	t.Helper()
	body := map[string]string{"secretKey": "testpassword123", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey="+accessKey, body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "pre-condition: create user %q", accessKey)
}

// updateGroupMembers calls POST /update-group-members with the given body.
func updateGroupMembers(t *testing.T, ts *TestServer, body map[string]interface{}) *http.Response {
	t.Helper()
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)
	return ts.AdminRequest(t, http.MethodPost, "/update-group-members", bodyBytes)
}

// ---- Tests -----------------------------------------------------------------

// TestListGroups_Empty verifies that listing groups on a fresh server returns an empty list.
func TestListGroups_Empty(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/groups", nil)
	defer DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var groups []string
	DecodeJSON(t, resp, &groups)
	assert.Empty(t, groups)
}

// TestCreateGroup_ViaUpdateMembers verifies that sending an add-members request
// to a non-existent group auto-creates it.
func TestCreateGroup_ViaUpdateMembers(t *testing.T) {
	ts := NewTestServer(t)
	createUser(t, ts, "alice")

	resp := updateGroupMembers(t, ts, map[string]interface{}{
		"group":    "devs",
		"members":  []string{"alice"},
		"isRemove": false,
	})
	defer DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify the group appears in the list
	resp2 := ts.AdminRequest(t, http.MethodGet, "/groups", nil)
	defer DrainAndClose(resp2)
	var groups []string
	DecodeJSON(t, resp2, &groups)
	assert.Contains(t, groups, "devs")
}

// TestGetGroupInfo_Success verifies that group info can be retrieved after creation.
func TestGetGroupInfo_Success(t *testing.T) {
	ts := NewTestServer(t)
	createUser(t, ts, "alice")

	// Create group with a member
	resp := updateGroupMembers(t, ts, map[string]interface{}{
		"group":    "devs",
		"members":  []string{"alice"},
		"isRemove": false,
	})
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Get group info
	resp2 := ts.AdminRequest(t, http.MethodGet, "/group?group=devs", nil)
	defer DrainAndClose(resp2)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var info map[string]interface{}
	DecodeJSON(t, resp2, &info)
	assert.Equal(t, "devs", info["name"])
	members, ok := info["members"].([]interface{})
	require.True(t, ok, "members field should be a list")
	assert.Len(t, members, 1)
	assert.Equal(t, "alice", members[0])
}

// TestGetGroupInfo_NotFound verifies that a 404 is returned for unknown groups.
func TestGetGroupInfo_NotFound(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/group?group=nobody", nil)
	defer DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestGetGroupInfo_MissingParam verifies that a 400 is returned when the group param is missing.
func TestGetGroupInfo_MissingParam(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/group", nil)
	defer DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestAddMembersToGroup_Success verifies adding multiple members.
func TestAddMembersToGroup_Success(t *testing.T) {
	ts := NewTestServer(t)
	createUser(t, ts, "alice")
	createUser(t, ts, "bob")

	// Create group and add both users
	resp := updateGroupMembers(t, ts, map[string]interface{}{
		"group":    "engineers",
		"members":  []string{"alice", "bob"},
		"isRemove": false,
	})
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify group info shows both members
	resp2 := ts.AdminRequest(t, http.MethodGet, "/group?group=engineers", nil)
	defer DrainAndClose(resp2)
	var info map[string]interface{}
	DecodeJSON(t, resp2, &info)
	members := info["members"].([]interface{})
	assert.Len(t, members, 2)
}

// TestAddMembersToGroup_UserNotFound verifies that a 404 is returned when
// adding a non-existent user to a group.
func TestAddMembersToGroup_UserNotFound(t *testing.T) {
	ts := NewTestServer(t)

	resp := updateGroupMembers(t, ts, map[string]interface{}{
		"group":    "devs",
		"members":  []string{"ghost"},
		"isRemove": false,
	})
	defer DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestRemoveMembersFromGroup_Success verifies removing a member.
func TestRemoveMembersFromGroup_Success(t *testing.T) {
	ts := NewTestServer(t)
	createUser(t, ts, "alice")
	createUser(t, ts, "bob")

	// Add both users
	resp := updateGroupMembers(t, ts, map[string]interface{}{
		"group":    "devs",
		"members":  []string{"alice", "bob"},
		"isRemove": false,
	})
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Remove alice
	resp2 := updateGroupMembers(t, ts, map[string]interface{}{
		"group":    "devs",
		"members":  []string{"alice"},
		"isRemove": true,
	})
	DrainAndClose(resp2)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	// Verify only bob remains
	resp3 := ts.AdminRequest(t, http.MethodGet, "/group?group=devs", nil)
	defer DrainAndClose(resp3)
	var info map[string]interface{}
	DecodeJSON(t, resp3, &info)
	members := info["members"].([]interface{})
	assert.Len(t, members, 1)
	assert.Equal(t, "bob", members[0])
}

// TestSetGroupStatus_Disable verifies disabling a group.
func TestSetGroupStatus_Disable(t *testing.T) {
	ts := NewTestServer(t)

	// Create the group first
	resp := updateGroupMembers(t, ts, map[string]interface{}{
		"group":    "ops",
		"members":  []string{},
		"isRemove": false,
	})
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Disable it
	resp2 := ts.AdminRequest(t, http.MethodPost, "/set-group-status?group=ops&status=disabled", nil)
	defer DrainAndClose(resp2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Verify status changed
	resp3 := ts.AdminRequest(t, http.MethodGet, "/group?group=ops", nil)
	defer DrainAndClose(resp3)
	var info map[string]interface{}
	DecodeJSON(t, resp3, &info)
	assert.Equal(t, "off", info["status"])
}

// TestSetGroupStatus_Enable verifies enabling a group.
func TestSetGroupStatus_Enable(t *testing.T) {
	ts := NewTestServer(t)

	// Create the group
	resp := updateGroupMembers(t, ts, map[string]interface{}{
		"group":    "ops",
		"members":  []string{},
		"isRemove": false,
	})
	DrainAndClose(resp)

	// Disable then re-enable
	ts.AdminRequest(t, http.MethodPost, "/set-group-status?group=ops&status=disabled", nil)

	resp2 := ts.AdminRequest(t, http.MethodPost, "/set-group-status?group=ops&status=enabled", nil)
	defer DrainAndClose(resp2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	resp3 := ts.AdminRequest(t, http.MethodGet, "/group?group=ops", nil)
	defer DrainAndClose(resp3)
	var info map[string]interface{}
	DecodeJSON(t, resp3, &info)
	assert.Equal(t, "on", info["status"])
}

// TestSetGroupStatus_NotFound verifies 404 for an unknown group.
func TestSetGroupStatus_NotFound(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodPost, "/set-group-status?group=nobody&status=disabled", nil)
	defer DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestSetGroupStatus_InvalidStatus verifies 400 for an invalid status value.
func TestSetGroupStatus_InvalidStatus(t *testing.T) {
	ts := NewTestServer(t)

	// Create a group first
	resp := updateGroupMembers(t, ts, map[string]interface{}{
		"group":    "ops",
		"members":  []string{},
		"isRemove": false,
	})
	DrainAndClose(resp)

	resp2 := ts.AdminRequest(t, http.MethodPost, "/set-group-status?group=ops&status=invalid", nil)
	defer DrainAndClose(resp2)
	assert.Equal(t, http.StatusBadRequest, resp2.StatusCode)
}

// TestAddMembersToGroup_Idempotent verifies that adding the same member twice is idempotent.
func TestAddMembersToGroup_Idempotent(t *testing.T) {
	ts := NewTestServer(t)
	createUser(t, ts, "alice")

	for range 2 {
		resp := updateGroupMembers(t, ts, map[string]interface{}{
			"group":    "devs",
			"members":  []string{"alice"},
			"isRemove": false,
		})
		DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// Alice should appear only once
	resp := ts.AdminRequest(t, http.MethodGet, "/group?group=devs", nil)
	defer DrainAndClose(resp)
	var info map[string]interface{}
	DecodeJSON(t, resp, &info)
	members := info["members"].([]interface{})
	assert.Len(t, members, 1)
}
