# Bug #013: Multipart Upload Returns 405 Method Not Allowed (boto3)

**Status:** ✅ RESOLVED
**Priority:** High
**Discovered:** 2026-01-31
**Resolved:** 2026-02-16
**Affects:** boto3 (confirmed), likely AWS CLI
**Resolution:** Implemented all 5 multipart upload handlers (CreateMultipartUpload, UploadPart, UploadPartCopy, CompleteMultipartUpload, AbortMultipartUpload, ListParts)

## Summary

When using boto3 to perform multipart upload operations, the server returns `405 Method Not Allowed` for one or more of the multipart API calls. This blocks programmatic large file uploads via boto3.

## Evidence

### boto3 Test Output

```python
# Create multipart upload
mpu = s3.create_multipart_upload(Bucket=bucket, Key="multipart.txt")
upload_id = mpu["UploadId"]

# Upload parts
part1 = s3.upload_part(
    Bucket=bucket,
    Key="multipart.txt",
    UploadId=upload_id,
    PartNumber=1,
    Body=b"part1 content"
)
# Returns: 405 Method Not Allowed

# OR

# Complete multipart upload
s3.complete_multipart_upload(
    Bucket=bucket,
    Key="multipart.txt",
    UploadId=upload_id,
    MultipartUpload={"Parts": [...]}
)
# Returns: 405 Method Not Allowed

# Test result: FAIL - Multipart upload
```

## Reproduction Steps

1. Attempt multipart upload with boto3:
   ```python
   import boto3
   s3 = boto3.client("s3", endpoint_url="http://localhost:8080", ...)

   # Initiate multipart upload
   response = s3.create_multipart_upload(
       Bucket="bucket",
       Key="large-file.dat"
   )
   upload_id = response["UploadId"]

   # Upload first part
   part1 = s3.upload_part(
       Bucket="bucket",
       Key="large-file.dat",
       UploadId=upload_id,
       PartNumber=1,
       Body=b"part 1 data"
   )
   # Expected: Returns ETag for part
   # Actual: 405 Method Not Allowed
   ```

2. Or with AWS CLI:
   ```bash
   # Initiate multipart upload
   aws s3api create-multipart-upload \
     --bucket bucket \
     --key large-file.dat

   # Upload part
   aws s3api upload-part \
     --bucket bucket \
     --key large-file.dat \
     --part-number 1 \
     --upload-id "upload-id-here" \
     --body part1.dat
   # Expected: Returns ETag
   # Actual: 405 Method Not Allowed (likely)
   ```

## Root Cause Analysis

The S3 multipart upload API uses query parameters to differentiate operations:
- **CreateMultipartUpload:** `POST /bucket/object?uploads`
- **UploadPart:** `PUT /bucket/object?partNumber=1&uploadId=xyz`
- **CompleteMultipartUpload:** `POST /bucket/object?uploadId=xyz`
- **AbortMultipartUpload:** `DELETE /bucket/object?uploadId=xyz`
- **ListParts:** `GET /bucket/object?uploadId=xyz`

Likely causes:
1. **Routing issue:** Query parameters not handled in routing configuration
2. **Method not registered:** POST/PUT/DELETE not registered for these query param combinations
3. **Handler missing:** Multipart handlers not implemented or not wired up
4. **Query parameter routing:** Router doesn't match requests with specific query params to correct handlers

This is similar to bug #004 (object tagging query param routing issue).

Location to investigate:
- `internal/api/router.go` - Route registration and query parameter handling
- `internal/api/handlers/multipart.go` - Multipart upload handlers
- HTTP method registration for multipart endpoints
- Query parameter matching in routing

## Impact

**Functionality Broken:**
- Cannot upload large files via boto3 (>5MB typically triggers multipart)
- Programmatic large file uploads impossible
- Blocks boto3 workflows for large data

