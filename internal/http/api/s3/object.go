package s3

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mallardduck/go-http-helpers/pkg/headers"

	"github.com/mallardduck/dirio/internal/http/response"

	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// GetObject handles GET /{bucket}/{key}
func (h *HTTPHandler) GetObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	objRequest := &s3.GetObjectRequest{
		Bucket: bucket,
		Key:    key,
	}
	obj, err := h.s3Service.GetObject(r.Context(), objRequest)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		switch {
		case errors.Is(err, s3types.ErrObjectNotFound):
			respondError(w, requestID, err, s3types.ErrCodeNoSuchKey, "encountered error getting object (no such key)", response.SetErrAsMessage(err))
		case errors.Is(err, s3types.ErrBucketNotFound):
			respondError(w, requestID, err, s3types.ErrCodeNoSuchBucket, "encountered error getting object (no such bucket)", response.SetErrAsMessage(err))
		default:
			respondError(w, requestID, err, s3types.ErrCodeInternalError, "encountered error getting object", response.SetErrAsMessage(err))
		}
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

	// Dispatch to range or full response
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		h.serveRangeResponse(w, r, obj, bucket, key, rangeHeader)
		return
	}
	h.serveFullResponse(w, r, obj, bucket, key)
}

func (h *HTTPHandler) serveRangeResponse(w http.ResponseWriter, r *http.Request, obj *s3.GetObjectResponse, bucket, key, rangeHeader string) {
	ranges, err := parseRangesHeader(rangeHeader, obj.Size)
	if err != nil || len(ranges) == 0 {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", obj.Size))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	if len(ranges) > 1 {
		h.serveMultiRangeResponse(w, r, obj, bucket, key, ranges)
		return
	}

	rng := ranges[0]
	rangeLength := rng.end - rng.start + 1

	w.Header().Set(headers.ContentLength, strconv.FormatInt(rangeLength, 10))
	w.Header().Set(headers.ContentRange, fmt.Sprintf("bytes %d-%d/%d", rng.start, rng.end, obj.Size))
	w.WriteHeader(http.StatusPartialContent)

	rangeReader := io.NewSectionReader(obj.Content, rng.start, rangeLength)
	if _, err := io.Copy(w, rangeReader); err != nil {
		log := logging.ComponentWithContext(r.Context(), "api")
		if isClientDisconnect(err) {
			log.Debug("client disconnected while writing range to response", "bucket", bucket, "key", key, "start", rng.start, "end", rng.end, "error", err)
		} else {
			log.Warn("failed to write range response body", "bucket", bucket, "key", key, "start", rng.start, "end", rng.end, "error", err)
		}
	}
}

func (h *HTTPHandler) serveMultiRangeResponse(w http.ResponseWriter, r *http.Request, obj *s3.GetObjectResponse, bucket, key string, ranges []rangeSpec) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	for _, rng := range ranges {
		partHeader := make(textproto.MIMEHeader)
		partHeader.Set("Content-Type", obj.ContentType)
		partHeader.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rng.start, rng.end, obj.Size))
		pw, err := mw.CreatePart(partHeader)
		if err != nil {
			// mime/multipart writer only errors on closed underlying writer — unreachable here
			return
		}
		if _, err := io.Copy(pw, io.NewSectionReader(obj.Content, rng.start, rng.end-rng.start+1)); err != nil {
			log := logging.ComponentWithContext(r.Context(), "api")
			log.Warn("failed to read range for multipart response", "bucket", bucket, "key", key, "start", rng.start, "end", rng.end, "error", err)
			return
		}
	}
	if err := mw.Close(); err != nil {
		return
	}

	w.Header().Set(headers.ContentType, "multipart/byteranges; boundary="+mw.Boundary())
	w.Header().Set(headers.ContentLength, strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusPartialContent)

	if _, err := io.Copy(w, &buf); err != nil {
		log := logging.ComponentWithContext(r.Context(), "api")
		if isClientDisconnect(err) {
			log.Debug("client disconnected while writing multipart range response", "bucket", bucket, "key", key, "error", err)
		} else {
			log.Warn("failed to write multipart range response body", "bucket", bucket, "key", key, "error", err)
		}
	}
}

func (h *HTTPHandler) serveFullResponse(w http.ResponseWriter, r *http.Request, obj *s3.GetObjectResponse, bucket, key string) {
	w.Header().Set(headers.ContentLength, strconv.FormatInt(obj.Size, 10))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, obj.Content); err != nil {
		log := logging.ComponentWithContext(r.Context(), "api")
		if isClientDisconnect(err) {
			log.Debug("client disconnected while writing object to response", "bucket", bucket, "key", key, "error", err)
		} else {
			log.Warn("failed to write object response body", "bucket", bucket, "key", key, "error", err)
		}
	}
}

