package api

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// GetObject handles GET /{bucket}/{key}
func (h *Handler) GetObject(w http.ResponseWriter, r *http.Request, bucket, key, requestID string) {
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

	// Copy object content to response
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, obj.Content); err != nil {
		// Can't send error response after headers written, but log for debugging
		slog.Warn("failed to write object content", "bucket", bucket, "key", key, "error", err)
	}
}

// PutObject handles PUT /{bucket}/{key}
func (h *Handler) PutObject(w http.ResponseWriter, r *http.Request, bucket, key, requestID string) {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Read object content from request body
	etag, err := h.storage.PutObject(bucket, key, r.Body, contentType)
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

	w.WriteHeader(http.StatusOK)
}

// DeleteObject handles DELETE /{bucket}/{key}
func (h *Handler) DeleteObject(w http.ResponseWriter, r *http.Request, bucket, key, requestID string) {
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
