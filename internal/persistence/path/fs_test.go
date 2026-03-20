package path

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"

	"github.com/mallardduck/dirio/internal/consts"
)

func TestValidatePathSafe(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		// Valid paths
		{
			name:    "simple filename",
			path:    "file.txt",
			wantErr: false,
		},
		{
			name:    "nested path",
			path:    "dir/subdir/file.txt",
			wantErr: false,
		},
		{
			name:    "path with dots in filename",
			path:    "my.file.txt",
			wantErr: false,
		},
		{
			name:    "bucket name",
			path:    "my-bucket",
			wantErr: false,
		},
		{
			name:    "bucket with dots",
			path:    "my.bucket.com",
			wantErr: false,
		},

		// Invalid paths - path traversal
		{
			name:    "parent directory traversal",
			path:    "../etc/passwd",
			wantErr: true,
		},
		{
			name:    "double dot at start",
			path:    "..",
			wantErr: true,
		},
		{
			name:    "double dot in middle",
			path:    "dir/../etc/passwd",
			wantErr: true,
		},
		{
			name:    "multiple traversals",
			path:    "../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "traversal with trailing slash",
			path:    "../",
			wantErr: true,
		},

		// Invalid paths - absolute paths
		{
			name:    "unix absolute path",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "absolute path with home",
			path:    "/home/user/file.txt",
			wantErr: true,
		},

		// Invalid paths - null bytes
		{
			name:    "null byte in path",
			path:    "file\x00.txt",
			wantErr: true,
		},
		{
			name:    "null byte at end",
			path:    "file.txt\x00",
			wantErr: true,
		},

		// Invalid paths - empty
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathSafe(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePathSafe(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name:    "already clean path",
			path:    "dir/file.txt",
			want:    "dir/file.txt",
			wantErr: false,
		},
		{
			name:    "path with extra slashes",
			path:    "dir//file.txt",
			want:    "dir/file.txt",
			wantErr: false,
		},
		{
			name:    "path with trailing slash",
			path:    "dir/",
			want:    "dir",
			wantErr: false,
		},
		{
			name:    "path with current dir reference",
			path:    "dir/./file.txt",
			want:    "dir/file.txt",
			wantErr: false,
		},
		{
			name:    "path traversal should fail",
			path:    "../etc/passwd",
			wantErr: true,
		},
		{
			name:    "absolute path should fail",
			path:    "/etc/passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("CleanPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("CleanPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestNewRootFS(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	fs, err := NewRootFS(tmpDir)
	if err != nil {
		t.Fatalf("NewRootFS() failed: %v", err)
	}

	if fs == nil {
		t.Fatal("NewRootFS() returned nil filesystem")
	}

	// Test that we can create a file in the root fs
	f, err := fs.Create("test.txt")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	f.Close()

	// Verify the file exists in the temp directory
	testPath := filepath.Join(tmpDir, "test.txt")
	if _, err := os.Stat(testPath); err != nil {
		t.Errorf("File not created in expected location: %v", err)
	}
}

func TestNewBucketFS(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS, err := NewRootFS(tmpDir)
	if err != nil {
		t.Fatalf("NewRootFS() failed: %v", err)
	}

	tests := []struct {
		name       string
		bucketName string
		wantErr    bool
	}{
		{
			name:       "valid bucket name",
			bucketName: "my-bucket",
			wantErr:    false,
		},
		{
			name:       "bucket with dots",
			bucketName: "my.bucket.com",
			wantErr:    false,
		},
		{
			name:       "path traversal attempt",
			bucketName: "../etc",
			wantErr:    true,
		},
		{
			name:       "absolute path attempt",
			bucketName: "/etc/passwd",
			wantErr:    true,
		},
		{
			name:       "null byte in bucket",
			bucketName: "bucket\x00name",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucketFS, err := NewBucketFS(rootFS, tt.bucketName)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBucketFS(%q) error = %v, wantErr %v", tt.bucketName, err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify bucket directory was created
				bucketPath := filepath.Join(tmpDir, tt.bucketName)
				info, err := os.Stat(bucketPath)
				if err != nil {
					t.Errorf("Bucket directory not created: %v", err)
					return
				}
				if !info.IsDir() {
					t.Errorf("Bucket path is not a directory")
				}

				// Verify we can create files in the bucket
				f, err := bucketFS.Create("object.txt")
				if err != nil {
					t.Errorf("Failed to create file in bucket: %v", err)
					return
				}
				f.Close()

				// Verify file is in bucket directory
				objectPath := filepath.Join(bucketPath, "object.txt")
				if _, err := os.Stat(objectPath); err != nil {
					t.Errorf("Object not created in bucket directory: %v", err)
				}
			}
		})
	}
}

func TestNewMetadataFS(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS, err := NewRootFS(tmpDir)
	if err != nil {
		t.Fatalf("NewRootFS() failed: %v", err)
	}

	metadataFS, err := NewMetadataFS(rootFS)
	if err != nil {
		t.Fatalf("NewMetadataFS() failed: %v", err)
	}

	if metadataFS == nil {
		t.Fatal("NewMetadataFS() returned nil filesystem")
	}

	// Verify metadata directory was created
	metadataPath := filepath.Join(tmpDir, consts.DirIOMetadataDir)
	info, err := os.Stat(metadataPath)
	if err != nil {
		t.Fatalf("metadata directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("metadata path is not a directory")
	}

	// Verify we can write to the metadata filesystem
	testData := []byte("test metadata")
	err = util.WriteFile(metadataFS, "test.json", testData, 0o644)
	if err != nil {
		t.Fatalf("Failed to write to metadata filesystem: %v", err)
	}

	// Verify file is in metadata directory
	testPath := filepath.Join(metadataPath, "test.json")
	if _, err := os.Stat(testPath); err != nil {
		t.Errorf("metadata file not created: %v", err)
	}
}

func TestNewMinIOFS(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS, err := NewRootFS(tmpDir)
	if err != nil {
		t.Fatalf("NewRootFS() failed: %v", err)
	}

	// Test without MinIO directory - should fail
	_, err = NewMinIOFS(rootFS)
	if err == nil {
		t.Error("NewMinIOFS() should fail when MinIO directory doesn't exist")
	}

	// Create MinIO directory
	minioPath := filepath.Join(tmpDir, consts.MinioMetadataDir)
	if err := os.MkdirAll(minioPath, 0o755); err != nil {
		t.Fatalf("Failed to create MinIO directory: %v", err)
	}

	// Test with MinIO directory - should succeed
	minioFS, err := NewMinIOFS(rootFS)
	if err != nil {
		t.Fatalf("NewMinIOFS() failed: %v", err)
	}

	if minioFS == nil {
		t.Fatal("NewMinIOFS() returned nil filesystem")
	}

	// Create a test file in MinIO directory
	testPath := filepath.Join(minioPath, "format.json")
	testData := []byte("{\"version\":\"1\"}")
	if err := os.WriteFile(testPath, testData, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Verify we can read from the MinIO filesystem
	data, err := util.ReadFile(minioFS, "format.json")
	if err != nil {
		t.Fatalf("Failed to read from MinIO filesystem: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("Read data = %q, want %q", string(data), string(testData))
	}
}

func TestScopedFS_BoundaryEnforcement(t *testing.T) {
	// Use memory filesystem for testing
	baseFS := memfs.New()

	// Create some directories
	baseFS.MkdirAll("bucket1/subdir", 0o755)
	baseFS.MkdirAll("bucket2", 0o755)

	// Create scoped filesystem for bucket1
	scopedFS := newScopedFS(baseFS, "bucket1")

	// Test that we can access files within bucket1
	f, err := scopedFS.Create("test.txt")
	if err != nil {
		t.Fatalf("Create() within scope failed: %v", err)
	}
	f.Close()

	// Verify the file is in the correct location
	_, err = baseFS.Stat("bucket1/test.txt")
	if err != nil {
		t.Errorf("File not created in scoped directory: %v", err)
	}

	// Test subdirectories work
	err = scopedFS.MkdirAll("nested/deep/dir", 0o755)
	if err != nil {
		t.Fatalf("MkdirAll() within scope failed: %v", err)
	}

	// Verify nested directory is in correct location
	info, err := baseFS.Stat("bucket1/nested/deep/dir")
	if err != nil {
		t.Errorf("Nested directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Nested path is not a directory")
	}
}

func TestScopedFS_ReadDir(t *testing.T) {
	baseFS := memfs.New()

	// Create directory structure
	baseFS.MkdirAll("bucket/dir1", 0o755)
	baseFS.MkdirAll("bucket/dir2", 0o755)
	f1, _ := baseFS.Create("bucket/file1.txt")
	f1.Close()
	f2, _ := baseFS.Create("bucket/file2.txt")
	f2.Close()

	// Create scoped filesystem
	scopedFS := newScopedFS(baseFS, "bucket")

	// Read directory
	entries, err := scopedFS.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir() failed: %v", err)
	}

	// Should have 4 entries (2 dirs + 2 files)
	if len(entries) != 4 {
		t.Errorf("ReadDir() returned %d entries, want 4", len(entries))
	}

	// Verify entry names
	names := make(map[string]bool)
	for _, entry := range entries {
		names[entry.Name()] = true
	}

	expectedNames := []string{"dir1", "dir2", "file1.txt", "file2.txt"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("ReadDir() missing expected entry: %s", name)
		}
	}
}

func TestScopedFS_Rename(t *testing.T) {
	baseFS := memfs.New()
	baseFS.MkdirAll("bucket", 0o755)

	scopedFS := newScopedFS(baseFS, "bucket")

	// Create a file
	f, _ := scopedFS.Create("old.txt")
	f.Close()

	// Rename it
	err := scopedFS.Rename("old.txt", "new.txt")
	if err != nil {
		t.Fatalf("Rename() failed: %v", err)
	}

	// Verify old file doesn't exist
	_, err = scopedFS.Stat("old.txt")
	if err == nil {
		t.Error("Old file still exists after rename")
	}

	// Verify new file exists
	_, err = scopedFS.Stat("new.txt")
	if err != nil {
		t.Errorf("New file doesn't exist after rename: %v", err)
	}

	// Verify in base filesystem
	_, err = baseFS.Stat("bucket/new.txt")
	if err != nil {
		t.Errorf("Renamed file not in correct location: %v", err)
	}
}

func TestScopedFS_TempFile(t *testing.T) {
	baseFS := memfs.New()
	baseFS.MkdirAll("bucket", 0o755)

	scopedFS := newScopedFS(baseFS, "bucket")

	// Create temp file
	tmpFile, err := scopedFS.TempFile("", "temp-")
	if err != nil {
		t.Fatalf("TempFile() failed: %v", err)
	}
	tmpName := filepath.Base(tmpFile.Name())
	tmpFile.Close()

	// Verify temp file exists in scoped filesystem
	_, err = scopedFS.Stat(tmpName)
	if err != nil {
		t.Errorf("Temp file not accessible in scoped filesystem: %v", err)
	}

	// Clean up
	scopedFS.Remove(tmpName)
}

func TestScopedFS_Chroot(t *testing.T) {
	baseFS := memfs.New()
	baseFS.MkdirAll("bucket/subdir/nested", 0o755)

	scopedFS := newScopedFS(baseFS, "bucket")

	// Chroot to subdirectory
	subFS, err := scopedFS.Chroot("subdir")
	if err != nil {
		t.Fatalf("Chroot() failed: %v", err)
	}

	// Create file in chrooted filesystem
	f, err := subFS.Create("test.txt")
	if err != nil {
		t.Fatalf("Create() in chrooted filesystem failed: %v", err)
	}
	f.Close()

	// Verify file is in correct location
	_, err = baseFS.Stat("bucket/subdir/test.txt")
	if err != nil {
		t.Errorf("File not created in chrooted location: %v", err)
	}
}
