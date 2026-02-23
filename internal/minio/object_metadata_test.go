package minio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImport_WithObjectMetadata tests importing objects with fs.json metadata
func TestImport_WithObjectMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal format.json
	minioSys := filepath.Join(tmpDir, ".minio.sys")
	require.NoError(t, os.MkdirAll(minioSys, 0755))
	_, formatJSON := formatJSONText()
	require.NoError(t, os.WriteFile(filepath.Join(minioSys, "format.json"), []byte(formatJSON), 0644))

	// Create bucket directory structure
	bucketsDir := filepath.Join(minioSys, "buckets", "test-bucket")
	require.NoError(t, os.MkdirAll(bucketsDir, 0755))

	// Create object metadata directory with fs.json
	objectDir := filepath.Join(bucketsDir, "test-file.txt")
	require.NoError(t, os.MkdirAll(objectDir, 0755))

	// Create fs.json metadata
	fsJSON := ObjectMetadata{
		Version: "1.0.2",
		Checksum: ChecksumInfo{
			Algorithm: "",
			BlockSize: 0,
			Hashes:    nil,
		},
		Meta: map[string]string{
			"content-type":        "text/plain",
			"etag":                "abcd1234",
			"cache-control":       "max-age=3600",
			"content-disposition": "attachment; filename=\"test.txt\"",
			"x-amz-meta-author":   "Alice",
		},
	}
	fsJSONData, err := json.Marshal(fsJSON)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(objectDir, "fs.json"), fsJSONData, 0644))

	// Create another object with subdirectory path
	subObjDir := filepath.Join(bucketsDir, "docs", "readme.md")
	require.NoError(t, os.MkdirAll(subObjDir, 0755))
	fsJSON2 := ObjectMetadata{
		Version: "1.0.2",
		Meta: map[string]string{
			"content-type": "text/markdown",
			"etag":         "xyz789",
		},
	}
	fsJSON2Data, err := json.Marshal(fsJSON2)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(subObjDir, "fs.json"), fsJSON2Data, 0644))

	// Run import
	minioFS := osfs.New(filepath.Join(tmpDir, ".minio.sys"))
	result, err := Import(minioFS)
	require.NoError(t, err)

	// Verify object metadata was imported
	assert.Len(t, result.ObjectMetadata, 1, "Should have metadata for one bucket")
	assert.Contains(t, result.ObjectMetadata, "test-bucket")

	bucketMetadata := result.ObjectMetadata["test-bucket"]
	assert.Len(t, bucketMetadata, 2, "Should have metadata for two objects")

	// Verify first object metadata
	assert.Contains(t, bucketMetadata, "test-file.txt")
	obj1 := bucketMetadata["test-file.txt"]
	assert.Equal(t, "text/plain", obj1.Meta["content-type"])
	assert.Equal(t, "abcd1234", obj1.Meta["etag"])
	assert.Equal(t, "max-age=3600", obj1.Meta["cache-control"])
	assert.Equal(t, "attachment; filename=\"test.txt\"", obj1.Meta["content-disposition"])
	assert.Equal(t, "Alice", obj1.Meta["x-amz-meta-author"])

	// Verify second object metadata
	assert.Contains(t, bucketMetadata, filepath.Join("docs", "readme.md"))
	obj2 := bucketMetadata[filepath.Join("docs", "readme.md")]
	assert.Equal(t, "text/markdown", obj2.Meta["content-type"])
	assert.Equal(t, "xyz789", obj2.Meta["etag"])
}
