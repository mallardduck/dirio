# CRITICAL: AWS SigV4 Chunked Encoding Bug

**Discovered:** January 31, 2026
**Status:** 🚨 CRITICAL - Blocks production use
**Impact:** ALL write operations corrupted
**Fix Status:** ⚠️ PARTIAL - Decoder implemented but not activating for real AWS clients

## Current Status (January 31, 2026 18:36 UTC)

- ✅ **Decoder implemented:** Full AWS SigV4 chunked encoding parser in `internal/auth/chunked.go`
- ✅ **Middleware created:** `internal/middleware/chunked.go` wraps request body with decoder
- ✅ **Integration tests pass:** All 6 chunked encoding tests pass with manual header
- ❌ **Client tests still fail:** Real AWS clients (boto3, mc) still show chunked markers in output
- ❌ **Middleware not activating:** Header detection failing for real client requests

**The decoder works perfectly when triggered. The issue is detecting when to use it.**

## Summary

AWS Signature V4 chunked transfer encoding headers are being written directly to object files instead of being parsed and removed. A decoder has been implemented and tested, but the middleware that activates it is not detecting chunked encoding from real AWS clients (boto3, MinIO mc). This causes data corruption in all write operations from these clients.

## Evidence

### 1. GetObject Returns Chunked Encoding Markers

When retrieving a simple text file that should contain `tagging test content`, the actual content returned is:

```
15;chunk-signature=27e683aa022df0a0d27ac3e1e28f24e86e861a7f9f83ffd0c64a16f66d0f998f
tagging test content

0;chunk-signature=48193c2564d7c6caee2d81b1...
```

The chunked transfer encoding frame headers are visible in the object content.

### 2. Multipart Upload Content Corruption

- **Expected file size:** 10,485,760 bytes (exactly 10 MiB)
- **Actual downloaded size:** 10,500,246 bytes
- **Extra data:** 14,486 bytes of chunked encoding artifacts

The multipart upload appears to succeed, metadata reports correct size, but downloaded content is corrupted with encoding markers.

### 3. Object Tagging Content Replacement

When setting tags on an object:
1. Original content: `tagging test content`
2. After setting tags: Content becomes `<Tagging><TagSet>...XML...</TagSet></Tagging>`

The tag XML completely replaces the object content instead of being stored separately.

## Root Cause Analysis

AWS Signature V4 with chunked transfer encoding sends data in this format:

```
{chunk-size};chunk-signature={signature}
{chunk-data}
{chunk-size};chunk-signature={signature}
{chunk-data}
...
0;chunk-signature={final-signature}
```

DirIO is currently writing this **entire payload** to object files, including:
- Chunk size declarations (hex numbers like `15`, `0`)
- Chunk signature headers
- Extra newlines between chunks
- Trailing final chunk markers

The server should:
1. Parse the chunked encoding format
2. Extract only the actual data chunks
3. Concatenate data chunks into the final object content
4. Verify chunk signatures if needed
5. Write ONLY the raw data to storage

## Affected Operations

- ✅ **PutObject** - Uploads succeed but store corrupted content
- ✅ **Multipart Upload** - Uploads complete but content has extra 14KB
- ✅ **Object Tagging (PUT)** - Tag XML stored as object content
- ✅ **GetObject** - Returns corrupted content with encoding markers
- ⚠️ **CopyObject** - May be affected (creates 0-byte files currently)
- ⚠️ **Custom Metadata** - May be affected by same parsing issue

## Test Results

### Integration Tests (Manual Chunked Encoding) - January 31, 2026
- ✅ **TestPutObject_ChunkedEncoding** - PASS: Single chunk decoded correctly
- ✅ **TestPutObject_MultipleChunks** - PASS: Multiple chunks decoded correctly
- ✅ **TestPutObject_LargeChunkedData** - PASS: 1KB chunk decoded correctly
- ✅ **TestPutObject_NonChunkedStillWorks** - PASS: Normal uploads still work
- ✅ **TestPutObject_EmptyChunkedUpload** - PASS: Empty chunks handled correctly
- ✅ All tests verify no chunked markers in decoded content
- ✅ All tests verify byte-for-byte content integrity

**Conclusion:** The decoder implementation is correct and fully functional when activated.

### Client Tests (Real AWS Clients) - January 31, 2026 18:30 UTC

#### Before Content Verification (False Positives)
- Multipart Upload: ✅ PASS (only checked metadata size)
- Object Tagging: ✅ PASS (only checked operation succeeded)
- GetObject: ✅ PASS (only checked data returned)

