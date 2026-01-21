package integration

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mallardduck/dirio/pkg/s3types"
)

func TestListBucketsEmpty(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.URL("/"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<Buckets></Buckets>") {
		t.Errorf("Expected empty buckets list, got: %s", body)
	}
}

func TestCreateBucket(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("PUT", ts.BucketURL("test-bucket"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestCreateBucketDuplicate(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// Create first bucket
	ts.CreateBucket(t, "test-bucket")

	// Try to create duplicate
	req, _ := http.NewRequest("PUT", ts.BucketURL("test-bucket"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("Expected status 409 Conflict, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "BucketAlreadyExists") {
		t.Errorf("Expected BucketAlreadyExists error, got: %s", body)
	}
}

func TestListBucketsAfterCreate(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// Create two buckets
	ts.CreateBucket(t, "bucket-alpha")
	ts.CreateBucket(t, "bucket-beta")

	resp, err := http.Get(ts.URL("/"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "<Name>bucket-alpha</Name>") {
		t.Errorf("Expected bucket-alpha in list, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "<Name>bucket-beta</Name>") {
		t.Errorf("Expected bucket-beta in list, got: %s", bodyStr)
	}
}

func TestHeadBucketExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	req, _ := http.NewRequest("HEAD", ts.BucketURL("test-bucket"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHeadBucketNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("HEAD", ts.BucketURL("nonexistent"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestGetBucketLocation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	resp, err := http.Get(ts.BucketURL("test-bucket") + "?location")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "us-east-1") {
		t.Errorf("Expected us-east-1 location, got: %s", body)
	}
}

func TestDeleteBucketEmpty(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	req, _ := http.NewRequest("DELETE", ts.BucketURL("test-bucket"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify bucket is gone
	headReq, _ := http.NewRequest("HEAD", ts.BucketURL("test-bucket"), nil)
	headResp, _ := http.DefaultClient.Do(headReq)
	defer headResp.Body.Close()

	if headResp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected bucket to be deleted, but HEAD returned %d", headResp.StatusCode)
	}
}

func TestDeleteBucketNotEmpty(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")
	ts.PutObject(t, "test-bucket", "file.txt", "content")

	req, _ := http.NewRequest("DELETE", ts.BucketURL("test-bucket"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("Expected status 409 Conflict, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "BucketNotEmpty") {
		t.Errorf("Expected BucketNotEmpty error, got: %s", body)
	}
}

func TestDeleteBucketNotExists(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	req, _ := http.NewRequest("DELETE", ts.BucketURL("nonexistent"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}

	var errResp s3types.ErrorResponse
	body, _ := io.ReadAll(resp.Body)
	xml.Unmarshal(body, &errResp)

	if errResp.Code != "NoSuchBucket" {
		t.Errorf("Expected NoSuchBucket error code, got: %s", errResp.Code)
	}
}