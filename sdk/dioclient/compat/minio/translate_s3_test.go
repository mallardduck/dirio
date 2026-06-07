package minio

import (
	"errors"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

func TestMapBucketInfo(t *testing.T) {
	ts := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	got := mapBucketInfo(minio.BucketInfo{Name: "my-bucket", CreationDate: ts})
	if got.Name != "my-bucket" {
		t.Errorf("Name = %q, want %q", got.Name, "my-bucket")
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ts)
	}
}

func TestMapBucketInfo_ZeroValue(t *testing.T) {
	got := mapBucketInfo(minio.BucketInfo{})
	if got.Name != "" {
		t.Errorf("Name = %q, want empty", got.Name)
	}
	if !got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt = %v, want zero", got.CreatedAt)
	}
}

func TestMapObjectInfo(t *testing.T) {
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	in := minio.ObjectInfo{
		Key:          "path/to/file.txt",
		Size:         1024,
		LastModified: ts,
		ETag:         "abc123",
		ContentType:  "text/plain",
		StorageClass: "STANDARD",
	}
	got := mapObjectInfo(in)
	if got.Key != "path/to/file.txt" {
		t.Errorf("Key = %q, want %q", got.Key, "path/to/file.txt")
	}
	if got.Size != 1024 {
		t.Errorf("Size = %d, want 1024", got.Size)
	}
	if !got.LastModified.Equal(ts) {
		t.Errorf("LastModified = %v, want %v", got.LastModified, ts)
	}
	if got.ETag != "abc123" {
		t.Errorf("ETag = %q, want %q", got.ETag, "abc123")
	}
	if got.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want %q", got.ContentType, "text/plain")
	}
	if got.StorageClass != "STANDARD" {
		t.Errorf("StorageClass = %q, want %q", got.StorageClass, "STANDARD")
	}
	if got.Err != nil {
		t.Errorf("Err = %v, want nil", got.Err)
	}
}

func TestMapObjectInfo_StorageClassPassthrough(t *testing.T) {
	got := mapObjectInfo(minio.ObjectInfo{StorageClass: "GLACIER"})
	if got.StorageClass != "GLACIER" {
		t.Errorf("StorageClass = %q, want GLACIER", got.StorageClass)
	}
}

func TestMapObjectInfo_ErrPassthrough(t *testing.T) {
	want := errors.New("access denied")
	got := mapObjectInfo(minio.ObjectInfo{Err: want})
	if !errors.Is(got.Err, want) {
		t.Errorf("Err = %v, want %v", got.Err, want)
	}
}

func TestMapObjectInfo_ZeroValue(t *testing.T) {
	got := mapObjectInfo(minio.ObjectInfo{})
	if got.Key != "" || got.Size != 0 || got.Err != nil {
		t.Errorf("expected zero ObjectInfo, got %+v", got)
	}
}
