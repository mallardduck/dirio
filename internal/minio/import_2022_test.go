package minio

import (
	"testing"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImport_MinIO2022_RealData(t *testing.T) {
	// This test requires the minio-data-2022-import directory to exist
	// Skip if not present
	dataRoot := "../../minio-data-2022-import"
	fs := osfs.New(dataRoot)

	// Check if .minio.sys exists
	if _, err := fs.Stat(".minio.sys"); err != nil {
		t.Skip("Skipping: minio-data-2022-import not found")
		return
	}

	// Create MinIO filesystem
	minioFS, err := fs.Chroot(".minio.sys")
	require.NoError(t, err)

	// Run import
	result, err := Import(minioFS)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify buckets
	t.Run("Buckets", func(t *testing.T) {
		assert.Len(t, result.Buckets, 4, "Should have 4 buckets (alpha, beta, gamma, delta)")

		// Verify expected buckets exist
		alpha, ok := result.Buckets["alpha"]
		require.True(t, ok, "Should have alpha bucket")
		assert.Equal(t, "alpha", alpha.Name)

		beta, ok := result.Buckets["beta"]
		require.True(t, ok, "Should have beta bucket")
		assert.Equal(t, "beta", beta.Name)

		gamma, ok := result.Buckets["gamma"]
		require.True(t, ok, "Should have gamma bucket")
		assert.Equal(t, "gamma", gamma.Name)

		delta, ok := result.Buckets["delta"]
		require.True(t, ok, "Should have delta bucket")
		assert.Equal(t, "delta", delta.Name)

		// Verify 'replication' is NOT imported as a bucket
		_, hasReplication := result.Buckets["replication"]
		assert.False(t, hasReplication, "Should NOT import 'replication' directory as a bucket - it's a MinIO system directory")
	})

	// Verify object metadata
	t.Run("ObjectMetadata", func(t *testing.T) {
		assert.Contains(t, result.ObjectMetadata, "alpha", "Should have alpha bucket objects")
		assert.Contains(t, result.ObjectMetadata, "beta", "Should have beta bucket objects")
		assert.Contains(t, result.ObjectMetadata, "gamma", "Should have gamma bucket objects")

		// Verify 'replication' is NOT in object metadata
		assert.NotContains(t, result.ObjectMetadata, "replication", "Should NOT import object metadata for 'replication' directory")

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
		// MinIO 2022 test data has empty region and compression disabled
		assert.Equal(t, "", result.DataConfig.Region)
		assert.False(t, result.DataConfig.Compression.Enabled)
	})

	// Verify users and multi-policy support
	t.Run("Users", func(t *testing.T) {
		assert.Contains(t, result.Users, "alice", "Should have alice user")
		assert.Contains(t, result.Users, "bob", "Should have bob user")

		alice := result.Users["alice"]
		assert.Equal(t, "alice", alice.AccessKey)
		assert.Equal(t, []string{"alpha-rw"}, alice.AttachedPolicy, "Alice should have alpha-rw policy")

		bob := result.Users["bob"]
		assert.Equal(t, "bob", bob.AccessKey)
		assert.Equal(t, []string{"beta-rw"}, bob.AttachedPolicy, "Bob should have beta-rw policy")

		// charlie: delta-rw direct + alpha-rw via alpha-users group membership
		charlie, ok := result.Users["charlie"]
		require.True(t, ok, "Should have charlie user")
		assert.Equal(t, "charlie", charlie.AccessKey)
		assert.Len(t, charlie.AttachedPolicy, 2, "Charlie should have 2 policies (delta-rw direct + alpha-rw via group)")
		assert.Contains(t, charlie.AttachedPolicy, "alpha-rw", "Charlie should have alpha-rw (via alpha-users group)")
		assert.Contains(t, charlie.AttachedPolicy, "delta-rw", "Charlie should have delta-rw (direct)")
	})
}

func TestIsSpecialMinIODirectory(t *testing.T) {
	tests := []struct {
		name     string
		dirName  string
		expected bool
	}{
		{
			name:     "replication is special",
			dirName:  "replication",
			expected: true,
		},
		{
			name:     "alpha is not special",
			dirName:  "alpha",
			expected: false,
		},
		{
			name:     "beta is not special",
			dirName:  "beta",
			expected: false,
		},
		{
			name:     "random-bucket is not special",
			dirName:  "random-bucket",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSpecialMinIODirectory(tt.dirName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
