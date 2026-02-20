package admin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	miniopkg "github.com/mallardduck/dirio/internal/minio"
)

// buildMinIOSys creates a .minio.sys directory structure within dataDir and returns the path.
// It writes the minimal format.json required by the MinIO importer.
func buildMinIOSys(t *testing.T, dataDir string) string {
	t.Helper()

	minioSys := filepath.Join(dataDir, ".minio.sys")
	require.NoError(t, os.MkdirAll(minioSys, 0o755))

	formatJSON := `{"version":"1","format":"fs","id":"test-import-fs","fs":{"version":"2"}}`
	require.NoError(t, os.WriteFile(filepath.Join(minioSys, "format.json"), []byte(formatJSON), 0o644))

	return minioSys
}

// writeMinIOUser creates a MinIO identity.json for a single user
func writeMinIOUser(t *testing.T, minioSys, accessKey, secretKey, status string) {
	t.Helper()

	usersDir := filepath.Join(minioSys, "config", "iam", "users", accessKey)
	require.NoError(t, os.MkdirAll(usersDir, 0o755))

	identity := miniopkg.UserIdentity{
		Version: 1,
		Credentials: miniopkg.UserCredentials{
			AccessKey: accessKey,
			SecretKey: secretKey,
			Status:    status,
		},
		UpdatedAt: time.Now(),
	}
	data, err := json.Marshal(identity)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(usersDir, "identity.json"), data, 0o644))
}

// writeMinIOPolicy creates a MinIO policy.json for a single policy
func writeMinIOPolicy(t *testing.T, minioSys, policyName, bucket string) {
	t.Helper()

	policiesDir := filepath.Join(minioSys, "config", "iam", "policies", policyName)
	require.NoError(t, os.MkdirAll(policiesDir, 0o755))

	policyDoc := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect":   "Allow",
				"Action":   []string{"s3:GetObject", "s3:PutObject"},
				"Resource": []string{"arn:aws:s3:::" + bucket + "/*"},
			},
		},
	}
	policyFile := miniopkg.PolicyFile{
		Version:    1,
		Policy:     policyDoc,
		CreateDate: time.Now(),
		UpdateDate: time.Now(),
	}
	data, err := json.Marshal(policyFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(policiesDir, "policy.json"), data, 0o644))
}

// writeMinIOUserPolicyMapping writes a policydb mapping file that attaches policies to a user
func writeMinIOUserPolicyMapping(t *testing.T, minioSys, accessKey string, policies []string) {
	t.Helper()

	policydbDir := filepath.Join(minioSys, "config", "iam", "policydb", "users")
	require.NoError(t, os.MkdirAll(policydbDir, 0o755))

	policyList := miniopkg.PolicyList(policies)
	mapping := miniopkg.UserPolicyMapping{Version: 1, Policy: policyList}
	data, err := json.Marshal(mapping)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(policydbDir, accessKey+".json"), data, 0o644))
}

