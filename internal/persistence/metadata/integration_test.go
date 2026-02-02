package metadata

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/mallardduck/dirio/internal/minio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMinIOImport_CreatesMetadataFiles tests that importing MinIO data actually creates metadata files
func TestMinIOImport_CreatesMetadataFiles(t *testing.T) {
	// Check if test MinIO data exists
	minioDataDir := "../../minio-data/.minio.sys"
	if _, err := os.Stat(minioDataDir); os.IsNotExist(err) {
		t.Skip("Skipping test: minio-data directory not found")
	}

	// Import from MinIO
	minioFS := osfs.New(minioDataDir)
	result, err := minio.Import(minioFS)
	require.NoError(t, err)

	// Create DirIO metadata manager
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)
	mgr, err := New(rootFS)
	require.NoError(t, err)

	// Import object metadata
	objectCount := 0
	for bucketName, objects := range result.ObjectMetadata {
		for objectKey, minioMeta := range objects {
			dirioMeta := &ObjectMetadata{
				ContentType:    minioMeta.Meta["content-type"],
				ETag:           minioMeta.Meta["etag"],
				CustomMetadata: make(map[string]string),
			}

			for key, value := range minioMeta.Meta {
				if key != "content-type" && key != "etag" {
					dirioMeta.CustomMetadata[key] = value
				}
			}

			err := mgr.PutObjectMetadata(context.Background(), bucketName, objectKey, dirioMeta)
			require.NoError(t, err, "Failed to save metadata for %s/%s", bucketName, objectKey)

			t.Logf("✓ Saved metadata for %s/%s", bucketName, objectKey)
			objectCount++
		}
	}

	t.Logf("Imported %d object metadata files", objectCount)
	assert.Greater(t, objectCount, 0, "Should have imported at least one object")

	// Debug: List all files in tmpDir
	t.Logf("Listing all files in %s:", tmpDir)
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			relPath, _ := filepath.Rel(tmpDir, path)
			if info.IsDir() {
				t.Logf("  DIR: %s", relPath)
			} else {
				t.Logf("  FILE: %s", relPath)
			}
		}
		return nil
	})
	require.NoError(t, err)

	// Verify metadata files were created
	metadataDir := filepath.Join(tmpDir, ".dirio", "objects")
	fileCount := 0
	err = filepath.Walk(metadataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Logf("Walk error: %v", err)
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			relPath, _ := filepath.Rel(tmpDir, path)
			t.Logf("  Metadata file: %s", relPath)
			fileCount++
		}
		return nil
	})
	if err != nil {
		t.Logf("Error walking metadata directory: %v", err)
	}

	assert.Equal(t, objectCount, fileCount, "Number of metadata files should match number of imported objects")
}
