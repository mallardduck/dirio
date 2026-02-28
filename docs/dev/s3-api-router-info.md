# S3 API Router Info

This is a list of the S3 API actions and how they could be mapped to the `teapot-rotuer`.

## S3 API Routes and Actions

| Scope     | S3 Action Name                  | Verb   | Path            | Query Param             | Description                                            |
|-----------|---------------------------------|--------|-----------------|-------------------------|--------------------------------------------------------|
| Service   | ListBuckets                     | GET    | /               | —                       | Lists all buckets; the "entry point" for most clients. |
| Bucket    | CreateBucket                    | PUT    | /{bucket}       | —                       | Creates a new bucket (handle LocationConstraint).      |
| Bucket    | DeleteBucket                    | DELETE | /{bucket}       | —                       | Deletes a bucket (must be empty).                      |
| Bucket    | HeadBucket                      | HEAD   | /{bucket}       | —                       | Checks if a bucket exists and you have access.         |
| Bucket    | ListObjectsV2                   | GET    | /{bucket}       | list-type=2             | Critical. Modern method for listing objects.           |
| Bucket    | ListObjects                     | GET    | /{bucket}       | —                       | Legacy listing (v1). Still required for old SDKs.      |
| Bucket    | GetBucketLocation               | GET    | /{bucket}       | location                | Returns the region string (e.g., us-east-1).           |
| Bucket    | GetBucketVersioning             | GET    | /{bucket}       | versioning              | Returns Enabled or Suspended.                          |
| Bucket    | PutBucketVersioning             | PUT    | /{bucket}       | versioning              | Enables/disables versioning for the bucket.            |
| Bucket    | GetBucketAcl                    | GET    | /{bucket}       | acl                     | Returns bucket-level permissions.                      |
| Bucket    | PutBucketAcl                    | PUT    | /{bucket}       | acl                     | Sets bucket-level permissions.                         |
| Bucket    | GetBucketPolicy                 | GET    | /{bucket}       | policy                  | Returns the JSON bucket policy.                        |
| Bucket    | PutBucketPolicy                 | PUT    | /{bucket}       | policy                  | Sets the JSON bucket policy.                           |
| Bucket    | DeleteBucketPolicy              | DELETE | /{bucket}       | policy                  | Removes the bucket policy.                             |
| Bucket    | GetBucketCors                   | GET    | /{bucket}       | cors                    | Critical for browser-based (JS) uploads.               |
| Bucket    | PutBucketCors                   | PUT    | /{bucket}       | cors                    | Sets Cross-Origin Resource Sharing rules.              |
| Bucket    | GetBucketLifecycle              | GET    | /{bucket}       | lifecycle               | Returns expiration/transition rules.                   |
| Bucket    | PutBucketLifecycle              | PUT    | /{bucket}       | lifecycle               | Sets automatic data deletion/transition.               |
| Bucket    | PutPublicAccessBlock            | PUT    | /{bucket}       | publicAccessBlock       | New standard. Restricts public access.                 |
| Object    | PutObject                       | PUT    | /{bucket}/{key} | —                       | Core. Uploads a single object (max 5GB).               |
| Object    | GetObject                       | GET    | /{bucket}/{key} | —                       | Core. Downloads object (supports Range).               |
| Object    | HeadObject                      | HEAD   | /{bucket}/{key} | —                       | Core. Gets size/type without downloading data.         |
| Object    | DeleteObject                    | DELETE | /{bucket}/{key} | —                       | Core. Removes a single object.                         |
| Object    | DeleteObjects                   | POST   | /{bucket}       | delete                  | Critical. Bulk delete (up to 1,000 objects).           |
| Object    | CopyObject                      | PUT    | /{bucket}/{key} | —                       | Server-side copy using x-amz-copy-source.              |
| Object    | GetObjectTagging                | GET    | /{bucket}/{key} | tagging                 | Returns key-value tags.                                |
| Object    | PutObjectTagging                | PUT    | /{bucket}/{key} | tagging                 | Sets key-value tags.                                   |
| Object    | GetObjectLegalHold              | GET    | /{bucket}/{key} | legal-hold              | Compliance: Checks if object is under legal hold.      |
| Object    | PutObjectRetention              | PUT    | /{bucket}/{key} | retention               | Compliance: Sets WORM/Immutability date.               |
| Multipart | CreateMultipartUpload           | POST   | /{bucket}/{key} | uploads                 | Starts the process for files >5GB.                     |
| Multipart | UploadPart                      | PUT    | /{bucket}/{key} | partNumber=X&uploadId=Y | Uploads a specific chunk of a large file.              |
| Multipart | UploadPartCopy                  | PUT    | /{bucket}/{key} | partNumber=X&uploadId=Y | Copies an existing object part as a new part.          |
| Multipart | CompleteMultipartUpload         | POST   | /{bucket}/{key} | uploadId=Y              | Merges all parts into a final object.                  |
| Multipart | AbortMultipartUpload            | DELETE | /{bucket}/{key} | uploadId=Y              | Stops upload and cleans up temp storage.               |
| Multipart | ListParts                       | GET    | /{bucket}/{key} | uploadId=Y              | Lists parts uploaded for an active ID.                 |
| Multipart | ListMultipartUploads            | GET    | /{bucket}       | uploads                 | Lists all unfinished multipart uploads.                |
| Security  | PutPublicAccessBlock            | PUT    | /{bucket}       | publicAccessBlock       | Blocks all public access to a bucket.                  |
| Security  | GetPublicAccessBlock            | GET    | /{bucket}       | publicAccessBlock       | Retrieves the current block settings.                  |
| Locking   | PutObjectLockConfiguration      | PUT    | /{bucket}       | object-lock             | Enables WORM (Write Once Read Many).                   |
| Locking   | GetObjectRetention              | GET    | /{bucket}/{key} | retention               | Gets retention date (governance/compliance).           |
| Locking   | PutObjectRetention              | PUT    | /{bucket}/{key} | retention               | Sets how long an object is immutable.                  |
| Locking   | PutObjectLegalHold              | PUT    | /{bucket}/{key} | legal-hold              | Places an indefinite hold on an object.                |
| Lifecycle | PutBucketLifecycleConfiguration | PUT    | /{bucket}       | lifecycle               | Auto-delete or transition old data.                    |
| Lifecycle | GetBucketLifecycleConfiguration | GET    | /{bucket}       | lifecycle               | Returns the lifecycle rule set.                        |
| Logging   | PutBucketLogging                | PUT    | /{bucket}       | logging                 | Sets where server access logs are sent.                |
| Events    | PutBucketNotification           | PUT    | /{bucket}       | notification            | Triggers events (Lambda/SQS) on upload.                |
| Payment   | GetBucketRequestPayment         | GET    | /{bucket}       | requestPayment          | Required by many SDKs to check "Requester Pays."       |
| Integrity | GetObjectAttributes             | GET    | /{bucket}/{key} | attributes              | Returns ETag, size, and storage class.                 |
| Archives  | GetObjectTorrent                | GET    | /{bucket}/{key} | torrent                 | Returns Bencoded torrent file (Legacy).                |
| Analysis  | GetBucketAnalyticsConfiguration | GET    | /{bucket}       | analytics               | Analyzes storage usage patterns.                       |


