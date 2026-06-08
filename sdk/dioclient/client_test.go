package dioclient

import (
	"context"
	"errors"
	"io"
	"testing"
)

// mockS3Backend is a hand-rolled test double for s3Backend.
// Every method returns zero values unless overridden by the test.
type mockS3Backend struct {
	listBucketsErr error
	putObjectErr   error
}

func (m *mockS3Backend) ListBuckets(_ context.Context) ([]BucketInfo, error) {
	return nil, m.listBucketsErr
}
func (m *mockS3Backend) ListObjects(_ context.Context, _, _ string, _ bool) <-chan ObjectInfo {
	ch := make(chan ObjectInfo)
	close(ch)
	return ch
}
func (m *mockS3Backend) PutObject(_ context.Context, _, _ string, _ io.Reader, _ int64, _ string) error {
	return m.putObjectErr
}
func (m *mockS3Backend) GetObject(_ context.Context, _, _ string) (io.ReadCloser, ObjectInfo, error) {
	return nil, ObjectInfo{}, nil
}
func (m *mockS3Backend) StatObject(_ context.Context, _, _ string) (ObjectInfo, error) {
	return ObjectInfo{}, nil
}
func (m *mockS3Backend) RemoveObject(_ context.Context, _, _ string) error { return nil }
func (m *mockS3Backend) CopyObject(_ context.Context, _, _, _, _ string) error {
	return nil
}

func TestNew_EmptyEndpointUsesDefault(t *testing.T) {
	client, err := New(Config{Endpoint: "", AccessKey: "key", SecretKey: "secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.cfg.Endpoint != "http://localhost:9000" {
		t.Errorf("Endpoint = %q, want http://localhost:9000", client.cfg.Endpoint)
	}
}

func TestNew_InvalidEndpoint(t *testing.T) {
	_, err := New(Config{Endpoint: "://bad", AccessKey: "k", SecretKey: "s"})
	if err == nil {
		t.Fatal("expected error for invalid endpoint URL")
	}
}

func TestClient_PutObject_DefaultContentType(t *testing.T) {
	mock := &mockS3Backend{}
	client := &Client{s3: mock}

	// We can't inspect what contentType was forwarded without a more elaborate mock,
	// but we verify that an empty contentType does not cause an error.
	err := client.PutObject(context.Background(), "bucket", "key", nil, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_ListBuckets_ErrorPropagated(t *testing.T) {
	want := errors.New("backend unavailable")
	mock := &mockS3Backend{listBucketsErr: want}
	client := &Client{s3: mock}

	_, got := client.ListBuckets(context.Background())
	if !errors.Is(got, want) {
		t.Errorf("error = %v, want %v", got, want)
	}
}
