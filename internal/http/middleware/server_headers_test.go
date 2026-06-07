package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/mallardduck/dirio/internal/global"
	"github.com/stretchr/testify/assert"
)

func TestSetDefaultHeadersMiddleware(t *testing.T) {
	id := uuid.MustParse("aaaabbbb-cccc-dddd-eeee-ffffffffffff")
	global.SetGlobalInstanceID(id)

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()

	var nextCalled bool
	SetDefaultHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})).ServeHTTP(rec, req)

	assert.True(t, nextCalled)
	assert.Equal(t, "DirIO-Server", rec.Header().Get("Server"))
	assert.Equal(t, id.String(), rec.Header().Get("X-Dirio-Instance-Id"))
}