### S3 API Operations Implementation Tier

**Tier 1 (Must Have - Core Functionality):**

* All Service operations (ListBuckets)
* Core Bucket ops (CreateBucket, DeleteBucket, HeadBucket, ListObjectsV2, GetBucketLocation)
* Core Object ops (PutObject, GetObject, DeleteObject, HeadObject, CopyObject)
* All Multipart ops (CreateMultipartUpload, UploadPart, CompleteMultipartUpload, AbortMultipartUpload, ListParts, ListMultipartUploads)
* 
**Tier 2 (Highly Recommended - Essential for Production):**

* DeleteObjects (bulk delete efficiency)
* ListObjects (legacy v1 support for older SDKs)
* GetBucketVersioning, PutBucketVersioning
* GetBucketCors, PutBucketCors (critical for browser-based uploads)
* GetObjectTagging, PutObjectTagging
* UploadPartCopy

**Tier 3 (Important - Security & Compliance):**

* GetBucketAcl, PutBucketAcl
* GetBucketPolicy, PutBucketPolicy, DeleteBucketPolicy
* PutPublicAccessBlock, GetPublicAccessBlock
* GetBucketRequestPayment (SDK compatibility)

**Tier 4 (Standard Features - Lifecycle & Automation):**

* GetBucketLifecycle, PutBucketLifecycle
* GetBucketLifecycleConfiguration, PutBucketLifecycleConfiguration
* PutBucketNotification
* PutBucketLogging
* GetObjectAttributes

