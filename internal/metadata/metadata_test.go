package metadata

import (
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectMetadata_PutAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)

	mgr, err := New(rootFS)
	require.NoError(t, err)

	// Create test metadata
	meta := &ObjectMetadata{
		Version:      ObjectMetadataVersion,
		ContentType:  "text/plain",
		Size:         1234,
		ETag:         `"abcd1234"`,
		LastModified: time.Now().Truncate(time.Second),
		CustomMetadata: map[string]string{
			"Cache-Control":       "max-age=3600",
			"Content-Disposition": "attachment; filename=\"test.txt\"",
			"x-amz-meta-author":   "Alice",
		},
	}

	// Store metadata
	err = mgr.PutObjectMetadata("test-bucket", "path/to/object.txt", meta)
	require.NoError(t, err)

	// Retrieve metadata (this verifies it was actually saved)
	retrieved, err := mgr.GetObjectMetadata("test-bucket", "path/to/object.txt")
	require.NoError(t, err)

	// Verify metadata matches
	assert.Equal(t, meta.ContentType, retrieved.ContentType)
	assert.Equal(t, meta.Size, retrieved.Size)
	assert.Equal(t, meta.ETag, retrieved.ETag)
	assert.Equal(t, meta.LastModified, retrieved.LastModified)
	assert.Equal(t, meta.CustomMetadata, retrieved.CustomMetadata)
}

func TestObjectMetadata_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)

	mgr, err := New(rootFS)
	require.NoError(t, err)

	// Create test metadata
	meta := &ObjectMetadata{
		Version:      ObjectMetadataVersion,
		ContentType:  "text/plain",
		Size:         1234,
		ETag:         `"abcd1234"`,
		LastModified: time.Now(),
	}

	// Store metadata
	err = mgr.PutObjectMetadata("test-bucket", "test-object.txt", meta)
	require.NoError(t, err)

	// Delete metadata
	err = mgr.DeleteObjectMetadata("test-bucket", "test-object.txt")
	require.NoError(t, err)

	// Verify metadata is gone
	_, err = mgr.GetObjectMetadata("test-bucket", "test-object.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "object metadata not found")
}

func TestObjectMetadata_GetNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)

	mgr, err := New(rootFS)
	require.NoError(t, err)

	// Try to get non-existent metadata
	_, err = mgr.GetObjectMetadata("test-bucket", "nonexistent.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "object metadata not found")
}

func TestObjectMetadata_CompactJSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)

	mgr, err := New(rootFS)
	require.NoError(t, err)

	// Create test metadata
	meta := &ObjectMetadata{
		Version:      ObjectMetadataVersion,
		ContentType:  "text/plain",
		Size:         1234,
		ETag:         `"abcd1234"`,
		LastModified: time.Now().Truncate(time.Second),
		CustomMetadata: map[string]string{
			"x-amz-meta-author": "Alice",
		},
	}

	// Store metadata
	err = mgr.PutObjectMetadata("test-bucket", "test.txt", meta)
	require.NoError(t, err)

	// Read raw JSON file directly
	metaPath := ".dirio/objects/test-bucket/test.txt.json"
	data, err := util.ReadFile(rootFS, metaPath)
	require.NoError(t, err)

	jsonStr := string(data)

	// Verify it's compact JSON (single line - no newlines except at the very end)
	lines := 0
	for _, ch := range jsonStr {
		if ch == '\n' {
			lines++
		}
	}
	assert.LessOrEqual(t, lines, 1, "JSON should be compact (single line)")

	// Verify version field exists
	assert.Contains(t, jsonStr, `"version":"1.0.0"`)

	// Log the JSON for manual inspection
	t.Logf("Compact JSON: %s", jsonStr)
}
