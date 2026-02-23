# Bug #014: ListObjects V1 Returns Empty CommonPrefixes

**Status:** ✅ RESOLVED  
**Priority:** High
**Discovered:** 2026-02-23
**Resolved:** 2026-02-23
**Affects:** Any client using ListObjectsV1 with a delimiter (aws CLI `s3api list-objects`, boto3 `list_objects`)
**Resolution:** Modify v1 list objects to output `storage.InternalResult` like v2 version does among other small refactors

## Summary

`ListObjects` (V1) never returns `CommonPrefixes` in its XML response even when a `delimiter` is provided and the bucket contains objects whose keys share a common prefix up to that delimiter. The `Contents` list is correct, but the `CommonPrefixes` element is always empty, breaking folder-simulation for V1 callers.

## Evidence

```
━━━ 8. Folder Structure — ListObjects prefix/delimiter ━━━
  ✗ alpha: expected common prefix 'folder1/' (got: None)
  ✗ alpha: expected common prefix 'folder2/' (got: None)
  ✓ alpha: root-file.txt visible in top-level (non-prefixed) listing
  ✓ alpha: 'folder1/file1.txt' present under prefix 'folder1/'
```

The prefix-filtered listing (no delimiter) returns objects correctly, showing the issue
is specifically with CommonPrefixes computation, not object retrieval.

V2 (`list-type=2`) works correctly — `ListObjectsV2` propagates CommonPrefixes from the
storage layer.

## Reproduction Steps

1. Have a bucket with keys `folder1/file1.txt`, `folder1/file2.txt`, `root-file.txt`
2. Run: `aws s3api list-objects --bucket alpha --delimiter "/"`
3. **Expected:** `CommonPrefixes` contains `[{ "Prefix": "folder1/" }]`
4. **Actual:** `CommonPrefixes` is absent/empty

## Root Cause Analysis

Three layers all fail to propagate CommonPrefixes in the V1 path:

1. **Service layer** — `service/s3/s3.go:ListObjects` (line 217) returns `[]s3types.Object`
   instead of a structured result. CommonPrefixes computed by the storage layer are
   discarded silently:
   ```go
   func (s *Service) ListObjects(...) ([]s3types.Object, error) {
       objects, err := s.storage.ListObjects(ctx, req.Bucket, req.Prefix, req.Delimiter, req.MaxKeys)
       ...
       return objects, nil  // CommonPrefixes thrown away here
   }
   ```

2. **HTTP handler** — `internal/http/api/s3/bucket.go:ListObjects` (line 198) builds
   the response without a `CommonPrefixes` field:
   ```go
   response := s3types.ListBucketResult{
       ...
       Contents: filteredObjects,
       // CommonPrefixes: never set
   }
   ```

3. **Type** — `s3types.ListBucketResult` has `CommonPrefixes []CommonPrefix` (line 23 of
   `pkg/s3types/responses.go`) but it is never populated.

The V2 path doesn't have this problem: `service/s3/s3.go:ListObjectsV2` returns
`storage.InternalResult` (which carries `CommonPrefixes`) and the V2 handler passes it
through correctly.

## Impact

**Functionality:**
- Folder browsing via ListObjectsV1 is broken — virtual directories never appear
- AWS CLI `s3 ls`, boto3 `list_objects`, and any other V1 caller that uses delimiter cannot
  simulate a folder hierarchy

**Clients Affected:**
- ❌ AWS CLI v1 `s3api list-objects --delimiter /`
- ❌ boto3 `list_objects` with `Delimiter='/'`
- ✅ AWS CLI v2 / boto3 `list_objects_v2` — not affected (V2 path is correct)

**Workarounds:**
- Use ListObjectsV2 (`list-type=2`) instead of V1

## Proposed Fix

1. Change `storage.ListObjects` to return a structured result (like `storage.InternalResult`)
   that includes both `Objects` and `CommonPrefixes`, OR add a separate storage method.

2. Update `service/s3/s3.go:ListObjects` to return that structured result instead of
   `[]s3types.Object`.

3. Update `internal/http/api/s3/bucket.go:ListObjects` to populate `CommonPrefixes` in
   the `ListBucketResult` response from the service result.

## Testing

Confirmed in: `scripts/validate-setup.sh` (section 8, first two assertions)

```bash
S3_ENDPOINT=http://localhost:9000 \
  aws --endpoint-url http://localhost:9000 --output json \
  s3api list-objects --bucket alpha --delimiter "/"
# CommonPrefixes should be present but is absent
```

## Related Issues

- Bug #007: ListObjects MaxKeys ignored (same code path, V1 listing)
- ListObjectsV2 (`api/s3/bucket.go:ListObjectsV2`) handles CommonPrefixes correctly —
  can serve as a reference implementation for the fix

## Files to Investigate

- `internal/persistence/storage/storage.go` — `ListObjects` return type
- `internal/service/s3/s3.go:217` — `ListObjects` discards CommonPrefixes
- `internal/http/api/s3/bucket.go:163` — handler builds response without CommonPrefixes
- `pkg/s3types/responses.go:13` — `ListBucketResult.CommonPrefixes` field exists but unused
