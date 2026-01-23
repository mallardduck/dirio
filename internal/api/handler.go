package api

import (
	"encoding/xml"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/mallardduck/dirio/internal/auth"
	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/middleware"
	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// Handler handles S3 API requests
type Handler struct {
	storage    *storage.Storage
	metadata   *metadata.Manager
	auth       *auth.Authenticator
	urlBuilder URLBuilder
}

// URLBuilder defines the interface for generating URLs in S3 API responses
type URLBuilder interface {
	BucketURL(r *http.Request, bucket string) string
	ObjectURL(r *http.Request, bucket, key string) string
}

// New creates a new API handler
func New(storage *storage.Storage, metadata *metadata.Manager, auth *auth.Authenticator, urlBuilder URLBuilder) *Handler {
	return &Handler{
		storage:    storage,
		metadata:   metadata,
		auth:       auth,
		urlBuilder: urlBuilder,
	}
}

// ListBuckets handles GET / (list all buckets)
func (h *Handler) ListBuckets(w http.ResponseWriter, r *http.Request) {
	if data, ok := loggingHttp.GetLogData(r.Context()); ok {
		data.Action = "ListBuckets"
	}

	requestID := middleware.GetRequestID(r.Context())

	buckets, err := h.storage.ListBuckets()
	if err != nil {
		writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
		return
	}

	response := s3types.ListBucketsResponse{
		Buckets: buckets,
		Owner: s3types.Owner{
			ID:          "root",
			DisplayName: "root",
		},
	}

	writeXMLResponse(w, http.StatusOK, response)
}

// BucketHandler routes bucket operations based on query params and method
func (h *Handler) BucketHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := middleware.GetRequestID(ctx)
	vars := mux.Vars(r)
	bucket := vars["bucket"]

	// Check query parameters to determine operation
	query := r.URL.Query()

	// Handle special query operations
	if _, ok := query["location"]; ok {
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "GetBucketLocation"
		}
		h.GetBucketLocation(w, r, bucket, requestID)
		return
	}

	if query.Get("list-type") == "2" {
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "ListObjectsV2"
		}
		h.ListObjectsV2(w, r, bucket, requestID)
		return
	}

	// Handle standard bucket operations
	switch r.Method {
	case "GET":
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "ListObjects"
		}
		h.ListObjects(w, r, bucket, requestID)
	case "PUT":
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "CreateBucket"
		}
		h.CreateBucket(w, r, bucket, requestID)
	case "HEAD":
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "HeadBucket"
		}
		h.HeadBucket(w, r, bucket, requestID)
	case "DELETE":
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "DeleteBucket"
		}
		h.DeleteBucket(w, r, bucket, requestID)
	default:
		writeErrorResponse(w, requestID, s3types.ErrMethodNotAllowed, nil)
	}
}

// ObjectHandler routes object operations based on method
func (h *Handler) ObjectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := middleware.GetRequestID(ctx)
	vars := mux.Vars(r)
	bucket := vars["bucket"]
	key := vars["key"]

	switch r.Method {
	case "GET":
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "GetObject"
		}
		h.GetObject(w, r, bucket, key, requestID)
	case "PUT":
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "PutObject"
		}
		h.PutObject(w, r, bucket, key, requestID)
	case "HEAD":
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "HeadObject"
		}
		h.HeadObject(w, r, bucket, key, requestID)
	case "DELETE":
		if data, ok := loggingHttp.GetLogData(ctx); ok {
			data.Action = "DeleteObject"
		}
		h.DeleteObject(w, r, bucket, key, requestID)
	default:
		writeErrorResponse(w, requestID, s3types.ErrMethodNotAllowed, nil)
	}
}

// Helper functions

func writeXMLResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)

	w.Write([]byte(xml.Header))
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	encoder.Encode(data)
}

func writeErrorResponse(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode, err error) {
	w.Header().Set("Content-Type", "application/xml")

	statusCode := errCode.HTTPStatus()
	w.WriteHeader(statusCode)

	errMsg := errCode.Description()
	if err != nil {
		errMsg = err.Error()
	}

	response := s3types.ErrorResponse{
		Code:      errCode.String(),
		Message:   errMsg,
		RequestID: requestID,
	}

	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(response)
}

func getBucketAndKey(r *http.Request) (bucket, key string) {
	vars := mux.Vars(r)
	bucket = vars["bucket"]
	key = vars["key"]
	// Remove leading slash if present
	key = strings.TrimPrefix(key, "/")
	return
}
