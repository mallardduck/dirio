# S3 Service Layer - Complete Implementation

## Summary

Successfully refactored the service layer into a **unified S3 service** that handles both bucket and object operations, matching the S3 API structure. The service layer now has three main services: **User**, **Policy**, and **S3**.

## What Changed

### ✅ Renamed bucket → s3
- `internal/service/bucket/` → `internal/service/s3/`
- Service now handles BOTH buckets and objects (matches S3 API paradigm)
- `ServicesFactory.Bucket()` → `ServicesFactory.S3()`

### ✅ S3 Service Operations

#### Bucket Operations (6 methods)
- `CreateBucket(ctx, req)` - Create bucket with S3-compliant name validation
- `GetBucket(ctx, bucket)` - Get bucket metadata
- `HeadBucket(ctx, bucket)` - Check if bucket exists
- `DeleteBucket(ctx, bucket)` - Delete bucket (checks if empty)
- `ListBuckets(ctx)` - List all buckets
- `GetBucketLocation(ctx, bucket)` - Get bucket region/location

#### Object Operations (7 methods)
- `PutObject(ctx, req)` - Upload object with metadata
- `GetObject(ctx, req)` - Download object
- `HeadObject(ctx, req)` - Get object metadata (no download)
- `DeleteObject(ctx, req)` - Delete object (idempotent per S3 spec)
- `ListObjects(ctx, req)` - List objects V1
- `ListObjectsV2(ctx, req)` - List objects V2 (preferred)
- `ObjectExists(ctx, bucket, key)` - Check if object exists

####Helper/Future Methods
- `CopyObject()` - Copy object between locations (placeholder)
- `GetObjectWithRange()` - Range requests (placeholder)

**Total:** 13+ S3 operations

## Service Method Names Match S3 API

All service methods now use S3 operation names:
- ✅ `CreateBucket` (not "Create")
- ✅ `PutObject` (not "Upload")
- ✅ `GetObject` (not "Download")
- ✅ `DeleteObject` (not "Remove")
- ✅ `HeadBucket` / `HeadObject` (not "Exists")
- ✅ `ListObjects` / `ListObjectsV2`

This makes the service layer API match the S3 protocol exactly!

## Validation

### Bucket Names (S3-compliant)
- 3-63 characters
- Lowercase letters, numbers, hyphens, periods
- Must start/end with lowercase letter or number
- No consecutive periods or period-hyphen adjacency
- Cannot be IP address format
- Cannot start with "xn--" or end with "-s3alias"

### Object Keys (S3-compliant)
- 1-1024 bytes (UTF-8 encoded)
- Cannot be empty
- Cannot start with '/'
- No control characters (except tab, newline, carriage return)
- Must be valid UTF-8

## Error Handling

Added object-specific errors:
- `ErrObjectNotFound` → NoSuchKey (404)
- `ErrInvalidObjectKey` → InvalidObjectKey (400)

All errors map directly to S3 error codes!

## Request/Response Types

Structured types for all operations:

**Bucket:**
- `CreateBucketRequest`
- `UpdateBucketRequest`

**Object:**
- `PutObjectRequest` - with Content (io.Reader), ContentType, CustomMetadata
- `GetObjectRequest` + `GetObjectResponse` - includes all metadata
- `HeadObjectRequest` + `HeadObjectResponse` - metadata only
- `DeleteObjectRequest`
- `ListObjectsRequest`
- `ListObjectsV2Request`

## Files Created/Modified

### Created:
1. ✅ `internal/service/s3/s3.go` - Unified S3 service (buckets + objects)
2. ✅ `internal/service/s3/types.go` - All bucket and object request/response types
3. ✅ `internal/service/validation/object.go` - Object key validation

### Modified:
1. ✅ `internal/service/factory.go` - S3() instead of Bucket()
2. ✅ `internal/service/errors/errors.go` - Added object errors
3. ✅ `internal/api/s3/s3.go` - Updated to use s3Service

## Compilation Status

✅ **S3 Service compiles successfully**

Remaining work:
- Refactor `internal/api/s3/bucket.go` to use `h.s3Service` methods
- Refactor `internal/api/s3/object.go` to use `h.s3Service` methods

## Current Architecture

