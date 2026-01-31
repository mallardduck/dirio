package s3

import (
	"bytes"
	"encoding/xml"
	"fmt"
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

	buckets, err := h.storage.ListBuckets(r.Context())
	if err != nil {
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing buckets and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error listing buckets")
		return
	}

	response := s3types.ListBucketsResponse{
		Buckets: buckets,
		Owner: s3types.Owner{
			ID:          "root",
			DisplayName: "root",
		},
	}

	if writeErr := writeXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}

func writeXMLResponse(w http.ResponseWriter, statusCode int, data interface{}) error {
	var buf bytes.Buffer
	buf.Write([]byte(xml.Header))

	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return err
	}

	// Optional: warn if response is unexpectedly large
	if buf.Len() > 10*1024*1024 { // 10 MB
		s3Logger.With("length", buf.Len()).Warn("Large XML response")
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)
	_, err := w.Write(buf.Bytes())
	return err
}

func writeErrorResponse(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode, err error) error {
	errMsg := errCode.Description()
	if err != nil {
		errMsg = err.Error()
	}

	response := s3types.ErrorResponse{
		Code:      errCode.String(),
		Message:   errMsg,
		RequestID: requestID,
	}

	var buf bytes.Buffer
	buf.Write([]byte(xml.Header))

	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encoder.Encode(response); err != nil {
		return fmt.Errorf("failed to encode error response: %w", err)
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(errCode.HTTPStatus())
	_, err = w.Write(buf.Bytes())
	return err
}
