package dirioapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mallardduck/teapot-router/pkg/teapot"

	"github.com/mallardduck/dirio/consoleapi"
	dcontext "github.com/mallardduck/dirio/internal/context"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
)

// stub returns a 200 OK handler used when the real api is unavailable (CLI listing).
func stub() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// requireAuth rejects the request with 401 when the caller is anonymous.
// Returns true when the request should be aborted.
func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if dcontext.IsAnonymousRequest(r.Context()) {
		writeUnauthorized(w)
		return true
	}
	return false
}

// callerAccessKey returns the authenticated caller's access key from the request context.
// Returns an empty string when no user is present (should not happen after requireAuth).
func callerAccessKey(r *http.Request) string {
	ctx := r.Context()
	if user, err := dcontext.GetUser(ctx); err == nil && user != nil {
		return user.AccessKey
	}
	return ""
}

// mapServiceError converts common service-layer errors into DirIO API error responses.
// Returns true when the error was handled (caller should return).
func mapServiceError(w http.ResponseWriter, err error, resource string) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, svcerrors.ErrBucketNotFound):
		writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist", resource)
	case errors.Is(err, svcerrors.ErrObjectNotFound):
		writeError(w, http.StatusNotFound, "NoSuchObject", "The specified object does not exist", resource)
	case errors.Is(err, svcerrors.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "NoSuchUser", "The specified access key does not exist", resource)
	default:
		writeInternalError(w)
	}
	return true
}

// HandleGetBucketOwner handles GET /.dirio/api/v1/buckets/{bucket}/owner
func (h *Handler) HandleGetBucketOwner() http.Handler {
	if h.api == nil {
		return stub()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requireAuth(w, r) {
			return
		}
		bucket := teapot.URLParam(r, "bucket")
		owner, err := h.api.GetBucketOwner(r.Context(), bucket)
		if mapServiceError(w, err, "/"+bucket) {
			return
		}
		writeJSON(w, http.StatusOK, owner)
	})
}

// HandleTransferBucketOwner handles PUT /.dirio/api/v1/buckets/{bucket}/owner
func (h *Handler) HandleTransferBucketOwner() http.Handler {
	if h.api == nil {
		return stub()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requireAuth(w, r) {
			return
		}
		if !h.isAdmin(callerAccessKey(r)) {
			writeAccessDenied(w)
			return
		}
		bucket := teapot.URLParam(r, "bucket")

		var req struct {
			AccessKey string `json:"accessKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AccessKey == "" {
			writeError(w, http.StatusBadRequest, "InvalidRequest", "Request body must contain a non-empty accessKey field", "")
			return
		}

		if err := h.api.TransferBucketOwnership(r.Context(), bucket, req.AccessKey); err != nil {
			mapServiceError(w, err, "/"+bucket)
			return
		}

		owner, err := h.api.GetBucketOwner(r.Context(), bucket)
		if mapServiceError(w, err, "/"+bucket) {
			return
		}
		writeJSON(w, http.StatusOK, owner)
	})
}

// HandleGetObjectOwner handles GET /.dirio/api/v1/buckets/{bucket}/objects/{key:.*}
func (h *Handler) HandleGetObjectOwner() http.Handler {
	if h.api == nil {
		return stub()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requireAuth(w, r) {
			return
		}
		bucket := teapot.URLParam(r, "bucket")
		key := teapot.URLParam(r, "key")
		owner, err := h.api.GetObjectOwner(r.Context(), bucket, key)
		if mapServiceError(w, err, "/"+bucket+"/"+key) {
			return
		}
		writeJSON(w, http.StatusOK, owner)
	})
}

// HandleSimulate handles POST /.dirio/api/v1/simulate
func (h *Handler) HandleSimulate() http.Handler {
	if h.api == nil {
		return stub()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requireAuth(w, r) {
			return
		}
		var req consoleapi.SimulateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "InvalidRequest", "Could not parse request body", "")
			return
		}
		if req.AccessKey == "" || req.Bucket == "" || req.Action == "" {
			writeError(w, http.StatusBadRequest, "InvalidRequest", "accessKey, bucket, and action are required", "")
			return
		}

		// Non-admin callers may only simulate their own access key.
		caller := callerAccessKey(r)
		if !h.isAdmin(caller) && req.AccessKey != caller {
			writeAccessDenied(w)
			return
		}

		result, err := h.api.SimulateRequest(r.Context(), req)
		if mapServiceError(w, err, "/"+req.Bucket) {
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// HandleGetEffectivePermissions handles GET /.dirio/api/v1/buckets/{bucket}/permissions/{accessKey}
func (h *Handler) HandleGetEffectivePermissions() http.Handler {
	if h.api == nil {
		return stub()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requireAuth(w, r) {
			return
		}
		bucket := teapot.URLParam(r, "bucket")
		accessKey := teapot.URLParam(r, "accessKey")

		// Non-admin callers may only query their own permissions.
		caller := callerAccessKey(r)
		if !h.isAdmin(caller) && accessKey != caller {
			writeAccessDenied(w)
			return
		}

		perms, err := h.api.GetEffectivePermissions(r.Context(), accessKey, bucket)
		if mapServiceError(w, err, "/"+bucket) {
			return
		}
		writeJSON(w, http.StatusOK, perms)
	})
}
