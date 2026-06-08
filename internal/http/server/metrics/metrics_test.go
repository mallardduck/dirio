package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_NilTelemetry(t *testing.T) {
	h := New(nil)
	assert.Nil(t, h.telemetry)
}

func TestHandlePrometheus_NilProvider(t *testing.T) {
	h := New(nil)
	handler := h.HandlePrometheus()
	require.NotNil(t, handler)

	// With nil telemetry the handler is the teapot NoopHandler: returns 200.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.dirio/metrics", http.NoBody))
	assert.Equal(t, http.StatusOK, rec.Code)
}
