package s3

import (
	"errors"
	"net/http"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// CreateBucket handles PUT /{bucket}
func (h *Handler) CreateBucket(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	// Validate bucket name according to S3 naming rules
	if err := ValidateS3BucketName(bucket); err != nil {
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInvalidBucketName, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error validating bucket name and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error validating bucket name")
		return
	}

	// TODO: Parse bucket configuration from request body if present

	if err := h.storage.CreateBucket(bucket); err != nil {
		if errors.Is(err, storage.ErrBucketExists) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrBucketAlreadyExists, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error creating bucket (already exists) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error creating bucket (already exists)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error creating bucket and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error creating bucket")
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
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error checking bucket existence and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error checking bucket existence")
		return
	}

	if !exists {
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, nil); writeErr != nil {
			s3Logger.With("write_err", writeErr).Warn("encountered error writing no such bucket error response")
			return
		}
		return
	}

	// TODO: Per AWS S3 docs, we should implement:
	// 	Request side:
	// 		- `x-amz-expected-bucket-owner` - account ID of expected bucket owner. If not matching actual owner, 403.
	// It needs to be an optional header we check since it is not always sent.
	// Auth ACL things should already be blocking this, but if an admin level account tries to modify buckets with the same name owned by diffrent users it is helpful.

	// Set bucket region header (best practice per AWS documentation)
	w.Header().Set("x-amz-bucket-region", consts.DefaultBucketLocation)
	w.WriteHeader(http.StatusOK)
}

// DeleteBucket handles DELETE /{bucket}
func (h *Handler) DeleteBucket(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	if err := h.storage.DeleteBucket(bucket); err != nil {
		if errors.Is(err, storage.ErrNoSuchBucket) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting bucket (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error deleting bucket (no such bucket)")
			return
		}
		if errors.Is(err, storage.ErrBucketNotEmpty) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrBucketNotEmpty, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting bucket (not empty) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error deleting bucket (not empty)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting bucket and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error deleting bucket")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetBucketLocation handles GET /{bucket}?location
func (h *Handler) GetBucketLocation(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	exists, err := h.storage.BucketExists(bucket)
	if err != nil {
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error checking bucket existence and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error checking bucket existence")
		return
	}

	if !exists {
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, nil); writeErr != nil {
			s3Logger.With("write_err", writeErr).Warn("encountered error writing no such bucket error response")
			return
		}
		return
	}

	response := s3types.LocationResponse{
		Location: consts.DefaultBucketLocation, // Hardcoded as discussed
	}

	if writeErr := writeXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
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
		if errors.Is(err, storage.ErrNoSuchBucket) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error listing objects (no such bucket)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error listing objects")
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

	if writeErr := writeXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}

// ListObjectsV2 handles GET /{bucket}?list-type=2
func (h *Handler) ListObjectsV2(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	query := r.URL.Query()
	prefix := query.Get("prefix")
	delimiter := query.Get("delimiter")
	continuationToken := query.Get("continuation-token")

	objects, err := h.storage.ListObjects(bucket, prefix, delimiter, 1000)
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchBucket) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects v2 (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error listing objects v2 (no such bucket)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects v2 and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error listing objects v2")
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

	if writeErr := writeXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}
