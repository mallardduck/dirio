package logging

import (
	"fmt"
	"net/http"

	"github.com/mallardduck/dirio/internal/logging"
)

// RecoveryMiddleware is a middleware that recovers from any panics and returns a consistent error response.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic
				logging.Default().Error(fmt.Sprintf("Panic recovered: %v", err))

				// Write a generic error response
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error": "Internal server error"}`))
			}
		}()

		next.ServeHTTP(w, r)
	})
}
