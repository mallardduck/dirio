# Bug #004: Object Tagging Stores Tags as Content

**Status:** ✅ RESOLVED  
**Priority:** High  
**Discovered:** 2026-01-31  
**Resolved:** 2026-02-16  
**Affects:** boto3, MinIO mc (all clients that support object tagging)  
**Root Cause:** ~~Bug #001 (AWS SigV4 Chunked Encoding Corruption)~~ **FIXED** - Missing handler implementation **IMPLEMENTED**

## Resolution Summary (2026-02-16)

**What Was Fixed:**

1. **Added Tagging Types** (`pkg/s3types/responses.go`):
   - `Tagging` - Response type for GetObjectTagging
   - `PutObjectTaggingRequest` - Request type for PutObjectTagging
   - `Tag` - Key-value tag structure
   - `ErrCodeMalformedXML` - Error code for invalid XML

2. **Updated ObjectMetadata** (`internal/persistence/metadata/metadata.go`):
   - Added `Tags map[string]string` field to store object tags separately from content

3. **Implemented Service Methods** (`internal/service/s3/s3.go`):
   - `PutObjectTagging` - Sets tags on existing objects
   - `GetObjectTagging` - Retrieves tags from objects
   - Added request types in `internal/service/s3/types.go`

4. **Implemented HTTP Handlers** (`internal/http/api/s3/object_tagging.go`):
   - `PutObjectTagging` - Handles `PUT /{bucket}/{key}?tagging`
   - `GetObjectTagging` - Handles `GET /{bucket}/{key}?tagging`
   - Proper XML parsing and error handling

5. **Wired Up Routes** (`internal/http/server/routes.go`):
   - Connected handlers to existing query-based routes
   - Routes already existed and were correctly configured

**What Was Already Fixed:**
- Bug #001 (Chunked Encoding Corruption) - Fully resolved on 2026-02-16
- Query parameter routing - Already implemented correctly
- Routes properly dispatch `?tagging` query parameter to dedicated handlers

## Summary

When setting tags on an object using `PutObjectTagging`, the operation returns "not yet implemented" because the handlers don't exist. Once implemented, tags need to be stored as separate metadata (not as object content).

## Evidence

### MinIO mc Test Output

```bash
# Upload object with known content
echo "tagging test content" > /tmp/tagging-test.txt
mc cp /tmp/tagging-test.txt dirio/bucket/tagging-test.txt

# Verify content before tagging
mc cat dirio/bucket/tagging-test.txt
# Output: "tagging test content" ✅

# Set tags on object
mc tag set dirio/bucket/tagging-test.txt "key1=value1&key2=value2"
# Operation succeeds ✅

# Get tags back
mc tag list dirio/bucket/tagging-test.txt
# Output shows tags ✅

# CRITICAL: Check object content after tagging
mc cat dirio/bucket/tagging-test.txt
# Expected: "tagging test content"
# Actual: "<Tagging><TagSet>...</TagSet></Tagging>" ❌
```

### boto3 Test Output

```python
# Upload object
s3.put_object(Bucket=bucket, Key="test.txt", Body=b"test content")

# Set tags
s3.put_object_tagging(
    Bucket=bucket,
    Key="test.txt",
    Tagging={"TagSet": [{"Key": "env", "Value": "test"}]}
)

# Get object content after tagging
response = s3.get_object(Bucket=bucket, Key="test.txt")
content = response["Body"].read()

# Expected: b"test content"
# Actual: Content replaced with XML tagging payload
```

## Reproduction Steps

1. Create an object with known content:
   ```bash
   echo "test content" > test.txt
   aws s3 cp test.txt s3://bucket/test.txt
   ```

2. Verify content:
   ```bash
   aws s3 cp s3://bucket/test.txt -
   # Output: "test content" ✅
   ```

3. Set tags on the object:
   ```bash
   aws s3api put-object-tagging --bucket bucket --key test.txt \
     --tagging 'TagSet=[{Key=env,Value=prod}]'
   ```

4. Check content again:
   ```bash
   aws s3 cp s3://bucket/test.txt -
   # Expected: "test content"
   # Actual: XML tagging structure
   ```

## Root Cause Analysis

**This is a symptom of Bug #001 (AWS SigV4 Chunked Encoding Corruption).**

The S3 API uses different endpoints and query parameters for tagging:
- **PutObject:** `PUT /bucket/object` (stores object content)
- **PutObjectTagging:** `PUT /bucket/object?tagging` (stores tags metadata)

