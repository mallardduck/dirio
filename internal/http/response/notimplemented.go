package response

import (
	"net/http"

	"github.com/mallardduck/go-http-helpers/pkg/headers"
)

// NotImplemented is a placeholder handler for routes that are registered
// but not yet implemented. It returns a 501 JSON response.
var NotImplemented http.HandlerFunc = func(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set(headers.ContentType, "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte(`{"status":"error","error":"This operation is not yet implemented"}`))
}
