# Bug #006: ListObjectsV2 Delimiter Returns 0 CommonPrefixes (boto3)

**Status:** Open
**Priority:** Medium
**Discovered:** 2026-01-31
**Affects:** boto3 only (MinIO mc shows folders correctly)

## Summary

When using boto3 to list objects with a delimiter (to get folder-like groupings), the response returns 0 `CommonPrefixes` even though folders exist. MinIO mc with the same data structure correctly shows folders.

## Evidence

Test output from boto3:
```python
# Create folder structure with delimiter test
response = s3.list_objects_v2(Bucket=bucket_name, Delimiter='/')
# Expected: CommonPrefixes containing folder names
# Actual: CommonPrefixes = [] (empty list)
```

Meanwhile, mc with same data:
```bash
mc ls dirio/bucket/
# Output shows: folder1/, folder2/ (correctly)
```

## Reproduction Steps

1. Create objects with path structure:
   - `folder1/file1.txt`
   - `folder1/file2.txt`
   - `folder2/file1.txt`
   - `root-file.txt`
2. List with delimiter using boto3: `list_objects_v2(Bucket=bucket, Delimiter='/')`
3. Expected: `CommonPrefixes=[{Prefix: 'folder1/'}, {Prefix: 'folder2/'}]`
4. Actual: `CommonPrefixes=[]` (empty)

## Root Cause Analysis

The delimiter logic is implemented (mc works), but likely:
1. **Query parameter parsing issue:** boto3 may send delimiter differently than mc
2. **Response format issue:** CommonPrefixes may be populated but not serialized correctly for boto3
3. **API version difference:** boto3 may request different API version with different response format
4. **Content-Type handling:** Response XML structure may differ between clients

## Impact

- boto3 users cannot navigate folder-like structures
- Makes large buckets difficult to browse
- Breaks applications that rely on folder navigation
- MinIO mc users are unaffected

## Investigation Notes

This is particularly interesting because:
- ✅ Integration tests pass (see `tests/integration/list_objects_test.go`)
- ✅ MinIO mc shows folders correctly
- ❌ boto3 returns empty CommonPrefixes

Suggests the delimiter logic works internally but there's a client-specific issue with:
- Request parsing
- Response formatting
- Query parameter handling

## Proposed Fix

1. Add debug logging to see delimiter parameter in boto3 requests
2. Compare request/response between mc (works) and boto3 (fails)
3. Check XML response generation in `internal/api/handlers/bucket.go`
4. Verify CommonPrefixes are being included in XML output
5. Add boto3-specific integration test

## Testing

Confirmed in: `tests/clients/scripts/boto3.py`
```python
# List with delimiter to check for folders
response = s3.list_objects_v2(Bucket=bucket_name, Delimiter='/')
common_prefixes = response.get('CommonPrefixes', [])
# Expected: 2+, Actual: 0
```

## Related Documentation

- AWS S3 API: ListObjectsV2 with Delimiter
- Integration test (passing): `tests/integration/list_objects_test.go` - TestListObjectsV2_WithDelimiter
- Client test (failing): `tests/clients/scripts/boto3.py` - delimiter test
