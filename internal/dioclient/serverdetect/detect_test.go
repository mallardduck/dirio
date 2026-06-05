package serverdetect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serve returns an httptest.Server that responds 200 to the given paths and
// 404 to everything else.
func serve(t *testing.T, paths ...string) *httptest.Server {
	t.Helper()
	ok := make(map[string]bool, len(paths))
	for _, p := range paths {
		ok[p] = true
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok[r.URL.Path] {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDetect_DirIO(t *testing.T) {
	// DirIO serves both /.dirio/health and /minio/health/live; detection must
	// identify DirIO first.
	srv := serve(t, "/.dirio/health", "/minio/health/live")

	st, err := Detect(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, ServerTypeDirIO, st)
}

func TestDetect_MinIO(t *testing.T) {
	// MinIO serves /minio/health/live but not /.dirio/health.
	srv := serve(t, "/minio/health/live")

	st, err := Detect(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, ServerTypeMinIO, st)
}

func TestDetect_S3Generic(t *testing.T) {
	// A server that doesn't respond to either health probe.
	srv := serve(t /* no paths — all 404 */)

	st, err := Detect(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, ServerTypeS3Generic, st)
}

func TestDetect_InvalidEndpoint(t *testing.T) {
	_, err := Detect(context.Background(), "://bad-url")
	assert.Error(t, err)
}

func TestDetect_UnreachableEndpoint(t *testing.T) {
	// Port 1 is typically not listening; Detect should return S3Generic, not error.
	st, err := Detect(context.Background(), "http://127.0.0.1:1")
	require.NoError(t, err)
	assert.Equal(t, ServerTypeS3Generic, st)
}
