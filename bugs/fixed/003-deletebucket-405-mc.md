# Bug #003: DeleteBucket Returns 405 Method Not Allowed (MinIO mc)

**Status:** ✅ RESOLVED  
**Priority:** High  
**Discovered:** 2026-01-31  
**Resolved:** 2026-02-16  
**Affects:** MinIO mc client only (AWS CLI and boto3 work correctly)  
**Resolution:** Added POST fallback route for QueryPOST auto-promotion in teapot-router (same fix as bug #002)

## Summary

When using MinIO mc client to delete buckets (`mc rb`), the server returns `405 Method Not Allowed`. The same operation works perfectly with AWS CLI and boto3.

## Evidence

Test output from MinIO mc:
```
mc: <ERROR> Failed to remove bucket `dirio/mc-test-bucket-1769881133`. 405 Method Not Allowed
FAIL: DeleteBucket (mc rb)
```

## Reproduction Steps

1. Create a bucket: `mc mb dirio/bucket` ✅ Works
2. Upload and delete objects to empty the bucket ❌ Blocked by bug #002 (DeleteObject also fails)
3. Try to delete bucket: `mc rb dirio/bucket` ❌ Fails with 405
4. Compare with AWS CLI: `aws s3 rb s3://bucket` ✅ Works

## Root Cause

Likely one of:
1. **Routing issue:** mc client may use different HTTP method or endpoint path
2. **Method handler missing:** DELETE method not registered for mc's request format
3. **Content-Type issue:** mc may send different headers causing route mismatch
4. **Query parameter difference:** mc may include query params that AWS CLI doesn't

Very likely the same root cause as bug #002 (DeleteObject 405 for mc).

## Impact

- MinIO mc users cannot delete buckets
- Blocks complete mc workflow (can create but not clean up)
- Blocks testing and development using mc client
- AWS CLI and boto3 are unaffected

## Proposed Fix

1. Enable request logging to compare:
   - AWS CLI DELETE request (works)
   - mc DELETE request (fails)
2. Check routing configuration in `internal/api/router.go`
3. Verify HTTP method handlers in `internal/api/handlers/bucket.go`
4. Add integration test specifically for mc DeleteBucket
5. Fix together with bug #002 (likely same root cause)

## Related Issues

- #002: DeleteObject returns 405 for MinIO mc (likely same root cause)
- This bug is BLOCKED BY #002, since DeleteBucket requires an empty bucket, and DeleteObject must work to empty it

## Investigation Notes

Since AWS CLI and boto3 work, the DELETE operation itself is implemented. The issue is specific to how mc makes the request, suggesting:
- Different URL path format
- Different headers
- Different query parameters
- Different HTTP method variant (DELETE with body?)

This is the same pattern as bug #002, indicating a systematic mc-specific routing issue.

## Testing

Confirmed in: `tests/clients/clients_test.go` - TestMinIOMC
```bash
mc rb ${MC_ALIAS}/${BUCKET} 2>&1
# ERROR: 405 Method Not Allowed
```

## Priority Justification

While AWS CLI and boto3 work, MinIO mc is a key target for compatibility since DirIO is designed to import and replace MinIO instances. Users migrating from MinIO will expect mc to work.
