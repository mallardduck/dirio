package server

import (
	"io"
	"net/http"

	"github.com/mallardduck/dirio/internal/api"
	"github.com/mallardduck/dirio/internal/auth"
	"github.com/mallardduck/dirio/internal/middleware"
	"github.com/mallardduck/dirio/internal/server/debug"
	"github.com/mallardduck/dirio/internal/server/favicon"
	"github.com/mallardduck/teapot-router/pkg/teapot"
)

// RouteDependencies contains all dependencies needed for route handlers
type RouteDependencies struct {
	Auth       *auth.Authenticator
	APIHandler *api.Handler
	Debug      bool
}

// SetupRoutes configures all application routes on the provided router.
// When deps is nil, routes are registered with nil handlers (for CLI route listing).
func SetupRoutes(r *teapot.Router, deps *RouteDependencies) {
	// Helper to safely get handlers when deps might be nil
	var (
		faviconHandler http.HandlerFunc
		debugHandler   http.HandlerFunc
		listBuckets    http.HandlerFunc
		bucketHead     http.HandlerFunc
		bucketStore    http.HandlerFunc
		bucketShow     http.HandlerFunc
		bucketDestroy  http.HandlerFunc
		objectHead     http.HandlerFunc
		objectStore    http.HandlerFunc
		objectShow     http.HandlerFunc
		objectDestroy  http.HandlerFunc
		notImplemented http.HandlerFunc
	)

	if deps != nil {
		faviconHandler = favicon.HandleFavicon
		debugHandler = debug.HandleRoutes(r)
		listBuckets = deps.APIHandler.S3Handler.ListBuckets
		bucketHandler := deps.APIHandler.BucketResourceHandler()
		bucketHead = bucketHandler.HeadHandler
		bucketStore = bucketHandler.StoreHandler
		bucketShow = bucketHandler.ShowHandler
		bucketDestroy = bucketHandler.DestroyHandler
		objectHandler := deps.APIHandler.ObjectResourceHandler()
		objectHead = objectHandler.HeadHandler
		objectStore = objectHandler.StoreHandler
		objectShow = objectHandler.ShowHandler
		objectDestroy = objectHandler.DestroyHandler
		notImplemented = deps.notImplemented
	}

	// Public Routes (no auth required)
	r.MiddlewareGroup(func(r *teapot.Router) {
		r.GET("/favicon.ico", faviconHandler).Name("favicon")
		r.GET("/healthz", notImplemented).Name("health")
		// Debug routes (only when debug mode is enabled)
		if deps != nil && deps.Debug {
			r.GET("/.internal/routes", debugHandler).Name("debug.routes")
		}
	})

	if deps != nil &&
		deps.APIHandler != nil &&
		deps.APIHandler.IAMHandler != nil {
		userHandler := deps.APIHandler.IAMHandler.UserResourceHandler()
		policyHandler := deps.APIHandler.IAMHandler.PolicyResourceHandler()

		// MinIO Admin API Routes
		// Configurable prefix, defaults to "/minio/admin/v3" for mc compatibility
		// Can be configured to use same port (default) or separate admin port
		r.MiddlewareGroup(func(r *teapot.Router) {
			// TODO: Make this prefix configurable via Settings (default: /minio/admin/v3)
			r.NamedGroup("/minio/admin/v3", "admin", func(r *teapot.Router) {
				// User Management
				r.GET("/list-users", userHandler.ListHandler).Name("users.list")
				r.PUT("/add-user", userHandler.AddHandler).Name("users.add")
				r.POST("/remove-user", userHandler.RemoveHandler).Name("users.remove")
				r.GET("/user-info", userHandler.InfoHandler).Name("users.info")
				r.POST("/set-user-status", userHandler.StatusHandler).Name("users.setstatus")

				// Service Account Management
				r.GET("/list-service-accounts", notImplemented).Name("serviceaccounts.list")
				r.POST("/add-service-account", notImplemented).Name("serviceaccounts.add")
				r.POST("/delete-service-account", notImplemented).Name("serviceaccounts.delete")
				r.GET("/info-service-account", notImplemented).Name("serviceaccounts.info")
				r.POST("/update-service-account", notImplemented).Name("serviceaccounts.update")

				// Group Management
				r.POST("/update-group-members", notImplemented).Name("groups.updatemembers")
				r.GET("/group", notImplemented).Name("groups.info")
				r.GET("/groups", notImplemented).Name("groups.list")
				r.POST("/set-group-status", notImplemented).Name("groups.setstatus")

				// Policy Management (Canned Policies)
				r.GET("/list-canned-policies", policyHandler.ListHandler).Name("policies.list")
				r.POST("/add-canned-policy", policyHandler.AddHandler).Name("policies.add")
				r.PUT("/add-canned-policy", policyHandler.AddHandler).Name("policies.add")
				r.POST("/remove-canned-policy", policyHandler.RemoveHandler).Name("policies.remove")
				r.GET("/info-canned-policy", policyHandler.InfoHandler).Name("policies.info")

				// Policy Attachments
				// Old API: mc admin policy set (deprecated)
				r.POST("/set-policy", policyHandler.SetHandler).Name("policies.set")
				// New API: mc admin policy attach --user=...
				r.POST("/idp/builtin/policy/attach", policyHandler.SetHandler).Name("policies.attach")
				r.GET("/policy-entities", policyHandler.ListEntitiesHandler).Name("policies.entities")

				// Server Info & Health (useful for mc admin)
				r.GET("/info", notImplemented).Name("server.info")
				r.GET("/health", notImplemented).Name("server.health")
			})
		}, deps.Auth.AuthMiddleware)
	}

	// Authenticated Bucket and Object Routes - must go at end due to wildcards
	// Build middleware list conditionally
	var s3Middlewares []func(http.Handler) http.Handler
	if deps != nil {
		s3Middlewares = []func(http.Handler) http.Handler{
			deps.Auth.AuthMiddleware,
			// Chunked encoding middleware
			middleware.ChunkedEncoding(func(r io.Reader) io.Reader {
				return auth.NewChunkedReader(r)
			}),
		}
	}

	r.MiddlewareGroup(func(r *teapot.Router) {
		// Root - ListBuckets
		r.GET("/", listBuckets).Name("index")

		// Bucket operations
		r.HEAD("/{bucket}", bucketHead).Name("buckets.head")
		r.PUT("/{bucket}", bucketStore).Name("buckets.store")
		r.GET("/{bucket}", bucketShow).Name("buckets.show")
		r.DELETE("/{bucket}", bucketDestroy).Name("buckets.destroy")

		// Object operations (use {key:.*} to capture entire path)
		r.HEAD("/{bucket}/{key:.*}", objectHead).Name("objects.head")
		r.PUT("/{bucket}/{key:.*}", objectStore).Name("objects.create")
		r.GET("/{bucket}/{key:.*}", objectShow).Name("objects.show")
		r.DELETE("/{bucket}/{key:.*}", objectDestroy).Name("objects.destroy")
	}, s3Middlewares...)
}

// notImplemented is a placeholder for MinIO Admin API endpoints
// MinIO Admin API typically returns JSON responses
func (d *RouteDependencies) notImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	// MinIO Admin API error format
	w.Write([]byte(`{"status":"error","error":"This Admin API operation is not yet implemented"}`))
}
