package dirioapi

import (
	"encoding/json"
	"net/http"

	"github.com/mallardduck/dirio/consoleapi"
	"github.com/mallardduck/dirio/internal/policy"
)

// RouteHandlers defines the HTTP handler surface for the DirIO REST API (/.dirio/api/v1/).
type RouteHandlers interface {
	HandleGetBucketOwner() http.Handler
	HandleTransferBucketOwner() http.Handler
	HandleGetObjectOwner() http.Handler
	HandleSimulate() http.Handler
	HandleGetEffectivePermissions() http.Handler
}

var _ RouteHandlers = (*Handler)(nil)

// Handler implements RouteHandlers by delegating to consoleapi.API.
// When api is nil (e.g. CLI route listing) all handlers return 200 OK stubs.
type Handler struct {
	api       consoleapi.API
	adminKeys policy.AdminKeyChecker
}

// New creates a Handler. api and adminKeys must both be non-nil for production use;
// pass nil for both when registering stub routes (CLI route listing).
func New(api consoleapi.API, adminKeys policy.AdminKeyChecker) *Handler {
	return &Handler{api: api, adminKeys: adminKeys}
}

// isAdmin reports whether accessKey matches either admin credential slot.
func (h *Handler) isAdmin(accessKey string) bool {
	if h.adminKeys == nil {
		return false
	}
	pk := h.adminKeys.PrimaryRootAccessKey()
	alt := h.adminKeys.AltRootAccessKey()
	return accessKey == pk || (alt != "" && accessKey == alt)
}

// --- Error envelope ----------------------------------------------------------

// apiError is the outer wrapper for all DirIO API error responses.
type apiError struct {
	Error apiErrorDetail `json:"error"`
}

// apiErrorDetail carries the error code, human-readable message, and optional resource path.
type apiErrorDetail struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Resource string `json:"resource,omitempty"`
}

// writeError writes a JSON error envelope to w with the given HTTP status code.
func writeError(w http.ResponseWriter, status int, code, message, resource string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{
		Error: apiErrorDetail{Code: code, Message: message, Resource: resource},
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, "Unauthorized", "Request must be authenticated", "")
}

func writeAccessDenied(w http.ResponseWriter) {
	writeError(w, http.StatusForbidden, "AccessDenied", "You do not have permission to perform this operation", "")
}

func writeInternalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "InternalError", "An unexpected error occurred", "")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
