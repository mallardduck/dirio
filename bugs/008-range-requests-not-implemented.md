# Bug #008: Range Requests Not Implemented

**Status:** ✅ RESOLVED
**Priority:** Medium
**Discovered:** 2026-01-31
**Resolved:** 2026-02-16
**Affects:** boto3 (confirmed), MinIO mc (confirmed), likely all clients
**Resolution:** Implemented Range header parsing and 206 Partial Content responses in GetObject handler

## Summary

HTTP Range requests for partial object downloads are not implemented. When clients request a byte range using the `Range` header, the server ignores it and returns the full object content instead.

## Evidence

### boto3 Test Output

```python
# Upload 100-byte object
large_content = b"0123456789" * 10  # 100 bytes
s3.put_object(Bucket=bucket, Key="range-test.txt", Body=large_content)

# Request only first 10 bytes
response = s3.get_object(Bucket=bucket, Key="range-test.txt", Range="bytes=0-9")
body = response["Body"].read()

# Expected: b"0123456789" (10 bytes)
# Actual: Full 100-byte content returned

print(f"Expected 10 bytes, got {len(body)} bytes")  # FAIL
```

### MinIO mc Test Output (via curl)

```bash
# Generate pre-signed URL
RANGE_URL=$(mc share download dirio/bucket/test.txt)

# Request first 10 bytes using Range header
curl -r 0-9 "$RANGE_URL"

# Expected: 10 bytes
# Actual: 0 bytes (or full content)
```

## Reproduction Steps

1. Upload a test file with known content:
   ```bash
   dd if=/dev/zero of=test.dat bs=1M count=1  # 1 MB file
   aws s3 cp test.dat s3://bucket/test.dat
   ```

2. Request partial content using Range header:
   ```bash
   aws s3api get-object \
     --bucket bucket \
     --key test.dat \
     --range bytes=0-1023 \
     output.dat
   ```

3. **Expected:**
   - HTTP response: `206 Partial Content`
   - `Content-Range` header: `bytes 0-1023/1048576`
   - `Content-Length`: 1024
   - Body: First 1024 bytes only

4. **Actual:**
   - HTTP response: `200 OK` (not 206)
   - No `Content-Range` header
   - `Content-Length`: 1048576 (full file)
   - Body: Full 1 MB content

## Root Cause Analysis

Likely causes:
1. **Range header not parsed:** GetObject handler doesn't check for `Range` header
2. **Not implemented:** No logic to extract and serve partial content
3. **206 status not supported:** Missing support for HTTP 206 Partial Content response
4. **Content-Range header missing:** Response doesn't include required headers

Location to investigate:
- `internal/api/handlers/object.go` - GetObject handler
- HTTP header parsing for `Range` requests
- File reading logic (needs to support seeking/partial reads)
- Response building for 206 status and `Content-Range` header

## Impact

**Functionality Broken:**
- Resumable downloads impossible
- Video/audio streaming players cannot seek
- Large file downloads cannot be resumed after interruption
- Parallel download tools cannot split files into chunks

**Use Cases Affected:**
- ✗ Video streaming (HTML5 video player seeking)
- ✗ Audio streaming (media player seeking)
- ✗ Resumable downloads (download managers)
- ✗ Parallel downloads (aria2, axel, etc.)
- ✗ Partial file inspection (reading headers, tails)
- ✗ Large file transfers over unreliable networks

**S3 Compatibility:**
- Breaks S3 API compliance for Range requests
- Applications expecting Range support will fail or work inefficiently
- Makes DirIO unsuitable for media streaming use cases

**Clients Affected:**
- ✅ boto3: Range parameter ignored, returns full content
- ✅ MinIO mc: Range requests fail (0 bytes or full content)
- ❓ AWS CLI: Needs verification

## Current Behavior

| Request | Expected Response | Actual Response |
|---------|------------------|-----------------|
| `Range: bytes=0-9` | 206 Partial Content, 10 bytes | 200 OK, full file |
| `Range: bytes=100-199` | 206 Partial Content, 100 bytes | 200 OK, full file |
| `Range: bytes=-500` | 206 Partial Content, last 500 bytes | 200 OK, full file |
| No Range header | 200 OK, full file | 200 OK, full file ✅ |

## Proposed Fix

