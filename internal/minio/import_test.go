package minio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func formatJSONText() (serverUID uuid.UUID, config string) {
	testUid := uuid.New()
	return testUid, `{"version":"1","format":"fs","id":"` + testUid.String() + `","fs":{"version":"2"}}`
}

// TestImport_EmptyDirectory tests importing from an empty directory
func TestImport_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal format.json for validation
	minioSys := filepath.Join(tmpDir, ".minio.sys")
	require.NoError(t, os.MkdirAll(minioSys, 0o755))

	_, formatJSON := formatJSONText()
	require.NoError(t, os.WriteFile(filepath.Join(minioSys, "format.json"), []byte(formatJSON), 0o644))

	// Create billy filesystem scoped to .minio.sys directory
	minioFS := osfs.New(filepath.Join(tmpDir, ".minio.sys"))
	result, err := Import(minioFS)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Users)
	assert.Empty(t, result.Buckets)
	assert.Empty(t, result.Policies)
}

// TestImport_WithUsers tests importing MinIO users
func TestImport_WithUsers(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal format.json
	minioSys := filepath.Join(tmpDir, ".minio.sys")
	require.NoError(t, os.MkdirAll(minioSys, 0o755))
	_, formatJSON := formatJSONText()
	require.NoError(t, os.WriteFile(filepath.Join(minioSys, "format.json"), []byte(formatJSON), 0o644))

	// Create test user
	usersDir := filepath.Join(minioSys, "config", "iam", "users", "testuser")
	require.NoError(t, os.MkdirAll(usersDir, 0o755))

	identity := UserIdentity{
		Version: 1,
		Credentials: UserCredentials{
			AccessKey: "testuser",
			SecretKey: "testpass",
			Status:    "enabled",
		},
		UpdatedAt: time.Now(),
	}
	identityJSON, err := json.Marshal(identity)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(usersDir, "identity.json"), identityJSON, 0o644))

	// Create billy filesystem scoped to .minio.sys directory
	minioFS := osfs.New(filepath.Join(tmpDir, ".minio.sys"))
	result, err := Import(minioFS)
	require.NoError(t, err)
	assert.Len(t, result.Users, 1)
	assert.Contains(t, result.Users, "testuser")

	user := result.Users["testuser"]
	assert.Equal(t, "testuser", user.AccessKey)
	assert.Equal(t, "testpass", user.SecretKey)
	assert.Equal(t, "enabled", user.Status)
}

// TestImport_WithPolicies tests importing MinIO policies
func TestImport_WithPolicies(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal format.json
	minioSys := filepath.Join(tmpDir, ".minio.sys")
	require.NoError(t, os.MkdirAll(minioSys, 0o755))
	_, formatJSON := formatJSONText()
	require.NoError(t, os.WriteFile(filepath.Join(minioSys, "format.json"), []byte(formatJSON), 0o644))

	// Create test policy
	policiesDir := filepath.Join(minioSys, "config", "iam", "policies", "test-policy")
	require.NoError(t, os.MkdirAll(policiesDir, 0o755))

	policyDoc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Effect":   "Allow",
				"Action":   []string{"s3:GetObject"},
				"Resource": []string{"arn:aws:s3:::mybucket/*"},
			},
		},
	}

	policyFile := PolicyFile{
		Version:    1,
		Policy:     policyDoc,
		CreateDate: time.Now(),
		UpdateDate: time.Now(),
	}
	policyJSON, err := json.Marshal(policyFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(policiesDir, "policy.json"), policyJSON, 0o644))

	// Create billy filesystem scoped to .minio.sys directory
	minioFS := osfs.New(filepath.Join(tmpDir, ".minio.sys"))
	result, err := Import(minioFS)
	require.NoError(t, err)
	assert.Len(t, result.Policies, 1)
	assert.Contains(t, result.Policies, "test-policy")

	policy := result.Policies["test-policy"]
	assert.Equal(t, "test-policy", policy.Name)
	assert.NotEmpty(t, policy.PolicyJSON)
}

// TestImport_WithUserPolicyMappings tests attaching policies to users
func TestImport_WithUserPolicyMappings(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal format.json
	minioSys := filepath.Join(tmpDir, ".minio.sys")
	require.NoError(t, os.MkdirAll(minioSys, 0o755))
	_, formatJSON := formatJSONText()
	require.NoError(t, os.WriteFile(filepath.Join(minioSys, "format.json"), []byte(formatJSON), 0o644))

	// Create test user
	usersDir := filepath.Join(minioSys, "config", "iam", "users", "alice")
	require.NoError(t, os.MkdirAll(usersDir, 0o755))
	identity := UserIdentity{
		Version: 1,
		Credentials: UserCredentials{
			AccessKey: "alice",
			SecretKey: "alicepass",
			Status:    "enabled",
		},
		UpdatedAt: time.Now(),
	}
	identityJSON, err := json.Marshal(identity)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(usersDir, "identity.json"), identityJSON, 0o644))

	// Create policy mapping
	policydbDir := filepath.Join(minioSys, "config", "iam", "policydb", "users")
	require.NoError(t, os.MkdirAll(policydbDir, 0o755))
	mapping := UserPolicyMapping{
		Version: 1,
		Policy:  PolicyList{"readwrite"},
	}
	mappingJSON, err := json.Marshal(mapping)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(policydbDir, "alice.json"), mappingJSON, 0o644))

	// Create billy filesystem scoped to .minio.sys directory
	minioFS := osfs.New(filepath.Join(tmpDir, ".minio.sys"))
	result, err := Import(minioFS)
	require.NoError(t, err)
	assert.Len(t, result.Users, 1)

	user := result.Users["alice"]
	assert.Equal(t, []string{"readwrite"}, user.AttachedPolicy)
}

