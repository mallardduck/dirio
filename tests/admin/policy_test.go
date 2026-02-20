package admin

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListPolicies_Empty verifies ListPolicies returns an empty map on a fresh server
func TestListPolicies_Empty(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/list-canned-policies", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var policies map[string]interface{}
	DecodeJSON(t, resp, &policies)
	assert.Empty(t, policies)
}

// TestCreatePolicy_Success creates a policy and verifies it appears in list
func TestCreatePolicy_Success(t *testing.T) {
	ts := NewTestServer(t)

	body := samplePolicyDocument("my-bucket")
	resp := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=my-policy", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.AdminRequest(t, http.MethodGet, "/list-canned-policies", nil)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var policies map[string]interface{}
	DecodeJSON(t, resp2, &policies)
	assert.Contains(t, policies, "my-policy")
}

// TestCreatePolicy_AlreadyExists returns 409 when creating a duplicate policy
func TestCreatePolicy_AlreadyExists(t *testing.T) {
	ts := NewTestServer(t)

	body := samplePolicyDocument("my-bucket")
	resp := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=dup-policy", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=dup-policy", body)
	DrainAndClose(resp2)
	assert.Equal(t, http.StatusConflict, resp2.StatusCode)
}

// TestCreatePolicy_MissingName returns 400 when the name query param is absent
func TestCreatePolicy_MissingName(t *testing.T) {
	ts := NewTestServer(t)

	body := samplePolicyDocument("my-bucket")
	resp := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy", body)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestCreatePolicy_InvalidDocument returns 400 for invalid policy JSON
func TestCreatePolicy_InvalidDocument(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=bad-policy", []byte(`{not valid json`))
	DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestGetPolicyInfo_Success creates a policy and reads it back
func TestGetPolicyInfo_Success(t *testing.T) {
	ts := NewTestServer(t)

	body := samplePolicyDocument("my-bucket")
	resp := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=info-policy", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.AdminRequest(t, http.MethodGet, "/info-canned-policy?name=info-policy", nil)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var policy map[string]interface{}
	DecodeJSON(t, resp2, &policy)
	assert.Equal(t, "info-policy", policy["name"])
}

// TestGetPolicyInfo_NotFound returns 404 for an unknown policy
func TestGetPolicyInfo_NotFound(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/info-canned-policy?name=no-such-policy", nil)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestDeletePolicy_Success creates and then deletes a policy
func TestDeletePolicy_Success(t *testing.T) {
	ts := NewTestServer(t)

	body := samplePolicyDocument("del-bucket")
	resp := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=del-policy", body)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2 := ts.AdminRequest(t, http.MethodPost, "/remove-canned-policy?name=del-policy", nil)
	DrainAndClose(resp2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	resp3 := ts.AdminRequest(t, http.MethodGet, "/info-canned-policy?name=del-policy", nil)
	DrainAndClose(resp3)
	assert.Equal(t, http.StatusNotFound, resp3.StatusCode)
}

// TestDeletePolicy_NotFound returns 404 when deleting a non-existent policy
func TestDeletePolicy_NotFound(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodPost, "/remove-canned-policy?name=ghost-policy", nil)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestSetPolicy_AttachToUser creates a user and policy, then attaches the policy using the legacy query-param format
func TestSetPolicy_AttachToUser(t *testing.T) {
	ts := NewTestServer(t)

	// Create user
	userBody := map[string]string{"secretKey": "alicesecretkey123", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=alice", userBody)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Create policy
	policyBody := samplePolicyDocument("alice-bucket")
	resp2 := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=alice-policy", policyBody)
	DrainAndClose(resp2)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	// Attach policy to user (legacy format: query params, no encrypted body)
	resp3 := ts.AdminRequest(t, http.MethodPost, "/set-policy?policyName=alice-policy&userOrGroup=alice&isGroup=false", nil)
	DrainAndClose(resp3)
	assert.Equal(t, http.StatusOK, resp3.StatusCode)

	// Confirm policy is listed in user info
	resp4 := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=alice", nil)
	require.Equal(t, http.StatusOK, resp4.StatusCode)

	var info map[string]interface{}
	DecodeJSON(t, resp4, &info)
	attachedPolicies, ok := info["attachedPolicies"].([]interface{})
	require.True(t, ok, "attachedPolicies should be an array")
	assert.Contains(t, attachedPolicies, "alice-policy")
}

// TestSetPolicy_PolicyNotFound returns 404 when attaching a non-existent policy
func TestSetPolicy_PolicyNotFound(t *testing.T) {
	ts := NewTestServer(t)

	// Create user
	userBody := map[string]string{"secretKey": "bobsecretkey1234", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=bob", userBody)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Attach non-existent policy
	resp2 := ts.AdminRequest(t, http.MethodPost, "/set-policy?policyName=no-such-policy&userOrGroup=bob&isGroup=false", nil)
	DrainAndClose(resp2)
	assert.Equal(t, http.StatusNotFound, resp2.StatusCode)
}

// TestSetPolicy_UserNotFound returns 404 when attaching to a non-existent user
func TestSetPolicy_UserNotFound(t *testing.T) {
	ts := NewTestServer(t)

	// Create policy
	policyBody := samplePolicyDocument("some-bucket")
	resp := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=some-policy", policyBody)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Attach to non-existent user
	resp2 := ts.AdminRequest(t, http.MethodPost, "/set-policy?policyName=some-policy&userOrGroup=nobody&isGroup=false", nil)
	DrainAndClose(resp2)
	assert.Equal(t, http.StatusNotFound, resp2.StatusCode)
}

// TestDetachPolicy_Success attaches then detaches a policy from a user
func TestDetachPolicy_Success(t *testing.T) {
	ts := NewTestServer(t)

	// Create user
	userBody := map[string]string{"secretKey": "carlasecretkey12", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=carla", userBody)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Create policy
	policyBody := samplePolicyDocument("carla-bucket")
	resp2 := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=carla-policy", policyBody)
	DrainAndClose(resp2)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	// Attach
	resp3 := ts.AdminRequest(t, http.MethodPost, "/set-policy?policyName=carla-policy&userOrGroup=carla&isGroup=false", nil)
	DrainAndClose(resp3)
	require.Equal(t, http.StatusOK, resp3.StatusCode)

	// Detach (legacy query param format)
	resp4 := ts.AdminRequest(t, http.MethodPost, "/idp/builtin/policy/detach?policyName=carla-policy&userOrGroup=carla&isGroup=false", nil)
	DrainAndClose(resp4)
	assert.Equal(t, http.StatusOK, resp4.StatusCode)

	// Confirm policy is no longer attached
	resp5 := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=carla", nil)
	require.Equal(t, http.StatusOK, resp5.StatusCode)

	var info map[string]interface{}
	DecodeJSON(t, resp5, &info)
	attachedPolicies, _ := info["attachedPolicies"].([]interface{})
	assert.NotContains(t, attachedPolicies, "carla-policy")
}

// TestPolicyEntities_Success creates a user and policy, attaches, then checks policy-entities
func TestPolicyEntities_Success(t *testing.T) {
	ts := NewTestServer(t)

	// Create user
	userBody := map[string]string{"secretKey": "deansecretkey123", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=dean", userBody)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Create policy
	policyBody := samplePolicyDocument("dean-bucket")
	resp2 := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=dean-policy", policyBody)
	DrainAndClose(resp2)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	// Attach
	resp3 := ts.AdminRequest(t, http.MethodPost, "/set-policy?policyName=dean-policy&userOrGroup=dean&isGroup=false", nil)
	DrainAndClose(resp3)
	require.Equal(t, http.StatusOK, resp3.StatusCode)

	// Check policy-entities
	resp4 := ts.AdminRequest(t, http.MethodGet, "/policy-entities?policy=dean-policy", nil)
	require.Equal(t, http.StatusOK, resp4.StatusCode)

	var entities map[string]interface{}
	DecodeJSON(t, resp4, &entities)
	userMappings, ok := entities["userMappings"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, userMappings, "dean")
}

// TestPolicyEntities_MissingName returns 400 when the policy query param is absent
func TestPolicyEntities_MissingName(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.AdminRequest(t, http.MethodGet, "/policy-entities", nil)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestSetPolicy_Idempotent verifies that attaching the same policy twice is idempotent
func TestSetPolicy_Idempotent(t *testing.T) {
	ts := NewTestServer(t)

	userBody := map[string]string{"secretKey": "ellensecretkey1", "status": "enabled"}
	resp := ts.EncryptedAdminRequest(t, http.MethodPut, "/add-user?accessKey=ellen", userBody)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	policyBody := samplePolicyDocument("ellen-bucket")
	resp2 := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=ellen-policy", policyBody)
	DrainAndClose(resp2)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	// Attach twice
	resp3 := ts.AdminRequest(t, http.MethodPost, "/set-policy?policyName=ellen-policy&userOrGroup=ellen&isGroup=false", nil)
	DrainAndClose(resp3)
	require.Equal(t, http.StatusOK, resp3.StatusCode)

	resp4 := ts.AdminRequest(t, http.MethodPost, "/set-policy?policyName=ellen-policy&userOrGroup=ellen&isGroup=false", nil)
	DrainAndClose(resp4)
	require.Equal(t, http.StatusOK, resp4.StatusCode)

	// Confirm policy appears exactly once
	resp5 := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=ellen", nil)
	require.Equal(t, http.StatusOK, resp5.StatusCode)

	var info map[string]interface{}
	DecodeJSON(t, resp5, &info)
	attachedPolicies, ok := info["attachedPolicies"].([]interface{})
	require.True(t, ok)

	count := 0
	for _, p := range attachedPolicies {
		if p == "ellen-policy" {
			count++
		}
	}
	assert.Equal(t, 1, count, "policy should appear exactly once after idempotent attach")
}
