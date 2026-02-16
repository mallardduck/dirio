# Bug #009: CopyObject Creates 0-Byte Files

**Status:** ✅ RESOLVED  
**Priority:** Medium  
**Discovered:** 2026-01-31  
**Resolved:** 2026-02-16  
**Affects:** boto3 (creates 0-byte files), MinIO mc (EOF error)  
**Resolution:** Implemented CopyObject handler with x-amz-copy-source header parsing and ISO 8601 date format for MinIO mc compatibility

## Summary

Server-side copy operations (CopyObject) fail across multiple clients. boto3 creates empty 0-byte files instead of copying content, while MinIO mc reports EOF errors.

## Evidence

### boto3 behavior:
```python
# Source object has content
s3.put_object(Bucket=bucket, Key='source.txt', Body=b'test content')

# Copy operation succeeds (no error)
s3.copy_object(
    CopySource={'Bucket': bucket, 'Key': 'source.txt'},
    Bucket=bucket,
    Key='destination.txt'
)

# But destination is empty
response = s3.get_object(Bucket=bucket, Key='destination.txt')
content = response['Body'].read()
# Expected: b'test content'
# Actual: b'' (empty)
```

### MinIO mc behavior:
```bash
mc cp dirio/bucket/test.txt dirio/bucket/test-copy.txt
# Output: mc: <ERROR> Failed to copy. EOF
```

## Reproduction Steps

1. Create source object: `echo "test content" > file.txt`
2. Upload: `aws s3 cp file.txt s3://bucket/source.txt`
3. Server-side copy: `aws s3 cp s3://bucket/source.txt s3://bucket/dest.txt`
4. Download and check: `aws s3 cp s3://bucket/dest.txt -`
5. Expected: "test content"
6. Actual: Empty file (0 bytes)

## Root Cause Analysis

CopyObject uses the `x-amz-copy-source` header instead of a request body:
```
PUT /bucket/destination.txt HTTP/1.1
x-amz-copy-source: /bucket/source.txt
```

Likely issues:
1. **Copy source header not parsed:** Handler doesn't recognize `x-amz-copy-source`
2. **Treating as regular PUT:** Empty body creates empty file
3. **Source object not read:** Copy logic not implemented
4. **EOF error in mc:** May be related to response format

The fact that boto3 creates a 0-byte file (rather than error) suggests the operation is treated as a regular PUT with empty body.

## Impact

- Users cannot copy objects within the server (must download and re-upload)
- Breaks backup/duplication workflows
- Affects applications that rely on server-side copy for efficiency
- No workaround except client-side download/upload

## Current Behavior

| Client | Result | Details |
|--------|--------|---------|
| boto3 | ❌ 0-byte file | Operation succeeds but creates empty file |
| MinIO mc | ❌ EOF error | `mc cp s3-to-s3` fails with EOF |
| AWS CLI | ❓ Untested | Need to verify behavior |

## Proposed Fix

1. Implement `x-amz-copy-source` header parsing in PUT handler
2. Detect copy operation vs regular PUT
3. Read source object from storage
4. Write source content to destination key
5. Handle metadata copying (preserve or replace)
6. Add integration tests for CopyObject
7. Test with all three clients

## Implementation Notes

AWS S3 CopyObject API details:
- Uses PUT method with `x-amz-copy-source` header
- Source format: `/source-bucket/source-key` or `source-bucket/source-key`
- Can copy to same or different bucket
- Optional: `x-amz-metadata-directive` (COPY or REPLACE)
- Optional: `x-amz-copy-source-range` for partial copy
- Response includes ETag, LastModified

## Testing

Confirmed in: `tests/clients/scripts/boto3.py` and `tests/clients/scripts/mc.sh`

boto3:
```python
copy_result = s3.copy_object(...)
copied_content = s3.get_object(Bucket=bucket_name, Key='copy-test.txt')['Body'].read()
# Expected: b'copy test content'
# Actual: b'' (0 bytes)
```

mc:
```bash
mc cp ${MC_ALIAS}/${BUCKET}/test.txt ${MC_ALIAS}/${BUCKET}/test-copy.txt
# ERROR: EOF
```

## Related Issues

- May be related to #001 (chunked encoding) if copy logic tries to read/write data
- Once copy is implemented, need to verify it doesn't suffer from chunked encoding corruption

## Priority Justification

While this is a missing feature (not a corruption bug), it's commonly used:
- Backup and versioning systems
- Data migration tools
- Application state management
- Efficient rename operations (copy + delete)
