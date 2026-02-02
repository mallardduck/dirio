package s3

import (
	"io"
	"net/http"

	"github.com/mallardduck/dirio/internal/jsonutil"
	"github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/iam"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// PutBucketPolicy handles PUT /{bucket}?policy
func (h *HTTPHandler) PutBucketPolicy(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	// Read the policy document from the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s3Logger.With("err", err).Warn("failed to read bucket policy request body")
		writeErrorResponse(w, requestID, s3types.ErrCodeInvalidRequest, err)
		return
	}

	// Parse the policy document
	var policyDoc iam.PolicyDocument
	if err := jsonutil.Unmarshal(bodyBytes, &policyDoc); err != nil {
		s3Logger.With("err", err).Warn("failed to parse bucket policy JSON")
		writeErrorResponse(w, requestID, s3types.ErrCodeMalformedPolicy, err)
		return
	}

	// Set the bucket policy
	if err := h.s3Service.PutBucketPolicy(r.Context(), &s3.PutBucketPolicyRequest{
		Bucket:         bucket,
		PolicyDocument: &policyDoc,
	}); err != nil {
		s3Logger.With("err", err, "bucket", bucket).Warn("failed to set bucket policy")
		writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err)
		return
	}

	s3Logger.With("bucket", bucket).Debug("set bucket policy")
	w.WriteHeader(http.StatusNoContent)
}

// GetBucketPolicy handles GET /{bucket}?policy
func (h *HTTPHandler) GetBucketPolicy(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	policy, err := h.s3Service.GetBucketPolicy(r.Context(), bucket)
	if err != nil {
		if err == s3types.ErrNoSuchBucketPolicy {
			writeErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucketPolicy, err)
			return
		}
		s3Logger.With("err", err, "bucket", bucket).Warn("failed to get bucket policy")
		writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err)
		return
	}

	if policy == nil {
		writeErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucketPolicy, nil)
		return
	}

	// Return the policy as JSON
	policyJSON, err := jsonutil.Marshal(policy)
	if err != nil {
		s3Logger.With("err", err).Warn("failed to marshal bucket policy")
		writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(policyJSON)
}

// DeleteBucketPolicy handles DELETE /{bucket}?policy
func (h *HTTPHandler) DeleteBucketPolicy(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
	if err := h.s3Service.DeleteBucketPolicy(r.Context(), bucket); err != nil {
		s3Logger.With("err", err, "bucket", bucket).Warn("failed to delete bucket policy")
		writeErrorResponse(w, requestID, s3types.ErrCodeInternalError, err)
		return
	}

	s3Logger.With("bucket", bucket).Debug("deleted bucket policy")
	w.WriteHeader(http.StatusNoContent)
}
