package debug

import (
	"net/http"

	"github.com/mallardduck/teapot-router/pkg/teapot"

	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
)

// HandleRoutes returns a handler that lists all registered routes as JSON
func HandleRoutes(r *teapot.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "ListRoutes"
		}

		routes := r.Routes()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Use library's JSON formatter
		err := teapot.FormatRoutesJSON(w, routes)
		if err != nil {
			// TODO log error?
			return
		}
	}
}
