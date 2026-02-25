package server

import (
	"io"
	"net/http"
	"net/http/pprof"

	"github.com/go-git/go-billy/v5"
	"github.com/mallardduck/teapot-router/pkg/teapot"

	"github.com/mallardduck/dirio/internal/http/api"
	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/http/middleware"
	httpresponse "github.com/mallardduck/dirio/internal/http/response"
	"github.com/mallardduck/dirio/internal/http/server/favicon"
	"github.com/mallardduck/dirio/internal/http/server/health"
	miniohttp "github.com/mallardduck/dirio/internal/minio/http"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/policy"
)

// RouteDependencies contains all dependencies needed for route handlers.
type RouteDependencies struct {
	Auth         *auth.Authenticator
	PolicyEngine *policy.Engine
	Metadata     *metadata.Manager      // For ownership-based authorization
	AdminKeys    policy.AdminKeyChecker // Live admin key source (auth.Authenticator)
	APIHandler   *api.Handler
	RootFS       billy.Filesystem // For health probes
	Debug        bool
}

// SetupRoutes configures all application routes on the provided router.
// When deps is nil, routes are registered with nil handlers (for CLI route listing).
func SetupRoutes(r *teapot.Router, deps *RouteDependencies) {
	// Favicon must be at site the root for full compatability
	r.Func().GET("/favicon.ico", favicon.HandleFavicon).Name("favicon")
	// /.dirio/* — DirIO-specific routes.
	// Dot-prefix guarantees no collision with S3 bucket names (bucket names must
	// start with a letter or digit per the S3 spec and AWS validation rules).
	r.GET("/.dirio/routes", teapot.NewListRoutesHandler(r, nil)).Name("debug.routes").Action("dirio:ListRoutes")

	// DirIO health endpoints (unauthenticated).
	// These are under /.dirio/ so they never collide with user bucket names.
	var healthMeta health.Pinger
	var healthFS billy.Filesystem
	if deps != nil {
		healthMeta = deps.Metadata
		healthFS = deps.RootFS
	}
	health.RegisterRoutes(r, healthMeta, healthFS)

	// pprof profiling endpoints — only registered when --debug is set.
	// Unauthenticated: debug mode is not intended for production use.
	if deps != nil && deps.Debug {
		r.Func().GET("/debug/pprof/", pprof.Index)
		r.Func().GET("/debug/pprof/cmdline", pprof.Cmdline)
		r.Func().GET("/debug/pprof/profile", pprof.Profile)
		r.Func().GET("/debug/pprof/symbol", pprof.Symbol)
		r.Func().POST("/debug/pprof/symbol", pprof.Symbol)
		r.Func().GET("/debug/pprof/trace", pprof.Trace)
		r.Func().GET("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
		r.Func().GET("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
		r.Func().GET("/debug/pprof/allocs", pprof.Handler("allocs").ServeHTTP)
		r.Func().GET("/debug/pprof/block", pprof.Handler("block").ServeHTTP)
		r.Func().GET("/debug/pprof/mutex", pprof.Handler("mutex").ServeHTTP)
		r.Func().GET("/debug/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
	}

	// MinIO Admin API routes (authenticated)
	var iamHandler *miniohttp.Handler
	var adminMW []func(http.Handler) http.Handler
	if deps != nil {
		iamHandler = deps.APIHandler.IAMHandler
		adminMW = []func(http.Handler) http.Handler{deps.Auth.AuthMiddleware}
	}
	r.MiddlewareGroup(func(r *teapot.Router) {
		r.NamedGroup("/minio/admin/v3", "admin", func(r *teapot.Router) {
			miniohttp.RegisterAdminRouter(r, iamHandler)
		})
	}, adminMW...)

	// S3 API routes (authenticated + chunked encoding)
	var s3Deps *s3RouteDeps
	var s3MW []func(http.Handler) http.Handler
	if deps != nil {
		s3Deps = &s3RouteDeps{
			listBuckets:             deps.APIHandler.S3Handler.ListBuckets,
			headBucket:              bucket(deps.APIHandler.S3Handler.HeadBucket),
			createBucket:            bucket(deps.APIHandler.S3Handler.CreateBucket),
			listObjects:             bucket(deps.APIHandler.S3Handler.ListObjects),
			deleteBucket:            bucket(deps.APIHandler.S3Handler.DeleteBucket),
			postObject:              bucket(deps.APIHandler.S3Handler.PostObject),
			listObjectsV2:           bucket(deps.APIHandler.S3Handler.ListObjectsV2),
			getBucketLocation:       bucket(deps.APIHandler.S3Handler.GetBucketLocation),
			getBucketPolicy:         bucket(deps.APIHandler.S3Handler.GetBucketPolicy),
			putBucketPolicy:         bucket(deps.APIHandler.S3Handler.PutBucketPolicy),
			delBucketPolicy:         bucket(deps.APIHandler.S3Handler.DeleteBucketPolicy),
			getBucketVersioning:     httpresponse.NotImplemented,
			putBucketVersioning:     httpresponse.NotImplemented,
			getBucketACL:            httpresponse.NotImplemented,
			putBucketACL:            httpresponse.NotImplemented,
			getBucketCors:           httpresponse.NotImplemented,
			putBucketCors:           httpresponse.NotImplemented,
			listObjectVersions:      httpresponse.NotImplemented,
			listMultipartUploads:    httpresponse.NotImplemented,
			deleteObjects:           bucket(deps.APIHandler.S3Handler.DeleteObjects),
			headObject:              object(deps.APIHandler.S3Handler.HeadObject),
			putObject:               object(deps.APIHandler.S3Handler.PutObject),
			copyObject:              object(deps.APIHandler.S3Handler.CopyObject),
			getObject:               object(deps.APIHandler.S3Handler.GetObject),
			deleteObject:            object(deps.APIHandler.S3Handler.DeleteObject),
			getObjectACL:            httpresponse.NotImplemented,
			putObjectACL:            httpresponse.NotImplemented,
			getObjectTagging:        object(deps.APIHandler.S3Handler.GetObjectTagging),
			putObjectTagging:        object(deps.APIHandler.S3Handler.PutObjectTagging),
			multipartCreate:         object(deps.APIHandler.S3Handler.CreateMultipartUpload),
			multipartUploadPart:     object(deps.APIHandler.S3Handler.UploadPart),
			multipartUploadPartCopy: object(deps.APIHandler.S3Handler.UploadPartCopy),
			multipartComplete:       object(deps.APIHandler.S3Handler.CompleteMultipartUpload),
			multipartAbort:          object(deps.APIHandler.S3Handler.AbortMultipartUpload),
			multipartListParts:      object(deps.APIHandler.S3Handler.ListParts),
		}

		// Build authorization middleware config
		authzConfig := &policy.AuthorizationConfig{
			Engine:    deps.PolicyEngine,
			Metadata:  deps.Metadata,
			AdminKeys: deps.AdminKeys,
		}

		s3MW = []func(http.Handler) http.Handler{
			deps.Auth.AuthMiddleware,
			policy.AuthorizationMiddleware(authzConfig),
			middleware.ChunkedEncoding(func(r io.Reader) io.Reader {
				return auth.NewChunkedReader(r)
			}),
		}
	}
	r.MiddlewareGroup(func(r *teapot.Router) {
		setupS3Routes(r, s3Deps)
	}, s3MW...)

	r.Finalize()
}

// bucket wraps an S3 bucket-level handler, extracting {bucket} from
// the incoming request. It also applies S3 bucket name validation middleware.
func bucket(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	// Create the base handler that extracts parameters
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fn(w, r, teapot.URLParam(r, "bucket"))
	})

	// Apply validation middleware
	validated := middleware.ValidateS3BucketNameMiddleware(
		func(r *http.Request) string { return teapot.URLParam(r, "bucket") },
		api.WriteErrorResponse,
	)(baseHandler)

	return validated.ServeHTTP
}

// object wraps an S3 object-level handler, extracting {bucket}, {key},
// and request ID from the incoming request. It also applies S3 bucket name and key validation middleware.
func object(fn func(http.ResponseWriter, *http.Request, string, string)) http.HandlerFunc {
	// Create the base handler that extracts parameters
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fn(w, r, teapot.URLParam(r, "bucket"), teapot.URLParam(r, "key"))
	})

	// Apply bucket name validation middleware first
	validated := middleware.ValidateS3BucketNameMiddleware(
		func(r *http.Request) string { return teapot.URLParam(r, "bucket") },
		api.WriteErrorResponse,
	)(baseHandler)

	// Then apply key validation middleware
	validated = middleware.ValidateS3KeyMiddleware(
		func(r *http.Request) string { return teapot.URLParam(r, "key") },
		api.WriteErrorResponse,
	)(validated)

	return validated.ServeHTTP
}