#### After Content Verification (Exposed Bugs)
- Multipart Upload: ❌ FAIL - Downloaded 10,500,246 bytes instead of 10,485,760
- Object Tagging: ❌ FAIL - Content replaced with XML tags
- GetObject: ❌ FAIL - Content contains chunk markers like `d;chunk-signature=...`

**Conclusion:** Middleware is not activating for real AWS client requests. Chunked data is passing through un-decoded.

## Impact Assessment

### Data Integrity: CRITICAL
- All uploaded objects contain corrupted data
- Downloaded objects cannot be used (extra encoding headers)
- Silent data corruption (operations report success)

### Client Compatibility: BROKEN
- AWS CLI: May be affected (needs verification)
- boto3: Confirmed affected (all write operations)
- MinIO mc: Confirmed affected (all write operations)

### Feature Status: BLOCKED
- Cannot implement object tagging (would corrupt content)
- Cannot implement multipart uploads properly
- Cannot trust any PutObject operations
- Cannot implement CopyObject (relies on content integrity)

## Files Involved

### Already Implemented (Decoder)

1. **Chunked encoding decoder:**
   - `internal/auth/chunked.go` - Full AWS SigV4 chunked encoding parser (303 lines)
   - `internal/auth/chunked_test.go` - Unit tests for decoder
   - ✅ Implementation complete and tested

2. **Middleware:**
   - `internal/middleware/chunked.go` - Middleware to activate decoder (53 lines)
   - Checks for `X-Amz-Content-Sha256: STREAMING-AWS4-HMAC-SHA256-PAYLOAD`
   - ⚠️ Not activating for real AWS clients

3. **Server integration:**
   - `internal/server/server.go` - Line 149-151: Middleware registered
   - Placed after auth middleware, before handlers
   - ✅ Correctly positioned in middleware chain

4. **Integration tests:**
   - `tests/integration/chunked_encoding_test.go` - 6 comprehensive tests
   - ✅ All tests pass when header is manually set

### Need Investigation (Why Not Activating)

1. **Header detection:**
   - `internal/middleware/chunked.go:25` - Header check logic
   - Need to log what headers real clients send
   - May need fallback detection mechanism

2. **Request body parsing:**
   - `internal/api/handlers/object.go` - PutObject handler
   - `internal/api/handlers/multipart.go` - Multipart upload handlers
   - Verify middleware runs before these handlers

3. **Authentication middleware:**
   - `internal/auth/signature.go` - AWS SigV4 verification
   - May need to inspect headers during auth
   - Check if auth modifies or removes relevant headers

## Reproduction Steps

1. Create test object with known content: `echo "test content" | aws s3 cp - s3://bucket/test.txt`
2. Download object: `aws s3 cp s3://bucket/test.txt -`
3. Observe: Output contains chunked encoding markers instead of just "test content"

Or for multipart:
1. Create 10MB file: `dd if=/dev/zero of=test.dat bs=1M count=10`
2. Upload via MinIO mc: `mc cp test.dat dirio/bucket/test.dat`
3. Download back: `mc cp dirio/bucket/test.dat downloaded.dat`
4. Compare sizes: `ls -l test.dat downloaded.dat`
5. Observe: Downloaded file is ~14KB larger

## Attempted Fix (Incomplete)

### What Was Implemented

A chunked encoding decoder was implemented with the following components:

1. **Middleware** (`internal/middleware/chunked.go`):
   - Checks for `X-Amz-Content-Sha256: STREAMING-AWS4-HMAC-SHA256-PAYLOAD` header
   - If detected, wraps request body with decoder
   - Registered in server middleware chain (line 149 of `internal/server/server.go`)

2. **Decoder** (`internal/auth/chunked.go`):
   - Full AWS SigV4 chunked encoding parser
   - Parses chunk headers: `{size};chunk-signature={sig}\r\n`
   - Extracts raw data chunks
   - Optional signature verification support
   - Handles multiple chunks, empty chunks, large data

3. **Integration Tests** (`tests/integration/chunked_encoding_test.go`):
   - 6 comprehensive tests covering various scenarios
   - **ALL INTEGRATION TESTS PASS** ✅
   - Tests verify decoded content has no encoding markers
   - Tests verify byte-for-byte content integrity

### Why It's Still Broken

**The middleware only activates when the request includes:**
```
X-Amz-Content-Sha256: STREAMING-AWS4-HMAC-SHA256-PAYLOAD
```

**Problem:** Real AWS clients (boto3, MinIO mc, AWS CLI) appear to NOT be sending this header consistently, or are sending chunked data in a different format.

**Evidence:**
- Integration tests (manual header): ✅ PASS - Middleware activates, decoding works
- Client tests (real AWS clients): ❌ FAIL - Chunked markers still in output

### Root Cause Analysis (Updated)

The issue has two possible explanations:

