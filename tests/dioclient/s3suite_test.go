package dioclient_test

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/sdk/dioclient"
)

var bucketSeq atomic.Int64

// runListBuckets creates 3 uniquely-named buckets via mc then asserts that
// client.ListBuckets returns all 3 among its results.
func runListBuckets(t *testing.T, client *dioclient.Client, mc *minio.Client) {
	t.Helper()
	ctx := context.Background()

	// Use a test-run prefix so parallel tests don't collide.
	prefix := fmt.Sprintf("test-lb-%s-", uniqueSuffix())
	names := []string{prefix + "alpha", prefix + "beta", prefix + "gamma"}
	seedBuckets(t, mc, names...)

	buckets, err := client.ListBuckets(ctx)
	require.NoError(t, err)

	got := make(map[string]bool, len(buckets))
	for _, b := range buckets {
		got[b.Name] = true
	}
	for _, name := range names {
		assert.True(t, got[name], "expected bucket %q in ListBuckets result", name)
	}
}

// runListObjectsFlat seeds 3 objects in a flat layout and asserts all appear in ListObjects.
func runListObjectsFlat(t *testing.T, client *dioclient.Client, mc *minio.Client) {
	t.Helper()
	ctx := context.Background()

	bucket := "test-lo-flat-" + uniqueSuffix()
	seedBuckets(t, mc, bucket)
	seedObjects(t, mc, bucket, map[string]string{
		"file-a.txt": "aaa",
		"file-b.txt": "bbb",
		"file-c.txt": "ccc",
	})

	var got []string
	for obj := range client.ListObjects(ctx, bucket, "", false) {
		require.NotEqual(t, int64(-1), obj.Size, "unexpected error object in ListObjects")
		got = append(got, obj.Key)
	}
	sort.Strings(got)
	assert.Equal(t, []string{"file-a.txt", "file-b.txt", "file-c.txt"}, got)
}

// runListObjectsWithPrefix seeds objects under two prefixes and asserts prefix filtering works.
func runListObjectsWithPrefix(t *testing.T, client *dioclient.Client, mc *minio.Client) {
	t.Helper()
	ctx := context.Background()

	bucket := "test-lo-prefix-" + uniqueSuffix()
	seedBuckets(t, mc, bucket)
	seedObjects(t, mc, bucket, map[string]string{
		"docs/readme.md":  "readme",
		"docs/design.md":  "design",
		"imgs/logo.png":   "logo",
		"imgs/banner.png": "banner",
	})

	// List only docs/ prefix.
	var got []string
	for obj := range client.ListObjects(ctx, bucket, "docs/", false) {
		require.NotEqual(t, int64(-1), obj.Size, "unexpected error object in ListObjects")
		got = append(got, obj.Key)
	}
	sort.Strings(got)
	assert.Equal(t, []string{"docs/design.md", "docs/readme.md"}, got)
}

// runListObjectsRecursiveVsDelimited seeds nested objects and verifies that recursive=false
// returns virtual directory entries while recursive=true returns all leaf objects.
func runListObjectsRecursiveVsDelimited(t *testing.T, client *dioclient.Client, mc *minio.Client) {
	t.Helper()
	ctx := context.Background()

	bucket := "test-lo-recur-" + uniqueSuffix()
	seedBuckets(t, mc, bucket)
	seedObjects(t, mc, bucket, map[string]string{
		"a/b/file1.txt": "1",
		"a/b/file2.txt": "2",
		"a/c/file3.txt": "3",
		"root.txt":      "r",
	})

	// Non-recursive: should see "a/" prefix and "root.txt" at top level.
	var nonRecursiveKeys []string
	for obj := range client.ListObjects(ctx, bucket, "", false) {
		require.NotEqual(t, int64(-1), obj.Size, "unexpected error object in ListObjects (non-recursive)")
		nonRecursiveKeys = append(nonRecursiveKeys, obj.Key)
	}
	sort.Strings(nonRecursiveKeys)
	assert.Equal(t, []string{"a/", "root.txt"}, nonRecursiveKeys,
		"non-recursive list should group objects under virtual prefix a/")

	// Recursive: should see all 4 leaf objects.
	var recursiveKeys []string
	for obj := range client.ListObjects(ctx, bucket, "", true) {
		require.NotEqual(t, int64(-1), obj.Size, "unexpected error object in ListObjects (recursive)")
		recursiveKeys = append(recursiveKeys, obj.Key)
	}
	sort.Strings(recursiveKeys)
	assert.Equal(t, []string{"a/b/file1.txt", "a/b/file2.txt", "a/c/file3.txt", "root.txt"}, recursiveKeys)
}

// uniqueSuffix returns a short unique suffix for bucket names so parallel tests don't collide.
func uniqueSuffix() string {
	return fmt.Sprintf("%04d", bucketSeq.Add(1))
}
