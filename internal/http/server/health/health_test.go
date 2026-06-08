package health

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── test doubles ─────────────────────────────────────────────────────────────

type stubPinger struct{ err error }

func (s stubPinger) Ping() error { return s.err }

// stubFS implements just enough of billy.Filesystem for health checks.
type stubFS struct{ readDirErr error }

func (s stubFS) ReadDir(string) ([]os.FileInfo, error) { return nil, s.readDirErr }

// Satisfy the remaining billy.Filesystem interface methods with no-ops so the
// compiler is happy; health checks only call ReadDir.
func (stubFS) Create(string) (billy.File, error)                     { return nil, nil }
func (stubFS) Open(string) (billy.File, error)                       { return nil, nil }
func (stubFS) OpenFile(string, int, os.FileMode) (billy.File, error) { return nil, nil }
func (stubFS) Stat(string) (os.FileInfo, error)                      { return nil, nil }
func (stubFS) Rename(string, string) error                           { return nil }
func (stubFS) Remove(string) error                                   { return nil }
func (stubFS) Join(elem ...string) string                            { return "" }
func (stubFS) TempFile(string, string) (billy.File, error)           { return nil, nil }
func (stubFS) MkdirAll(string, os.FileMode) error                    { return nil }
func (stubFS) Lstat(string) (os.FileInfo, error)                     { return nil, nil }
func (stubFS) Symlink(string, string) error                          { return nil }
func (stubFS) Readlink(string) (string, error)                       { return "", nil }
func (stubFS) Chroot(string) (billy.Filesystem, error)               { return nil, nil }
func (stubFS) Root() string                                          { return "" }

// ── HandleLive ────────────────────────────────────────────────────────────────

func TestHandleLive_AlwaysOK(t *testing.T) {
	h := New(stubPinger{}, stubFS{})
	rec := httptest.NewRecorder()
	h.HandleLive().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.dirio/health/live", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ── HandleReady ───────────────────────────────────────────────────────────────

func TestHandleReady_AllHealthy(t *testing.T) {
	h := New(stubPinger{err: nil}, stubFS{readDirErr: nil})
	rec := httptest.NewRecorder()
	h.HandleReady().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.dirio/health/ready", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleReady_MetadataDown(t *testing.T) {
	h := New(stubPinger{err: errors.New("bolt timeout")}, stubFS{})
	rec := httptest.NewRecorder()
	h.HandleReady().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.dirio/health/ready", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleReady_StorageDown(t *testing.T) {
	h := New(stubPinger{err: nil}, stubFS{readDirErr: errors.New("disk unavailable")})
	rec := httptest.NewRecorder()
	h.HandleReady().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.dirio/health/ready", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// ── HandleHealth ──────────────────────────────────────────────────────────────

func TestHandleHealth_AllHealthy(t *testing.T) {
	h := New(stubPinger{}, stubFS{})
	rec := httptest.NewRecorder()
	h.HandleHealth().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.dirio/health", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp healthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "ok", resp.Components["metadata_db"].Status)
	assert.Equal(t, "ok", resp.Components["storage"].Status)
	assert.NotEmpty(t, resp.Uptime)
}

func TestHandleHealth_MetadataDown(t *testing.T) {
	h := New(stubPinger{err: errors.New("connection refused")}, stubFS{})
	rec := httptest.NewRecorder()
	h.HandleHealth().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.dirio/health", nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp healthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "degraded", resp.Status)
	assert.Equal(t, "error", resp.Components["metadata_db"].Status)
	assert.Contains(t, resp.Components["metadata_db"].Error, "connection refused")
}

func TestHandleHealth_StorageDown(t *testing.T) {
	h := New(stubPinger{}, stubFS{readDirErr: errors.New("read-only filesystem")})
	rec := httptest.NewRecorder()
	h.HandleHealth().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.dirio/health", nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp healthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "degraded", resp.Status)
	assert.Equal(t, "error", resp.Components["storage"].Status)
}

func TestHandleHealth_BothDown(t *testing.T) {
	h := New(stubPinger{err: errors.New("db err")}, stubFS{readDirErr: errors.New("fs err")})
	rec := httptest.NewRecorder()
	h.HandleHealth().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.dirio/health", nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp healthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "error", resp.Components["metadata_db"].Status)
	assert.Equal(t, "error", resp.Components["storage"].Status)
}

func TestHandleHealth_UptimeIncreases(t *testing.T) {
	h := New(stubPinger{}, stubFS{})
	// startTime is set in New(); just verify uptime is parseable and non-negative.
	rec := httptest.NewRecorder()
	h.HandleHealth().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.dirio/health", nil))

	var resp healthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	d, err := time.ParseDuration(resp.Uptime)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, d, time.Duration(0))
}
