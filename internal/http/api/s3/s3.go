package s3

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/mallardduck/dirio/internal/http/middleware"
	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
	"github.com/mallardduck/dirio/internal/service"
	svcs3 "github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// HTTPHandler handles S3 API requests
type HTTPHandler struct {
	s3Service  *svcs3.Service
	urlBuilder URLBuilder
}

// URLBuilder defines the interface for generating URLs in S3 API responses
type URLBuilder interface {
	BucketURL(r *http.Request, bucket string) string
	ObjectURL(r *http.Request, bucket, key string) string
}

// New creates a new S3 API handler
func New(serviceFactory *service.ServicesFactory, urlBuilder URLBuilder) *HTTPHandler {
	return &HTTPHandler{
		s3Service:  serviceFactory.S3(),
		urlBuilder: urlBuilder,
	}
}

// ListBuckets handles GET / (list all buckets; for the root index route)
func (h *HTTPHandler) ListBuckets(w http.ResponseWriter, r *http.Request) {
	if data, ok := loggingHttp.GetLogData(r.Context()); ok {
		data.Action = "ListBuckets"
	}

	buckets, err := h.s3Service.ListBuckets(r.Context())
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
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
	defer func() { _ = encoder.Flush() }() // Best effort cleanup on error paths

	if err := encoder.Encode(data); err != nil {
		return err
	}

	if err := encoder.Flush(); err != nil {
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

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(errCode.HTTPStatus())

	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return err
	}

	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	defer func() { _ = encoder.Flush() }() // Best effort cleanup on error paths

	if err := encoder.Encode(response); err != nil {
		// Note: At this point, headers are sent, so we can't change the status code.
		// We just log or return the error.
		return fmt.Errorf("failed to encode error response: %w", err)
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	return encoder.Flush() // Return flush error on success path
}
