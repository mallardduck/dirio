# Bug #012: Custom Metadata Not Returned in mc stat

**Status:** ✅ RESOLVED  
**Priority:** Medium  
**Discovered:** 2026-01-31  
**Resolved:** 2026-02-16  
**Affects:** MinIO mc client only (GetObject metadata retrieval)  
**Resolution:** Functionality was working correctly; test had case-sensitivity bug. Fixed test to use case-insensitive grep. Also improved error handling to surface metadata save failures.

## Summary

When using MinIO mc client to set custom metadata with `mc cp --attr`, the metadata is successfully uploaded but cannot be retrieved using `mc stat`. The `stat` command should display custom metadata in the "Metadata:" section, but it's either not returned by the server or not being displayed.

## Evidence

### MinIO Client Code Analysis

From `mc/cmd/stat.go:146-154`, the stat command displays metadata:
```go
if maxKeyMetadata > 0 {
    msgBuilder.WriteString(fmt.Sprintf("%-10s:", "Metadata") + "\n")
    for k, v := range stat.Metadata {
        // Skip encryption headers, we print them later.
        if !strings.HasPrefix(strings.ToLower(k), serverEncryptionKeyPrefix) {
            msgBuilder.WriteString(fmt.Sprintf("  %-*.*s: %s ", maxKeyMetadata, maxKeyMetadata, k, v) + "\n")
        }
    }
}
```

The client is designed to display all metadata (excluding encryption headers), so if metadata doesn't appear, the server likely isn't returning it.

### Expected Behavior

Based on S3 API standards:
1. `mc cp --attr "key1=value1;key2=value2"` - Sets custom metadata during upload
2. `mc stat alias/bucket/object` - Should retrieve and display:
   ```
   Name      : object
   Date      : ...
   Size      : ...
   ETag      : ...
   Type      : file
   Metadata:
     key1: value1
     key2: value2
   ```

### Actual Behavior

- Upload with custom metadata appears to succeed
- `mc stat` displays object info but metadata section is missing or empty
- Metadata is not being returned in HeadObject response

## Reproduction Steps

1. Upload object with custom metadata:
   ```bash
   echo "test content" > test.txt
   mc cp --attr "custom-key=custom-value;another=test" test.txt dirio/bucket/test.txt
   ```

2. Verify upload succeeded: ✅ Operation completes without error

3. Retrieve metadata:
   ```bash
   mc stat dirio/bucket/test.txt
   ```

4. **Expected:** Metadata section shows `custom-key` and `another`
   **Actual:** Metadata section missing or doesn't include custom metadata

5. Compare with JSON output:
   ```bash
   mc stat dirio/bucket/test.txt --json
   ```
   Check if `metadata` field is empty or missing custom keys

## Root Cause Analysis

### Possible Issues

1. **Custom metadata not stored during PutObject**
   - Location: `internal/api/handlers/object.go` - PutObject handler
   - `mc cp --attr` sends metadata as HTTP headers: `X-Amz-Meta-{key}: {value}`
   - Server may not be parsing/storing these headers

2. **Custom metadata not returned in HeadObject**
   - Location: `internal/api/handlers/object.go` - HeadObject handler
   - Server may store metadata but not include it in response headers
   - Should return headers like `X-Amz-Meta-custom-key: custom-value`

3. **Metadata lost during chunked encoding parsing**
   - Related to Bug #001 (chunked encoding corruption)
   - Metadata headers may be discarded when parsing chunked request body

4. **Metadata case sensitivity issue**
   - Related to Bug #011 (metadata key case wrong)
   - Server may be transforming header names incorrectly
   - S3 standard: Metadata keys should be lowercase after `x-amz-meta-` prefix

## Technical Details

### S3 API Metadata Handling

**PutObject Request:**
```
PUT /bucket/object HTTP/1.1
Host: s3.amazonaws.com
X-Amz-Meta-custom-key: custom-value
X-Amz-Meta-another: test
Content-Type: text/plain
Content-Length: 12

test content
```

