package s3

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// CreateBucket handles PUT /{bucket}
func (h *HTTPHandler) CreateBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	// Validate bucket name according to S3 naming rules
	if err := ValidateS3BucketName(bucket); err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeInvalidBucketName, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error validating bucket name and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error validating bucket name")
		return
	}

	// TODO: Parse bucket configuration from request body if present

	createBucketRequest := &s3.CreateBucketRequest{
		Name: bucket,
	}
	metadata, err := h.s3Service.CreateBucket(r.Context(), createBucketRequest)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrBucketAlreadyExists) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeBucketAlreadyExists, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error creating bucket (already exists) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error creating bucket (already exists)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error creating bucket and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error creating bucket")
		return
	}

	s3Logger.With("bucket_metadata", metadata).Debug("created bucket")

	// Generate Location header per S3 spec
	location := h.urlBuilder.BucketURL(r, bucket)
	w.Header().Set("Location", location)

	w.WriteHeader(http.StatusOK)
}

// HeadBucket handles HEAD /{bucket}
func (h *HTTPHandler) HeadBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	exists, err := h.s3Service.HeadBucket(r.Context(), bucket)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error checking bucket existence and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error checking bucket existence")
		return
	}

	if !exists {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, nil); writeErr != nil {
			s3Logger.With("write_err", writeErr).Warn("encountered error writing no such bucket error response")
			return
		}
		return
	}

	// TODO: Per AWS S3 docs, we should implement:
	// 	Request side:
	// 		- `x-amz-expected-bucket-owner` - account ID of expected bucket owner. If not matching actual owner, 403.
	// It needs to be an optional header we check since it is not always sent.
	// Auth ACL things should already be blocking this, but if an admin level account tries to modify buckets with the same name owned by different users it is helpful.

	// Set bucket region header (best practice per AWS documentation)
	w.Header().Set("x-amz-bucket-region", consts.DefaultBucketLocation)
	w.WriteHeader(http.StatusOK)
}

// DeleteBucket handles DELETE /{bucket}
func (h *HTTPHandler) DeleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := h.s3Service.DeleteBucket(r.Context(), bucket); err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting bucket (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error deleting bucket (no such bucket)")
			return
		}
		if errors.Is(err, s3types.ErrBucketNotEmpty) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeBucketNotEmpty, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting bucket (not empty) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error deleting bucket (not empty)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting bucket and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error deleting bucket")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetBucketLocation handles GET /{bucket}?location
func (h *HTTPHandler) GetBucketLocation(w http.ResponseWriter, r *http.Request, bucket string) {
	region, err := h.s3Service.GetBucketLocation(r.Context(), bucket)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error checking bucket existence (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error checking bucket existence (no such bucket)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error checking bucket existence and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error checking bucket existence")
		return
	}

	if region == "" {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, nil); writeErr != nil {
			s3Logger.With("write_err", writeErr).Warn("encountered error writing no such bucket error response")
			return
		}
		return
	}

	response := s3types.LocationResponse{
		Location: region,
	}

	if writeErr := writeXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}

// ListObjects handles GET /{bucket} (ListObjectsV1)
func (h *HTTPHandler) ListObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	query := r.URL.Query()
	prefix := query.Get("prefix")
	delimiter := query.Get("delimiter")
	marker := query.Get("marker")
	maxKeys := parseMaxKeys(query.Get("max-keys"))

	listRequest := &s3.ListObjectsRequest{
		Bucket:    bucket,
		Prefix:    prefix,
		Delimiter: delimiter,
		// Marker: marker,
		MaxKeys: maxKeys,
	}
	objects, err := h.s3Service.ListObjects(r.Context(), listRequest)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error listing objects (no such bucket)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
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
		MaxKeys:     maxKeys,
		IsTruncated: false,
		Contents:    objects,
	}

	if writeErr := writeXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}

// ListObjectsV2 handles GET /{bucket}?list-type=2
func (h *HTTPHandler) ListObjectsV2(w http.ResponseWriter, r *http.Request, bucket string) {
	query := r.URL.Query()
	continuationToken := query.Get("continuation-token")
	delimiter := query.Get("delimiter")
	// encoding-type
	fetchOwner := query.Get("fetch-owner") == "true"
	maxKeys := parseMaxKeys(query.Get("max-keys"))
	prefix := query.Get("prefix")
	startAfter := query.Get("start-after")

	listRequest := &s3.ListObjectsV2Request{
		Bucket:            bucket,
		Prefix:            prefix,
		ContinuationToken: continuationToken,
		StartAfter:        startAfter,
		Delimiter:         delimiter,
		MaxKeys:           maxKeys,
		FetchOwner:        fetchOwner,
	}
	objects, err := h.s3Service.ListObjectsV2(r.Context(), listRequest)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects v2 (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error listing objects v2 (no such bucket)")
			return
		}
		if writeErr := writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects v2 and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error listing objects v2")
		return
	}

	// Per S3 spec: KeyCount is the number of keys returned, including both objects and common prefixes
	// "each common prefix counts as a single return when calculating the number of returns"
	keyCount := len(objects.Objects) + len(objects.CommonPrefixes)

	response := s3types.ListBucketV2Result{
		Name:              bucket,
		Prefix:            prefix,
		Delimiter:         delimiter,
		MaxKeys:           maxKeys,
		KeyCount:          keyCount,
		IsTruncated:       objects.IsTruncated,
		ContinuationToken: continuationToken,
		Contents:          objects.Objects,
		CommonPrefixes:    objects.CommonPrefixes,
	}

	if writeErr := writeXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}

// parseMaxKeys parses the max-keys query parameter with S3-compatible defaults and limits
func parseMaxKeys(maxKeysStr string) int {
	const (
		defaultMaxKeys = 1000
		maxMaxKeys     = 1000
	)

	// Default to 1000 if not provided
	if maxKeysStr == "" {
		return defaultMaxKeys
	}

	// Parse the value
	maxKeys, err := strconv.Atoi(maxKeysStr)
	if err != nil {
		// Invalid value, use default
		return defaultMaxKeys
	}

	// Enforce S3 limits: minimum 1, maximum 1000
	if maxKeys < 1 {
		return 1
	}
	if maxKeys > maxMaxKeys {
		return maxMaxKeys
	}

	return maxKeys
}

func (h *HTTPHandler) DeleteObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	// TODO implemenet basic multiple object delete, will need to add service funcs
	// something like: h.s3Service.DeleteObjects
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"status":"error","error":"This operation is not yet implemented"}`))
}