What's happening:
1. Client sends `PUT /bucket/object?tagging` with XML body containing tags
2. Server's routing doesn't handle the `?tagging` query parameter correctly
3. Request is routed to PutObject handler instead of PutObjectTagging handler
4. PutObject handler stores the XML body as object content
5. Chunked encoding headers are also written to the file (bug #001)
6. Original object content is completely replaced

This is a **query parameter routing issue** combined with the chunked encoding bug.

## Impact

**Data Loss:**
- Original object content is destroyed and replaced with XML
- No way to recover original content after tagging
- Silent data corruption (operation reports success)

**Feature Status:**
- Object tagging appears to work (tags can be set and retrieved)
- But comes at the cost of destroying object data
- Makes object tagging completely unusable

**Clients Affected:**
- ✅ boto3: Operation succeeds but corrupts content
- ✅ MinIO mc: Operation succeeds but corrupts content
- ❓ AWS CLI: Needs verification

## Proposed Fix

### Phase 1: Fix Query Parameter Routing
1. Update routing in `internal/api/router.go` to handle query parameters
2. Route `PUT /bucket/object?tagging` to PutObjectTagging handler
3. Route `GET /bucket/object?tagging` to GetObjectTagging handler
4. Route `DELETE /bucket/object?tagging` to DeleteObjectTagging handler
5. Ensure regular `PUT /bucket/object` still goes to PutObject handler

### Phase 2: Implement Tagging Storage
1. Store tags separately from object content (in metadata file)
2. PutObjectTagging should only update tags, not touch object data
3. GetObjectTagging should return tags from metadata
4. Verify object content remains intact after tagging operations

### Phase 3: Test with All Clients
1. Add integration tests for object tagging
2. Verify content integrity before and after tagging
3. Test with boto3, mc, and AWS CLI
4. Ensure tags are stored and retrieved correctly

**IMPORTANT:** Must also fix Bug #001 (chunked encoding) to prevent the XML payload from being corrupted with encoding headers.

## Testing

Confirmed in: `tests/clients/scripts/boto3.py` and `tests/clients/scripts/mc.sh`

### boto3 test (lines 296-324):
```python
# Verify object content before tagging
content_before = s3.get_object(Bucket=bucket, Key="test.txt")["Body"].read()

# Set tags
s3.put_object_tagging(Bucket=bucket, Key="test.txt", ...)

# CRITICAL: Verify content not corrupted
content_after = s3.get_object(Bucket=bucket, Key="test.txt")["Body"].read()

if content_before != content_after:
    # FAIL: Tagging corrupted object content
```

### mc test (lines 215-250):
```bash
# Verify content before tagging
CONTENT_BEFORE=$(mc cat ${MC_ALIAS}/${BUCKET}/tagging-test.txt)

# Set tags
mc tag set ${MC_ALIAS}/${BUCKET}/tagging-test.txt "key1=value1&key2=value2"

# Verify content after tagging
CONTENT_AFTER=$(mc cat ${MC_ALIAS}/${BUCKET}/tagging-test.txt)

if [ "$CONTENT_AFTER" != "$CONTENT_BEFORE" ]; then
    # FAIL: Content corrupted by tagging
fi
```

## Related Issues

- **#001: AWS SigV4 Chunked Encoding Corruption** (ROOT CAUSE) - The XML tagging payload also gets corrupted with chunked encoding headers before being written
- Both bugs must be fixed for object tagging to work correctly

## Technical Details

### S3 API Tagging Endpoints

**PutObjectTagging:**
```
PUT /bucket/object?tagging HTTP/1.1
Content-Type: application/xml

<Tagging>
  <TagSet>
    <Tag>
      <Key>env</Key>
      <Value>prod</Value>
    </Tag>
  </TagSet>
</Tagging>
```

**GetObjectTagging:**
```
GET /bucket/object?tagging HTTP/1.1

Response:
<Tagging>
  <TagSet>
    <Tag>...</Tag>
  </TagSet>
</Tagging>
```

**DeleteObjectTagging:**
```
DELETE /bucket/object?tagging HTTP/1.1
```

### Expected Storage

Tags should be stored in the object's metadata file (e.g., `object.meta` or similar), NOT as the object content. The object data file should remain completely unchanged.

## Priority Justification

This is a HIGH priority bug because:
1. It causes silent data loss (original content destroyed)
2. Tagging is a commonly used S3 feature
3. Operation appears to succeed but corrupts data
4. No workaround available
5. Blocks claiming S3 compatibility for any tagging-dependent applications
