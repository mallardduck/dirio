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
	})

	// Debug routes (only when debug mode is enabled)
	if deps != nil && deps.Debug {
		r.MiddlewareGroup(func(r *router.Router) {
			r.Get("/.internal/routes", debugHandler, "debug.routes")
		})
	}

	// Authenticated Routes
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

	// IAM API Routes
	r.MiddlewareGroup(func(r *router.Router) {
		if deps != nil {
			r.Use(deps.Auth.AuthMiddleware)
		}

		r.NameGroup("/api/iam", "iam", func(r *router.Router) {
			// User Management
			r.Get("/users", notImplemented, "users.list")
			r.Post("/users", notImplemented, "users.create")
			r.Get("/users/{username}", notImplemented, "users.get")
			r.Put("/users/{username}", notImplemented, "users.update")
			r.Delete("/users/{username}", notImplemented, "users.delete")

			// Group Management
			r.Post("/groups", notImplemented, "groups.create")
			r.Get("/groups", notImplemented, "groups.list")
			r.Get("/groups/{groupname}", notImplemented, "groups.get")
			r.Delete("/groups/{groupname}", notImplemented, "groups.delete")
			r.Post("/groups/{groupname}/users/{username}", notImplemented, "groups.adduser")
			r.Delete("/groups/{groupname}/users/{username}", notImplemented, "groups.removeuser")

			// Role Management
			r.Post("/roles", notImplemented, "roles.create")
			r.Get("/roles", notImplemented, "roles.list")
			r.Get("/roles/{rolename}", notImplemented, "roles.get")
			r.Delete("/roles/{rolename}", notImplemented, "roles.delete")

			// Policy Management
			r.Post("/policies", notImplemented, "policies.create")
			r.Get("/policies", notImplemented, "policies.list")
			r.Get("/policies/{policyarn}", notImplemented, "policies.get")
			r.Delete("/policies/{policyarn}", notImplemented, "policies.delete")

			// Policy Attachments - Users
			r.Post("/users/{username}/policies/{policyarn}", notImplemented, "users.attachpolicy")
			r.Delete("/users/{username}/policies/{policyarn}", notImplemented, "users.detachpolicy")
			r.Put("/users/{username}/policies/inline/{policyname}", notImplemented, "users.putpolicy")
			r.Delete("/users/{username}/policies/inline/{policyname}", notImplemented, "users.deletepolicy")

			// Policy Attachments - Groups
			r.Post("/groups/{groupname}/policies/{policyarn}", notImplemented, "groups.attachpolicy")
			r.Delete("/groups/{groupname}/policies/{policyarn}", notImplemented, "groups.detachpolicy")
			r.Put("/groups/{groupname}/policies/inline/{policyname}", notImplemented, "groups.putpolicy")
			r.Delete("/groups/{groupname}/policies/inline/{policyname}", notImplemented, "groups.deletepolicy")

			// Policy Attachments - Roles
			r.Post("/roles/{rolename}/policies/{policyarn}", notImplemented, "roles.attachpolicy")
			r.Delete("/roles/{rolename}/policies/{policyarn}", notImplemented, "roles.detachpolicy")
			r.Put("/roles/{rolename}/policies/inline/{policyname}", notImplemented, "roles.putpolicy")
			r.Delete("/roles/{rolename}/policies/inline/{policyname}", notImplemented, "roles.deletepolicy")

			// Access Key Management
			r.Post("/users/{username}/access-keys", notImplemented, "accesskeys.create")
			r.Get("/users/{username}/access-keys", notImplemented, "accesskeys.list")
			r.Put("/users/{username}/access-keys/{accesskeyid}", notImplemented, "accesskeys.update")
			r.Delete("/users/{username}/access-keys/{accesskeyid}", notImplemented, "accesskeys.delete")

			// Account & Authorization
			r.Get("/account/authorization-details", notImplemented, "account.authdetails")
			r.Post("/simulate-policy", notImplemented, "simulate.policy")
		})
	})
}

// notImplemented is a placeholder method needed for the route dependencies
func (d *RouteDependencies) notImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"This IAM operation is not yet implemented","code":"NotImplemented"}`))
}
