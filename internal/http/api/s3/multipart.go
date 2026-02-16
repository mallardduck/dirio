package s3

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/s3types"
	"github.com/mallardduck/go-http-helpers/pkg/headers"
)

// writeS3Error writes an S3 error response
func writeS3Error(w http.ResponseWriter, r *http.Request, statusCode int, errorCode, message string) {
	response := s3types.ErrorResponse{
		Code:    errorCode,
		Message: message,
	}

	w.Header().Set(headers.ContentType, "application/xml")
	w.WriteHeader(statusCode)
	xml.NewEncoder(w).Encode(response)
}

// CreateMultipartUpload initiates a multipart upload
func (h *HTTPHandler) CreateMultipartUpload(w http.ResponseWriter, r *http.Request, bucket, key string) {
	// Get content type from request
	contentType := r.Header.Get(headers.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Extract custom metadata from headers
	customMetadata := make(map[string]string)
	for headerKey, values := range r.Header {
		lowerKey := strings.ToLower(headerKey)
		if strings.HasPrefix(lowerKey, "x-amz-meta-") && len(values) > 0 {
			customMetadata[lowerKey] = values[0]
		}
	}

	// Create multipart upload
	req := &s3.CreateMultipartUploadRequest{
		Bucket:         bucket,
		Key:            key,
		ContentType:    contentType,
		CustomMetadata: customMetadata,
	}

	resp, err := h.s3Service.CreateMultipartUpload(r.Context(), req)
	if err != nil {
		writeS3Error(w, r, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}

	// Build response
	response := s3types.InitiateMultipartUploadResult{
		XMLName:  xml.Name{Space: "http://s3.amazonaws.com/doc/2006-03-01/", Local: "InitiateMultipartUploadResult"},
		Bucket:   resp.Bucket,
		Key:      resp.Key,
		UploadID: resp.UploadID,
	}

	w.Header().Set(headers.ContentType, "application/xml")
	w.WriteHeader(http.StatusOK)
	if err := xml.NewEncoder(w).Encode(response); err != nil {
		// Already wrote headers, can't change status code
		return
	}
}

// UploadPart uploads a single part for a multipart upload
func (h *HTTPHandler) UploadPart(w http.ResponseWriter, r *http.Request, bucket, key string) {
	// Get query parameters
	uploadID := r.URL.Query().Get("uploadId")
	partNumberStr := r.URL.Query().Get("partNumber")

	if uploadID == "" || partNumberStr == "" {
		writeS3Error(w, r, http.StatusBadRequest, "InvalidRequest", "uploadId and partNumber are required")
		return
	}

	partNumber, err := strconv.Atoi(partNumberStr)
	if err != nil || partNumber < 1 || partNumber > 10000 {
		writeS3Error(w, r, http.StatusBadRequest, "InvalidPart", "part number must be between 1 and 10000")
		return
	}

	// Upload part
	req := &s3.UploadPartRequest{
		Bucket:     bucket,
		Key:        key,
		UploadID:   uploadID,
		PartNumber: partNumber,
		Content:    r.Body,
	}

	resp, err := h.s3Service.UploadPart(r.Context(), req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeS3Error(w, r, http.StatusNotFound, "NoSuchUpload", "The specified multipart upload does not exist")
		} else {
			writeS3Error(w, r, http.StatusInternalServerError, "InternalError", err.Error())
		}
		return
	}

	// Return ETag in header
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, resp.ETag))
	w.WriteHeader(http.StatusOK)
}