```
internal/service/
├── s3/                          # S3 Service (buckets + objects)
│   ├── s3.go                    # 13+ operations
│   └── types.go                 # Request/response types
├── user/                        # IAM User Service
│   ├── user.go                  # 7 operations
│   └── types.go
├── policy/                      # IAM Policy Service
│   ├── policy.go                # 5 operations
│   └── types.go
├── validation/
│   ├── bucket.go                # Bucket name validation
│   ├── object.go                # Object key validation
│   ├── user.go                  # User validation
│   └── policy.go                # Policy validation
├── errors/
│   └── errors.go                # Domain errors
└── factory.go                   # ServicesFactory

Total: 25+ service methods across 3 domains
```

## Next Step: Refactor HTTP Handlers

The S3 HTTP handlers currently call `h.storage` directly. They should be refactored to use `h.s3Service`:

### Example: PutObject Handler

**Current:**
```go
func (h *HTTPHandler) PutObject(w http.ResponseWriter, r *http.Request, bucket, key, requestID string) {
    // Validate key
    if err := ValidateS3Key(key); err != nil {
        writeErrorResponse(w, requestID, s3types.ErrInvalidObjectKey, err)
        return
    }

    // Extract metadata...
    customMetadata := make(map[string]string)
    // ...

    // Call storage directly
    etag, err := h.storage.PutObject(ctx, bucket, key, r.Body, contentType, customMetadata)
    // Error handling...
}
```

**Refactored:**
```go
func (h *HTTPHandler) PutObject(w http.ResponseWriter, r *http.Request, bucket, key, requestID string) {
    // Extract metadata...
    customMetadata := make(map[string]string)
    // ... (metadata extraction logic stays in handler)

    // Use S3 service (validation happens inside)
    etag, err := h.s3Service.PutObject(r.Context(), &svcs3.PutObjectRequest{
        Bucket:         bucket,
        Key:            key,
        Content:        r.Body,
        ContentType:    contentType,
        CustomMetadata: customMetadata,
    })

    if err != nil {
        // Map service errors to S3 error codes
        if svcerrors.IsNotFound(err) {
            writeErrorResponse(w, requestID, s3types.ErrNoSuchBucket, err)
            return
        }
        if svcerrors.IsValidation(err) {
            writeErrorResponse(w, requestID, s3types.ErrInvalidObjectKey, err)
            return
        }
        writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
        return
    }

    w.Header().Set("ETag", etag)
    w.WriteHeader(http.StatusOK)
}
```

### Benefits of Refactoring Handlers

1. ✅ **Centralized validation** - No more duplicate validation in handlers
2. ✅ **Consistent error handling** - Service errors map directly to S3 error codes
3. ✅ **Cleaner handlers** - Focus on HTTP concerns (headers, status codes)
4. ✅ **Testable business logic** - Can unit test S3 operations without HTTP
5. ✅ **Reusable code** - Same validation/logic for API, CLI, imports

## Handler Refactoring Checklist

### Bucket Handlers (`internal/api/s3/bucket.go`)
- [ ] `CreateBucket` → use `s3Service.CreateBucket()`
- [ ] `HeadBucket` → use `s3Service.HeadBucket()`
- [ ] `DeleteBucket` → use `s3Service.DeleteBucket()`
- [ ] `GetBucketLocation` → use `s3Service.GetBucketLocation()`
- [ ] `ListObjects` → use `s3Service.ListObjects()`
- [ ] `ListObjectsV2` → use `s3Service.ListObjectsV2()`

### Object Handlers (`internal/api/s3/object.go`)
- [ ] `PutObject` → use `s3Service.PutObject()`
- [ ] `GetObject` → use `s3Service.GetObject()`
- [ ] `HeadObject` → use `s3Service.HeadObject()`
- [ ] `DeleteObject` → use `s3Service.DeleteObject()`

## Architecture Benefits

The unified S3 service provides:

1. **Natural grouping** - Buckets and objects belong together (like in S3 API)
2. **Method name consistency** - Matches S3 operation names exactly
3. **Single import** - Just import `service/s3` instead of separate bucket/object packages
4. **Easier discovery** - All S3 operations in one place
5. **Matches mental model** - Users think "S3 operations", not "bucket vs object"

---

**Status:** ✅ S3 Service Layer Complete - Ready for handler refactoring