// TestMinIOImport_Users verifies that MinIO IAM users are imported on server startup
// and accessible via the admin API
func TestMinIOImport_Users(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "dirio-import-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	minioSys := buildMinIOSys(t, dataDir)
	writeMinIOUser(t, minioSys, "alice", "alicesecretkey123", "enabled")
	writeMinIOUser(t, minioSys, "bob", "bobsecretkey12345", "enabled")

	ts := NewTestServerWithDataDir(t, dataDir)

	resp := ts.AdminRequest(t, http.MethodGet, "/list-users", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var users []string
	DecodeJSON(t, resp, &users)
	assert.Contains(t, users, "alice")
	assert.Contains(t, users, "bob")
}

// TestMinIOImport_Policies verifies that MinIO IAM policies are imported on server startup
func TestMinIOImport_Policies(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "dirio-import-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	minioSys := buildMinIOSys(t, dataDir)
	writeMinIOPolicy(t, minioSys, "readonly", "shared-bucket")
	writeMinIOPolicy(t, minioSys, "readwrite", "shared-bucket")

	ts := NewTestServerWithDataDir(t, dataDir)

	resp := ts.AdminRequest(t, http.MethodGet, "/list-canned-policies", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var policies map[string]interface{}
	DecodeJSON(t, resp, &policies)
	assert.Contains(t, policies, "readonly")
	assert.Contains(t, policies, "readwrite")
}

// TestMinIOImport_UserPolicyMappings verifies that policy attachments are preserved after import
func TestMinIOImport_UserPolicyMappings(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "dirio-import-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	minioSys := buildMinIOSys(t, dataDir)
	writeMinIOUser(t, minioSys, "alice", "alicesecretkey123", "enabled")
	writeMinIOPolicy(t, minioSys, "alice-rw", "alice-bucket")
	writeMinIOUserPolicyMapping(t, minioSys, "alice", []string{"alice-rw"})

	ts := NewTestServerWithDataDir(t, dataDir)

	resp := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=alice", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var info map[string]interface{}
	DecodeJSON(t, resp, &info)

	attachedPolicies, ok := info["attachedPolicies"].([]interface{})
	require.True(t, ok, "attachedPolicies should be an array")
	assert.Contains(t, attachedPolicies, "alice-rw")
}

// TestMinIOImport_DisabledUser verifies that a disabled MinIO user is imported with disabled status
func TestMinIOImport_DisabledUser(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "dirio-import-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	minioSys := buildMinIOSys(t, dataDir)
	writeMinIOUser(t, minioSys, "disableduser", "disabledsecret12", "disabled")

	ts := NewTestServerWithDataDir(t, dataDir)

	resp := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=disableduser", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var info map[string]interface{}
	DecodeJSON(t, resp, &info)
	// MinIO "disabled" maps to DirIO "off"
	assert.Equal(t, "off", info["status"])
}

// TestMinIOImport_IdempotentRestart verifies that re-starting the server with already-imported
// MinIO data does not duplicate users or fail
func TestMinIOImport_IdempotentRestart(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "dirio-import-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	minioSys := buildMinIOSys(t, dataDir)
	writeMinIOUser(t, minioSys, "alice", "alicesecretkey123", "enabled")
	writeMinIOPolicy(t, minioSys, "alice-rw", "alice-bucket")
	writeMinIOUserPolicyMapping(t, minioSys, "alice", []string{"alice-rw"})

	// First server start: import happens
	ts1 := NewTestServerWithDataDir(t, dataDir)
	resp := ts1.AdminRequest(t, http.MethodGet, "/list-users", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var users []string
	DecodeJSON(t, resp, &users)
	require.Contains(t, users, "alice")

	// Second server start on the same dataDir: import should be skipped (state file exists)
	ts2 := NewTestServerWithDataDir(t, dataDir)

	resp2 := ts2.AdminRequest(t, http.MethodGet, "/list-users", nil)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	var users2 []string
	DecodeJSON(t, resp2, &users2)

	// Should still have exactly one alice, not duplicated
	count := 0
	for _, u := range users2 {
		if u == "alice" {
			count++
		}
	}
	assert.Equal(t, 1, count, "alice should appear exactly once after idempotent re-import")
}

// TestMinIOImport_Complete tests a full scenario: users, policies, and mappings
func TestMinIOImport_Complete(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "dirio-import-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	minioSys := buildMinIOSys(t, dataDir)

	writeMinIOUser(t, minioSys, "alice", "alicesecretkey123", "enabled")
	writeMinIOUser(t, minioSys, "bob", "bobsecretkey12345", "enabled")

	writeMinIOPolicy(t, minioSys, "alpha-rw", "alpha-bucket")
	writeMinIOPolicy(t, minioSys, "beta-rw", "beta-bucket")

	writeMinIOUserPolicyMapping(t, minioSys, "alice", []string{"alpha-rw"})
	writeMinIOUserPolicyMapping(t, minioSys, "bob", []string{"beta-rw"})

	ts := NewTestServerWithDataDir(t, dataDir)

	// Verify users
	resp := ts.AdminRequest(t, http.MethodGet, "/list-users", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var users []string
	DecodeJSON(t, resp, &users)
	assert.Contains(t, users, "alice")
	assert.Contains(t, users, "bob")

	// Verify policies
	resp2 := ts.AdminRequest(t, http.MethodGet, "/list-canned-policies", nil)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	var policies map[string]interface{}
	DecodeJSON(t, resp2, &policies)
	assert.Contains(t, policies, "alpha-rw")
	assert.Contains(t, policies, "beta-rw")

	// Verify alice's policy attachment
	resp3 := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=alice", nil)
	require.Equal(t, http.StatusOK, resp3.StatusCode)
	var aliceInfo map[string]interface{}
	DecodeJSON(t, resp3, &aliceInfo)
	alicePolicies, _ := aliceInfo["attachedPolicies"].([]interface{})
	assert.Contains(t, alicePolicies, "alpha-rw")

	// Verify bob's policy attachment
	resp4 := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=bob", nil)
	require.Equal(t, http.StatusOK, resp4.StatusCode)
	var bobInfo map[string]interface{}
	DecodeJSON(t, resp4, &bobInfo)
	bobPolicies, _ := bobInfo["attachedPolicies"].([]interface{})
	assert.Contains(t, bobPolicies, "beta-rw")
}

// TestMinIOImport_PostImportUserManagement verifies that users imported from MinIO
// can be further managed (disabled, have policies attached) via the admin API
func TestMinIOImport_PostImportUserManagement(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "dirio-import-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	minioSys := buildMinIOSys(t, dataDir)
	writeMinIOUser(t, minioSys, "importeduser", "importedsecret12", "enabled")

	ts := NewTestServerWithDataDir(t, dataDir)

	// Confirm user was imported
	resp := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=importeduser", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	DrainAndClose(resp)

	// Disable the imported user
	resp2 := ts.AdminRequest(t, http.MethodPost, "/set-user-status?accessKey=importeduser&status=disabled", nil)
	DrainAndClose(resp2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Create and attach a new policy
	policyBody := samplePolicyDocument("imported-bucket")
	resp3 := ts.AdminRequest(t, http.MethodPut, "/add-canned-policy?name=imported-policy", policyBody)
	DrainAndClose(resp3)
	require.Equal(t, http.StatusOK, resp3.StatusCode)

	resp4 := ts.AdminRequest(t, http.MethodPost, "/set-policy?policyName=imported-policy&userOrGroup=importeduser&isGroup=false", nil)
	DrainAndClose(resp4)
	assert.Equal(t, http.StatusOK, resp4.StatusCode)

	// Verify final state
	resp5 := ts.AdminRequest(t, http.MethodGet, "/user-info?accessKey=importeduser", nil)
	require.Equal(t, http.StatusOK, resp5.StatusCode)
	var info map[string]interface{}
	DecodeJSON(t, resp5, &info)
	assert.Equal(t, "off", info["status"])
	attachedPolicies, _ := info["attachedPolicies"].([]interface{})
	assert.Contains(t, attachedPolicies, "imported-policy")
}
