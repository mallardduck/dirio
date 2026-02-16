package s3

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	// Set common response headers
	w.Header().Set(headers.ContentType, obj.ContentType)
	w.Header().Set(headers.ETag, obj.ETag)
	w.Header().Set(headers.LastModified, obj.LastModified.Format(http.TimeFormat))
	w.Header().Set(headers.AcceptRanges, "bytes")

	// Set custom metadata headers
	for key, value := range obj.CustomMetadata {
		w.Header().Set(key, value)
	}

	// Check for Range header
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		// Parse range header (e.g., "bytes=0-1023" or "bytes=1024-")
		start, end, err := parseRangeHeader(rangeHeader, obj.Size)
		if err != nil {
			// Invalid range - return 416 Range Not Satisfiable
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", obj.Size))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		// Read full content into buffer to support seeking
		// TODO: Optimize by implementing seekable storage layer
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, obj.Content); err != nil {
			requestID := middleware.GetRequestID(r.Context())
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("error reading object content for range request")
				return
			}
			return
		}

		// Create section reader for the requested range
		rangeReader := io.NewSectionReader(bytes.NewReader(buf.Bytes()), start, end-start+1)
		rangeSize := end - start + 1

		// Set range-specific headers
		w.Header().Set(headers.ContentLength, strconv.FormatInt(rangeSize, 10))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, obj.Size))
		w.WriteHeader(http.StatusPartialContent)

		// Copy range to response
		if _, err := io.Copy(w, rangeReader); err != nil {
			log := logging.ComponentWithContext(r.Context(), "api")
			log.Warn("failed to write range content", "bucket", bucket, "key", key, "start", start, "end", end, "error", err)
		}
		return
	}

	// No range request - return full object
	w.Header().Set(headers.ContentLength, strconv.FormatInt(obj.Size, 10))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, obj.Content); err != nil {
		// Can't send error response after headers written, but log for debugging
		log := logging.ComponentWithContext(r.Context(), "api")
		log.Warn("failed to write object content", "bucket", bucket, "key", key, "error", err)
	}
}

// parseRangeHeader parses HTTP Range header and returns start and end byte positions
// Supports formats: "bytes=0-1023", "bytes=1024-", "bytes=-1024"
func parseRangeHeader(rangeHeader string, fileSize int64) (start, end int64, err error) {
	// Remove "bytes=" prefix
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, errors.New("invalid range header format")
	}
	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")

	// Split on dash
	parts := strings.Split(rangeSpec, "-")
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid range specification")
	}

	// Parse start and end
	if parts[0] == "" {
		// Suffix range: bytes=-1024 (last 1024 bytes)
		suffixLength, parseErr := strconv.ParseInt(parts[1], 10, 64)
		if parseErr != nil {
			return 0, 0, parseErr
		}
		if suffixLength > fileSize {
			suffixLength = fileSize
		}
		return fileSize - suffixLength, fileSize - 1, nil
	}

	start, parseErr := strconv.ParseInt(parts[0], 10, 64)
	if parseErr != nil {
		return 0, 0, parseErr
	}

	if parts[1] == "" {
		// Open-ended range: bytes=1024-
		end = fileSize - 1
	} else {
		end, parseErr = strconv.ParseInt(parts[1], 10, 64)
		if parseErr != nil {
			return 0, 0, parseErr
		}
	}

	// Validate range
	if start < 0 || start >= fileSize {
		return 0, 0, errors.New("start position out of range")
	}
	if end >= fileSize {
		end = fileSize - 1
	}
	if start > end {
		return 0, 0, errors.New("invalid range: start > end")
	}

	return start, end, nil
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
	// ✅ FIXED(Phase 3.2 #9): Custom metadata keys now normalized to lowercase
	//   - Go's HTTP package canonicalizes headers to Title-Case
	//   - boto3 and other clients expect lowercase keys in Metadata dict
	//   - Solution: Normalize all metadata keys to lowercase for consistent retrieval
	//   - This matches behavior of other S3-compatible systems
	metadataHeaders := []string{
		headers.CacheControl,
		headers.ContentDisposition,
		headers.ContentEncoding,
		headers.ContentLanguage,
		headers.Expires,
	}
	for _, header := range metadataHeaders {
		if value := r.Header.Get(header); value != "" {
			// Store with lowercase keys for consistent retrieval
			customMetadata[strings.ToLower(header)] = value
		}
	}

	// Extract user-defined metadata (x-amz-meta-*)
	// Normalize keys to lowercase for consistent retrieval across clients
	// (Go's HTTP package canonicalizes headers to Title-Case, but S3 clients expect lowercase)
	for key, values := range r.Header {
		lowerKey := strings.ToLower(key)
		if strings.HasPrefix(lowerKey, "x-amz-meta-") && len(values) > 0 {
			customMetadata[lowerKey] = values[0]
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

// CopyObject handles PUT /{bucket}/{key} with X-Amz-Copy-Source header
func (h *HTTPHandler) CopyObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	// Parse X-Amz-Copy-Source header (format: /sourceBucket/sourceKey or sourceBucket/sourceKey)
	copySource := r.Header.Get("X-Amz-Copy-Source")
	if copySource == "" {
		requestID := middleware.GetRequestID(r.Context())
		err := errors.New("missing X-Amz-Copy-Source header")
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("missing copy source header")
			return
		}
		return
	}

	// Remove leading slash if present
	copySource = strings.TrimPrefix(copySource, "/")

	// Split into source bucket and key
	parts := strings.SplitN(copySource, "/", 2)
	if len(parts) != 2 {
		requestID := middleware.GetRequestID(r.Context())
		err := errors.New("invalid X-Amz-Copy-Source format, expected /bucket/key")
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("invalid copy source format")
			return
		}
		return
	}

	sourceBucket := parts[0]
	sourceKey := parts[1]

	// Call service layer to copy object
	err := h.s3Service.CopyObject(r.Context(), sourceBucket, sourceKey, bucket, key)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrObjectNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchKey, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("copy source object not found")
				return
			}
			return
		}
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, err); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("copy source or dest bucket not found")
				return
			}
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("error copying object")
			return
		}
		return
	}

	// Get the newly copied object to return metadata
	obj, err := h.s3Service.GetObject(r.Context(), &s3.GetObjectRequest{
		Bucket: bucket,
		Key:    key,
	})
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, err); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("error getting copied object metadata")
			return
		}
		return
	}
	obj.Content.Close() // We don't need the content, just metadata

	// Return CopyObjectResult XML response
	result := s3types.CopyObjectResult{
		LastModified: obj.LastModified.Format(time.RFC3339), // ISO 8601 format for XML responses
		ETag:         obj.ETag,
	}

	if err := WriteXMLResponse(w, http.StatusOK, result); err != nil {
		s3Logger.With("err", err).Warn("error writing CopyObject XML response")
	}
}
