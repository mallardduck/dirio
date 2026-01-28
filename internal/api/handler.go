package api

import (
	"net/http"

	"github.com/mallardduck/dirio/internal/api/s3"
	"github.com/mallardduck/dirio/internal/auth"
	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/middleware"
	"github.com/mallardduck/dirio/internal/router"
	"github.com/mallardduck/dirio/internal/storage"
)

type routeHandler struct {
	HeadHandler    http.HandlerFunc
	StoreHandler   http.HandlerFunc
	ShowHandler    http.HandlerFunc
	DestroyHandler http.HandlerFunc
}

// Handler handles S3 API requests
type Handler struct {
	S3Handler *s3.Handler
}

// URLBuilder defines the interface for generating URLs in S3 API responses
type URLBuilder interface {
	BucketURL(r *http.Request, bucket string) string
	ObjectURL(r *http.Request, bucket, key string) string
}

// New creates a new DirIO API handler
func New(storage *storage.Storage, metadata *metadata.Manager, auth *auth.Authenticator, urlBuilder URLBuilder) *Handler {
	return &Handler{
		S3Handler: s3.New(
			storage,
			metadata,
			auth,
			urlBuilder,
		),
	}
}

// BucketResourceHandler routes bucket operations based on method
func (h *Handler) BucketResourceHandler() routeHandler {
	return routeHandler{
		HeadHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")

			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "HeadBucket"
			}
			h.S3Handler.HeadBucket(w, r, bucket, requestID)
		}, // HeadBucket
		StoreHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")

			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "CreateBucket"
			}

			h.S3Handler.CreateBucket(w, r, bucket, requestID)
		}, // CreateBucket
		ShowHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")

			// Check query parameters to determine operation
			query := r.URL.Query()

			// GetBucketLocation (backwards compatibility - AWS recommends HeadBucket instead)
			if _, ok := query["location"]; ok {
				if data, ok := loggingHttp.GetLogData(ctx); ok {
					data.Action = "GetBucketLocation"
				}
				h.S3Handler.GetBucketLocation(w, r, bucket, requestID)
				return
			}

			if query.Get("list-type") == "2" {
				if data, ok := loggingHttp.GetLogData(ctx); ok {
					data.Action = "ListObjectsV2"
				}
				h.S3Handler.ListObjectsV2(w, r, bucket, requestID)
				return
			}

			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "ListObjects"
			}
			h.S3Handler.ListObjects(w, r, bucket, requestID)
		}, // ListObjects, ListObjectsV2, GetBucketLocation
		DestroyHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")

			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "DeleteBucket"
			}
			h.S3Handler.DeleteBucket(w, r, bucket, requestID)
		}, // DeleteBucket
	}
}

// ObjectResourceHandler routes object operations based on method
func (h *Handler) ObjectResourceHandler() routeHandler {
	return routeHandler{
		HeadHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")
			// Chi uses "*" for catch-all wildcard parameter
			key := router.URLParam(r, "*")

			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "HeadObject"
			}
			h.S3Handler.HeadObject(w, r, bucket, key, requestID)
		}, // HeadObject
		StoreHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")
			// Chi uses "*" for catch-all wildcard parameter
			key := router.URLParam(r, "*")

			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "PutObject"
			}
			h.S3Handler.PutObject(w, r, bucket, key, requestID)
		}, // PutObject, TODO add CopyObject (x-amz-copy-source)
		ShowHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")
			// Chi uses "*" for catch-all wildcard parameter
			key := router.URLParam(r, "*")

			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "GetObject"
			}
			h.S3Handler.GetObject(w, r, bucket, key, requestID)
		}, // GetObject
		DestroyHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")
			// Chi uses "*" for catch-all wildcard parameter
			key := router.URLParam(r, "*")
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "DeleteObject"
			}
			h.S3Handler.DeleteObject(w, r, bucket, key, requestID)
		}, // DeleteObject
	}
}
