package api

import (
	"net/http"

	"github.com/mallardduck/dirio/internal/http/api/iam"
	"github.com/mallardduck/dirio/internal/http/api/s3"
	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/persistence/storage"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/service"
)

// Handler handles S3 API requests
type Handler struct {
	auth           *auth.Authenticator
	serviceFactory *service.ServicesFactory
	S3Handler      *s3.HTTPHandler
	IAMHandler     *iam.Handler
}

// URLBuilder defines the interface for generating URLs in S3 API responses
type URLBuilder interface {
	BucketURL(r *http.Request, bucket string) string
	ObjectURL(r *http.Request, bucket, key string) string
}

// New creates a new DirIO API handler
func New(
	storage *storage.Storage,
	metadata *metadata.Manager,
	auth *auth.Authenticator,
	urlBuilder URLBuilder,
	policyEngine *policy.Engine,
	adminKeys policy.AdminKeyChecker,
) *Handler {
	serviceFactory := service.NewServiceFactory(storage, metadata, policyEngine)
	return &Handler{
		auth:           auth,
		serviceFactory: serviceFactory,
		S3Handler: s3.New(
			serviceFactory,
			urlBuilder,
			metadata,
			policyEngine,
			adminKeys,
		),
		IAMHandler: iam.New(
			serviceFactory,
		),
	}
}

// WriteXMLResponse writes an S3 response in XML format.
// It is exported so that middleware can use it.
var WriteXMLResponse = s3.WriteXMLResponse

// WriteErrorResponse writes an S3 error response in XML format.
// It is exported so that middleware can use it for validation errors.
var WriteErrorResponse = s3.WriteErrorResponse
