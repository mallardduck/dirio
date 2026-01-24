package api

import (
	"net/http"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// CreateBucket handles PUT /{bucket}
func (h *Handler) CreateBucket(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	// Validate bucket name according to S3 naming rules
	if err := ValidateS3BucketName(bucket); err != nil {
		writeErrorResponse(w, requestID, s3types.ErrInvalidBucketName, err)
		return
	}

	// TODO: Parse bucket configuration from request body if present

	if err := h.storage.CreateBucket(bucket); err != nil {
		if err == storage.ErrBucketExists {
			writeErrorResponse(w, requestID, s3types.ErrBucketAlreadyExists, err)
			return
		}
		writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
		return
	}

	// Generate Location header per S3 spec
	location := h.urlBuilder.BucketURL(r, bucket)
	w.Header().Set("Location", location)

	w.WriteHeader(http.StatusOK)
}

// HeadBucket handles HEAD /{bucket}
func (h *Handler) HeadBucket(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	exists, err := h.storage.BucketExists(bucket)
	if err != nil {
		writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
		return
	}

	if !exists {
		writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, nil)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteBucket handles DELETE /{bucket}
func (h *Handler) DeleteBucket(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	if err := h.storage.DeleteBucket(bucket); err != nil {
		if err == storage.ErrNoSuchBucket {
			writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err)
			return
		}
		if err == storage.ErrBucketNotEmpty {
			writeErrorResponse(w, requestID, s3types.ErrBucketNotEmpty, err)
			return
		}
		writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetBucketLocation handles GET /{bucket}?location
func (h *Handler) GetBucketLocation(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	exists, err := h.storage.BucketExists(bucket)
	if err != nil {
		writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
		return
	}

	if !exists {
		writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, nil)
		return
	}

	response := s3types.LocationResponse{
		Location: consts.DefaultBucketLocation, // Hardcoded as discussed
	}

	writeXMLResponse(w, http.StatusOK, response)
}

// ListObjects handles GET /{bucket} (ListObjectsV1)
func (h *Handler) ListObjects(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	query := r.URL.Query()
	prefix := query.Get("prefix")
	delimiter := query.Get("delimiter")
	marker := query.Get("marker")
	_ = query.Get("max-keys") // TODO: use for pagination

	objects, err := h.storage.ListObjects(bucket, prefix, delimiter, 1000)
	if err != nil {
		if err == storage.ErrNoSuchBucket {
			writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err)
			return
		}
		writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
		return
	}

	response := s3types.ListBucketResult{
		Name:        bucket,
		Prefix:      prefix,
		Delimiter:   delimiter,
		Marker:      marker,
		MaxKeys:     1000,
		IsTruncated: false,
		Contents:    objects,
	}

	writeXMLResponse(w, http.StatusOK, response)
}

// ListObjectsV2 handles GET /{bucket}?list-type=2
func (h *Handler) ListObjectsV2(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	query := r.URL.Query()
	prefix := query.Get("prefix")
	delimiter := query.Get("delimiter")
	continuationToken := query.Get("continuation-token")

	objects, err := h.storage.ListObjects(bucket, prefix, delimiter, 1000)
	if err != nil {
		if err == storage.ErrNoSuchBucket {
			writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err)
			return
		}
		writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
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
