package integration

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mallardduck/dirio/pkg/s3types"
)

func TestListObjectsV2Empty(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	resp, err := http.Get(ts.BucketURL("test-bucket") + "?list-type=2")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	if err := xml.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if result.KeyCount != 0 {
		t.Errorf("Expected 0 objects, got %d", result.KeyCount)
	}
	if len(result.Contents) != 0 {
		t.Errorf("Expected empty contents, got %d items", len(result.Contents))
	}
}

func TestListObjectsV2WithObjects(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"file1.txt":          "content1",
		"file2.txt":          "content2",
		"photos/photo1.jpg":  "photo1",
		"photos/photo2.jpg":  "photo2",
		"docs/readme.md":     "readme",
		"docs/sub/nested.md": "nested",
	})

	resp, err := http.Get(ts.BucketURL("test-bucket") + "?list-type=2")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	if err := xml.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if result.KeyCount != 6 {
		t.Errorf("Expected 6 objects, got %d", result.KeyCount)
	}

	// Check that all keys are present
	keys := make(map[string]bool)
	for _, obj := range result.Contents {
		keys[obj.Key] = true
	}

	expectedKeys := []string{
		"file1.txt", "file2.txt",
		"photos/photo1.jpg", "photos/photo2.jpg",
		"docs/readme.md", "docs/sub/nested.md",
	}
	for _, key := range expectedKeys {
		if !keys[key] {
			t.Errorf("Expected key %s not found in results", key)
		}
	}
}

func TestListObjectsV2WithPrefix(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"file1.txt":         "content1",
		"file2.txt":         "content2",
		"photos/photo1.jpg": "photo1",
		"photos/photo2.jpg": "photo2",
		"docs/readme.md":    "readme",
	})

	// Test prefix=photos/
	resp, err := http.Get(ts.BucketURL("test-bucket") + "?list-type=2&prefix=photos/")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	if err := xml.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if result.KeyCount != 2 {
		t.Errorf("Expected 2 objects with prefix 'photos/', got %d", result.KeyCount)
	}

	if result.Prefix != "photos/" {
		t.Errorf("Expected Prefix 'photos/', got %q", result.Prefix)
	}

	for _, obj := range result.Contents {
		if !strings.HasPrefix(obj.Key, "photos/") {
			t.Errorf("Object %s does not have prefix 'photos/'", obj.Key)
		}
	}
}

func TestListObjectsV2WithPrefixPartialMatch(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"file1.txt":  "content1",
		"file2.txt":  "content2",
		"filter.log": "log",
		"photos.zip": "zip",
	})

	// Test prefix=file (should match file1.txt, file2.txt, filter.log)
	resp, err := http.Get(ts.BucketURL("test-bucket") + "?list-type=2&prefix=file")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	if err := xml.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if result.KeyCount != 2 {
		t.Errorf("Expected 2 objects with prefix 'file', got %d", result.KeyCount)
	}
}

func TestListObjectsV2NonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.BucketURL("nonexistent") + "?list-type=2")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "NoSuchBucket") {
		t.Errorf("Expected NoSuchBucket error, got: %s", body)
	}
}

func TestListObjectsV1(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
	})

	// V1 is the default (no list-type param)
	resp, err := http.Get(ts.BucketURL("test-bucket"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result s3types.ListBucketResult
	body, _ := io.ReadAll(resp.Body)
	if err := xml.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(result.Contents) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result.Contents))
	}

	if result.Name != "test-bucket" {
		t.Errorf("Expected bucket name 'test-bucket', got %q", result.Name)
	}
}

func TestListObjectsV1WithPrefix(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.CreateTestObjects(t, "test-bucket", map[string]string{
		"logs/app.log":    "app",
		"logs/error.log":  "error",
		"config/app.yaml": "config",
	})

	resp, err := http.Get(ts.BucketURL("test-bucket") + "?prefix=logs/")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var result s3types.ListBucketResult
	body, _ := io.ReadAll(resp.Body)
	if err := xml.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(result.Contents) != 2 {
		t.Errorf("Expected 2 objects with prefix 'logs/', got %d", len(result.Contents))
	}

	if result.Prefix != "logs/" {
		t.Errorf("Expected Prefix 'logs/', got %q", result.Prefix)
	}
}

func TestListObjectsV1NonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.BucketURL("nonexistent"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "NoSuchBucket") {
		t.Errorf("Expected NoSuchBucket error, got: %s", body)
	}
}

func TestListObjectsResponseFields(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.PutObject(t, "test-bucket", "test.txt", "test content")

	resp, err := http.Get(ts.BucketURL("test-bucket") + "?list-type=2")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var result s3types.ListBucketV2Result
	body, _ := io.ReadAll(resp.Body)
	if err := xml.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("Expected 1 object, got %d", len(result.Contents))
	}

	obj := result.Contents[0]
	if obj.Key != "test.txt" {
		t.Errorf("Expected Key 'test.txt', got %q", obj.Key)
	}
	if obj.Size != 12 {
		t.Errorf("Expected Size 12, got %d", obj.Size)
	}
	if obj.StorageClass != "STANDARD" {
		t.Errorf("Expected StorageClass 'STANDARD', got %q", obj.StorageClass)
	}
	if obj.LastModified.IsZero() {
		t.Error("Expected LastModified to be set")
	}
}