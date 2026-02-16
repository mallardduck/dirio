# Bug #005: Multipart Upload Content Corruption

**Status:** ✅ RESOLVED  
**Priority:** High  
**Discovered:** 2026-01-31  
**Resolved:** 2026-02-16  
**Affects:** MinIO mc (large file uploads), likely boto3 multipart operations  
**Root Cause:** Bug #001 (AWS SigV4 Chunked Encoding Corruption)  
**Resolution:** Multipart upload implementation now handles chunked encoding correctly - content integrity verified for all clients

## Summary

Multipart uploads appear to succeed, but the downloaded content is corrupted with AWS SigV4 chunked transfer encoding artifacts. A 10 MiB file becomes 10,500,246 bytes (14,486 extra bytes) when downloaded.

## Evidence

### MinIO mc Test Output

```bash
# Create exact 10 MiB file
dd if=/dev/zero of=/tmp/large-file.dat bs=1M count=10
# Creates: 10,485,760 bytes (exactly 10 * 1024 * 1024)

# Upload via mc (triggers multipart for files >5MB)
mc cp /tmp/large-file.dat dirio/bucket/large-file.dat
# Upload succeeds ✅

# Check metadata
mc stat dirio/bucket/large-file.dat
# Size: 10 MiB ✅ (metadata shows correct size)

# Download the file
mc cp dirio/bucket/large-file.dat /tmp/downloaded.dat

# Compare file sizes
ls -l /tmp/large-file.dat /tmp/downloaded.dat
# Original:   10,485,760 bytes
# Downloaded: 10,500,246 bytes ❌
# Difference: +14,486 bytes
```

### Content Analysis

When inspecting the downloaded file, it contains AWS SigV4 chunked encoding headers interspersed with the actual data:

```
{chunk-size-hex};chunk-signature={signature-hash}
{actual-data-chunk}
{chunk-size-hex};chunk-signature={signature-hash}
{actual-data-chunk}
...
0;chunk-signature={final-signature}
```

The extra 14,486 bytes come from:
- Chunk size declarations (hex numbers)
- Chunk signature lines
- Newlines between chunks
- Final zero-size chunk marker

## Reproduction Steps

1. Create a 10 MiB test file:
   ```bash
   dd if=/dev/zero of=test-10mb.dat bs=1M count=10
   ```

2. Check original size:
   ```bash
   ls -l test-10mb.dat
   # Should be exactly 10,485,760 bytes
   ```

3. Upload via MinIO mc (uses multipart for >5MB):
   ```bash
   mc cp test-10mb.dat dirio/bucket/test-10mb.dat
   ```

4. Download the file:
   ```bash
   mc cp dirio/bucket/test-10mb.dat downloaded.dat
   ```

5. Compare sizes:
   ```bash
   ls -l test-10mb.dat downloaded.dat
   # Expected: Both 10,485,760 bytes
   # Actual: Downloaded is ~14KB larger
   ```

6. Compare byte-for-byte:
   ```bash
   cmp test-10mb.dat downloaded.dat
   # Files differ
   ```

## Root Cause Analysis

**This is a direct symptom of Bug #001 (AWS SigV4 Chunked Encoding Corruption).**

AWS Signature V4 with chunked transfer encoding is used for large uploads. The format:

```
{chunk-size};chunk-signature={signature}
{chunk-data}
{chunk-size};chunk-signature={signature}
{chunk-data}
...
0;chunk-signature={final-signature}
```

What's happening:
1. Client uploads multipart using chunked transfer encoding
2. Each part is sent with AWS SigV4 chunked encoding headers
3. Server's UploadPart handler writes **entire chunked payload** to storage
4. Includes chunk size declarations, signatures, and newlines
5. CompleteMultipartUpload assembles parts (with encoding artifacts)
6. Metadata shows correct size (from Content-Length header)
7. Actual stored content is corrupted with ~14KB of encoding overhead

## Impact

**Data Corruption:**
- All multipart uploads result in corrupted files
- Downloaded files cannot be used (wrong size, wrong content)
- Silent corruption (upload reports success, metadata looks correct)