**Current Workarounds:**
- Use MinIO mc instead (mc multipart works, but has bug #005 corruption issue)
- Upload small files only (<5MB)
- Use AWS CLI (status unknown, likely also fails)

**Clients Affected:**
- ✅ boto3: Confirmed - multipart operations return 405
- ⚠️ MinIO mc: Multipart works but corrupts content (bug #005)
- ❓ AWS CLI: Needs verification (likely also fails)

## Current Behavior

| Operation | boto3 | MinIO mc | Notes |
|-----------|-------|----------|-------|
| CreateMultipartUpload | ❌ 405? | ✅ Works | Need to verify exact failing operation |
| UploadPart | ❌ 405 | ✅ Works | mc uploads but content corrupted (bug #005) |
| CompleteMultipartUpload | ❌ 405? | ✅ Works | mc completes but content corrupted |
| AbortMultipartUpload | ❓ | ❓ | Not tested |
| ListParts | ❓ | ❓ | Not tested |

**Note:** Need to determine which specific multipart operation(s) return 405 for boto3.

## Proposed Fix

### Phase 1: Identify Failing Operation
1. Add detailed logging to identify which multipart operation returns 405
2. Check if it's CreateMultipartUpload, UploadPart, or CompleteMultipartUpload
3. Verify routing configuration for that operation

### Phase 2: Fix Routing
1. Update `internal/api/router.go` to handle multipart query parameters:
   - `POST /bucket/object?uploads` → CreateMultipartUpload handler
   - `PUT /bucket/object?partNumber=X&uploadId=Y` → UploadPart handler
   - `POST /bucket/object?uploadId=Y` → CompleteMultipartUpload handler
   - `DELETE /bucket/object?uploadId=Y` → AbortMultipartUpload handler
   - `GET /bucket/object?uploadId=Y` → ListParts handler

2. Ensure query parameter routing works correctly
3. Register correct HTTP methods (POST, PUT, DELETE, GET) for each endpoint

### Phase 3: Verify Handlers Exist
1. Check `internal/api/handlers/multipart.go` for handler implementations
2. Implement missing handlers if needed
3. Wire handlers to routes

### Phase 4: Testing
1. Add integration tests for boto3 multipart upload
2. Test CreateMultipartUpload
3. Test UploadPart (multiple parts)
4. Test CompleteMultipartUpload
5. Test AbortMultipartUpload
6. Test ListParts
7. Verify end-to-end: upload large file, download, verify content integrity
8. **IMPORTANT:** Must first fix bug #001 (chunked encoding) to avoid content corruption

## Testing

Confirmed in: `tests/clients/scripts/boto3.py` (lines 250-294)

```python
# Multipart upload test
try:
    # Create multipart upload
    mpu = s3.create_multipart_upload(Bucket=bucket, Key="multipart.txt")
    upload_id = mpu["UploadId"]

    # Upload parts
    part1 = s3.upload_part(
        Bucket=bucket,
        Key="multipart.txt",
        UploadId=upload_id,
        PartNumber=1,
        Body=b"part1 content"
    )
    part2 = s3.upload_part(
        Bucket=bucket,
        Key="multipart.txt",
        UploadId=upload_id,
        PartNumber=2,
        Body=b"part2 content"
    )

    # Complete multipart upload
    s3.complete_multipart_upload(
        Bucket=bucket,
        Key="multipart.txt",
        UploadId=upload_id,
        MultipartUpload={
            "Parts": [
                {"PartNumber": 1, "ETag": part1["ETag"]},
                {"PartNumber": 2, "ETag": part2["ETag"]},
            ]
        }
    )

    # Verify content
    response = s3.get_object(Bucket=bucket, Key="multipart.txt")
    content = response["Body"].read()

    if content == b"part1 contentpart2 content":
        log_pass("Multipart upload")
    else:
        log_fail("Multipart upload", "content mismatch")
except Exception as e:
    log_fail("Multipart upload", str(e))
    # Exception likely includes "405 Method Not Allowed"
```

## Related Issues

- #004: Object Tagging query parameter routing issue (similar pattern)
- #005: Multipart Upload content corruption (MinIO mc) - Different issue (chunked encoding bug #001)
- #001: AWS SigV4 Chunked Encoding Corruption - Must be fixed for multipart to work correctly

## Technical Details

### S3 Multipart Upload API

**1. CreateMultipartUpload:**
```http
POST /bucket/object?uploads HTTP/1.1
Response: <UploadId>xyz</UploadId>
```

**2. UploadPart:**
```http
PUT /bucket/object?partNumber=1&uploadId=xyz HTTP/1.1
Content-Length: 5242880

{part data}

Response: ETag: "etag-value"
```

**3. CompleteMultipartUpload:**
```http
POST /bucket/object?uploadId=xyz HTTP/1.1

<CompleteMultipartUpload>
  <Part>
    <PartNumber>1</PartNumber>
    <ETag>etag1</ETag>
  </Part>
  <Part>
    <PartNumber>2</PartNumber>
    <ETag>etag2</ETag>
  </Part>
</CompleteMultipartUpload>

Response: <ETag>final-etag</ETag>
```

**4. AbortMultipartUpload:**
```http
DELETE /bucket/object?uploadId=xyz HTTP/1.1
Response: 204 No Content
```

**5. ListParts:**
```http
GET /bucket/object?uploadId=xyz HTTP/1.1
Response: <Part><PartNumber>1</PartNumber>...</Part>
```

### Routing Configuration Needed

```go
// Example routing setup (pseudo-code)
router.HandleFunc("/bucket/object", func(w, r) {
    query := r.URL.Query()

    // CreateMultipartUpload: POST with ?uploads
    if r.Method == "POST" && query.Has("uploads") {
        return CreateMultipartUploadHandler(w, r)
    }

    // UploadPart: PUT with ?partNumber&uploadId
    if r.Method == "PUT" && query.Has("partNumber") && query.Has("uploadId") {
        return UploadPartHandler(w, r)
    }

    // CompleteMultipartUpload: POST with ?uploadId (no "uploads")
    if r.Method == "POST" && query.Has("uploadId") {
        return CompleteMultipartUploadHandler(w, r)
    }

    // AbortMultipartUpload: DELETE with ?uploadId
    if r.Method == "DELETE" && query.Has("uploadId") {
        return AbortMultipartUploadHandler(w, r)
    }

    // ListParts: GET with ?uploadId
    if r.Method == "GET" && query.Has("uploadId") {
        return ListPartsHandler(w, r)
    }

    // Regular object operations (no query params or different query params)
    // PutObject, GetObject, DeleteObject, etc.
})
```

## Priority Justification

HIGH priority because:
1. Blocks large file uploads via boto3 (common use case)
2. boto3 is a primary S3 client for Python applications
3. No workaround for boto3 users (mc has corruption bug)
4. Required for S3 API compatibility
5. Large file support is essential for real-world usage
6. Likely affects AWS CLI as well (needs verification)
7. Query parameter routing is a fundamental S3 API pattern (also affects tagging, bug #004)