**Tier 5 (Advanced - Compliance & Locking):**

* PutObjectLockConfiguration
* GetObjectRetention, PutObjectRetention
* GetObjectLegalHold, PutObjectLegalHold

**Tier 6 (Optional - Analytics & Legacy):**

* GetBucketAnalyticsConfiguration
* GetObjectTorrent (legacy feature, rarely used)

### `teapot-router` Implementation Example

```go
// setupRoutes demonstrates a comprehensive S3 API implementation
// This showcases the router's capabilities for handling complex APIs with:
//   - Multiple HTTP methods on same paths
//   - Query parameter-based routing
//   - Path parameters with wildcards
//   - Named routes and actions
func setupRoutes() *teapot.Router {
	router := teapot.New()

	// ==================== SERVICE-LEVEL OPERATIONS ====================
	// S3 service-level operations (no bucket in path)
	router.GET("/", listBuckets).Name("s3.service.list-buckets").Action("api:s3:ListBuckets")

	// ==================== BUCKET OPERATIONS ====================
	// Mix of direct routes (PUT, DELETE, HEAD, GET) and query-based routes (QueryGET, QueryPUT).
	// The router automatically promotes to dispatcher-based routing when needed.

	router.PUT("/{bucket}", createBucket).Name("s3.bucket.create").Action("api:s3:CreateBucket")
	router.DELETE("/{bucket}", deleteBucket).Name("s3.bucket.delete").Action("api:s3:DeleteBucket")
	router.HEAD("/{bucket}", headBucket).Name("s3.bucket.head").Action("api:s3:HeadBucket")
	router.GET("/{bucket}", listObjectsV1).Name("s3.bucket.list-objects-v1").Action("api:s3:ListObjects")

	// Query-based bucket operations
	// ListObjectsV2 (preferred over v1)
	router.QueryGET("/{bucket}", listObjectsV2).QueryValue("list-type", "2").Name("s3.bucket.list-objects-v2").Action("api:s3:ListObjectsV2")

	// Bucket configuration endpoints
	router.QueryGET("/{bucket}", getBucketLocation).Query("location").Name("s3.bucket.get-location").Action("api:s3:GetBucketLocation")
	router.QueryGET("/{bucket}", getBucketVersioning).Query("versioning").Name("s3.bucket.get-versioning").Action("api:s3:GetBucketVersioning")
	router.QueryPUT("/{bucket}", putBucketVersioning).Query("versioning").Name("s3.bucket.put-versioning").Action("api:s3:PutBucketVersioning")
	router.QueryGET("/{bucket}", getBucketAcl).Query("acl").Name("s3.bucket.get-acl").Action("api:s3:GetBucketAcl")
	router.QueryPUT("/{bucket}", putBucketAcl).Query("acl").Name("s3.bucket.put-acl").Action("api:s3:PutBucketAcl")

	// Bucket policy endpoints
	router.QueryGET("/{bucket}", getBucketPolicy).Query("policy").Name("s3.bucket.get-policy").Action("api:s3:GetBucketPolicy")
	router.QueryPUT("/{bucket}", putBucketPolicy).Query("policy").Name("s3.bucket.put-policy").Action("api:s3:PutBucketPolicy")
	router.QueryDELETE("/{bucket}", deleteBucketPolicy).Query("policy").Name("s3.bucket.delete-policy").Action("api:s3:DeleteBucketPolicy")

	// Bucket CORS endpoints
	router.QueryGET("/{bucket}", getBucketCors).Query("cors").Name("s3.bucket.get-cors").Action("api:s3:GetBucketCors")
	router.QueryPUT("/{bucket}", putBucketCors).Query("cors").Name("s3.bucket.put-cors").Action("api:s3:PutBucketCors")

	// Bucket lifecycle configuration
	// Note: Legacy GetBucketLifecycle/PutBucketLifecycle share the same path and query param
	//       as the modern *Configuration variants; one route per method covers both.
	router.QueryGET("/{bucket}", getBucketLifecycleConfiguration).Query("lifecycle").Name("s3.bucket.get-lifecycle-configuration").Action("api:s3:GetBucketLifecycleConfiguration")
	router.QueryPUT("/{bucket}", putBucketLifecycleConfiguration).Query("lifecycle").Name("s3.bucket.put-lifecycle-configuration").Action("api:s3:PutBucketLifecycleConfiguration")

	// Public access block
	router.QueryGET("/{bucket}", getPublicAccessBlock).Query("publicAccessBlock").Name("s3.bucket.get-public-access-block").Action("api:s3:GetPublicAccessBlock")
	router.QueryPUT("/{bucket}", putPublicAccessBlock).Query("publicAccessBlock").Name("s3.bucket.put-public-access-block").Action("api:s3:PutPublicAccessBlock")

	// Object lock configuration
	router.QueryPUT("/{bucket}", putObjectLockConfiguration).Query("object-lock").Name("s3.bucket.put-object-lock-configuration").Action("api:s3:PutObjectLockConfiguration")

	// Logging, events, payment, and analytics
	router.QueryPUT("/{bucket}", putBucketLogging).Query("logging").Name("s3.bucket.put-logging").Action("api:s3:PutBucketLogging")
	router.QueryPUT("/{bucket}", putBucketNotification).Query("notification").Name("s3.bucket.put-notification").Action("api:s3:PutBucketNotification")
	router.QueryGET("/{bucket}", getBucketRequestPayment).Query("requestPayment").Name("s3.bucket.get-request-payment").Action("api:s3:GetBucketRequestPayment")
	router.QueryGET("/{bucket}", getBucketAnalyticsConfiguration).Query("analytics").Name("s3.bucket.get-analytics-configuration").Action("api:s3:GetBucketAnalyticsConfiguration")

	// List object versions (for versioned buckets)
	router.QueryGET("/{bucket}", listObjectVersions).Query("versions").Name("s3.bucket.list-object-versions").Action("api:s3:ListObjectVersions")

	// List multipart uploads in bucket
	router.QueryGET("/{bucket}", listMultipartUploads).Query("uploads").Name("s3.bucket.list-multipart-uploads").Action("api:s3:ListMultipartUploads")

	// Bulk delete objects
	router.QueryPOST("/{bucket}", deleteObjects).Query("delete").Name("s3.bucket.delete-objects").Action("api:s3:DeleteObjects")

	// ==================== OBJECT OPERATIONS ====================
	// Direct routes for operations without query params
	router.GET("/{bucket}/{key:.*}", getObject).Name("s3.object.get").Action("api:s3:GetObject")
	router.PUT("/{bucket}/{key:.*}", putObject).Name("s3.object.put").Action("api:s3:PutObject")
	router.DELETE("/{bucket}/{key:.*}", deleteObject).Name("s3.object.delete").Action("api:s3:DeleteObject")
	router.HEAD("/{bucket}/{key:.*}", headObject).Name("s3.object.head").Action("api:s3:HeadObject")
	// Note: CopyObject uses PUT /{bucket}/{key} with x-amz-copy-source header.
	//       The putObject handler detects this header and can adjust action context
	//       for logging/metrics (e.g., override to "api:s3:CopyObject")

	// Query-based object operations
	router.QueryGET("/{bucket}/{key:.*}", getObjectAcl).Query("acl").Name("s3.object.get-acl").Action("api:s3:GetObjectAcl")
	router.QueryPUT("/{bucket}/{key:.*}", putObjectAcl).Query("acl").Name("s3.object.put-acl").Action("api:s3:PutObjectAcl")

	// Object tagging
	router.QueryGET("/{bucket}/{key:.*}", getObjectTagging).Query("tagging").Name("s3.object.get-tagging").Action("api:s3:GetObjectTagging")
	router.QueryPUT("/{bucket}/{key:.*}", putObjectTagging).Query("tagging").Name("s3.object.put-tagging").Action("api:s3:PutObjectTagging")

	// Object legal hold and retention (compliance)
	// Note: PutObjectRetention appears under both "Object" and "Locking" scopes in the
	//       S3 API docs; it is a single route here, as is PutObjectLegalHold.
	router.QueryGET("/{bucket}/{key:.*}", getObjectLegalHold).Query("legal-hold").Name("s3.object.get-legal-hold").Action("api:s3:GetObjectLegalHold")
	router.QueryPUT("/{bucket}/{key:.*}", putObjectLegalHold).Query("legal-hold").Name("s3.object.put-legal-hold").Action("api:s3:PutObjectLegalHold")
	router.QueryGET("/{bucket}/{key:.*}", getObjectRetention).Query("retention").Name("s3.object.get-retention").Action("api:s3:GetObjectRetention")
	router.QueryPUT("/{bucket}/{key:.*}", putObjectRetention).Query("retention").Name("s3.object.put-retention").Action("api:s3:PutObjectRetention")

	// Object attributes and torrent (legacy)
	router.QueryGET("/{bucket}/{key:.*}", getObjectAttributes).Query("attributes").Name("s3.object.get-attributes").Action("api:s3:GetObjectAttributes")
	router.QueryGET("/{bucket}/{key:.*}", getObjectTorrent).Query("torrent").Name("s3.object.get-torrent").Action("api:s3:GetObjectTorrent")

	// ==================== MULTIPART UPLOAD OPERATIONS ====================
	// Initiate multipart upload
	router.QueryPOST("/{bucket}/{key:.*}", createMultipartUpload).Query("uploads").Name("s3.multipart.create").Action("api:s3:CreateMultipartUpload")

	// Upload part (requires both partNumber and uploadId query params)
	// Note: UploadPartCopy uses the same route with x-amz-copy-source header.
	//       The uploadPart handler detects this and can adjust action context accordingly.
	router.QueryPUT("/{bucket}/{key:.*}", uploadPart).Query("partNumber").Query("uploadId").Name("s3.multipart.upload-part").Action("api:s3:UploadPart")

	// Complete multipart upload
	router.QueryPOST("/{bucket}/{key:.*}", completeMultipartUpload).Query("uploadId").Name("s3.multipart.complete").Action("api:s3:CompleteMultipartUpload")

	// Abort multipart upload
	router.QueryDELETE("/{bucket}/{key:.*}", abortMultipartUpload).Query("uploadId").Name("s3.multipart.abort").Action("api:s3:AbortMultipartUpload")

	// List parts of a multipart upload
	router.QueryGET("/{bucket}/{key:.*}", listParts).Query("uploadId").Name("s3.multipart.list-parts").Action("api:s3:ListParts")

	// ==================== DEBUG ROUTES ====================
	// Debug route (conditionally registered)
	if isDebug() {
		router.GET("/.internal/routes", teapot.NewListRoutesHandler(router, nil)).Name("debug.routes")
	}

	router.GET("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {}).Name("favicon")

	return router
}
```