1. **Header Detection Failure:**
   - Real AWS clients may not send `X-Amz-Content-Sha256: STREAMING-AWS4-HMAC-SHA256-PAYLOAD`
   - OR they send it only for specific operations (multipart uploads?)
   - Middleware doesn't activate, raw chunked data passes through

2. **Dual Chunking:**
   - Clients might use HTTP Transfer-Encoding: chunked (standard HTTP)
   - AND AWS SigV4 payload chunking (application-level)
   - Go's HTTP server auto-decodes HTTP chunking
   - But AWS SigV4 chunking still needs decoding

### What Still Needs to Be Fixed

1. **Investigate header detection:**
   - Add logging to see what headers real clients send
   - Check if `X-Amz-Content-Sha256` header is present
   - Verify header value matches `STREAMING-AWS4-HMAC-SHA256-PAYLOAD`

2. **Fallback detection mechanism:**
   - If header isn't reliable, detect chunked encoding by inspecting body format
   - Look for pattern: `{hex};chunk-signature=`
   - Peek at first bytes of request body to detect encoding

3. **Verify middleware placement:**
   - Ensure middleware runs AFTER auth but BEFORE handlers
   - Confirm middleware is actually executing for failing requests

4. **Test with real clients:**
   - Add debug logging to middleware
   - Run client tests with logging enabled
   - Verify why middleware isn't activating

## Fix Priority

**This must be fixed before:**
- Any production use
- Implementing object tagging
- Implementing multipart uploads properly
- Claiming compatibility with any S3 client

**Current Status:**
- ✅ Decoder implementation complete and tested
- ✅ Middleware infrastructure in place
- ❌ Middleware activation detection failing for real AWS clients
- ❌ Client compatibility tests still failing

**Next Steps:**
1. Add debug logging to chunked encoding middleware
2. Run client tests to capture what headers are actually sent
3. Determine why `X-Amz-Content-Sha256` header detection isn't working
4. Implement fallback detection if header is unreliable
5. Re-run client tests to verify complete fix

## Debugging Steps

To diagnose why the middleware isn't activating for real AWS clients:

### 1. Add Logging to Middleware

```go
// In internal/middleware/chunked.go:24
func ChunkedEncoding(decoderFactory ChunkedDecoderFactory) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            contentSHA256 := r.Header.Get(headers.ContentSHA256)

            // DEBUG: Log header value
            log.Printf("DEBUG: X-Amz-Content-Sha256 = %q", contentSHA256)
            log.Printf("DEBUG: All headers: %+v", r.Header)

            if contentSHA256 == consts.ContentSHA256Streaming {
                log.Printf("DEBUG: Activating chunked encoding decoder")
                // ... decoder activation
            } else {
                log.Printf("DEBUG: NOT activating decoder (header mismatch)")
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

### 2. Run Client Tests with Debugging

```bash
# Run MinIO mc test to see what headers are sent
go test -v ./tests/clients/... -run TestMinIOMC 2>&1 | grep -A5 "DEBUG:"

# Look for:
# - What value is in X-Amz-Content-Sha256 header?
# - Is the header present at all?
# - Are there other headers that indicate chunked encoding?
```

### 3. Check for Alternative Headers

AWS clients might use different indicators:
- `Transfer-Encoding: chunked` (HTTP-level, auto-decoded by Go)
- `Content-Encoding: aws-chunked` (alternative marker)
- `x-amz-decoded-content-length` (present with chunked uploads)

### 4. Inspect Raw Request Body

Add logging to peek at first 100 bytes of request body:
```go
// Read first bytes to detect format
buf := make([]byte, 100)
n, _ := r.Body.Read(buf)
log.Printf("DEBUG: First %d bytes: %q", n, buf[:n])
// Create reader that includes the peeked bytes
r.Body = io.MultiReader(bytes.NewReader(buf[:n]), r.Body)
```

Look for pattern: `{hex};chunk-signature=` at start of body

### 5. Test with AWS CLI

AWS CLI may behave differently than boto3/mc:
```bash
# Enable AWS CLI debug logging
aws --debug s3 cp test.txt s3://bucket/test.txt 2>&1 | grep -i "chunk\|streaming"
```

## References

- AWS Signature Version 4 with chunked transfer encoding: https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-streaming.html
- AWS SDK chunked upload behavior: https://github.com/aws/aws-sdk-go/issues/1816
- MinIO client chunked encoding: https://github.com/minio/minio-go
- Integration tests: `tests/integration/chunked_encoding_test.go` (all passing)
- Client test evidence: `tests/clients/scripts/mc.sh` lines 215-243 (object tagging), 246-274 (multipart)
- Test results: `bugs/TEST_RESULTS_2026-01-31.md`
