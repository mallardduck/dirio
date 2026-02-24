package middleware

import (
	"net/http"

	"github.com/mallardduck/dirio/internal/global"
)

// SetDefaultHeadersMiddleware is a middleware that sets a specific header
func SetDefaultHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set the default header. Use w.Header().Set() to ensure a single value.
		w.Header().Set("Server", "DirIO-Server")
		w.Header().Set("X-Dirio-Instance-Id", global.GlobalInstanceID().String())
		w.Header().Set("X-Minio-Deployment-Id", global.GlobalInstanceID().String())

		// Call the next handler in the chain
		next.ServeHTTP(w, r)
	})
}