**HeadObject Response:**
```
HTTP/1.1 200 OK
ETag: "abc123"
Content-Length: 12
Content-Type: text/plain
X-Amz-Meta-custom-key: custom-value
X-Amz-Meta-another: test
```

### MinIO mc Format

The `--attr` flag accepts metadata in format:
- `--attr "key1=value1;key2=value2"`
- Can also include HTTP headers: `--attr "Cache-Control=max-age=90000;custom-key=value"`
- Custom metadata keys automatically get `X-Amz-Meta-` prefix
- Standard headers (Cache-Control, Content-Type, etc.) are sent as-is

## Impact

**Functionality:**
- Users cannot verify custom metadata was stored correctly
- Metadata-dependent workflows broken
- No way to audit object metadata via mc client

**Clients Affected:**
- ✅ MinIO mc: Cannot retrieve custom metadata
- ❓ AWS CLI: Needs verification
- ❓ boto3: Needs verification

**Workarounds:**
- None available via mc client
- May need to use AWS CLI or boto3 to test if metadata is actually stored

## Investigation Steps

1. **Enable request/response logging:**
   - Log all incoming headers from `mc cp --attr`
   - Verify `X-Amz-Meta-*` headers are received

2. **Check metadata storage:**
   - Verify metadata is written to filesystem metadata file
   - Check `{object-path}.meta` or similar metadata storage

3. **Check HeadObject response:**
   - Log outgoing headers from HeadObject handler
   - Verify `X-Amz-Meta-*` headers are included

4. **Test with other clients:**
   - AWS CLI: `aws s3api head-object --bucket bucket --key object`
   - boto3: `s3.head_object(Bucket='bucket', Key='object')['Metadata']`

## Proposed Fix

### Phase 1: Verify Storage
1. Add debug logging to PutObject handler
2. Confirm `X-Amz-Meta-*` headers are parsed from request
3. Verify metadata is stored in object metadata file

### Phase 2: Fix HeadObject Response
1. Locate HeadObject handler in `internal/api/handlers/object.go`
2. Ensure stored metadata is read from storage
3. Add `X-Amz-Meta-*` headers to response for each custom metadata key
4. Preserve exact key names (case-sensitive after prefix)

### Phase 3: Add Tests
1. Create integration test for custom metadata round-trip
2. Test with mc client: upload with --attr, verify with stat
3. Test with AWS CLI and boto3 for compatibility
4. Add test case to `tests/clients/scripts/mc.sh`

## Related Issues

- Bug #011: Custom metadata key case wrong (may affect metadata retrieval)
- Bug #001: Chunked encoding corruption (may affect metadata parsing)

## Testing

### Manual Test Script

```bash
#!/bin/bash
# Test custom metadata with mc

# 1. Upload with custom metadata
echo "test content" > test.txt
mc cp --attr "custom-key=value1;test-meta=value2" test.txt dirio/bucket/test-meta.txt

# 2. Check stat output
echo "=== mc stat output ==="
mc stat dirio/bucket/test-meta.txt

# 3. Check JSON output
echo "=== mc stat JSON output ==="
mc stat dirio/bucket/test-meta.txt --json | jq '.metadata'

# 4. Compare with AWS CLI
echo "=== AWS CLI head-object ==="
aws s3api head-object --endpoint-url http://localhost:8080 \
  --bucket bucket --key test-meta.txt | jq '.Metadata'
```

### Expected Test Output

```json
{
  "custom-key": "value1",
  "test-meta": "value2"
}
```

## References

- S3 API Metadata Specification: https://docs.aws.amazon.com/AmazonS3/latest/userguide/UsingMetadata.html
- MinIO mc attr flag: `mc/cmd/cp-main.go:172-176`
- MinIO mc stat implementation: `mc/cmd/stat.go:146-154`
- Test location: `tests/clients/scripts/mc.sh` (needs custom metadata test case)