type s3RouteDeps struct {
	// Service
	listBuckets http.HandlerFunc
	// Bucket — direct routes (become fallbacks when query routes are added)
	headBucket   http.HandlerFunc
	createBucket http.HandlerFunc
	listObjects  http.HandlerFunc
	deleteBucket http.HandlerFunc
	postObject   http.HandlerFunc
	// Bucket — query-dispatched operations
	listObjectsV2        http.HandlerFunc
	getBucketLocation    http.HandlerFunc
	getBucketPolicy      http.HandlerFunc
	putBucketPolicy      http.HandlerFunc
	delBucketPolicy      http.HandlerFunc
	getBucketVersioning  http.HandlerFunc
	putBucketVersioning  http.HandlerFunc
	getBucketACL         http.HandlerFunc
	putBucketACL         http.HandlerFunc
	getBucketCors        http.HandlerFunc
	putBucketCors        http.HandlerFunc
	listObjectVersions   http.HandlerFunc
	listMultipartUploads http.HandlerFunc
	deleteObjects        http.HandlerFunc
	// Object — direct routes (use {key:.*} to capture entire path including slashes)
	headObject   http.HandlerFunc
	putObject    http.HandlerFunc
	copyObject   http.HandlerFunc
	getObject    http.HandlerFunc
	deleteObject http.HandlerFunc
	// Object — query-dispatched operations
	getObjectACL     http.HandlerFunc
	putObjectACL     http.HandlerFunc
	getObjectTagging http.HandlerFunc
	putObjectTagging http.HandlerFunc
	// Multipart upload operations
	multipartCreate         http.HandlerFunc
	multipartUploadPart     http.HandlerFunc
	multipartUploadPartCopy http.HandlerFunc
	multipartComplete       http.HandlerFunc
	multipartAbort          http.HandlerFunc
	multipartListParts      http.HandlerFunc
}

