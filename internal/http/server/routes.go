package server

import (
	"io"
	"net/http"

	"github.com/mallardduck/teapot-router/pkg/teapot"

	"github.com/mallardduck/dirio/internal/http/api"
	auth2 "github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/http/server/favicon"
)

// RouteDependencies contains all dependencies needed for route handlers
type RouteDependencies struct {
	Auth       *auth2.Authenticator
	APIHandler *api.Handler
	Debug      bool
}

// SetupRoutes configures all application routes on the provided router.
// When deps is nil, routes are registered with nil handlers (for CLI route listing).
func SetupRoutes(r *teapot.Router, deps *RouteDependencies) {
	// Public routes (no auth required)
	r.GET("/favicon.ico", favicon.HandleFavicon).Name("favicon")
	r.GET("/.internal/routes", teapot.NewListRoutesHandler(r, nil)).Name("debug.routes").Action("ListRoutes")

	var ni http.HandlerFunc
	if deps != nil {
		ni = deps.notImplemented
	}
	r.GET("/healthz", ni).Name("health")

	// MinIO Admin API routes (authenticated)
	var adminMW []func(http.Handler) http.Handler
	if deps != nil {
		adminMW = []func(http.Handler) http.Handler{deps.Auth.AuthMiddleware}
	}
	r.MiddlewareGroup(func(r *teapot.Router) {
		// TODO: Make this prefix configurable via Settings (default: /minio/admin/v3)
		r.NamedGroup("/minio/admin/v3", "admin", func(r *teapot.Router) {
			setupAdminRoutes(r, deps, ni)
		})
	}, adminMW...)

	// S3 API routes (authenticated + chunked encoding)
	var s3MW []func(http.Handler) http.Handler
	if deps != nil {
		s3MW = []func(http.Handler) http.Handler{
			deps.Auth.AuthMiddleware,
			middleware.ChunkedEncoding(func(r io.Reader) io.Reader {
				return auth2.NewChunkedReader(r)
			}),
		}
	}
	r.MiddlewareGroup(func(r *teapot.Router) {
		setupS3Routes(r, deps)
	}, s3MW...)

	r.Finalize()
}

// setupAdminRoutes registers MinIO Admin API routes within an already-prefixed group.
func setupAdminRoutes(r *teapot.Router, deps *RouteDependencies, ni http.HandlerFunc) {
	var (
		userList, userAdd, userRemove, userInfo, userStatus                        http.HandlerFunc
		policyList, policyAdd, policyRemove, policyInfo, policySet, policyEntities http.HandlerFunc
	)
	if deps != nil && deps.APIHandler != nil && deps.APIHandler.IAMHandler != nil {
		uh := deps.APIHandler.IAMHandler.UserResourceHandler()
		userList = uh.ListHandler
		userAdd = uh.AddHandler
		userRemove = uh.RemoveHandler
		userInfo = uh.InfoHandler
		userStatus = uh.StatusHandler

		ph := deps.APIHandler.IAMHandler.PolicyResourceHandler()
		policyList = ph.ListHandler
		policyAdd = ph.AddHandler
		policyRemove = ph.RemoveHandler
		policyInfo = ph.InfoHandler
		policySet = ph.SetHandler
		policyEntities = ph.ListEntitiesHandler
	}

	// User Management
	r.GET("/list-users", userList).Name("users.list")
	r.PUT("/add-user", userAdd).Name("users.add")
	r.POST("/remove-user", userRemove).Name("users.remove")
	r.GET("/user-info", userInfo).Name("users.info")
	r.POST("/set-user-status", userStatus).Name("users.setstatus")

	// Service Account Management (not yet implemented)
	r.GET("/list-service-accounts", ni).Name("serviceaccounts.list")
	r.POST("/add-service-account", ni).Name("serviceaccounts.add")
	r.POST("/delete-service-account", ni).Name("serviceaccounts.delete")
	r.GET("/info-service-account", ni).Name("serviceaccounts.info")
	r.POST("/update-service-account", ni).Name("serviceaccounts.update")

	// Group Management (not yet implemented)
	r.POST("/update-group-members", ni).Name("groups.updatemembers")
	r.GET("/group", ni).Name("groups.info")
	r.GET("/groups", ni).Name("groups.list")
	r.POST("/set-group-status", ni).Name("groups.setstatus")

	// Policy Management
	r.GET("/list-canned-policies", policyList).Name("policies.list")
	r.POST("/add-canned-policy", policyAdd).Name("policies.add")
	r.PUT("/add-canned-policy", policyAdd).Name("policies.add")
	r.POST("/remove-canned-policy", policyRemove).Name("policies.remove")
	r.GET("/info-canned-policy", policyInfo).Name("policies.info")

	// Policy Attachments
	r.POST("/set-policy", policySet).Name("policies.set") // deprecated: mc admin policy set
	r.POST("/idp/builtin/policy/attach", policySet).Name("policies.attach")
	r.GET("/policy-entities", policyEntities).Name("policies.entities")

	// Server Info & Health (not yet implemented)
	r.GET("/info", ni).Name("server.info")
	r.GET("/health", ni).Name("server.health")
}

