# Bug #011: Custom Metadata Key Case Wrong

**Status:** Open
**Priority:** Medium
**Discovered:** 2026-01-31
**Affects:** boto3 (confirmed - returns Title-Case), MinIO mc (metadata not returned at all - see bug #012)

## Summary

When retrieving custom metadata via HeadObject or GetObject, the metadata keys are returned in the wrong case format. boto3 receives keys in Title-Case format (e.g., `Custom-Key`) instead of the expected lowercase format (e.g., `custom-key`).

## Evidence

### boto3 Test Output

```python
# Upload object with custom metadata
s3.put_object(
    Bucket=bucket,
    Key="metadata.txt",
    Body=b"test with metadata",
    Metadata={"custom-key": "custom-value"}
)

# Retrieve metadata
response = s3.head_object(Bucket=bucket, Key="metadata.txt")
metadata = response.get("Metadata", {})

# Expected: {"custom-key": "custom-value"}
# Actual: {"Custom-Key": "custom-value"} or similar title-cased key

print(f"Metadata: {metadata}")
# Output shows wrong key case
```

## Reproduction Steps

1. Upload object with custom metadata:
   ```python
   import boto3
   s3 = boto3.client("s3", endpoint_url="http://localhost:8080", ...)

   s3.put_object(
       Bucket="bucket",
       Key="test.txt",
       Body=b"content",
       Metadata={
           "custom-key": "value1",
           "another-meta": "value2"
       }
   )
   ```

2. Retrieve metadata:
   ```python
   response = s3.head_object(Bucket="bucket", Key="test.txt")
   metadata = response["Metadata"]
   print(metadata)
   ```

3. **Expected:**
   ```python
   {
       "custom-key": "value1",
       "another-meta": "value2"
   }
   ```

4. **Actual:**
   ```python
   {
       "Custom-Key": "value1",  # Wrong case!
       "Another-Meta": "value2"  # Wrong case!
   }
   ```

## Root Cause Analysis

S3 API metadata key case rules:
- **Client sends:** `X-Amz-Meta-custom-key: value` (HTTP header)
- **Server stores:** Metadata should preserve or normalize to lowercase
- **Server returns:** `X-Amz-Meta-custom-key: value` (lowercase after prefix)
- **Client receives:** SDK converts to `Metadata` dict with lowercase keys

What's likely happening:
1. **HTTP header normalization issue:** Go's `http.Header` type normalizes header names to Title-Case (canonical form)
2. **Metadata extraction bug:** When extracting `X-Amz-Meta-*` headers, the server preserves the Title-Cased form
3. **Response headers wrong:** Server sends `X-Amz-Meta-Custom-Key` instead of `X-Amz-Meta-custom-key`
4. **Client SDK confused:** boto3 extracts metadata keys from header names, gets wrong case

Location to investigate:
- `internal/api/handlers/object.go` - HeadObject and GetObject handlers
- Metadata extraction from request headers
- Metadata storage format
- Metadata response header generation
- HTTP header case normalization

## Impact

**Functionality:**
- Metadata keys returned but in wrong case
- Applications using case-sensitive key lookups will fail
- Inconsistent with S3 API behavior
- Breaks metadata-dependent workflows

**Compatibility:**
- boto3: Returns Title-Case keys instead of lowercase
- Applications expecting `metadata["custom-key"]` will fail
- Need to use case-insensitive lookups (workaround, non-standard)

**Clients Affected:**
- ✅ boto3: Confirmed - returns Title-Case keys
- ❌ MinIO mc: Metadata not returned at all (bug #012)
- ❓ AWS CLI: Needs verification

## Current Behavior

| Client | Metadata Set | Metadata Retrieved | Notes |
|--------|--------------|-------------------|-------|
| boto3 | ✅ Works | ⚠️ Wrong key case | Returns Title-Case instead of lowercase |
| MinIO mc | ✅ Works (`--attr`) | ❌ Not returned | See bug #012 |
| AWS CLI | ❓ Untested | ❓ Untested | Needs verification |

## Proposed Fix

### Phase 1: Understand Case Normalization
1. Investigate how Go's `http.Header` normalizes header names
2. Identify where metadata is extracted from request headers
3. Check how metadata is stored (filesystem metadata file)
4. Verify stored case vs returned case

### Phase 2: Fix Header Case on Response
1. When reading metadata from storage, preserve original case
2. When building HeadObject/GetObject response:
   - Convert metadata keys to lowercase
   - Set headers as `X-Amz-Meta-{lowercase-key}: {value}`
   - Ensure Go's header normalization doesn't interfere

3. Consider using `response.Header()["X-Amz-Meta-custom-key"]` (bracket notation)
   vs `response.Header.Set()` which may normalize

### Phase 3: Fix Storage (if needed)
1. If metadata is stored with wrong case, fix storage layer
2. Store metadata keys in lowercase (after `x-amz-meta-` prefix)
3. Ensure consistency between storage and retrieval

### Phase 4: Testing
1. Add integration test for custom metadata case sensitivity
2. Test with boto3: set lowercase, verify lowercase returned
3. Test with AWS CLI: verify same behavior
4. Test with MinIO mc: verify metadata returned (bug #012)
5. Test various key formats:
   - `custom-key` (lowercase with dash)
   - `CustomKey` (mixed case)
   - `custom_key` (underscore)
   - `CUSTOM-KEY` (uppercase)

## Testing

Confirmed in: `tests/clients/scripts/boto3.py` (lines 174-202)

```python
# PutObject with metadata
s3.put_object(
    Bucket=bucket,
    Key="metadata.txt",
    Body=b"test with metadata",
    Metadata={"custom-key": "custom-value"}
)

# GetObject metadata
response = s3.head_object(Bucket=bucket, Key="metadata.txt")
metadata = response.get("Metadata", {})

if metadata.get("custom-key") == "custom-value":
    log_pass("GetObject metadata")
else:
    # FAIL: Key case wrong or metadata missing
    log_fail("GetObject metadata", f"metadata not returned correctly: {metadata}")
```

## Related Issues

- #012: Custom metadata not returned in mc stat (related issue, different symptom)

## Technical Details

### S3 API Metadata Behavior

**PutObject Request:**
```http
PUT /bucket/object HTTP/1.1
X-Amz-Meta-custom-key: custom-value
X-Amz-Meta-another-meta: another-value
```

**HeadObject Response (Expected):**
```http
HTTP/1.1 200 OK
X-Amz-Meta-custom-key: custom-value
X-Amz-Meta-another-meta: another-value
```

**boto3 Metadata Dict (Expected):**
```python
{
    "custom-key": "custom-value",
    "another-meta": "another-value"
}
```

### Go HTTP Header Normalization

Go's `net/http` package automatically normalizes header names to canonical form (Title-Case):
```go
// http.Header.Set() normalizes keys
header.Set("x-amz-meta-custom-key", "value")
// Becomes: "X-Amz-Meta-Custom-Key"
```

To preserve exact case, use bracket notation:
```go
// Bracket notation preserves case
header["x-amz-meta-custom-key"] = []string{"value"}
// Becomes: "x-amz-meta-custom-key" (lowercase preserved)
```

However, HTTP/1.1 specifies headers are case-insensitive, so the issue is likely in how the metadata key is extracted from the header name.

### Correct Implementation

When extracting metadata from headers:
```go
for headerName, values := range r.Header {
    if strings.HasPrefix(strings.ToLower(headerName), "x-amz-meta-") {
        // Extract key after "x-amz-meta-" prefix
        metaKey := strings.ToLower(headerName[len("x-amz-meta-"):])
        metadata[metaKey] = values[0]
    }
}
```

When returning metadata in response:
```go
for metaKey, metaValue := range metadata {
    // Use lowercase key
    headerName := "x-amz-meta-" + strings.ToLower(metaKey)
    // Use bracket notation to preserve case
    w.Header()[headerName] = []string{metaValue}
}
```

## References

- AWS S3 User-Defined Metadata: https://docs.aws.amazon.com/AmazonS3/latest/userguide/UsingMetadata.html#UserMetadata
- S3 metadata keys must be lowercase: https://docs.aws.amazon.com/AmazonS3/latest/API/API_PutObject.html
- Go http.Header documentation: https://pkg.go.dev/net/http#Header
- Test location: `tests/clients/scripts/boto3.py`

## Priority Justification

MEDIUM priority because:
- Metadata feature partially works (value is correct, just key case wrong)
- Workaround exists (case-insensitive key lookup)
- Not data-corrupting (unlike bugs #001, #004, #005)
- Affects S3 API compliance
- Can break applications expecting exact key case
- Related to bug #012 (mc metadata issue)
