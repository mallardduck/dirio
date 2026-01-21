package api

import (
	"net/http"

	"github.com/yourusername/dirio/pkg/s3types"
)

// CreateBucket handles PUT /{bucket}
func (h *Handler) CreateBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	// TODO: Parse bucket configuration from request body if present
	
	if err := h.storage.CreateBucket(bucket); err != nil {
		if err == storage.ErrBucketExists {
			writeErrorResponse(w, s3types.ErrBucketAlreadyExists, err)
			return
		}
		writeErrorResponse(w, s3types.ErrInternalError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HeadBucket handles HEAD /{bucket}
func (h *Handler) HeadBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	exists, err := h.storage.BucketExists(bucket)
	if err != nil {
		writeErrorResponse(w, s3types.ErrInternalError, err)
		return
	}

	if !exists {
		writeErrorResponse(w, s3types.ErrNoSuchBucket, nil)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteBucket handles DELETE /{bucket}
func (h *Handler) DeleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := h.storage.DeleteBucket(bucket); err != nil {
		if err == storage.ErrNoSuchBucket {
			writeErrorResponse(w, s3types.ErrNoSuchBucket, err)
			return
		}
		if err == storage.ErrBucketNotEmpty {
			writeErrorResponse(w, s3types.ErrBucketNotEmpty, err)
			return
		}
		writeErrorResponse(w, s3types.ErrInternalError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetBucketLocation handles GET /{bucket}?location
func (h *Handler) GetBucketLocation(w http.ResponseWriter, r *http.Request, bucket string) {
	exists, err := h.storage.BucketExists(bucket)
	if err != nil {
		writeErrorResponse(w, s3types.ErrInternalError, err)
		return
	}

	if !exists {
		writeErrorResponse(w, s3types.ErrNoSuchBucket, nil)
		return
	}

	response := s3types.LocationResponse{
		Location: "us-east-1", // Hardcoded as discussed
	}

	writeXMLResponse(w, http.StatusOK, response)
}

// ListObjects handles GET /{bucket} (ListObjectsV1)
func (h *Handler) ListObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	query := r.URL.Query()
	prefix := query.Get("prefix")
	delimiter := query.Get("delimiter")
	marker := query.Get("marker")
	maxKeys := query.Get("max-keys")

	// TODO: Implement ListObjectsV1
	// For now, return empty result
	response := s3types.ListBucketResult{
		Name:        bucket,
		Prefix:      prefix,
		Delimiter:   delimiter,
		Marker:      marker,
		MaxKeys:     1000,
		IsTruncated: false,
		Contents:    []s3types.Object{},
	}

	writeXMLResponse(w, http.StatusOK, response)
}

// ListObjectsV2 handles GET /{bucket}?list-type=2
func (h *Handler) ListObjectsV2(w http.ResponseWriter, r *http.Request, bucket string) {
	query := r.URL.Query()
	prefix := query.Get("prefix")
	delimiter := query.Get("delimiter")
	continuationToken := query.Get("continuation-token")

	objects, err := h.storage.ListObjects(bucket, prefix, delimiter, 1000)
	if err != nil {
		if err == storage.ErrNoSuchBucket {
			writeErrorResponse(w, s3types.ErrNoSuchBucket, err)
			return
		}
		writeErrorResponse(w, s3types.ErrInternalError, err)
		return
	}

	response := s3types.ListBucketV2Result{
		Name:              bucket,
		Prefix:            prefix,
		Delimiter:         delimiter,
		MaxKeys:           1000,
		KeyCount:          len(objects),
		IsTruncated:       false,
		ContinuationToken: continuationToken,
		Contents:          objects,
	}

	writeXMLResponse(w, http.StatusOK, response)
}
