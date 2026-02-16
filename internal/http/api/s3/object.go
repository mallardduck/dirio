package s3

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/mallardduck/go-http-helpers/pkg/headers"

	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// GetObject handles GET /{bucket}/{key}
// TODO(Phase 3.2 #5): Add Range request support for resumable downloads and video streaming
//   - Parse Range header (bytes=0-1023, bytes=1024-, etc.)
//   - Return 206 Partial Content with Content-Range header
//   - Handle multiple ranges (multipart/byteranges response)
//   - Storage layer needs range support in GetObject
//   - Test with video streaming and aws s3api get-object --range
func (h *HTTPHandler) GetObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	objRequest := &s3.GetObjectRequest{
		Bucket: bucket,
		Key:    key,
	}
	obj, err := h.s3Service.GetObject(r.Context(), objRequest)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrObjectNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchKey, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object (no such key) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error getting object (no such key)")
			return
		}
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error getting object (no such bucket)")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error getting object")
		return
	}
	defer obj.Content.Close()

	// Set response headers
	w.Header().Set(headers.ContentType, obj.ContentType)
	w.Header().Set(headers.ContentLength, strconv.FormatInt(obj.Size, 10))
	w.Header().Set(headers.ETag, obj.ETag)
	w.Header().Set(headers.LastModified, obj.LastModified.Format(http.TimeFormat))
	w.Header().Set(headers.AcceptRanges, "bytes")

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
func (h *HTTPHandler) PutObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	contentType := r.Header.Get(headers.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Extract custom metadata from request headers
	customMetadata := make(map[string]string)

	// Extract S3-standard metadata headers
	// TODO(Phase 3.2 #9): Fix custom metadata key case preservation bug
	//   - Currently returns wrong case in GetObject/HeadObject responses
	//   - S3 standard: preserves exact case of x-amz-meta-* headers
	//   - Issue: somewhere in the chain we're lowercasing or changing case
	//   - Test: PUT with x-amz-meta-Author, GET should return x-amz-meta-Author (not x-amz-meta-author)
	//   - Check: storage layer, metadata serialization, response header setting
	metadataHeaders := []string{
		headers.CacheControl,
		headers.ContentDisposition,
		headers.ContentEncoding,
		headers.ContentLanguage,
		headers.Expires,
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

	putRequest := &s3.PutObjectRequest{
		Bucket:         bucket,
		Key:            key,
		Content:        r.Body,
		ContentType:    contentType,
		CustomMetadata: customMetadata,
	}

	// Read object content from request body
	// Note: Chunked encoding is automatically decoded by middleware
	etag, err := h.s3Service.PutObject(r.Context(), putRequest)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error putting object (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error putting object (no such bucket)")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error putting object and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error putting object")
		return
	}

	// Set response headers
	w.Header().Set(headers.ETag, etag)
	w.WriteHeader(http.StatusOK)
}

// HeadObject handles HEAD /{bucket}/{key}
func (h *HTTPHandler) HeadObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	headRequest := &s3.HeadObjectRequest{
		Bucket: bucket,
		Key:    key,
	}
	meta, err := h.s3Service.HeadObject(r.Context(), headRequest)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrObjectNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchKey, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object metadata (no such key) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error getting object metadata (no such key)")
			return
		}
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object metadata (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error getting object metadata (no such bucket)")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error getting object metadata and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error getting object metadata")
		return
	}

	// Set response headers
	w.Header().Set(headers.ContentType, meta.ContentType)
	w.Header().Set(headers.ContentLength, strconv.FormatInt(meta.Size, 10))
	w.Header().Set(headers.ETag, meta.ETag)
	w.Header().Set(headers.LastModified, meta.LastModified.Format(http.TimeFormat))
	w.Header().Set(headers.AcceptRanges, "bytes")

	// Set custom metadata headers
	for key, value := range meta.CustomMetadata {
		w.Header().Set(key, value)
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteObject handles DELETE /{bucket}/{key}
// TODO(Phase 3.2 #1): Fix 405 Method Not Allowed for MinIO mc client
//   - Currently returns 405 for MinIO mc, but policy engine is ready (s3:DeleteObject)
//   - Likely a routing issue, not authorization - check teapot-router DELETE route registration
//   - Test: mc rm alias/bucket/object.txt
//   - Verify route matches in routes.go line ~340: r.DELETE("/{bucket}/{key:.*}", ...)
func (h *HTTPHandler) DeleteObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	deleteRequest := &s3.DeleteObjectRequest{
		Bucket: bucket,
		Key:    key,
	}
	err := h.s3Service.DeleteObject(r.Context(), deleteRequest)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		// S3 returns 204 even if object doesn't exist
		if !errors.Is(err, s3types.ErrObjectNotFound) {
			if errors.Is(err, s3types.ErrBucketNotFound) {
				if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
					s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting object (no such bucket) and additional error writing XML error response")
					return
				}
				s3Logger.With("err", err).Warn("encountered error deleting object (no such bucket)")
				return
			}
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error deleting object and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error deleting object")
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