// setupS3Routes registers S3 API routes. Direct routes are registered first —
// they become fallbacks when query-dispatched routes are added to the same
// method+pattern via the router's auto-promotion logic.
func setupS3Routes(r *teapot.Router, deps *RouteDependencies) {
	var (
		listBuckets     http.HandlerFunc
		headBucket      http.HandlerFunc
		createBucket    http.HandlerFunc
		listObjects     http.HandlerFunc
		deleteBucket    http.HandlerFunc
		listObjectsV2   http.HandlerFunc
		getBucketLoc    http.HandlerFunc
		getBucketPolicy http.HandlerFunc
		putBucketPolicy http.HandlerFunc
		delBucketPolicy http.HandlerFunc
		headObject      http.HandlerFunc
		putObject       http.HandlerFunc
		getObject       http.HandlerFunc
		deleteObject    http.HandlerFunc
	)

	if deps != nil {
		s3h := deps.APIHandler.S3Handler

		// bucket wraps an S3 bucket-level handler, extracting {bucket} and
		// request ID from the incoming request.
		bucket := func(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				fn(w, r, teapot.URLParam(r, "bucket"))
			}
		}
		// object wraps an S3 object-level handler, extracting {bucket}, {key},
		// and request ID from the incoming request.
		object := func(fn func(http.ResponseWriter, *http.Request, string, string)) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				fn(w, r, teapot.URLParam(r, "bucket"), teapot.URLParam(r, "key"))
			}
		}

		listBuckets = s3h.ListBuckets
		headBucket = bucket(s3h.HeadBucket)
		createBucket = bucket(s3h.CreateBucket)
		listObjects = bucket(s3h.ListObjects)
		deleteBucket = bucket(s3h.DeleteBucket)
		listObjectsV2 = bucket(s3h.ListObjectsV2)
		getBucketLoc = bucket(s3h.GetBucketLocation)
		getBucketPolicy = bucket(s3h.GetBucketPolicy)
		putBucketPolicy = bucket(s3h.PutBucketPolicy)
		delBucketPolicy = bucket(s3h.DeleteBucketPolicy)
		headObject = object(s3h.HeadObject)
		putObject = object(s3h.PutObject)
		getObject = object(s3h.GetObject)
		deleteObject = object(s3h.DeleteObject)
	}

	// Service
	r.GET("/", listBuckets).Name("index").Action("ListBuckets")

	// Bucket — direct routes (become fallbacks when query routes are added)
	r.HEAD("/{bucket}", headBucket).Name("buckets.head").Action("HeadBucket")
	r.PUT("/{bucket}", createBucket).Name("buckets.store").Action("CreateBucket")
	r.GET("/{bucket}", listObjects).Name("buckets.show").Action("ListObjects")
	r.DELETE("/{bucket}", deleteBucket).Name("buckets.destroy").Action("DeleteBucket")

	// Bucket — query-dispatched operations
	r.QueryGET("/{bucket}", listObjectsV2).QueryValue("list-type", "2").Name("buckets.listv2").Action("ListObjectsV2")
	r.QueryGET("/{bucket}", getBucketLoc).Query("location").Name("buckets.location").Action("GetBucketLocation")
	r.QueryGET("/{bucket}", getBucketPolicy).Query("policy").Name("buckets.policy.show").Action("GetBucketPolicy")
	r.QueryPUT("/{bucket}", putBucketPolicy).Query("policy").Name("buckets.policy.store").Action("PutBucketPolicy")
	r.QueryDELETE("/{bucket}", delBucketPolicy).Query("policy").Name("buckets.policy.destroy").Action("DeleteBucketPolicy")

	// Object operations (use {key:.*} to capture entire path including slashes)
	r.HEAD("/{bucket}/{key:.*}", headObject).Name("objects.head").Action("HeadObject")
	r.PUT("/{bucket}/{key:.*}", putObject).Name("objects.store").Action("PutObject")
	r.GET("/{bucket}/{key:.*}", getObject).Name("objects.show").Action("GetObject")
	r.DELETE("/{bucket}/{key:.*}", deleteObject).Name("objects.destroy").Action("DeleteObject")
}

// notImplemented is a placeholder for unimplemented MinIO Admin API endpoints.
func (d *RouteDependencies) notImplemented(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"status":"error","error":"This Admin API operation is not yet implemented"}`))
}
