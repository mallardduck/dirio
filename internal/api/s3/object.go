package s3

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// GetObject handles GET /{bucket}/{key}
func (h *Handler) GetObject(w http.ResponseWriter, r *http.Request, bucket, key, requestID string) {
	// Validate key according to S3 naming rules
	if err := ValidateS3Key(key); err != nil {
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInvalidObjectKey, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error validating object key and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error validating object key")
		return
	}

	obj, err := h.storage.GetObject(bucket, key)
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchKey) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchKey, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object (no such key) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error getting object (no such key)")
			return
		}
		if errors.Is(err, storage.ErrNoSuchBucket) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error getting object (no such bucket)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error getting object")
		return
	}
	defer obj.Content.Close()

	// Set response headers
	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Last-Modified", obj.LastModified.Format(http.TimeFormat))
	w.Header().Set("Accept-Ranges", "bytes")

	// Set custom metadata headers
	for key, value := range obj.CustomMetadata {
		w.Header().Set(key, value)
	}

	// Copy object content to response
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, obj.Content); err != nil {
		// Can't send error response after headers written, but log for debugging
		log := logging.ComponentWithContext(r.Context(), "api")
		log.Warn("failed to write object content", "bucket", bucket, "key", key, "error", err)
	}
}

// PutObject handles PUT /{bucket}/{key}
func (h *Handler) PutObject(w http.ResponseWriter, r *http.Request, bucket, key, requestID string) {
	// Validate key according to S3 naming rules
	if err := ValidateS3Key(key); err != nil {
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInvalidObjectKey, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error validating object key and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error validating object key")
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Extract custom metadata from request headers
	customMetadata := make(map[string]string)

	// Extract S3-standard metadata headers
	metadataHeaders := []string{
		"Cache-Control",
		"Content-Disposition",
		"Content-Encoding",
		"Content-Language",
		"Expires",
	}
	for _, header := range metadataHeaders {
		if value := r.Header.Get(header); value != "" {
			customMetadata[header] = value
		}
	}

	// Extract user-defined metadata (x-amz-meta-*)
	for key, values := range r.Header {
		if strings.HasPrefix(strings.ToLower(key), "x-amz-meta-") && len(values) > 0 {
			customMetadata[key] = values[0]
		}
	}

	// Read object content from request body
	etag, err := h.storage.PutObject(bucket, key, r.Body, contentType, customMetadata)
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchBucket) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error putting object (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error putting object (no such bucket)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error putting object and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error putting object")
		return
	}

	// Set response headers
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
}

// HeadObject handles HEAD /{bucket}/{key}
func (h *Handler) HeadObject(w http.ResponseWriter, r *http.Request, bucket, key, requestID string) {
	// Validate key according to S3 naming rules
	if err := ValidateS3Key(key); err != nil {
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInvalidObjectKey, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error validating object key and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error validating object key")
		return
	}

	meta, err := h.storage.GetObjectMetadata(bucket, key)
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchKey) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchKey, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object metadata (no such key) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error getting object metadata (no such key)")
			return
		}
		if errors.Is(err, storage.ErrNoSuchBucket) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object metadata (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error getting object metadata (no such bucket)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object metadata and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error getting object metadata")
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	w.Header().Set("ETag", meta.ETag)
	w.Header().Set("Last-Modified", meta.LastModified.Format(http.TimeFormat))
	w.Header().Set("Accept-Ranges", "bytes")

	// Set custom metadata headers
	for key, value := range meta.CustomMetadata {
		w.Header().Set(key, value)
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteObject handles DELETE /{bucket}/{key}
func (h *Handler) DeleteObject(w http.ResponseWriter, r *http.Request, bucket, key, requestID string) {
	// Validate key according to S3 naming rules
	if err := ValidateS3Key(key); err != nil {
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInvalidObjectKey, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error validating object key and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error validating object key")
		return
	}

	err := h.storage.DeleteObject(bucket, key)
	if err != nil {
		// S3 returns 204 even if object doesn't exist
		if !errors.Is(err, storage.ErrNoSuchKey) {
			if errors.Is(err, storage.ErrNoSuchBucket) {
				if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err); writeErr != nil {
					s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting object (no such bucket) and additional error writing XML error response")
					return
				}
				s3Logger.With("err", err).Warn("encountered error deleting object (no such bucket)")
				return
			}
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting object and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error deleting object")
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
