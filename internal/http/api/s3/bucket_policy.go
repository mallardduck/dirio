package s3

import (
	"errors"
	"io"
	"net/http"

	"github.com/mallardduck/go-http-helpers/pkg/headers"

	"github.com/mallardduck/dirio/internal/http/response"

	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/jsonutil"
	"github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/iam"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// PutBucketPolicy handles PUT /{bucket}?policy
func (h *HTTPHandler) PutBucketPolicy(w http.ResponseWriter, r *http.Request, bucket string) {
	// Read the policy document from the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		s3Logger.With("err", err).Warn("failed to read bucket policy request body")
		_ = WriteErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, response.SetErrAsMessage(err))
		return
	}

	// Parse the policy document
	var policyDoc iam.PolicyDocument
	if err := jsonutil.Unmarshal(bodyBytes, &policyDoc); err != nil {
		requestID := middleware.GetRequestID(r.Context())
		s3Logger.With("err", err).Warn("failed to parse bucket policy JSON")
		_ = WriteErrorResponse(w, requestID, s3types.ErrCodeMalformedPolicy, response.SetErrAsMessage(err))
		return
	}

	// Set the bucket policy
	if err := h.s3Service.PutBucketPolicy(r.Context(), &s3.PutBucketPolicyRequest{
		Bucket:         bucket,
		PolicyDocument: &policyDoc,
	}); err != nil {
		requestID := middleware.GetRequestID(r.Context())
		s3Logger.With("err", err, "bucket", bucket).Warn("failed to set bucket policy")
		_ = WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, response.SetErrAsMessage(err))
		return
	}

	s3Logger.With("bucket", bucket).Debug("set bucket policy")
	w.WriteHeader(http.StatusNoContent)
}

// GetBucketPolicy handles GET /{bucket}?policy
func (h *HTTPHandler) GetBucketPolicy(w http.ResponseWriter, r *http.Request, bucket string) {
	policy, err := h.s3Service.GetBucketPolicy(r.Context(), bucket)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if errors.Is(err, s3types.ErrNoSuchBucketPolicy) {
			_ = WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucketPolicy, response.SetErrAsMessage(err))
			return
		}
		s3Logger.With("err", err, "bucket", bucket).Warn("failed to get bucket policy")
		_ = WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, response.SetErrAsMessage(err))
		return
	}

	if policy == nil {
		requestID := middleware.GetRequestID(r.Context())
		_ = WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucketPolicy)
		return
	}

	// Return the policy as JSON
	policyJSON, err := jsonutil.Marshal(policy)
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		s3Logger.With("err", err).Warn("failed to marshal bucket policy")
		_ = WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, response.SetErrAsMessage(err))
		return
	}

	w.Header().Set(headers.ContentType, "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(policyJSON)
}

// DeleteBucketPolicy handles DELETE /{bucket}?policy
func (h *HTTPHandler) DeleteBucketPolicy(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := h.s3Service.DeleteBucketPolicy(r.Context(), bucket); err != nil {
		requestID := middleware.GetRequestID(r.Context())
		s3Logger.With("err", err, "bucket", bucket).Warn("failed to delete bucket policy")
		_ = WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, response.SetErrAsMessage(err))
		return
	}

	s3Logger.With("bucket", bucket).Debug("deleted bucket policy")
	w.WriteHeader(http.StatusNoContent)
}
