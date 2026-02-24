package middleware

import (
	"net/http"
	"strings"

	"github.com/mallardduck/dirio/internal/global"
)

// isMinioClient reports whether the request originates from a MinIO SDK or
// tool. MinIO clients (mc, madmin-go, minio-go) always include "minio" or
// "madmin" in their User-Agent string.
func isMinioClient(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	return ua != "" && (strings.Contains(ua, "minio") || strings.Contains(ua, "madmin"))
}

// CompatHeaders sets MinIO-specific response headers only when the request
// originates from a MinIO SDK client (identified by User-Agent). This keeps
// MinIO wire-compatibility headers out of responses sent to native S3 clients,
// browsers, and other non-MinIO tools.
func CompatHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isMinioClient(r) {
			w.Header().Set("X-Minio-Deployment-Id", global.GlobalInstanceID().String())
		}
		next.ServeHTTP(w, r)
	})
}
