package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/mallardduck/dirio/internal/global"
)

func TestIsMinioClient(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		want      bool
	}{
		{"minio-go SDK", "MinIO (linux; amd64) minio-go/7.0.0", true},
		{"mc tool", "mc/RELEASE.2023-10-14 (linux; amd64)", false}, // "mc" alone does not contain "minio"
		{"madmin-go", "madmin-go/6.0.0", true},
		{"uppercase MINIO", "MINIO-tool/1.0", true},
		{"aws SDK", "aws-sdk-go/1.44.0 (go1.20; linux; amd64)", false},
		{"empty User-Agent", "", false},
		{"browser", "Mozilla/5.0 (Windows NT 10.0)", false},
		{"minio in path", "custom/minio-fork/1.0", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			if tc.userAgent != "" {
				req.Header.Set("User-Agent", tc.userAgent)
			}
			assert.Equal(t, tc.want, isMinioClient(req))
		})
	}
}

func TestCompatHeaders_MinioClient(t *testing.T) {
	id := uuid.MustParse("12345678-1234-1234-1234-123456789abc")
	global.SetGlobalInstanceID(id)

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("User-Agent", "minio-go/7.0.0")
	rec := httptest.NewRecorder()

	var nextCalled bool
	CompatHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})).ServeHTTP(rec, req)

	assert.True(t, nextCalled)
	assert.Equal(t, id.String(), rec.Header().Get("X-Minio-Deployment-Id"))
}

func TestCompatHeaders_NonMinioClient(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("User-Agent", "aws-sdk-go/1.44.0")
	rec := httptest.NewRecorder()

	CompatHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("X-Minio-Deployment-Id"), "non-minio clients should not receive deployment-id header")
}

func TestCompatHeaders_NoUserAgent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()

	CompatHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("X-Minio-Deployment-Id"))
}
