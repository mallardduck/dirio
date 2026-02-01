package server

import (
	"io"
	"net/http"

	"github.com/mallardduck/dirio/internal/api"
	"github.com/mallardduck/dirio/internal/auth"
	"github.com/mallardduck/dirio/internal/middleware"
	"github.com/mallardduck/dirio/internal/router"
	"github.com/mallardduck/dirio/internal/server/debug"
	"github.com/mallardduck/dirio/internal/server/favicon"
)

// RouteDependencies contains all dependencies needed for route handlers
type RouteDependencies struct {
	Auth       *auth.Authenticator
	APIHandler *api.Handler
	Debug      bool
}

// SetupRoutes configures all application routes on the provided router.
// When deps is nil, routes are registered with nil handlers (for CLI route listing).
func SetupRoutes(r *router.Router, deps *RouteDependencies) {
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
	r.MiddlewareGroup(func(r *router.Router) {
		r.Get("/favicon.ico", faviconHandler, "favicon")
		r.Get("/healthz", notImplemented, "health")
		// Debug routes (only when debug mode is enabled)
		if deps != nil && deps.Debug {
			r.Get("/.internal/routes", debugHandler, "debug.routes")
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
		r.MiddlewareGroup(func(r *router.Router) {
			r.Use(deps.Auth.AuthMiddleware)

			// TODO: Make this prefix configurable via Settings (default: /minio/admin/v3)
			r.NameGroup("/minio/admin/v3", "admin", func(r *router.Router) {
				// User Management
				r.Get("/list-users", userHandler.ListHandler, "users.list")
				r.Put("/add-user", userHandler.AddHandler, "users.add")
				r.Post("/remove-user", userHandler.RemoveHandler, "users.remove")
				r.Get("/user-info", userHandler.InfoHandler, "users.info")
				r.Post("/set-user-status", userHandler.StatusHandler, "users.setstatus")

				// Service Account Management
				r.Get("/list-service-accounts", notImplemented, "serviceaccounts.list")
				r.Post("/add-service-account", notImplemented, "serviceaccounts.add")
				r.Post("/delete-service-account", notImplemented, "serviceaccounts.delete")
				r.Get("/info-service-account", notImplemented, "serviceaccounts.info")
				r.Post("/update-service-account", notImplemented, "serviceaccounts.update")

				// Group Management
				r.Post("/update-group-members", notImplemented, "groups.updatemembers")
				r.Get("/group", notImplemented, "groups.info")
				r.Get("/groups", notImplemented, "groups.list")
				r.Post("/set-group-status", notImplemented, "groups.setstatus")

				// Policy Management (Canned Policies)
				r.Get("/list-canned-policies", policyHandler.ListHandler, "policies.list")
				r.Post("/add-canned-policy", policyHandler.AddHandler, "policies.add")
				r.Put("/add-canned-policy", policyHandler.AddHandler, "policies.add")
				r.Post("/remove-canned-policy", policyHandler.RemoveHandler, "policies.remove")
				r.Get("/info-canned-policy", policyHandler.InfoHandler, "policies.info")

				// Policy Attachments
				// Old API: mc admin policy set (deprecated)
				r.Post("/set-policy", policyHandler.SetHandler, "policies.set")
				// New API: mc admin policy attach --user=...
				r.Post("/idp/builtin/policy/attach", policyHandler.SetHandler, "policies.attach")
				r.Get("/policy-entities", policyHandler.ListEntitiesHandler, "policies.entities")

				// Server Info & Health (useful for mc admin)
				r.Get("/info", notImplemented, "server.info")
				r.Get("/health", notImplemented, "server.health")
			})
		})
	}

	// Authenticated Bucket and Object Routes - must go at end due to wildcards
	r.MiddlewareGroup(func(r *router.Router) {
		if deps != nil {
			r.Use(deps.Auth.AuthMiddleware)
			// Chunked encoding middleware
			r.Use(middleware.ChunkedEncoding(func(r io.Reader) io.Reader {
				return auth.NewChunkedReader(r)
			}))
		}

		// Root - ListBuckets
		r.Get("/", listBuckets, "index")

		// Bucket operations
		r.Head("/{bucket}", bucketHead, "buckets.head")
		r.Put("/{bucket}", bucketStore, "buckets.store")
		r.Get("/{bucket}", bucketShow, "buckets.show")
		r.Delete("/{bucket}", bucketDestroy, "buckets.destroy")

		// Object operations
		r.Head("/{bucket}/*", objectHead, "objects.head")
		r.Put("/{bucket}/*", objectStore, "objects.create")
		r.Get("/{bucket}/*", objectShow, "objects.show")
		r.Delete("/{bucket}/*", objectDestroy, "objects.destroy")
	})
}

// notImplemented is a placeholder for MinIO Admin API endpoints
// MinIO Admin API typically returns JSON responses
func (d *RouteDependencies) notImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	// MinIO Admin API error format
	w.Write([]byte(`{"status":"error","error":"This Admin API operation is not yet implemented"}`))
}