### Phase 1: Parse Range Header
1. Extract `Range` header from HTTP request in GetObject handler
2. Parse range format: `bytes=start-end`, `bytes=-suffix`, `bytes=start-`
3. Validate range values against object size
4. Handle invalid ranges (return 416 Range Not Satisfiable)

### Phase 2: Implement Partial Content Serving
1. Open object file with seek support
2. Seek to requested start position
3. Read only requested number of bytes
4. Return 206 Partial Content status code
5. Include `Content-Range` header: `bytes start-end/total`
6. Set `Content-Length` to actual bytes returned

### Phase 3: Handle Edge Cases
1. Multiple ranges: `bytes=0-99,200-299` (return 206 multipart/byteranges)
2. Open-ended range: `bytes=100-` (from byte 100 to end)
3. Suffix range: `bytes=-100` (last 100 bytes)
4. Invalid range: `bytes=1000-2000` for 500-byte file (return 416)
5. Unsatisfiable range: return 416 with `Content-Range: bytes */total`

### Phase 4: Testing
1. Add integration tests for Range requests
2. Test single byte range: `bytes=0-0`
3. Test middle range: `bytes=100-199`
4. Test suffix range: `bytes=-100`
5. Test open-ended range: `bytes=100-`
6. Test with all three clients (boto3, AWS CLI, mc)
7. Test video streaming scenario (HTML5 video player)

## Testing

Confirmed in: `tests/clients/scripts/boto3.py` (lines 204-216)

```python
# Range request test
large_content = b"0123456789" * 10  # 100 bytes
s3.put_object(Bucket=bucket, Key="range-test.txt", Body=large_content)

response = s3.get_object(Bucket=bucket, Key="range-test.txt", Range="bytes=0-9")
body = response["Body"].read()

if body == b"0123456789":
    log_pass("Range request")
else:
    log_fail("Range request", f"expected first 10 bytes, got {len(body)} bytes")
```

Confirmed in: `tests/clients/scripts/mc.sh` (lines 285-298)

```bash
# Range Requests via curl
RANGE_URL=$(mc share download ${MC_ALIAS}/${BUCKET}/test.txt)
PARTIAL=$(curl -f -s -r 0-9 "$RANGE_URL")

if [ ${#PARTIAL} -eq 10 ]; then
    pass "Range Requests"
else
    fail "Range Requests" "Expected 10 bytes, got ${#PARTIAL}"
fi
```

## Related Issues

None directly, but important for media streaming use cases.

## Technical Details

### HTTP Range Request Format

**Request:**
```http
GET /bucket/object.mp4 HTTP/1.1
Range: bytes=0-1023
```

**Expected Response (206 Partial Content):**
```http
HTTP/1.1 206 Partial Content
Content-Range: bytes 0-1023/1048576
Content-Length: 1024
Content-Type: video/mp4

[1024 bytes of data]
```

### Range Header Formats

1. **Specific range:** `bytes=100-200` (bytes 100 through 200 inclusive)
2. **Open-ended start:** `bytes=100-` (byte 100 to end of file)
3. **Suffix range:** `bytes=-500` (last 500 bytes)
4. **Multiple ranges:** `bytes=0-99,200-299` (non-contiguous ranges)

### Response Headers

- **Status:** 206 Partial Content (not 200 OK)
- **Content-Range:** `bytes start-end/total` (e.g., `bytes 0-1023/1048576`)
- **Content-Length:** Actual bytes returned (e.g., 1024)
- **Accept-Ranges:** `bytes` (indicates server supports range requests)

### Error Responses

- **416 Range Not Satisfiable:** Range exceeds file size
  - Response header: `Content-Range: bytes */total-size`
  - Example: `Content-Range: bytes */1048576`

## References

- HTTP Range Requests: https://developer.mozilla.org/en-US/docs/Web/HTTP/Range_requests
- RFC 7233 - Range Requests: https://tools.ietf.org/html/rfc7233
- AWS S3 Range GET: https://docs.aws.amazon.com/AmazonS3/latest/API/API_GetObject.html
- Test location: `tests/clients/scripts/boto3.py`, `tests/clients/scripts/mc.sh`

## Priority Justification

MEDIUM priority because:
- Feature commonly used for media streaming and large files
- Workaround exists (download full file, not ideal)
- Not data-corrupting (unlike bugs #001, #004, #005)
- Required for full S3 API compliance
- Blocks video/audio streaming use cases
- Important for user experience (resumable downloads)
