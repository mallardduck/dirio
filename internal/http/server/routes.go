package server

import (
	"io"
	"net/http"

	"github.com/mallardduck/teapot-router/pkg/teapot"

	"github.com/mallardduck/dirio/internal/http/api"
	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/http/server/favicon"
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
	// Public routes (no auth required)
	r.GET("/favicon.ico", favicon.HandleFavicon).Name("favicon")
	r.GET("/.internal/routes", teapot.NewListRoutesHandler(r, nil)).Name("debug.routes").Action("ListRoutes")

	r.GET("/healthz", RouteNotImplemented).Name("health")

	// MinIO Admin API routes (authenticated)
	var adminDeps *adminRouteDeps
	var adminMW []func(http.Handler) http.Handler
	if deps != nil {
		userHandler := deps.APIHandler.IAMHandler.UserResourceHandler()
		policyHandler := deps.APIHandler.IAMHandler.PolicyResourceHandler()

		adminDeps = &adminRouteDeps{
			listUsers:             userHandler.ListHandler,
			addUser:               userHandler.AddHandler,
			removeUser:            userHandler.RemoveHandler,
			getUserInfo:           userHandler.InfoHandler,
			setUserStatus:         userHandler.StatusHandler,
			listServiceAccounts:   RouteNotImplemented,
			addServiceAccount:     RouteNotImplemented,
			deleteServiceAccount:  RouteNotImplemented,
			getServiceAccountInfo: RouteNotImplemented,
			updateServiceAccount:  RouteNotImplemented,
			updateGroupMembers:    RouteNotImplemented,
			getGroupInfo:          RouteNotImplemented,
			listGroups:            RouteNotImplemented,
			setGroupStatus:        RouteNotImplemented,
			listCannedPolicies:    policyHandler.ListHandler,
			addCannedPolicy:       policyHandler.AddHandler,
			deleteCannedPolicy:    policyHandler.RemoveHandler,
			getCannedPolicyInfo:   policyHandler.InfoHandler,
			setPolicy:             policyHandler.SetHandler,
			attachPolicy:          policyHandler.AddHandler,
			listPolicyEntities:    policyHandler.ListEntitiesHandler,
			Info:                  RouteNotImplemented,
			Health:                RouteNotImplemented,
		}

		adminMW = []func(http.Handler) http.Handler{deps.Auth.AuthMiddleware}
	}
	r.MiddlewareGroup(func(r *teapot.Router) {
		r.NamedGroup("/minio/admin/v3", "admin", func(r *teapot.Router) {
			setupAdminRoutes(r, adminDeps)
		})
	}, adminMW...)

	// S3 API routes (authenticated + chunked encoding)
	var s3Deps *s3RouteDeps
	var s3MW []func(http.Handler) http.Handler
	if deps != nil {
		s3Deps = &s3RouteDeps{
			listBuckets:          deps.APIHandler.S3Handler.ListBuckets,
			headBucket:           bucket(deps.APIHandler.S3Handler.HeadBucket),
			createBucket:         bucket(deps.APIHandler.S3Handler.CreateBucket),
			listObjects:          bucket(deps.APIHandler.S3Handler.ListObjects),
			deleteBucket:         bucket(deps.APIHandler.S3Handler.DeleteBucket),
			listObjectsV2:        bucket(deps.APIHandler.S3Handler.ListObjectsV2),
			getBucketLocation:    bucket(deps.APIHandler.S3Handler.GetBucketLocation),
			getBucketPolicy:      bucket(deps.APIHandler.S3Handler.GetBucketPolicy),
			putBucketPolicy:      bucket(deps.APIHandler.S3Handler.PutBucketPolicy),
			delBucketPolicy:      bucket(deps.APIHandler.S3Handler.DeleteBucketPolicy),
			getBucketVersioning:  RouteNotImplemented,
			putBucketVersioning:  RouteNotImplemented,
			getBucketACL:         RouteNotImplemented,
			putBucketACL:         RouteNotImplemented,
			listObjectVersions:   RouteNotImplemented,
			listMultipartUploads: RouteNotImplemented,
			deleteObjects:        RouteNotImplemented,
			headObject:           object(deps.APIHandler.S3Handler.HeadObject),
			putObject:            object(deps.APIHandler.S3Handler.PutObject),
			getObject:            object(deps.APIHandler.S3Handler.GetObject),
			deleteObject:         object(deps.APIHandler.S3Handler.DeleteObject),
			getObjectACL:         RouteNotImplemented,
			putObjectACL:         RouteNotImplemented,
			multipartCreate:      RouteNotImplemented,
			multipartUploadPart:  RouteNotImplemented,
			multipartComplete:    RouteNotImplemented,
			multipartAbort:       RouteNotImplemented,
			multipartListParts:   RouteNotImplemented,
		}

		s3MW = []func(http.Handler) http.Handler{
			deps.Auth.AuthMiddleware,
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

// bucket wraps an S3 bucket-level handler, extracting {bucket} and
// request ID from the incoming request.
func bucket(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fn(w, r, teapot.URLParam(r, "bucket"))
	}
}

// object wraps an S3 object-level handler, extracting {bucket}, {key},
// and request ID from the incoming request.
func object(fn func(http.ResponseWriter, *http.Request, string, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fn(w, r, teapot.URLParam(r, "bucket"), teapot.URLParam(r, "key"))
	}
}

type adminRouteDeps struct {
	// User Management
	listUsers     http.HandlerFunc
	addUser       http.HandlerFunc
	removeUser    http.HandlerFunc
	getUserInfo   http.HandlerFunc
	setUserStatus http.HandlerFunc
	// Service Account Management (not yet implemented)
	listServiceAccounts   http.HandlerFunc
	addServiceAccount     http.HandlerFunc
	deleteServiceAccount  http.HandlerFunc
	getServiceAccountInfo http.HandlerFunc
	updateServiceAccount  http.HandlerFunc
	// Group Management (not yet implemented)
	updateGroupMembers http.HandlerFunc
	getGroupInfo       http.HandlerFunc
	listGroups         http.HandlerFunc
	setGroupStatus     http.HandlerFunc
	// Policy Management
	listCannedPolicies  http.HandlerFunc
	addCannedPolicy     http.HandlerFunc
	deleteCannedPolicy  http.HandlerFunc
	getCannedPolicyInfo http.HandlerFunc
	// Policy Attachments
	setPolicy          http.HandlerFunc
	attachPolicy       http.HandlerFunc
	listPolicyEntities http.HandlerFunc
	// Server Info & Health (not yet implemented)
	Info   http.HandlerFunc
	Health http.HandlerFunc
}

// setupAdminRoutes registers MinIO Admin API routes within an already-prefixed group.
func setupAdminRoutes(r *teapot.Router, deps *adminRouteDeps) {
	// TODO: eventually this could be cleaned up maybe
	if deps == nil {
		deps = &adminRouteDeps{}
	}

	// User Management
	r.GET("/list-users", deps.listUsers).Name("users.list")
	r.PUT("/add-user", deps.addUser).Name("users.add")
	r.POST("/remove-user", deps.removeUser).Name("users.remove")
	r.GET("/user-info", deps.getUserInfo).Name("users.info")
	r.POST("/set-user-status", deps.setUserStatus).Name("users.setstatus")

	// Service Account Management (not yet implemented)
	r.GET("/list-service-accounts", deps.listServiceAccounts).Name("serviceaccounts.list")
	r.POST("/add-service-account", deps.addServiceAccount).Name("serviceaccounts.add")
	r.POST("/delete-service-account", deps.deleteServiceAccount).Name("serviceaccounts.delete")
	r.GET("/info-service-account", deps.getServiceAccountInfo).Name("serviceaccounts.info")
	r.POST("/update-service-account", deps.updateServiceAccount).Name("serviceaccounts.update")

	// Group Management (not yet implemented)
	r.POST("/update-group-members", deps.updateGroupMembers).Name("groups.updatemembers")
	r.GET("/group", deps.getGroupInfo).Name("groups.info")
	r.GET("/groups", deps.listGroups).Name("groups.list")
	r.POST("/set-group-status", deps.setGroupStatus).Name("groups.setstatus")

	// Policy Management
	r.GET("/list-canned-policies", deps.listCannedPolicies).Name("policies.list")
	r.POST("/add-canned-policy", deps.addCannedPolicy).Name("policies.add")
	r.PUT("/add-canned-policy", deps.addCannedPolicy).Name("policies.add")
	r.POST("/remove-canned-policy", deps.deleteCannedPolicy).Name("policies.remove")
	r.GET("/info-canned-policy", deps.getCannedPolicyInfo).Name("policies.info")

	// Policy Attachments
	r.POST("/set-policy", deps.setPolicy).Name("policies.set") // deprecated: mc admin policy set
	r.POST("/idp/builtin/policy/attach", deps.attachPolicy).Name("policies.attach")
	r.GET("/policy-entities", deps.listPolicyEntities).Name("policies.entities")

	// Server Info & Health (not yet implemented)
	r.GET("/info", deps.Info).Name("server.info")
	r.GET("/health", deps.Health).Name("server.health")
}

type s3RouteDeps struct {
	// Service
	listBuckets http.HandlerFunc
	// Bucket — direct routes (become fallbacks when query routes are added)
	headBucket   http.HandlerFunc
	createBucket http.HandlerFunc
	listObjects  http.HandlerFunc
	deleteBucket http.HandlerFunc
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
	listObjectVersions   http.HandlerFunc
	listMultipartUploads http.HandlerFunc
	deleteObjects        http.HandlerFunc
	// Object — direct routes (use {key:.*} to capture entire path including slashes)
	headObject   http.HandlerFunc
	putObject    http.HandlerFunc
	getObject    http.HandlerFunc
	deleteObject http.HandlerFunc
	// Object — query-dispatched operations
	getObjectACL http.HandlerFunc
	putObjectACL http.HandlerFunc
	// Multipart upload operations
	multipartCreate     http.HandlerFunc
	multipartUploadPart http.HandlerFunc
	multipartComplete   http.HandlerFunc
	multipartAbort      http.HandlerFunc
	multipartListParts  http.HandlerFunc
}

// setupS3Routes registers S3 API routes. Direct routes are registered first —
// they become fallbacks when query-dispatched routes are added to the same
// method+pattern via the router's auto-promotion logic.
func setupS3Routes(r *teapot.Router, deps *s3RouteDeps) {
	// TODO: eventually this could be cleaned up maybe
	if deps == nil {
		deps = &s3RouteDeps{}
	}

	// Service
	r.GET("/", deps.listBuckets).Name("index").Action("ListBuckets")

	// Bucket — direct routes (become fallbacks when query routes are added)
	r.HEAD("/{bucket}", deps.headBucket).Name("buckets.head").Action("HeadBucket")
	r.PUT("/{bucket}", deps.createBucket).Name("buckets.store").Action("CreateBucket")
	r.GET("/{bucket}", deps.listObjects).Name("buckets.show").Action("ListObjects")
	r.DELETE("/{bucket}", deps.deleteBucket).Name("buckets.destroy").Action("DeleteBucket")

	// Bucket — query-dispatched operations
	r.QueryGET("/{bucket}", deps.listObjectsV2).QueryValue("list-type", "2").Name("buckets.listv2").Action("ListObjectsV2")
	r.QueryGET("/{bucket}", deps.getBucketLocation).Query("location").Name("buckets.location").Action("GetBucketLocation")
	r.QueryGET("/{bucket}", deps.getBucketPolicy).Query("policy").Name("buckets.policy.show").Action("GetBucketPolicy")
	r.QueryPUT("/{bucket}", deps.putBucketPolicy).Query("policy").Name("buckets.policy.store").Action("PutBucketPolicy")
	r.QueryDELETE("/{bucket}", deps.delBucketPolicy).Query("policy").Name("buckets.policy.destroy").Action("DeleteBucketPolicy")
	r.QueryGET("/{bucket}", deps.getBucketVersioning).Query("versioning").Name("buckets.versioning.show").Action("GetBucketVersioning")
	r.QueryPUT("/{bucket}", deps.putBucketVersioning).Query("versioning").Name("buckets.versioning.store").Action("PutBucketVersioning")
	r.QueryGET("/{bucket}", deps.getBucketACL).Query("acl").Name("buckets.acl.show").Action("GetBucketAcl")
	r.QueryPUT("/{bucket}", deps.putBucketACL).Query("acl").Name("buckets.acl.store").Action("PutBucketAcl")
	r.QueryGET("/{bucket}", deps.listObjectVersions).Query("versions").Name("buckets.versions").Action("ListObjectVersions")
	r.QueryGET("/{bucket}", deps.listMultipartUploads).Query("uploads").Name("buckets.uploads").Action("ListMultipartUploads")
	r.QueryPOST("/{bucket}", deps.deleteObjects).Query("delete").Name("buckets.delete-objects").Action("DeleteObjects")

	// Object — direct routes (use {key:.*} to capture entire path including slashes)
	r.HEAD("/{bucket}/{key:.*}", deps.headObject).Name("objects.head").Action("HeadObject")
	r.PUT("/{bucket}/{key:.*}", deps.putObject).Name("objects.store").Action("PutObject")
	// Note: CopyObject shares PUT /{bucket}/{key} with PutObject; it is distinguished
	// 			at the handler level by the presence of the x-amz-copy-source header.
	r.GET("/{bucket}/{key:.*}", deps.getObject).Name("objects.show").Action("GetObject")
	r.DELETE("/{bucket}/{key:.*}", deps.deleteObject).Name("objects.destroy").Action("DeleteObject")

	// Object — query-dispatched operations
	r.QueryGET("/{bucket}/{key:.*}", deps.getObjectACL).Query("acl").Name("objects.acl.show").Action("GetObjectAcl")
	r.QueryPUT("/{bucket}/{key:.*}", deps.putObjectACL).Query("acl").Name("objects.acl.store").Action("PutObjectAcl")

	// Multipart upload operations
	r.QueryPOST("/{bucket}/{key:.*}", deps.multipartCreate).Query("uploads").Name("multipart.create").Action("CreateMultipartUpload")
	// Note: UploadPartCopy shares this route with UploadPart; it is distinguished
	// 			at the handler level by the presence of the x-amz-copy-source header.
	r.QueryPUT("/{bucket}/{key:.*}", deps.multipartUploadPart).Query("partNumber").Query("uploadId").Name("multipart.upload-part").Action("UploadPart")
	r.QueryPOST("/{bucket}/{key:.*}", deps.multipartComplete).Query("uploadId").Name("multipart.complete").Action("CompleteMultipartUpload")
	r.QueryDELETE("/{bucket}/{key:.*}", deps.multipartAbort).Query("uploadId").Name("multipart.abort").Action("AbortMultipartUpload")
	r.QueryGET("/{bucket}/{key:.*}", deps.multipartListParts).Query("uploadId").Name("multipart.list-parts").Action("ListParts")
}

// RouteNotImplemented is a placeholder handler for routes that are registered
// but not yet implemented (Admin API, S3 API, etc.).
func RouteNotImplemented(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"status":"error","error":"This operation is not yet implemented"}`))
}