// setupS3Routes registers S3 API routes. Direct routes are registered first —
// they become fallbacks when query-dispatched routes are added to the same
// method+pattern via the router's auto-promotion logic.
func setupS3Routes(r *teapot.Router, deps *s3RouteDeps) {
	if deps == nil {
		deps = &s3RouteDeps{}
	}

	// Service
	r.GET("/", deps.listBuckets).Name("index").Action("s3:ListBuckets")

	// Bucket — direct routes (become fallbacks when query routes are added)
	r.HEAD("/{bucket}", deps.headBucket).Name("buckets.head").Action("s3:HeadBucket")
	r.PUT("/{bucket}", deps.createBucket).Name("buckets.store").Action("s3:CreateBucket")
	r.GET("/{bucket}", deps.listObjects).Name("buckets.show").Action("s3:ListObjects")
	r.DELETE("/{bucket}", deps.deleteBucket).Name("buckets.destroy").Action("s3:DeleteBucket")

	// POST Policy Uploads (browser-based form upload via multipart/form-data)
	// Credentials are embedded in the form body — auth middleware handles authentication,
	// authz middleware skips (no Action), and the handler validates policy conditions.
	// Spec: https://docs.aws.amazon.com/AmazonS3/latest/API/RESTObjectPOST.html
	r.POST("/{bucket}", deps.postObject).Name("buckets.post-policy-upload")

	// Query-based bucket operations
	// ListObjectsV2 (preferred over v1)
	r.QueryGET("/{bucket}", deps.listObjectsV2).QueryValue("list-type", "2").Name("buckets.listv2").Action("s3:ListObjectsV2")

	// Bucket configuration endpoints
	r.QueryGET("/{bucket}", deps.getBucketLocation).Query("location").Name("buckets.location").Action("s3:GetBucketLocation")
	r.QueryGET("/{bucket}", deps.getBucketVersioning).Query("versioning").Name("buckets.versioning.show").Action("s3:GetBucketVersioning")
	r.QueryPUT("/{bucket}", deps.putBucketVersioning).Query("versioning").Name("buckets.versioning.store").Action("s3:PutBucketVersioning")
	r.QueryGET("/{bucket}", deps.getBucketACL).Query("acl").Name("buckets.acl.show").Action("s3:GetBucketAcl")
	r.QueryPUT("/{bucket}", deps.putBucketACL).Query("acl").Name("buckets.acl.store").Action("s3:PutBucketAcl")

	// Bucket policy endpoints
	r.QueryGET("/{bucket}", deps.getBucketPolicy).Query("policy").Name("buckets.policy.show").Action("s3:GetBucketPolicy")
	r.QueryPUT("/{bucket}", deps.putBucketPolicy).Query("policy").Name("buckets.policy.store").Action("s3:PutBucketPolicy")
	r.QueryDELETE("/{bucket}", deps.delBucketPolicy).Query("policy").Name("buckets.policy.destroy").Action("s3:DeleteBucketPolicy")

	// Bucket CORS endpoints
	r.QueryGET("/{bucket}", deps.getBucketCors).Query("cors").Name("buckets.cors.show").Action("s3:GetBucketCors")
	r.QueryPUT("/{bucket}", deps.putBucketCors).Query("cors").Name("buckets.cors.store").Action("s3:PutBucketCors")

	// Bucket lifecycle configuration
	// Note: Legacy GetBucketLifecycle/PutBucketLifecycle share the same path and query param
	//       as the modern *Configuration variants; one route per method covers both.
	r.Func().QueryGET("/{bucket}", httpresponse.NotImplemented).Query("lifecycle").Name("bucket.get-lifecycle-configuration").Action("s3:GetBucketLifecycleConfiguration")
	r.Func().QueryPUT("/{bucket}", httpresponse.NotImplemented).Query("lifecycle").Name("bucket.put-lifecycle-configuration").Action("s3:PutBucketLifecycleConfiguration")

	// Public access block
	r.Func().QueryGET("/{bucket}", httpresponse.NotImplemented).Query("publicAccessBlock").Name("bucket.get-public-access-block").Action("s3:GetPublicAccessBlock")
	r.Func().QueryPUT("/{bucket}", httpresponse.NotImplemented).Query("publicAccessBlock").Name("bucket.put-public-access-block").Action("s3:PutPublicAccessBlock")

	// Object lock configuration
	r.Func().QueryPUT("/{bucket}", httpresponse.NotImplemented).Query("object-lock").Name("bucket.put-object-lock-configuration").Action("s3:PutObjectLockConfiguration")

	// List object versions (for versioned buckets)
	r.QueryGET("/{bucket}", deps.listObjectVersions).Query("versions").Name("buckets.versions").Action("s3:ListObjectVersions")

	// List multipart uploads in bucket
	r.QueryGET("/{bucket}", deps.listMultipartUploads).Query("uploads").Name("buckets.uploads").Action("s3:ListMultipartUploads")

	// Bulk delete objects
	r.QueryPOST("/{bucket}", deps.deleteObjects).Query("delete").Name("buckets.delete-objects").Action("s3:DeleteObjects")

	// ==================== OBJECT OPERATIONS ====================
	r.GET("/{bucket}/{key:.*}", deps.getObject).Name("objects.show").Action("s3:GetObject")
	r.DELETE("/{bucket}/{key:.*}", deps.deleteObject).Name("objects.destroy").Action("s3:DeleteObject")
	r.HEAD("/{bucket}/{key:.*}", deps.headObject).Name("objects.head").Action("s3:HeadObject")

	// PUT /{bucket}/{key} dispatches on X-Amz-Copy-Source header.
	// UploadPart / UploadPartCopy also live here: same method+path, and header
	// presence distinguishes the copy variant.  The remaining QueryPUT routes
	// below (acl, tagging, …) are added to this same dispatcher automatically.
	// PUT /{bucket}/{key} dispatcher
	// TODO(Phase 3.2 #4): Implement CopyObject handler (currently httpresponse.NotImplemented)
	//   - Parse X-Amz-Copy-Source header (bucket/key format)
	//   - Policy engine already supports dual permission checks (source read + dest write)
	//   - Copy object metadata, content-type, and custom metadata
	//   - Handle copy-if-* conditional headers (If-Match, If-None-Match, If-Modified-Since, If-Unmodified-Since)
	//   - Test: aws s3 cp s3://bucket/src.txt s3://bucket/dest.txt
	//   - See policy/middleware.go:169 for multi-resource action handling
	r.Dispatch("PUT", "/{bucket}/{key:.*}", func(d *teapot.DispatchBuilder, m teapot.Matchers) {
		// Query-based operations must come before default
		d.When(m.QueryExists("partNumber"), m.QueryExists("uploadId"), m.HeaderExists(consts.HeaderCopySource)).Do(deps.multipartUploadPartCopy).Name("multipart.upload-part-copy").Action("s3:UploadPartCopy")
		d.When(m.QueryExists("partNumber"), m.QueryExists("uploadId")).Do(deps.multipartUploadPart).Name("multipart.upload-part").Action("s3:UploadPart")
		d.When(m.QueryExists("acl")).Do(deps.putObjectACL).Name("objects.acl.store").Action("s3:PutObjectAcl")
		d.When(m.QueryExists("tagging")).Do(deps.putObjectTagging).Name("objects.tagging.store").Action("s3:PutObjectTagging")

		// Header-based copy operation
		d.When(m.HeaderExists(consts.HeaderCopySource)).Do(deps.copyObject).Name("object.copy").Action("s3:CopyObject")

		// Default: regular PUT object
		d.Default(deps.putObject).Name("object.put").Action("s3:PutObject")
	})

	// Query-based object operations
	r.QueryGET("/{bucket}/{key:.*}", deps.getObjectACL).Query("acl").Name("objects.acl.show").Action("s3:GetObjectAcl")

	// Object tagging
	r.QueryGET("/{bucket}/{key:.*}", deps.getObjectTagging).Query("tagging").Name("objects.tagging.show").Action("s3:GetObjectTagging")

	// Multipart upload operations
	r.QueryPOST("/{bucket}/{key:.*}", deps.multipartCreate).Query("uploads").Name("multipart.create").Action("s3:CreateMultipartUpload")
	r.QueryPOST("/{bucket}/{key:.*}", deps.multipartComplete).Query("uploadId").Name("multipart.complete").Action("s3:CompleteMultipartUpload")
	r.QueryDELETE("/{bucket}/{key:.*}", deps.multipartAbort).Query("uploadId").Name("multipart.abort").Action("s3:AbortMultipartUpload")
	r.QueryGET("/{bucket}/{key:.*}", deps.multipartListParts).Query("uploadId").Name("multipart.list-parts").Action("s3:ListParts")
}
