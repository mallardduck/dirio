package api

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/mallardduck/dirio/internal/logging"
	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
	"github.com/mallardduck/dirio/internal/middleware"
	"github.com/mallardduck/dirio/internal/router"
	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// GetObject handles GET /{bucket}/{key}
func (h *Handler) GetObject(w http.ResponseWriter, r *http.Request, bucket, key, requestID string) {
	// Validate key according to S3 naming rules
	if err := ValidateS3Key(key); err != nil {
		writeErrorResponse(w, requestID, s3types.ErrInvalidObjectKey, err)
		return
	}

	obj, err := h.storage.GetObject(bucket, key)
	if err != nil {
		if err == storage.ErrNoSuchKey {
			writeErrorResponse(w, requestID, s3types.ErrNoSuchKey, err)
			return
		}
		if err == storage.ErrNoSuchBucket {
			writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err)
			return
		}
		writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
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
		writeErrorResponse(w, requestID, s3types.ErrInvalidObjectKey, err)
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
		if err == storage.ErrNoSuchBucket {
			writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err)
			return
		}
		writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
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
		writeErrorResponse(w, requestID, s3types.ErrInvalidObjectKey, err)
		return
	}

	meta, err := h.storage.GetObjectMetadata(bucket, key)
	if err != nil {
		if err == storage.ErrNoSuchKey {
			writeErrorResponse(w, requestID, s3types.ErrNoSuchKey, err)
			return
		}
		if err == storage.ErrNoSuchBucket {
			writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err)
			return
		}
		writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
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
		writeErrorResponse(w, requestID, s3types.ErrInvalidObjectKey, err)
		return
	}

	err := h.storage.DeleteObject(bucket, key)
	if err != nil {
		// S3 returns 204 even if object doesn't exist
		if err != storage.ErrNoSuchKey {
			if err == storage.ErrNoSuchBucket {
				writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err)
				return
			}
			writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// ObjectResourceHandler routes object operations based on method
func (h *Handler) ObjectResourceHandler() routeHandler {
	return routeHandler{
		HeadHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")
			// Chi uses "*" for catch-all wildcard parameter
			key := router.URLParam(r, "*")

			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "HeadObject"
			}
			h.HeadObject(w, r, bucket, key, requestID)
		}, // HeadObject
		StoreHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")
			// Chi uses "*" for catch-all wildcard parameter
			key := router.URLParam(r, "*")

			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "PutObject"
			}
			h.PutObject(w, r, bucket, key, requestID)
		}, // PutObject, TODO add CopyObject (x-amz-copy-source)
		ShowHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")
			// Chi uses "*" for catch-all wildcard parameter
			key := router.URLParam(r, "*")

			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "GetObject"
			}
			h.GetObject(w, r, bucket, key, requestID)
		}, // GetObject
		DestroyHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")
			// Chi uses "*" for catch-all wildcard parameter
			key := router.URLParam(r, "*")
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "DeleteObject"
			}
			h.DeleteObject(w, r, bucket, key, requestID)
		}, // DeleteObject
	}
}