**Feature Status:**
- ❌ Multipart uploads appear to work but produce corrupted data
- ❌ Any file >5MB uploaded via mc will be corrupted
- ❌ boto3 multipart operations likely affected (returns 405 - bug #013)

**Use Cases Affected:**
- Large file uploads (>5MB)
- Video streaming applications
- Backup systems
- Data migration tools
- Any application relying on large file storage

**Clients Affected:**
- ✅ MinIO mc: Uploads succeed, content corrupted with +14KB
- ❌ boto3: Returns 405 Method Not Allowed (bug #013)
- ❓ AWS CLI: Needs verification for multipart operations

## Current Behavior

| Client | Upload Status | Metadata | Downloaded Content |
|--------|---------------|----------|-------------------|
| MinIO mc | ✅ Succeeds | ✅ Correct size | ❌ Corrupted (+14KB) |
| boto3 | ❌ 405 Error | N/A | N/A |
| AWS CLI | ❓ Untested | ❓ | ❓ |

## Proposed Fix

**This bug cannot be fixed independently - it requires fixing Bug #001 first.**

### After Bug #001 is Fixed:

1. **Verify Multipart Part Storage:**
   - Ensure UploadPart handler correctly parses chunked encoding
   - Store only raw data chunks, not encoding headers
   - Verify each part's content is clean

2. **Test CompleteMultipartUpload:**
   - Verify parts are assembled correctly
   - Check final object size matches sum of parts
   - Ensure no encoding artifacts in final object

3. **Add Content Verification Tests:**
   - Upload 10MB file via multipart
   - Download and verify byte-for-byte match
   - Test with multiple clients (mc, boto3, AWS CLI)
   - Test various file sizes (5MB, 10MB, 100MB, 1GB)

## Testing

Confirmed in: `tests/clients/scripts/mc.sh` (lines 252-283)

```bash
# Create 10MB file
dd if=/dev/zero of=/tmp/large-file.dat bs=1M count=10

# Upload via multipart
mc cp /tmp/large-file.dat ${MC_ALIAS}/${BUCKET}/large-file.dat

# Download and compare
mc cp ${MC_ALIAS}/${BUCKET}/large-file.dat /tmp/downloaded.dat
cmp -s /tmp/large-file.dat /tmp/downloaded.dat

# FAIL: Files differ
# Original:   10,485,760 bytes
# Downloaded: 10,500,246 bytes
# Difference: +14,486 bytes (chunked encoding overhead)
```

## Related Issues

- **#001: AWS SigV4 Chunked Encoding Corruption** (ROOT CAUSE) - Must be fixed first
- #013: Multipart upload 405 for boto3 - Different issue (routing/method handling)

## Technical Details

### AWS S3 Multipart Upload API

1. **CreateMultipartUpload:** `POST /bucket/object?uploads`
2. **UploadPart:** `PUT /bucket/object?partNumber=X&uploadId=Y` (sends part data with chunked encoding)
3. **CompleteMultipartUpload:** `POST /bucket/object?uploadId=Y` (assembles parts into final object)

### Expected Behavior

- Each UploadPart should store clean part data (no encoding headers)
- CompleteMultipartUpload should concatenate parts byte-for-byte
- Final object size should equal sum of part sizes
- Downloaded content should be identical to uploaded content

### Actual Behavior

- UploadPart stores part data WITH chunked encoding headers
- CompleteMultipartUpload assembles corrupted parts
- Final object is ~1.4% larger than expected (14KB overhead on 10MB)
- Downloaded content contains encoding artifacts

## Evidence Files

Test data location: `tests/clients/scripts/mc.sh`

Example corrupted content pattern:
```
1000;chunk-signature=abc123...
{1000 bytes of data}
1000;chunk-signature=def456...
{1000 bytes of data}
...
0;chunk-signature=xyz789...
```

## Priority Justification

This is a HIGH priority bug because:
1. Affects all large file uploads (>5MB threshold for multipart)
2. Silent data corruption (operations report success)
3. Makes DirIO unusable for any large file storage use cases
4. Blocks migration from MinIO (users would lose data integrity)
5. Root cause (bug #001) affects multiple features, making this widespread
