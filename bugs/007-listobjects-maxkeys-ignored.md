# Bug #007: ListObjectsV2 MaxKeys Parameter Ignored

**Status:** Open  
**Priority:** Medium  
**Discovered:** 2026-01-31  
**Affects:** boto3 (confirmed), likely all clients

## Summary

When using `ListObjectsV2` with the `MaxKeys` parameter to limit results, the server ignores the limit and returns all objects. This breaks pagination and can cause performance issues with large buckets.

## Evidence

### boto3 Test Output

```python
# Create 5 test objects
s3.put_object(Bucket=bucket, Key="obj1.txt", Body=b"data")
s3.put_object(Bucket=bucket, Key="obj2.txt", Body=b"data")
s3.put_object(Bucket=bucket, Key="obj3.txt", Body=b"data")
s3.put_object(Bucket=bucket, Key="obj4.txt", Body=b"data")
s3.put_object(Bucket=bucket, Key="obj5.txt", Body=b"data")

# Request only first 2 objects
response = s3.list_objects_v2(Bucket=bucket, MaxKeys=2)
contents = response.get("Contents", [])
is_truncated = response.get("IsTruncated", False)

# Expected:
# - Contents: 2 objects
# - IsTruncated: True
# - NextContinuationToken: present

# Actual:
# - Contents: 5 objects (all of them)
# - IsTruncated: False
# - NextContinuationToken: absent

print(f"Expected 2 objects, got {len(contents)}")  # FAIL
```

## Reproduction Steps

1. Create a bucket with multiple objects:
   ```bash
   aws s3 mb s3://test-bucket
   for i in {1..5}; do
     echo "object $i" | aws s3 cp - s3://test-bucket/obj$i.txt
   done
   ```

2. List with MaxKeys limit:
   ```bash
   aws s3api list-objects-v2 --bucket test-bucket --max-keys 2
   ```

3. **Expected:**
   - `KeyCount`: 2
   - `IsTruncated`: true
   - `NextContinuationToken`: (some token value)
   - `Contents`: Array with 2 objects

4. **Actual:**
   - `KeyCount`: 5
   - `IsTruncated`: false
   - `NextContinuationToken`: absent
   - `Contents`: Array with all 5 objects

## Root Cause Analysis

Likely causes:
1. **Query parameter not parsed:** `max-keys` query parameter not extracted from request
2. **Parameter ignored in handler:** ListObjectsV2 handler doesn't use MaxKeys value
3. **Default value used:** Handler always uses default (1000) instead of requested value
4. **Pagination not implemented:** Truncation and continuation logic missing

Location to investigate:
- `internal/api/handlers/bucket.go` - ListObjectsV2 handler
- Query parameter parsing for `max-keys`
- Response building for `IsTruncated` and `NextContinuationToken`

## Impact

**Functionality:**
- Pagination broken for all clients
- Cannot limit result set size
- Applications expecting pagination will fail or behave incorrectly

**Performance:**
- Cannot efficiently browse large buckets
- All objects returned in single response (memory/bandwidth waste)
- No way to implement incremental loading in client applications

**S3 Compatibility:**
- Breaks S3 API compliance for pagination
- Applications relying on pagination will malfunction
- Makes DirIO unsuitable for large bucket scenarios

**Clients Affected:**
- ✅ boto3: Confirmed - MaxKeys parameter ignored, returns all objects
- ❓ AWS CLI: Needs verification
- ❓ MinIO mc: Needs verification

## Current Behavior

| Parameter | Expected | Actual |
|-----------|----------|--------|
| MaxKeys=2 | Return 2 objects | Returns all 5 objects |
| IsTruncated | True (more results available) | False |
| NextContinuationToken | Present (for next page) | Absent |
| KeyCount | 2 | 5 |

## Proposed Fix

### Phase 1: Parse MaxKeys Parameter
1. Extract `max-keys` from query parameters in ListObjectsV2 handler
2. Validate value (must be between 1 and 1000)
3. Default to 1000 if not specified (S3 standard)

### Phase 2: Implement Result Truncation
1. Limit returned objects to MaxKeys count
2. Set `IsTruncated=true` if more results exist
3. Set `KeyCount` to actual number of keys returned

### Phase 3: Implement Continuation Tokens
1. Generate continuation token when results are truncated
2. Include `NextContinuationToken` in response
3. Handle `continuation-token` parameter in subsequent requests
4. Return correct subset of results based on token

### Phase 4: Testing
1. Add integration tests for MaxKeys parameter
2. Test pagination across multiple pages
3. Test with various MaxKeys values (1, 2, 10, 100, 1000)
4. Test with all three clients (boto3, AWS CLI, mc)
5. Test edge cases (MaxKeys > total objects, MaxKeys = 1, etc.)

## Testing

Confirmed in: `tests/clients/scripts/boto3.py` (lines 147-159)

```python
# ListObjectsV2 with max-keys
response = s3.list_objects_v2(Bucket=bucket, MaxKeys=2)
contents = response.get("Contents", [])
is_truncated = response.get("IsTruncated", False)

if len(contents) == 2 and is_truncated:
    log_pass("ListObjectsV2 (max-keys)")
elif len(contents) == 2:
    log_fail("ListObjectsV2 (max-keys)", "IsTruncated should be True")
else:
    log_fail("ListObjectsV2 (max-keys)", f"expected 2 objects, got {len(contents)}")
```

## Related Issues

None directly, but related to overall ListObjectsV2 implementation quality:
- #006: ListObjectsV2 delimiter returns 0 CommonPrefixes (boto3)
- Pagination is a core S3 API feature alongside delimiter support

## Technical Details

### S3 API ListObjectsV2 Parameters

**Request:**
```
GET /bucket?list-type=2&max-keys=2 HTTP/1.1
```

**Expected Response:**
```xml
<ListBucketResult>
  <Name>bucket</Name>
  <KeyCount>2</KeyCount>
  <MaxKeys>2</MaxKeys>
  <IsTruncated>true</IsTruncated>
  <NextContinuationToken>token123</NextContinuationToken>
  <Contents>
    <Key>obj1.txt</Key>
    ...
  </Contents>
  <Contents>
    <Key>obj2.txt</Key>
    ...
  </Contents>
</ListBucketResult>
```

**Pagination Flow:**
1. First request: `?max-keys=2` → Returns 2 objects + token
2. Second request: `?max-keys=2&continuation-token=token123` → Returns next 2 objects
3. Continue until `IsTruncated=false`

### Continuation Token Design

Options for token format:
1. **Base64-encoded last key:** Simple, stateless
2. **Opaque token:** More flexible, can include additional state
3. **Encrypted cursor:** Secure, prevents manipulation

Recommendation: Start with base64-encoded last key for simplicity.

## References

- AWS S3 API ListObjectsV2: https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListObjectsV2.html
- Pagination parameters: MaxKeys, ContinuationToken, IsTruncated, NextContinuationToken
- Test location: `tests/clients/scripts/boto3.py`

## Priority Justification

MEDIUM priority because:
- Feature partially works (returns objects, just not paginated)
- Workaround exists (client can filter/limit results)
- Becomes critical for buckets with thousands of objects
- Required for full S3 API compliance
- Less urgent than data corruption bugs (#001, #004, #005)
