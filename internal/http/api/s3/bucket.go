package s3

import (
	"encoding/xml"
	"errors"
	"io"
	"net/http"

	"github.com/mallardduck/go-http-helpers/pkg/headers"
	"github.com/mallardduck/go-http-helpers/pkg/query"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// CreateBucket handles PUT /{bucket}
func (h *HTTPHandler) CreateBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	// TODO: Parse bucket configuration from request body if present

	createBucketRequest := &s3.CreateBucketRequest{
		Name: bucket,
	}
	metadata, err := h.s3Service.CreateBucket(r.Context(), createBucketRequest)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrBucketAlreadyExists) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeBucketAlreadyExists, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error creating bucket (already exists) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error creating bucket (already exists)")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error creating bucket and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error creating bucket")
		return
	}

	s3Logger.With("bucket_metadata", metadata).Debug("created bucket")

	// Generate Location header per S3 spec
	location := h.urlBuilder.BucketURL(r, bucket)
	w.Header().Set(headers.Location, location)

	w.WriteHeader(http.StatusOK)
}

// HeadBucket handles HEAD /{bucket}
func (h *HTTPHandler) HeadBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	exists, err := h.s3Service.HeadBucket(r.Context(), bucket)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error checking bucket existence and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error checking bucket existence")
		return
	}

	if !exists {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, nil); writeErr != nil {
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
	w.Header().Set(consts.HeaderBucketRegion, consts.DefaultBucketLocation)
	w.WriteHeader(http.StatusOK)
}

// DeleteBucket handles DELETE /{bucket}
// TODO(Phase 3.2 #2): Fix 405 Method Not Allowed for MinIO mc client
//   - Currently returns 405 for MinIO mc, but policy engine is ready (s3:DeleteBucket)
//   - Likely a routing issue, not authorization - check teapot-router DELETE route registration
//   - Test: mc rb alias/bucket
//   - Verify route matches in routes.go line ~293: r.DELETE("/{bucket}", ...)
func (h *HTTPHandler) DeleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := h.s3Service.DeleteBucket(r.Context(), bucket); err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting bucket (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error deleting bucket (no such bucket)")
			return
		}
		if errors.Is(err, s3types.ErrBucketNotEmpty) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeBucketNotEmpty, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting bucket (not empty) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error deleting bucket (not empty)")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
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
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error checking bucket existence (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error checking bucket existence (no such bucket)")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error checking bucket existence and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error checking bucket existence")
		return
	}

	if region == "" {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, nil); writeErr != nil {
			s3Logger.With("write_err", writeErr).Warn("encountered error writing no such bucket error response")
			return
		}
		return
	}

	response := s3types.LocationResponse{
		Location: region,
	}

	if writeErr := WriteXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}

// ListObjects handles GET /{bucket} (ListObjectsV1)
func (h *HTTPHandler) ListObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	prefix := query.String(r, "prefix", "")
	delimiter := query.String(r, "delimiter", "")
	marker := query.String(r, "marker", "")
	maxKeys := parseMaxKeys(query.Int(r, "max-keys", defaultMaxKeys))

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
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error listing objects (no such bucket)")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error listing objects")
		return
	}

	// Filter objects based on permissions
	filteredObjects, err := h.filterObjects(r.Context(), bucket, objects, r)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error filtering objects and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error filtering objects")
		return
	}

	response := s3types.ListBucketResult{
		Name:        bucket,
		Prefix:      prefix,
		Delimiter:   delimiter,
		Marker:      marker,
		MaxKeys:     maxKeys,
		IsTruncated: false,
		Contents:    filteredObjects,
	}

	if writeErr := WriteXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}

// ListObjectsV2 handles GET /{bucket}?list-type=2
func (h *HTTPHandler) ListObjectsV2(w http.ResponseWriter, r *http.Request, bucket string) {
	continuationToken := query.String(r, "continuation-token", "")
	delimiter := query.String(r, "delimiter", "")
	// encoding-type
	fetchOwner := query.Bool(r, "fetch-owner", false)
	maxKeys := parseMaxKeys(query.Int(r, "max-keys", defaultMaxKeys))
	prefix := query.String(r, "prefix", "")
	startAfter := query.String(r, "start-after", "")

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
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects v2 (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error listing objects v2 (no such bucket)")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing objects v2 and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error listing objects v2")
		return
	}

	// Filter objects based on permissions
	filteredObjects, err := h.filterObjects(r.Context(), bucket, objects.Objects, r)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error filtering objects and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error filtering objects")
		return
	}

	// Per S3 spec: KeyCount is the number of keys returned, including both objects and common prefixes
	// "each common prefix counts as a single return when calculating the number of returns"
	// Recalculate after filtering
	keyCount := len(filteredObjects) + len(objects.CommonPrefixes)

	response := s3types.ListBucketV2Result{
		Name:                  bucket,
		Prefix:                prefix,
		Delimiter:             delimiter,
		MaxKeys:               maxKeys,
		KeyCount:              keyCount,
		IsTruncated:           objects.IsTruncated,
		ContinuationToken:     continuationToken,
		NextContinuationToken: objects.NextMarker,
		StartAfter:            startAfter,
		Contents:              filteredObjects,
		CommonPrefixes:        objects.CommonPrefixes,
	}

	if writeErr := WriteXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}

const (
	defaultMaxKeys = 1000
	maxMaxKeys     = 1000
)

// parseMaxKeys parses the max-keys query parameter with S3-compatible defaults and limits
func parseMaxKeys(maxKeys int) int {
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
	// Read and parse the XML request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error reading delete objects request body and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error reading delete objects request body")
		return
	}

	var xmlRequest s3types.DeleteObjectsRequest
	if err := xml.Unmarshal(bodyBytes, &xmlRequest); err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error parsing delete objects request and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error parsing delete objects request")
		return
	}

	// Validate that objects list is not empty
	if len(xmlRequest.Objects) == 0 {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, errors.New("delete request must contain at least one object")); writeErr != nil {
			s3Logger.With("write_err", writeErr).Warn("encountered error writing empty objects list error response")
			return
		}
		return
	}

	// Create service request
	deleteRequest := &s3.DeleteObjectsRequest{
		Bucket:  bucket,
		Objects: xmlRequest.Objects,
		Quiet:   xmlRequest.Quiet,
	}

	// Call service
	result, err := h.s3Service.DeleteObjects(r.Context(), deleteRequest)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting objects (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error deleting objects (no such bucket)")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting objects and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error deleting objects")
		return
	}

	// Build XML response
	response := s3types.DeleteObjectsResult{
		Deleted: result.Deleted,
		Errors:  result.Errors,
	}

	if writeErr := WriteXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}
