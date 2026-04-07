package s3

import (
	"net/http"

	"github.com/mallardduck/dirio/internal/http/middleware"
	loggingHttp "github.com/mallardduck/dirio/internal/http/middleware/logging"
	httpresponse "github.com/mallardduck/dirio/internal/http/response"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/service"
	"github.com/mallardduck/dirio/internal/service/observation"
	svcs3 "github.com/mallardduck/dirio/internal/service/s3"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// HTTPHandler handles S3 API requests
type HTTPHandler struct {
	s3Service      *svcs3.Service
	observationSvc *observation.Service
	urlBuilder     URLBuilder
	adminKeys      policy.AdminKeyChecker // Live admin key source for filtering bypass
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
	adminKeys policy.AdminKeyChecker,
) *HTTPHandler {
	return &HTTPHandler{
		s3Service:      serviceFactory.S3(),
		observationSvc: serviceFactory.Observation(),
		urlBuilder:     urlBuilder,
		adminKeys:      adminKeys,
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

// respondError writes an S3 XML error response and logs the outcome.
// The caller must return after this call.
func respondError(w http.ResponseWriter, requestID string, err error, errCode s3types.ErrorCode, msg string, mods ...httpresponse.ErrorModifier) {
	if writeErr := WriteErrorResponse(w, requestID, errCode, mods...); writeErr != nil {
		s3Logger.With("err", err, "write_err", writeErr).Warn(msg + " and additional error writing XML error response")
		return
	}
	s3Logger.With("err", err).Warn(msg)
}
