package s3

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	contextInt "github.com/mallardduck/dirio/internal/context"
	authpkg "github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/http/response"
	svcs3 "github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// PostObject handles POST /{bucket} — S3 POST policy browser-based form upload.
//
// Auth middleware has already verified the signature and stored the authenticated user
// and the base64-encoded policy document in the request context. This handler:
//  1. Retrieves the policy document from context and parses it
//  2. Extracts form fields (key, Content-Type, success response options)
//  3. Reads the uploaded file from the "file" form field
//  4. Validates policy conditions (bucket, key, content-type, content-length-range)
//  5. Stores the object via the S3 service
//  6. Returns the appropriate success response
func (h *HTTPHandler) PostObject(w http.ResponseWriter, r *http.Request, bucket string) {
	ctx := r.Context()
	requestID := middleware.GetRequestID(ctx)

	// Safety check — auth middleware must have set this
	if !contextInt.IsPostPolicyRequest(ctx) {
		if err := WriteErrorResponse(w, requestID, s3types.ErrCodeAccessDenied); err != nil {
			s3Logger.With("err", err).Warn("error writing access denied for non-post-policy request to PostObject")
		}
		return
	}

	// Parse the policy document stored in context by auth middleware
	policyB64 := contextInt.GetPostPolicyPolicyB64(ctx)
	doc, err := authpkg.ParsePostPolicyDocument(policyB64)
	if err != nil {
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, response.SetErrAsMessage(err)); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("error parsing post policy document")
		}
		return
	}

	// Extract fields from the already-parsed multipart form
	mf := r.MultipartForm
	if mf == nil {
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, response.SetErrAsMessage(fmt.Errorf("no multipart form data"))); writeErr != nil {
			s3Logger.With("write_err", writeErr).Warn("error writing invalid request response")
		}
		return
	}

	getField := func(name string) string {
		if vals, ok := mf.Value[name]; ok && len(vals) > 0 {
			return vals[0]
		}
		return ""
	}

	key := getField("key")
	contentType := getField("Content-Type")
	if contentType == "" {
		contentType = getField("content-type")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	successRedirect := getField("success_action_redirect")
	successStatus := getField("success_action_status")

	// Get the uploaded file
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, response.SetErrAsMessage(fmt.Errorf("missing file field: %w", err))); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("error writing invalid request (missing file field)")
		}
		return
	}
	defer file.Close()

	// Resolve ${filename} placeholder in the key
	if strings.Contains(key, "${filename}") {
		key = strings.ReplaceAll(key, "${filename}", fileHeader.Filename)
	}

	// Validate policy conditions against the actual upload parameters
	if err := authpkg.ValidatePostPolicyConditions(doc, bucket, key, contentType, fileHeader.Size); err != nil {
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeAccessDenied, response.SetErrAsMessage(err)); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("post policy condition validation failed")
		}
		return
	}

	// Store the object
	putRequest := &svcs3.PutObjectRequest{
		Bucket:      bucket,
		Key:         key,
		Content:     file,
		ContentType: contentType,
	}
	etag, err := h.s3Service.PutObject(ctx, putRequest)
	if err != nil {
		if errors.Is(err, s3types.ErrBucketNotFound) {
			if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, response.SetErrAsMessage(err)); writeErr != nil {
				s3Logger.With("err", err, "write_err", writeErr).Warn("bucket not found during POST policy upload")
			}
			return
		}
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, response.SetErrAsMessage(err)); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("error storing object during POST policy upload")
		}
		return
	}

	// Success response depends on form fields
	if successRedirect != "" {
		redirectURL := fmt.Sprintf("%s?bucket=%s&key=%s&etag=%s",
			successRedirect,
			url.QueryEscape(bucket),
			url.QueryEscape(key),
			url.QueryEscape(etag),
		)
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	switch successStatus {
	case "200":
		w.WriteHeader(http.StatusOK)
	case "201":
		location := fmt.Sprintf("/%s/%s", bucket, key)
		result := s3types.PostResponse{
			Location: location,
			Bucket:   bucket,
			Key:      key,
			ETag:     etag,
		}
		if err := WriteXMLResponse(w, http.StatusCreated, result); err != nil {
			s3Logger.With("err", err).Warn("error writing 201 PostResponse XML")
		}
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
