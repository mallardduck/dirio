package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPutObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	content := "Hello, DirIO!"
	req, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "hello.txt"), strings.NewReader(content))
	req.Header.Set("Content-Type", "text/plain")
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify ETag is returned
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Error("Expected ETag header to be set")
	}
}

func TestPutObjectInSubfolder(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	content := "Nested content"
	req, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "folder/subfolder/file.txt"), strings.NewReader(content))
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestPutObjectToNonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("PUT", ts.ObjectURL("nonexistent", "file.txt"), strings.NewReader("content"))
	req.ContentLength = 7

	resp, err := http.DefaultClient.Do(req)
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

func TestGetObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	content := "Hello, DirIO!"
	ts.PutObject(t, "test-bucket", "hello.txt", content)

	resp, err := http.Get(ts.ObjectURL("test-bucket", "hello.txt"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != content {
		t.Errorf("Expected content %q, got %q", content, string(body))
	}

	// Verify headers
	if resp.Header.Get("ETag") == "" {
		t.Error("Expected ETag header")
	}
	if resp.Header.Get("Content-Length") != "13" {
		t.Errorf("Expected Content-Length 13, got %s", resp.Header.Get("Content-Length"))
	}
	if resp.Header.Get("Last-Modified") == "" {
		t.Error("Expected Last-Modified header")
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Errorf("Expected Accept-Ranges: bytes, got %s", resp.Header.Get("Accept-Ranges"))
	}
}

func TestGetObjectNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	resp, err := http.Get(ts.ObjectURL("test-bucket", "nonexistent.txt"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "NoSuchKey") {
		t.Errorf("Expected NoSuchKey error, got: %s", body)
	}
}

func TestGetObjectFromNonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.ObjectURL("nonexistent", "file.txt"))
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

func TestHeadObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.PutObject(t, "test-bucket", "hello.txt", "Hello, DirIO!")

	req, _ := http.NewRequest("HEAD", ts.ObjectURL("test-bucket", "hello.txt"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// HEAD should return headers but no body
	if resp.Header.Get("Content-Length") != "13" {
		t.Errorf("Expected Content-Length 13, got %s", resp.Header.Get("Content-Length"))
	}
	if resp.Header.Get("ETag") == "" {
		t.Error("Expected ETag header")
	}
	if resp.Header.Get("Last-Modified") == "" {
		t.Error("Expected Last-Modified header")
	}
}

func TestHeadObjectNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	req, _ := http.NewRequest("HEAD", ts.ObjectURL("test-bucket", "nonexistent.txt"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestDeleteObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.PutObject(t, "test-bucket", "hello.txt", "Hello, DirIO!")

	req, _ := http.NewRequest("DELETE", ts.ObjectURL("test-bucket", "hello.txt"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify object is gone
	getResp, _ := http.Get(ts.ObjectURL("test-bucket", "hello.txt"))
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected object to be deleted, but GET returned %d", getResp.StatusCode)
	}
}

func TestDeleteObjectNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// S3 returns 204 even when deleting non-existent object
	req, _ := http.NewRequest("DELETE", ts.ObjectURL("test-bucket", "nonexistent.txt"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204 (S3 behavior), got %d", resp.StatusCode)
	}
}

func TestDeleteObjectFromNonexistentBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("DELETE", ts.ObjectURL("nonexistent", "file.txt"), nil)
	resp, err := http.DefaultClient.Do(req)
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

func TestPutAndGetLargeObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Create a 1MB object
	content := strings.Repeat("A", 1024*1024)
	req, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "large.bin"), strings.NewReader(content))
	req.ContentLength = int64(len(content))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Retrieve and verify
	getResp, err := http.Get(ts.ObjectURL("test-bucket", "large.bin"))
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer getResp.Body.Close()

	body, _ := io.ReadAll(getResp.Body)
	if len(body) != len(content) {
		t.Errorf("Expected %d bytes, got %d", len(content), len(body))
	}
}