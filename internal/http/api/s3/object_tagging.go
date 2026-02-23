package s3

import (
	"encoding/xml"
	"errors"
	"io"
	"net/http"

	"github.com/mallardduck/go-http-helpers/pkg/headers"

	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/http/response"
	"github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// PutObjectTagging handles PUT /{bucket}/{key}?tagging
// Sets or replaces tags on an existing object
func (h *HTTPHandler) PutObjectTagging(w http.ResponseWriter, r *http.Request, bucket, key string) {
	requestID := middleware.GetRequestID(r.Context())

	// Parse the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, response.SetErrAsMessage(err)); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("failed to read tagging request body and failed to write error response")
			return
		}
		s3Logger.With("err", err).Warn("failed to read tagging request body")
		return
	}

	// Parse XML
	var taggingReq s3types.PutObjectTaggingRequest
	if err := xml.Unmarshal(body, &taggingReq); err != nil {
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeMalformedXML, response.SetErrAsMessage(err)); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("failed to parse tagging XML and failed to write error response")
			return
		}
		s3Logger.With("err", err).Warn("failed to parse tagging XML")
		return
	}

	// Convert tags to map
	tags := make(map[string]string)
	for _, tag := range taggingReq.TagSet {
		tags[tag.Key] = tag.Value
	}

	// Set tags via service
	err = h.s3Service.PutObjectTagging(r.Context(), &s3.PutObjectTaggingRequest{
		Bucket: bucket,
		Key:    key,
		Tags:   tags,
	})
	if err != nil {
		if errors.Is(err, s3types.ErrObjectNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchKey, response.SetErrAsMessage(err)); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("object not found for tagging and failed to write error response")
				return
			}
			s3Logger.With("err", err).Warn("object not found for tagging")
			return
		}
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, response.SetErrAsMessage(err)); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("bucket not found for tagging and failed to write error response")
				return
			}
			s3Logger.With("err", err).Warn("bucket not found for tagging")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, response.SetErrAsMessage(err)); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("failed to set object tags and failed to write error response")
			return
		}
		s3Logger.With("err", err).Warn("failed to set object tags")
		return
	}

	// Success - return 200 OK with empty body
	w.WriteHeader(http.StatusOK)
}

// GetObjectTagging handles GET /{bucket}/{key}?tagging
// Returns the tags associated with an object
func (h *HTTPHandler) GetObjectTagging(w http.ResponseWriter, r *http.Request, bucket, key string) {
	requestID := middleware.GetRequestID(r.Context())

	// Get tags via service
	tags, err := h.s3Service.GetObjectTagging(r.Context(), &s3.GetObjectTaggingRequest{
		Bucket: bucket,
		Key:    key,
	})
	if err != nil {
		if errors.Is(err, s3types.ErrObjectNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchKey, response.SetErrAsMessage(err)); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("object not found for getting tags and failed to write error response")
				return
			}
			s3Logger.With("err", err).Warn("object not found for getting tags")
			return
		}
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, response.SetErrAsMessage(err)); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("bucket not found for getting tags and failed to write error response")
				return
			}
			s3Logger.With("err", err).Warn("bucket not found for getting tags")
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, response.SetErrAsMessage(err)); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("failed to get object tags and failed to write error response")
			return
		}
		s3Logger.With("err", err).Warn("failed to get object tags")
		return
	}

	// Convert map to tag slice
	tagSlice := make([]s3types.Tag, 0, len(tags))
	for key, value := range tags {
		tagSlice = append(tagSlice, s3types.Tag{
			Key:   key,
			Value: value,
		})
	}

	// Build response
	response := s3types.Tagging{
		TagSet: tagSlice,
	}

	// Return XML response
	w.Header().Set(headers.ContentType, "application/xml")
	if writeErr := WriteXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("failed to write tagging XML response")
	}
}
