package s3

// TODO(Phase 3.2): Quick wins and critical features ready for implementation
//   Policy engine is COMPLETE - all these features have authorization infrastructure ready!
//
//   Quick Wins (likely routing issues):
//   1. Fix DeleteObject 405 for MinIO mc (see object.go:DeleteObject)
//   2. Fix DeleteBucket 405 for MinIO mc (see bucket.go:DeleteBucket)
//
//   Critical Path (NEXT PRIORITY):
//   3. Implement Pre-signed URLs (see auth/signature.go top comment)
//      - Query string authentication (X-Amz-Signature in URL)
//      - Expiration validation
//      - Essential for temporary access sharing
//
//   Medium Priority:
//   4. Implement CopyObject (see routes.go line ~347 comment)
//   5. Add Range request support to GetObject (see object.go:GetObject)
//   6. Fix custom metadata key case in responses (simple bug fix)
//   7. Fix object tagging content corruption (Bug #001 remnant)

import (
	"net/http"

	"github.com/mallardduck/dirio/internal/http/middleware"
	httpresponse "github.com/mallardduck/dirio/internal/http/response"
	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/service"
	svcs3 "github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// HTTPHandler handles S3 API requests
type HTTPHandler struct {
	s3Service    *svcs3.Service
	urlBuilder   URLBuilder
	metadata     *metadata.Manager      // For ownership lookups during filtering
	policyEngine *policy.Engine         // For permission checks during filtering
	adminKeys    policy.AdminKeyChecker // Live admin key source for filtering bypass
}

// URLBuilder defines the interface for generating URLs in S3 API responses
type URLBuilder interface {
	BucketURL(r *http.Request, bucket string) string
	ObjectURL(r *http.Request, bucket, key string) string
}

// New creates a new S3 API handler
func New(
	serviceFactory *service.ServicesFactory,
	urlBuilder URLBuilder,
	metadata *metadata.Manager,
	policyEngine *policy.Engine,
	adminKeys policy.AdminKeyChecker,
) *HTTPHandler {
	return &HTTPHandler{
		s3Service:    serviceFactory.S3(),
		urlBuilder:   urlBuilder,
		metadata:     metadata,
		policyEngine: policyEngine,
		adminKeys:    adminKeys,
	}
}

// ListBuckets handles GET / (list all buckets; for the root index route)
func (h *HTTPHandler) ListBuckets(w http.ResponseWriter, r *http.Request) {
	if data, ok := loggingHttp.GetLogData(r.Context()); ok {
		data.Action = "ListBuckets"
	}

	buckets, err := h.s3Service.ListBuckets(r.Context())
	if err != nil {
		requestID := middleware.GetRequestID(r.Context())
		if writeErr := WriteErrorResponse(w, requestID, s3types.ErrCodeInternalError, httpresponse.SetErrAsMessage(err)); writeErr != nil {
			s3Logger.With("err", err, "write_err", writeErr).Warn("encountered error listing buckets and additional error writing XML error response")
			return
		}
		s3Logger.With("err", err).Warn("encountered error listing buckets")
		return
	}

	// Filter buckets based on permissions
	filteredBuckets := h.filterBuckets(r.Context(), buckets, r)

	// Get owner from request context
	owner := buildOwnerFromContext(r.Context())

	response := s3types.ListBucketsResponse{
		Buckets: filteredBuckets,
		Owner:   owner,
	}

	if writeErr := WriteXMLResponse(w, http.StatusOK, response); writeErr != nil {
		s3Logger.With("err", writeErr).Warn("encountered error writing XML OK response")
	}
}

// WriteXMLResponse writes an S3 response in XML format.
// It is exported so that middleware can use it.
var WriteXMLResponse = httpresponse.WriteXMLResponse

// WriteErrorResponse writes an S3 error response in XML format.
// It is exported so that middleware can use it for validation errors.
var WriteErrorResponse = httpresponse.WriteErrorResponse