// TestImport_WithBucketsNoMetadata tests importing buckets without metadata
func TestImport_WithBucketsNoMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal format.json
	minioSys := filepath.Join(tmpDir, ".minio.sys")
	require.NoError(t, os.MkdirAll(minioSys, 0o755))
	_, formatJSON := formatJSONText()
	require.NoError(t, os.WriteFile(filepath.Join(minioSys, "format.json"), []byte(formatJSON), 0o644))

	// Create bucket directory without metadata
	bucketsDir := filepath.Join(minioSys, "buckets", "test-bucket")
	require.NoError(t, os.MkdirAll(bucketsDir, 0o755))

	// Create billy filesystem scoped to .minio.sys directory
	minioFS := osfs.New(filepath.Join(tmpDir, ".minio.sys"))
	result, err := Import(minioFS)
	require.NoError(t, err)
	assert.Len(t, result.Buckets, 1)
	assert.Contains(t, result.Buckets, "test-bucket")

	bucket := result.Buckets["test-bucket"]
	assert.Equal(t, "test-bucket", bucket.Name)
	// Note: BucketMetadata doesn't have Owner field - it's added during DirIO conversion
}

// TestImport_InvalidFormat tests that import fails with invalid format
func TestImport_InvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid format.json (erasure mode instead of fs)
	minioSys := filepath.Join(tmpDir, ".minio.sys")
	require.NoError(t, os.MkdirAll(minioSys, 0o755))
	formatJSON := `{"version":"1","format":"erasure","id":"test-erasure"}`
	require.NoError(t, os.WriteFile(filepath.Join(minioSys, "format.json"), []byte(formatJSON), 0o644))

	// Create billy filesystem scoped to .minio.sys directory
	minioFS := osfs.New(filepath.Join(tmpDir, ".minio.sys"))
	_, err := Import(minioFS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "format validation failed")
}

// TestImport_MissingFormatFile tests that import fails without format.json
func TestImport_MissingFormatFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create billy filesystem scoped to .minio.sys directory
	minioFS := osfs.New(filepath.Join(tmpDir, ".minio.sys"))
	_, err := Import(minioFS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "format validation failed")
}

// TestImport_CompleteSetup tests a complete MinIO import scenario
func TestImport_CompleteSetup(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup format.json
	minioSys := filepath.Join(tmpDir, ".minio.sys")
	require.NoError(t, os.MkdirAll(minioSys, 0o755))
	_, formatJSON := formatJSONText()
	require.NoError(t, os.WriteFile(filepath.Join(minioSys, "format.json"), []byte(formatJSON), 0o644))

	// Create two users
	for _, username := range []string{"alice", "bob"} {
		usersDir := filepath.Join(minioSys, "config", "iam", "users", username)
		require.NoError(t, os.MkdirAll(usersDir, 0o755))
		identity := UserIdentity{
			Version: 1,
			Credentials: UserCredentials{
				AccessKey: username,
				SecretKey: username + "pass",
				Status:    "enabled",
			},
			UpdatedAt: time.Now(),
		}
		identityJSON, err := json.Marshal(identity)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(usersDir, "identity.json"), identityJSON, 0o644))
	}

	// Create two policies
	for _, policyName := range []string{"alpha-rw", "beta-rw"} {
		policiesDir := filepath.Join(minioSys, "config", "iam", "policies", policyName)
		require.NoError(t, os.MkdirAll(policiesDir, 0o755))
		policyDoc := map[string]any{
			"Version": "2012-10-17",
			"Statement": []map[string]any{
				{
					"Effect":   "Allow",
					"Action":   []string{"s3:*"},
					"Resource": []string{"arn:aws:s3:::" + policyName[:5] + "/*"},
				},
			},
		}
		policyFile := PolicyFile{
			Version:    1,
			Policy:     policyDoc,
			CreateDate: time.Now(),
			UpdateDate: time.Now(),
		}
		policyJSON, err := json.Marshal(policyFile)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(policiesDir, "policy.json"), policyJSON, 0o644))
	}

	// Create user-policy mappings
	policydbDir := filepath.Join(minioSys, "config", "iam", "policydb", "users")
	require.NoError(t, os.MkdirAll(policydbDir, 0o755))
	mappings := map[string]PolicyList{"alice": {"alpha-rw"}, "bob": {"beta-rw"}}
	for user, policy := range mappings {
		mapping := UserPolicyMapping{Version: 1, Policy: policy}
		mappingJSON, err := json.Marshal(mapping)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(policydbDir, user+".json"), mappingJSON, 0o644))
	}

	// Create two buckets
	for _, bucketName := range []string{"alpha", "beta"} {
		bucketsDir := filepath.Join(minioSys, "buckets", bucketName)
		require.NoError(t, os.MkdirAll(bucketsDir, 0o755))
	}

	// Run import
	// Create billy filesystem scoped to .minio.sys directory
	minioFS := osfs.New(filepath.Join(tmpDir, ".minio.sys"))
	result, err := Import(minioFS)
	require.NoError(t, err)

	// Verify users
	assert.Len(t, result.Users, 2)
	assert.Contains(t, result.Users, "alice")
	assert.Contains(t, result.Users, "bob")
	assert.Equal(t, []string{"alpha-rw"}, result.Users["alice"].AttachedPolicy)
	assert.Equal(t, []string{"beta-rw"}, result.Users["bob"].AttachedPolicy)

	// Verify policies
	assert.Len(t, result.Policies, 2)
	assert.Contains(t, result.Policies, "alpha-rw")
	assert.Contains(t, result.Policies, "beta-rw")

	// Verify buckets
	assert.Len(t, result.Buckets, 2)
	assert.Contains(t, result.Buckets, "alpha")
	assert.Contains(t, result.Buckets, "beta")
}
