package s3

import (
	"encoding/xml"
	"net/http"

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

// New creates a new S3 API handler
func New(storage *storage.Storage, metadata *metadata.Manager, auth *auth.Authenticator, urlBuilder URLBuilder) *Handler {
	return &Handler{
		storage:    storage,
		metadata:   metadata,
		auth:       auth,
		urlBuilder: urlBuilder,
	}
}

// ListBuckets handles GET / (list all buckets; for the root index route)
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
