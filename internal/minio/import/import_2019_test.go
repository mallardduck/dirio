package minioimport

import (
	"testing"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/internal/consts"
)

func TestImport_MinIO2019_RealData(t *testing.T) {
	// This test requires the minio-data-2019 directory to exist
	// Skip if not present
	dataRoot := "../../minio-data-2019"
	fs := osfs.New(dataRoot)

	// Check if .minio.sys exists
	if _, err := fs.Stat(consts.MinioMetadataDir); err != nil {
		t.Skip("Skipping: minio-data-2019 not found")
		return
	}

	// Create MinIO filesystem
	minioFS, err := fs.Chroot(consts.MinioMetadataDir)
	require.NoError(t, err)

	// Run import
	result, err := Import(minioFS)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify users
	t.Run("Users", func(t *testing.T) {
		assert.Len(t, result.Users, 3, "Should have 3 users")

		alice, ok := result.Users["alice"]
		require.True(t, ok, "Should have alice user")
		assert.Equal(t, "alice", alice.AccessKey, "Alice accessKey should be 'alice'")
		assert.Equal(t, "alicepass1234", alice.SecretKey, "Alice should have correct password")
		assert.Equal(t, "enabled", alice.Status, "Alice should be enabled")
		assert.Equal(t, []string{"alpha-rw"}, alice.AttachedPolicy, "Alice should have alpha-rw policy")

		bob, ok := result.Users["bob"]
		require.True(t, ok, "Should have bob user")
		assert.Equal(t, "bob", bob.AccessKey, "Bob accessKey should be 'bob'")
		assert.Equal(t, "bobpass1234", bob.SecretKey, "Bob should have correct password")
		assert.Equal(t, "enabled", bob.Status, "Bob should be enabled")
		assert.Equal(t, []string{"beta-rw"}, bob.AttachedPolicy, "Bob should have beta-rw policy")

		charlie, ok := result.Users["charlie"]
		require.True(t, ok, "Should have charlie user")
		assert.Equal(t, "charlie", charlie.AccessKey, "Charlie accessKey should be 'charlie'")
		assert.Equal(t, "charliepass1234", charlie.SecretKey, "Charlie should have correct password")
		assert.Equal(t, "enabled", charlie.Status, "Charlie should be enabled")
		// MinIO 2019 only supports one direct policy per user (no group support in the 2019 mc).
		// charlie gets delta-rw as his single direct policy; multi-policy via groups
		// is exercised in the 2022 import test where the mc supports group management.
		assert.Equal(t, []string{"delta-rw"}, charlie.AttachedPolicy, "Charlie should have delta-rw as his direct 2019 policy")

		t.Logf("Users imported: %+v", result.Users)
	})

	// Verify IAM policies
	t.Run("Policies", func(t *testing.T) {
		assert.Len(t, result.Policies, 3, "Should have 3 IAM policies")

		alphaRW, ok := result.Policies["alpha-rw"]
		require.True(t, ok, "Should have alpha-rw policy")
		assert.Contains(t, alphaRW.PolicyJSON, "alpha", "Policy should mention alpha bucket")

		betaRW, ok := result.Policies["beta-rw"]
		require.True(t, ok, "Should have beta-rw policy")
		assert.Contains(t, betaRW.PolicyJSON, "beta", "Policy should mention beta bucket")

		deltaRW, ok := result.Policies["delta-rw"]
		require.True(t, ok, "Should have delta-rw policy")
		assert.Contains(t, deltaRW.PolicyJSON, "delta", "Policy should mention delta bucket")

		t.Logf("Policies imported: %+v", result.Policies)
	})

	// Verify buckets
	t.Run("Buckets", func(t *testing.T) {
		// At minimum the 4 core buckets must be present.
		// SETUP_POLICY_TESTS=true creates additional policy-test buckets, so we
		// don't assert an exact count here.
		assert.GreaterOrEqual(t, len(result.Buckets), 4, "Should have at least 4 core buckets")

		alpha, ok := result.Buckets["alpha"]
		require.True(t, ok, "Should have alpha bucket")
		assert.Equal(t, "alpha", alpha.Name)

		beta, ok := result.Buckets["beta"]
		require.True(t, ok, "Should have beta bucket")
		assert.Equal(t, "beta", beta.Name)
		// Beta should have a bucket policy (public-read)
		assert.NotEmpty(t, beta.PolicyConfigJSON, "Beta should have bucket policy")
		t.Logf("Beta bucket policy: %s", string(beta.PolicyConfigJSON))

		gamma, ok := result.Buckets["gamma"]
		require.True(t, ok, "Should have gamma bucket")
		assert.Equal(t, "gamma", gamma.Name)
		// Gamma should also have a bucket policy (public-read)
		assert.NotEmpty(t, gamma.PolicyConfigJSON, "Gamma should have bucket policy")
		t.Logf("Gamma bucket policy: %s", string(gamma.PolicyConfigJSON))

		delta, ok := result.Buckets["delta"]
		require.True(t, ok, "Should have delta bucket")
		assert.Equal(t, "delta", delta.Name)
	})

	// Verify object metadata
	t.Run("ObjectMetadata", func(t *testing.T) {
		assert.Contains(t, result.ObjectMetadata, "alpha", "Should have alpha bucket objects")
		assert.Contains(t, result.ObjectMetadata, "beta", "Should have beta bucket objects")
		assert.Contains(t, result.ObjectMetadata, "gamma", "Should have gamma bucket objects")

		// Count total objects
		totalObjects := 0
		for bucket, objects := range result.ObjectMetadata {
			t.Logf("Bucket %s has %d objects", bucket, len(objects))
			totalObjects += len(objects)
		}
		assert.Greater(t, totalObjects, 10, "Should have imported multiple objects")
	})

	// Verify data config
	t.Run("DataConfig", func(t *testing.T) {
		require.NotNil(t, result.DataConfig, "Should have data config")
		assert.Equal(t, "minioadmin", result.DataConfig.Credentials.AccessKey)
		assert.Equal(t, "minioadmin", result.DataConfig.Credentials.SecretKey)
		// MinIO 2019 test data has empty region and compression disabled
		assert.Empty(t, result.DataConfig.Region)
		assert.False(t, result.DataConfig.Compression.Enabled)
	})
}
