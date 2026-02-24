package metadata

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/internal/persistence/path"
	"github.com/mallardduck/dirio/pkg/iam"
)

func TestObjectMetadata_PutAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)

	mgr, err := New(rootFS)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })

	// Create test metadata
	meta := &ObjectMetadata{
		Version:      ObjectMetadataVersion,
		ContentType:  "text/plain",
		Size:         1234,
		ETag:         "abcd1234",
		LastModified: time.Now().Truncate(time.Second),
		CustomMetadata: map[string]string{
			"Cache-Control":       "max-age=3600",
			"Content-Disposition": "attachment; filename=\"test.txt\"",
			"x-amz-meta-author":   "Alice",
		},
	}

	// Store metadata
	err = mgr.PutObjectMetadata(context.Background(), "test-bucket", "path/to/object.txt", meta)
	require.NoError(t, err)

	// Retrieve metadata (this verifies it was actually saved)
	retrieved, err := mgr.GetObjectMetadata(context.Background(), "test-bucket", "path/to/object.txt")
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
	t.Cleanup(func() { _ = mgr.Close() })

	// Create test metadata
	meta := &ObjectMetadata{
		Version:      ObjectMetadataVersion,
		ContentType:  "text/plain",
		Size:         1234,
		ETag:         "abcd1234",
		LastModified: time.Now(),
	}

	// Store metadata
	err = mgr.PutObjectMetadata(context.Background(), "test-bucket", "test-object.txt", meta)
	require.NoError(t, err)

	// Delete metadata
	err = mgr.DeleteObjectMetadata(context.Background(), "test-bucket", "test-object.txt")
	require.NoError(t, err)

	// Verify metadata is gone
	_, err = mgr.GetObjectMetadata(context.Background(), "test-bucket", "test-object.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "object metadata not found")
}

func TestObjectMetadata_GetNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)

	mgr, err := New(rootFS)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })

	// Try to get non-existent metadata
	_, err = mgr.GetObjectMetadata(context.Background(), "test-bucket", "nonexistent.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "object metadata not found")
}

func TestObjectMetadata_CompactJSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)

	mgr, err := New(rootFS)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })

	// Create test metadata
	meta := &ObjectMetadata{
		Version:      ObjectMetadataVersion,
		ContentType:  "text/plain",
		Size:         1234,
		ETag:         "abcd1234",
		LastModified: time.Now().Truncate(time.Second),
		CustomMetadata: map[string]string{
			"x-amz-meta-author": "Alice",
		},
	}

	// Store metadata
	err = mgr.PutObjectMetadata(context.Background(), "test-bucket", "test.txt", meta)
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

// ============================================================================
// Bolt index tests
// ============================================================================

func TestCreateUser_DuplicateUsername(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)
	mgr, err := New(rootFS)
	require.NoError(t, err)
	defer mgr.Close()

	ctx := context.Background()

	userA := &User{Username: "alice", AccessKey: "AKIDA", SecretKey: "secretA"}
	require.NoError(t, mgr.CreateOrUpdateUser(ctx, userA))

	userB := &User{Username: "alice", AccessKey: "AKIDB", SecretKey: "secretB"}
	err = mgr.CreateOrUpdateUser(ctx, userB)
	require.ErrorIs(t, err, ErrUsernameAlreadyTaken)
}

func TestCreateUser_DuplicateAccessKey(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)
	mgr, err := New(rootFS)
	require.NoError(t, err)
	defer mgr.Close()

	ctx := context.Background()

	userA := &User{Username: "alice", AccessKey: "AKIDA", SecretKey: "secretA"}
	require.NoError(t, mgr.CreateOrUpdateUser(ctx, userA))

	userB := &User{Username: "bob", AccessKey: "AKIDA", SecretKey: "secretB"}
	err = mgr.CreateOrUpdateUser(ctx, userB)
	require.ErrorIs(t, err, ErrAccessKeyAlreadyTaken)
}

func TestCreateUser_SameUserUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)
	mgr, err := New(rootFS)
	require.NoError(t, err)
	defer mgr.Close()

	ctx := context.Background()

	userA := &User{Username: "alice", AccessKey: "AKIDA", SecretKey: "secretA"}
	require.NoError(t, mgr.CreateOrUpdateUser(ctx, userA))

	// Updating the same user with the same username/accessKey must succeed.
	update := &User{UUID: userA.UUID, Username: "alice", AccessKey: "AKIDA", SecretKey: "newSecret"}
	require.NoError(t, mgr.CreateOrUpdateUser(ctx, update))

	got, err := mgr.GetUserByUsername(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, userA.UUID, got.UUID)
}

func TestUpdateUser_UsernameIndexSync(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)
	mgr, err := New(rootFS)
	require.NoError(t, err)
	defer mgr.Close()

	ctx := context.Background()

	userA := &User{Username: "alice", AccessKey: "AKIDA", SecretKey: "secretA"}
	require.NoError(t, mgr.CreateOrUpdateUser(ctx, userA))

	// Rename username via UpdateUser.
	require.NoError(t, mgr.UpdateUser(ctx, userA.UUID, &User{Username: "alicia"}))

	// Old username must no longer resolve.
	_, err = mgr.GetUserByUsername(ctx, "alice")
	require.ErrorIs(t, err, ErrUserNotFound)

	// New username must resolve to the same user.
	got, err := mgr.GetUserByUsername(ctx, "alicia")
	require.NoError(t, err)
	assert.Equal(t, userA.UUID, got.UUID)
}

func TestIndexRebuild(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)

	// First manager: create a user and close.
	mgr, err := New(rootFS)
	require.NoError(t, err)

	ctx := context.Background()
	userA := &User{Username: "alice", AccessKey: "AKIDA", SecretKey: "secretA"}
	require.NoError(t, mgr.CreateOrUpdateUser(ctx, userA))
	require.NoError(t, mgr.Close())

	// Delete the bolt DB to force a full rebuild.
	dbPath := filepath.Join(tmpDir, path.MetadataDir, "dirio.db")
	require.NoError(t, os.Remove(dbPath))

	// Second manager must rebuild indexes from JSON files.
	mgr2, err := New(rootFS)
	require.NoError(t, err)
	defer mgr2.Close()

	got, err := mgr2.GetUserByUsername(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, "AKIDA", got.AccessKey)

	got, err = mgr2.GetUserByAccessKey(ctx, "AKIDA")
	require.NoError(t, err)
	assert.Equal(t, "alice", got.Username)
}

func TestStartupReconciliation(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := osfs.New(tmpDir)

	// Create manager once to set up the directory layout.
	mgr, err := New(rootFS)
	require.NoError(t, err)

	ctx := context.Background()

	// Write a user JSON file directly, bypassing the index.
	uid := uuid.New()
	user := &User{
		Version:   iam.UserMetadataVersion,
		UUID:      uid,
		Username:  "bob",
		AccessKey: "AKIDB",
		SecretKey: "secretB",
		UpdatedAt: time.Now(),
	}
	require.NoError(t, mgr.SaveUser(ctx, uid, user))
	require.NoError(t, mgr.Close())

	// Reopen: reconcileIndexes must pick up the JSON-only user.
	mgr2, err := New(rootFS)
	require.NoError(t, err)
	defer mgr2.Close()

	got, err := mgr2.GetUserByUsername(ctx, "bob")
	require.NoError(t, err)
	assert.Equal(t, uid, got.UUID)

	got, err = mgr2.GetUserByAccessKey(ctx, "AKIDB")
	require.NoError(t, err)
	assert.Equal(t, uid, got.UUID)
}
