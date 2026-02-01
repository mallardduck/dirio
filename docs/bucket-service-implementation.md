# Bucket Service Layer Implementation

## Overview
Extended the service layer architecture to include bucket management operations, following the same clean CRUD pattern as User and Policy services.

## What Was Implemented

### 1. Package Structure

```
internal/service/
├── bucket/
│   ├── types.go                  # CreateBucketRequest, UpdateBucketRequest
│   └── bucket.go                 # BucketService implementation
├── validation/
│   └── bucket.go                 # S3 bucket name validation
├── errors/
│   └── errors.go                 # Updated with bucket errors
└── factory.go                    # Updated to include BucketService
```

### 2. Bucket Service Features

#### BucketService Methods
- `Create(ctx, req)` - Create bucket with S3-compliant name validation
- `Get(ctx, name)` - Retrieve bucket metadata
- `Exists(ctx, name)` - Check if bucket exists
- `Delete(ctx, name)` - Delete bucket (checks if empty)
- `List(ctx)` - List all bucket names
- `Update(ctx, name, req)` - Update bucket metadata (e.g., bucket policy)

### 3. S3-Compliant Bucket Name Validation

Implements all AWS S3 bucket naming rules:

✅ **Length**: 3-63 characters
✅ **Characters**: Lowercase letters, numbers, hyphens, periods
✅ **Start/End**: Must start and end with lowercase letter or number
✅ **Patterns**:
  - No consecutive periods
  - No periods adjacent to hyphens
  - Cannot be formatted as IP address (e.g., 192.168.1.1)
  - Cannot start with "xn--" (Punycode)
  - Cannot end with "-s3alias" (access points)

### 4. Error Handling

New domain errors added to `internal/service/errors/errors.go`:
- `ErrBucketNotFound` - Bucket doesn't exist
- `ErrBucketAlreadyExists` - Bucket already exists
- `ErrBucketNotEmpty` - Cannot delete non-empty bucket
- `ErrInvalidBucketName` - Bucket name validation failed

These map to S3 error codes:
- `ErrBucketNotFound` → NoSuchBucket (404)
- `ErrBucketAlreadyExists` → BucketAlreadyExists (409)
- `ErrBucketNotEmpty` → BucketNotEmpty (409)
- `ErrInvalidBucketName` → InvalidBucketName (400)

### 5. Updated ServicesFactory

```go
// Access the bucket service
func (f *ServicesFactory) Bucket() *bucket.Service
```

## Usage Examples

### Creating a Bucket

```go
bucketService := services.Bucket()
bucketMeta, err := bucketService.Create(ctx, &bucket.CreateBucketRequest{
    Name:  "my-bucket",
    Owner: "user-123",
})
if err != nil {
    if svcerrors.IsAlreadyExists(err) {
        // Handle bucket already exists
    }
    if svcerrors.IsValidation(err) {
        // Handle invalid bucket name
    }
}
```

### Checking Bucket Existence

```go
exists, err := bucketService.Exists(ctx, "my-bucket")
if err != nil {
    // Handle error
}
if !exists {
    // Bucket doesn't exist
}
```

### Deleting a Bucket

```go
err := bucketService.Delete(ctx, "my-bucket")
if err != nil {
    if errors.Is(err, svcerrors.ErrBucketNotEmpty) {
        // Cannot delete non-empty bucket
    }
    if errors.Is(err, svcerrors.ErrBucketNotFound) {
        // Bucket doesn't exist
    }
}
```

## Optional: Refactoring S3 Handlers

The S3 bucket handlers in `internal/api/s3/bucket.go` can optionally be refactored to use the bucket service. This would:

1. **Centralize validation** - Bucket name validation moves from handler to service
2. **Consistent error handling** - Service errors map directly to S3 error codes
3. **Better testability** - Business logic can be unit tested independently of HTTP layer

### Example Refactor for CreateBucket

**Current (direct storage access):**
```go
func (h *Handler) CreateBucket(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
    // Validate bucket name
    if err := ValidateS3BucketName(bucket); err != nil {
        writeErrorResponse(w, requestID, s3types.ErrInvalidBucketName, err)
        return
    }

    if err := h.storage.CreateBucket(r.Context(), bucket); err != nil {
        if errors.Is(err, storage.ErrBucketExists) {
            writeErrorResponse(w, requestID, s3types.ErrBucketAlreadyExists, err)
            return
        }
        writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
        return
    }

    w.WriteHeader(http.StatusOK)
}
```

**Refactored (using bucket service):**
```go
func (h *Handler) CreateBucket(w http.ResponseWriter, r *http.Request, bucket, requestID string) {
    bucketService := h.services.Bucket()

    _, err := bucketService.Create(r.Context(), &bucketpkg.CreateBucketRequest{
        Name:  bucket,
        Owner: getOwnerFromContext(r), // Get authenticated user
    })

    if err != nil {
        // Map service errors to S3 error codes
        if svcerrors.IsValidation(err) {
            writeErrorResponse(w, requestID, s3types.ErrInvalidBucketName, err)
            return
        }
        if svcerrors.IsAlreadyExists(err) {
            writeErrorResponse(w, requestID, s3types.ErrBucketAlreadyExists, err)
            return
        }
        writeErrorResponse(w, requestID, s3types.ErrInternalError, err)
        return
    }

    location := h.urlBuilder.BucketURL(r, bucket)
    w.Header().Set("Location", location)
    w.WriteHeader(http.StatusOK)
}
```

## Benefits

1. ✅ **S3-compliant validation** - Comprehensive bucket name validation per AWS spec
2. ✅ **Consistent error handling** - Service errors map cleanly to S3 error codes
3. ✅ **Reusable business logic** - Same validation/logic for API, CLI, imports
4. ✅ **Better testability** - Unit test bucket logic without HTTP mocking
5. ✅ **Follows established pattern** - Matches User and Policy service architecture

## Compilation Status

✅ **All code compiles successfully** - `go build ./...` completes without errors

## Next Steps (Optional)

### 1. Refactor S3 Handlers
Update `internal/api/s3/bucket.go` to use the bucket service:
- CreateBucket → `bucketService.Create()`
- DeleteBucket → `bucketService.Delete()`
- HeadBucket → `bucketService.Exists()`
- GetBucketLocation → `bucketService.Get()`

### 2. Add Bucket Policy Support
Implement bucket policy update in metadata.Manager:
```go
// SaveBucketMetadata in metadata.Manager
func (m *Manager) SaveBucketMetadata(ctx context.Context, meta *BucketMetadata) error
```

Then update `BucketService.Update()` to persist bucket policy changes.

### 3. Unit Tests
Create comprehensive tests:
- `internal/service/bucket/bucket_test.go`
- `internal/service/validation/bucket_test.go`

### 4. Object Service
Apply the same pattern to object operations:
- `internal/service/object/object.go`
- `internal/service/validation/object.go`
