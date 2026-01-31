# CRITICAL: AWS SigV4 Chunked Encoding Bug

**Discovered:** January 31, 2026
**Status:** 🚨 CRITICAL - Blocks production use
**Impact:** ALL write operations corrupted

## Summary

AWS Signature V4 chunked transfer encoding headers are being written directly to object files instead of being parsed and removed. This causes data corruption in all write operations.

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

### Before Content Verification (False Positives)
- Multipart Upload: ✅ PASS (only checked metadata size)
- Object Tagging: ✅ PASS (only checked operation succeeded)
- GetObject: ✅ PASS (only checked data returned)

### After Content Verification (Exposed Bugs)
- Multipart Upload: ❌ FAIL - Downloaded 10,500,246 bytes instead of 10,485,760
- Object Tagging: ❌ FAIL - Content replaced with XML tags
- GetObject: ❌ FAIL - Content contains chunk markers like `15;chunk-signature=...`

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

## Files to Investigate

Based on the codebase structure, likely locations:

1. **Request body parsing:**
   - `internal/api/handlers/object.go` - PutObject handler
   - `internal/api/handlers/multipart.go` - Multipart upload handlers
   - Look for where request body is read and written to storage

2. **Authentication middleware:**
   - `internal/auth/signature.go` - AWS SigV4 verification
   - May need to handle chunked encoding before/after signature verification

3. **Storage layer:**
   - `internal/storage/*.go` - Where data is written to disk
   - Check if raw request body is being passed through without parsing

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

## Fix Priority

**This must be fixed before:**
- Any production use
- Implementing object tagging
- Implementing multipart uploads properly
- Claiming compatibility with any S3 client

**Recommended approach:**
1. Identify where request body is read in PutObject handler
2. Add chunked transfer encoding parser
3. Extract raw data chunks, verify signatures
4. Write only decoded data to storage
5. Add integration tests that verify byte-for-byte content integrity
6. Re-run all client tests to verify fix

## References

- AWS Signature Version 4 with chunked transfer encoding: https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-streaming.html
- Related discussion in authentication code (if any)
- Test evidence: `tests/clients/scripts/mc.sh` lines 215-243 (object tagging), 246-274 (multipart)
