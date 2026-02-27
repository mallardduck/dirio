package api

import (
	"net/http"

	"github.com/mallardduck/dirio/internal/http/api/s3"
	"github.com/mallardduck/dirio/internal/http/auth"
	httpresponse "github.com/mallardduck/dirio/internal/http/response"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/service"
)

// Handler handles S3 API requests
type Handler struct {
	authHandler    *auth.Authenticator
	serviceFactory *service.ServicesFactory
	S3Handler      *s3.HTTPHandler
}

// URLBuilder defines the interface for generating URLs in S3 API responses
type URLBuilder interface {
	BucketURL(r *http.Request, bucket string) string
	ObjectURL(r *http.Request, bucket, key string) string
}

// New creates a new DirIO API handler
func New(
	serviceFactory *service.ServicesFactory,
	authHandler *auth.Authenticator,
	urlBuilder URLBuilder,
	adminKeys policy.AdminKeyChecker,
) *Handler {
	return &Handler{
		authHandler:    authHandler,
		serviceFactory: serviceFactory,
		S3Handler: s3.New(
			serviceFactory,
			urlBuilder,
			adminKeys,
		),
	}
}

// WriteXMLResponse writes an S3 response in XML format.
// It is exported so that middleware can use it.
var WriteXMLResponse = httpresponse.WriteXMLResponse

// WriteErrorResponse writes an S3 error response in XML format.
// It is exported so that middleware can use it for validation errors.
var WriteErrorResponse = httpresponse.WriteErrorResponse
