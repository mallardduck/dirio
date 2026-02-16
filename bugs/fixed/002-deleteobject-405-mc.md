# Bug #002: DeleteObject Returns 405 Method Not Allowed (MinIO mc)

**Status:** ✅ RESOLVED  
**Priority:** High  
**Discovered:** 2026-01-31  
**Resolved:** 2026-02-16  
**Affects:** MinIO mc client only (AWS CLI and boto3 work correctly)  
**Resolution:** Added POST fallback route for QueryPOST auto-promotion in teapot-router

## Summary

When using MinIO mc client to delete objects (`mc rm`), the server returns `405 Method Not Allowed`. The same operation works perfectly with AWS CLI and boto3.

## Evidence

Test output from MinIO mc:
```
mc: <ERROR> Failed to remove `dirio/mc-test-bucket-1769881133/test.txt`. 405 Method Not Allowed
FAIL: DeleteObject (mc rm)
```

## Reproduction Steps

1. Upload an object: `mc cp file.txt dirio/bucket/test.txt` ✅ Works
2. Try to delete: `mc rm dirio/bucket/test.txt` ❌ Fails with 405
3. Compare with AWS CLI: `aws s3 rm s3://bucket/test.txt` ✅ Works

## Root Cause

Likely one of:
1. **Routing issue:** mc client may use different HTTP method or endpoint path
2. **Method handler missing:** DELETE method not registered for mc's request format
3. **Content-Type issue:** mc may send different headers causing route mismatch
4. **Query parameter difference:** mc may include query params that AWS CLI doesn't

## Impact

- MinIO mc users cannot delete objects
- Blocks complete mc workflow (can upload but not clean up)
- DeleteBucket also fails because bucket cannot be emptied (bug #003)
- AWS CLI and boto3 are unaffected

## Proposed Fix

1. Enable request logging to compare:
   - AWS CLI DELETE request (works)
   - mc DELETE request (fails)
2. Check routing configuration in `internal/api/router.go`
3. Verify HTTP method handlers in `internal/api/handlers/object.go`
4. Add integration test specifically for mc DeleteObject

## Related Issues

- #003: DeleteBucket returns 405 for MinIO mc (blocked by this bug)

## Investigation Notes

Since AWS CLI and boto3 work, the DELETE operation itself is implemented. The issue is specific to how mc makes the request, suggesting:
- Different URL path format
- Different headers
- Different query parameters
- Different HTTP method variant (DELETE with body?)

## Testing

Confirmed in: `tests/clients/clients_test.go` - TestMinIOMC
```
mc rm ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
# ERROR: 405 Method Not Allowed
```