// isClientDisconnect reports whether the error is due to the client closing the connection.
// This is expected during video streaming and large downloads; callers should log at DEBUG.
func isClientDisconnect(err error) bool {
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
}

// rangeSpec holds the resolved start and end byte positions of a single range.
type rangeSpec struct {
	start, end int64
}

// parseRangesHeader parses an HTTP Range header and returns all requested ranges.
// Supports comma-separated multi-range: "bytes=0-499, 500-999"
func parseRangesHeader(rangeHeader string, fileSize int64) ([]rangeSpec, error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return nil, errors.New("invalid range header format")
	}
	spec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(spec, ",")
	ranges := make([]rangeSpec, 0, len(parts))
	for _, part := range parts {
		start, end, err := parseSingleRangePart(strings.TrimSpace(part), fileSize)
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, rangeSpec{start, end})
	}
	return ranges, nil
}

// parseRangeHeader parses an HTTP Range header for a single range.
// Supports formats: "bytes=0-1023", "bytes=1024-", "bytes=-1024"
func parseRangeHeader(rangeHeader string, fileSize int64) (start, end int64, err error) {
	ranges, err := parseRangesHeader(rangeHeader, fileSize)
	if err != nil {
		return 0, 0, err
	}
	return ranges[0].start, ranges[0].end, nil
}

// parseSingleRangePart parses one range part (e.g. "0-1023", "1024-", "-512").
func parseSingleRangePart(part string, fileSize int64) (start, end int64, err error) {
	parts := strings.Split(part, "-")
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid range specification")
	}

	if parts[0] == "" {
		// Suffix range: -1024 (last 1024 bytes)
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
		// Open-ended range: 1024-
		end = fileSize - 1
	} else {
		end, parseErr = strconv.ParseInt(parts[1], 10, 64)
		if parseErr != nil {
			return 0, 0, parseErr
		}
	}

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
	//   - boto3 and other clients expect lowercase keys in metadata dict
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
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, response.SetErrAsMessage(err)); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error putting object (no such bucket) and additional error writing XML error response")
				return
			}
			s3Logger.With("err", err).Warn("encountered error putting object (no such bucket)")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, response.SetErrAsMessage(err)); writeErr != nil {
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
		switch {
		case errors.Is(err, s3types.ErrObjectNotFound):
			respondError(w, requestID, err, s3types.ErrCodeNoSuchKey, "encountered error getting object metadata (no such key)", response.SetErrAsMessage(err))
		case errors.Is(err, s3types.ErrBucketNotFound):
			respondError(w, requestID, err, s3types.ErrCodeNoSuchBucket, "encountered error getting object metadata (no such bucket)", response.SetErrAsMessage(err))
		default:
			respondError(w, requestID, err, s3types.ErrCodeInternalError, "encountered error getting object metadata", response.SetErrAsMessage(err))
		}
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
	// S3 returns 204 even if object doesn't exist
	if err != nil && !errors.Is(err, s3types.ErrObjectNotFound) {
		requestID := middleware.GetRequestID(r.Context())
		switch {
		case errors.Is(err, s3types.ErrBucketNotFound):
			respondError(w, requestID, err, s3types.ErrCodeNoSuchBucket, "encountered error deleting object (no such bucket)", response.SetErrAsMessage(err))
		default:
			respondError(w, requestID, err, s3types.ErrCodeInternalError, "encountered error deleting object", response.SetErrAsMessage(err))
		}
		return
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
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, response.SetErrAsMessage(err)); writeErr != nil {
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
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, response.SetErrAsMessage(err)); writeErr != nil {
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
		switch {
		case errors.Is(err, s3types.ErrObjectNotFound):
			respondError(w, requestID, err, s3types.ErrCodeNoSuchKey, "copy source object not found", response.SetErrAsMessage(err))
		case errors.Is(err, s3types.ErrBucketNotFound):
			respondError(w, requestID, err, s3types.ErrCodeNoSuchBucket, "copy source or dest bucket not found", response.SetErrAsMessage(err))
		default:
			respondError(w, requestID, err, s3types.ErrCodeInternalError, "error copying object", response.SetErrAsMessage(err))
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
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, response.SetErrAsMessage(err)); writeErr != nil {
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