// CompleteMultipartUpload completes a multipart upload by assembling all parts
func (h *HTTPHandler) CompleteMultipartUpload(w http.ResponseWriter, r *http.Request, bucket, key string) {
	uploadID := r.URL.Query().Get("uploadId")
	if uploadID == "" {
		writeS3Error(w, r, http.StatusBadRequest, "InvalidRequest", "uploadId is required")
		return
	}

	// Parse request body
	var completeReq s3types.CompleteMultipartUpload
	if err := xml.NewDecoder(r.Body).Decode(&completeReq); err != nil {
		writeS3Error(w, r, http.StatusBadRequest, "MalformedXML", "Invalid XML in request body")
		return
	}

	// Convert to service request
	parts := make([]s3.CompletePart, len(completeReq.Parts))
	for i, part := range completeReq.Parts {
		parts[i] = s3.CompletePart{
			PartNumber: part.PartNumber,
			ETag:       part.ETag,
		}
	}

	req := &s3.CompleteMultipartUploadRequest{
		Bucket:   bucket,
		Key:      key,
		UploadID: uploadID,
		Parts:    parts,
	}

	resp, err := h.s3Service.CompleteMultipartUpload(r.Context(), req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeS3Error(w, r, http.StatusNotFound, "NoSuchUpload", "The specified multipart upload does not exist")
		} else if strings.Contains(err.Error(), "mismatch") {
			writeS3Error(w, r, http.StatusBadRequest, "InvalidPart", err.Error())
		} else {
			writeS3Error(w, r, http.StatusInternalServerError, "InternalError", err.Error())
		}
		return
	}

	// Build response
	response := s3types.CompleteMultipartUploadResult{
		XMLName:  xml.Name{Space: "http://s3.amazonaws.com/doc/2006-03-01/", Local: "CompleteMultipartUploadResult"},
		Location: h.urlBuilder.ObjectURL(r, bucket, key),
		Bucket:   resp.Bucket,
		Key:      resp.Key,
		ETag:     fmt.Sprintf(`"%s"`, resp.ETag),
	}

	w.Header().Set(headers.ContentType, "application/xml")
	w.WriteHeader(http.StatusOK)
	if err := xml.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

// AbortMultipartUpload aborts a multipart upload and cleans up all parts
func (h *HTTPHandler) AbortMultipartUpload(w http.ResponseWriter, r *http.Request, bucket, key string) {
	uploadID := r.URL.Query().Get("uploadId")
	if uploadID == "" {
		writeS3Error(w, r, http.StatusBadRequest, "InvalidRequest", "uploadId is required")
		return
	}

	req := &s3.AbortMultipartUploadRequest{
		Bucket:   bucket,
		Key:      key,
		UploadID: uploadID,
	}

	if err := h.s3Service.AbortMultipartUpload(r.Context(), req); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeS3Error(w, r, http.StatusNotFound, "NoSuchUpload", "The specified multipart upload does not exist")
		} else {
			writeS3Error(w, r, http.StatusInternalServerError, "InternalError", err.Error())
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListParts lists all uploaded parts for a multipart upload
func (h *HTTPHandler) ListParts(w http.ResponseWriter, r *http.Request, bucket, key string) {
	uploadID := r.URL.Query().Get("uploadId")
	if uploadID == "" {
		writeS3Error(w, r, http.StatusBadRequest, "InvalidRequest", "uploadId is required")
		return
	}

	maxParts := 1000
	if maxPartsStr := r.URL.Query().Get("max-parts"); maxPartsStr != "" {
		if parsed, err := strconv.Atoi(maxPartsStr); err == nil && parsed > 0 {
			maxParts = parsed
		}
	}

	req := &s3.ListPartsRequest{
		Bucket:   bucket,
		Key:      key,
		UploadID: uploadID,
		MaxParts: maxParts,
	}

	resp, err := h.s3Service.ListParts(r.Context(), req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeS3Error(w, r, http.StatusNotFound, "NoSuchUpload", "The specified multipart upload does not exist")
		} else {
			writeS3Error(w, r, http.StatusInternalServerError, "InternalError", err.Error())
		}
		return
	}

	// Convert to S3 response type
	parts := make([]s3types.Part, len(resp.Parts))
	for i, part := range resp.Parts {
		parts[i] = s3types.Part{
			PartNumber:   part.PartNumber,
			ETag:         fmt.Sprintf(`"%s"`, part.ETag),
			Size:         part.Size,
			LastModified: part.LastModified.Format("2006-01-02T15:04:05.000Z"),
		}
	}

	response := s3types.ListPartsResult{
		XMLName:  xml.Name{Space: "http://s3.amazonaws.com/doc/2006-03-01/", Local: "ListPartsResult"},
		Bucket:   resp.Bucket,
		Key:      resp.Key,
		UploadID: resp.UploadID,
		Parts:    parts,
	}

	w.Header().Set(headers.ContentType, "application/xml")
	w.WriteHeader(http.StatusOK)
	if err := xml.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

// UploadPartCopy copies data from an existing object as a part in a multipart upload
func (h *HTTPHandler) UploadPartCopy(w http.ResponseWriter, r *http.Request, bucket, key string) {
	// TODO: implement UploadPartCopy
	// This is similar to UploadPart but copies from an existing S3 object instead of request body
	writeS3Error(w, r, http.StatusNotImplemented, "NotImplemented", "UploadPartCopy is not yet implemented")
}
