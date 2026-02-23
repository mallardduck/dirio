# Bug #014: ListObjects V1 Returns Empty CommonPrefixes

**Status:** ✅ RESOLVED
**Priority:** High
**Discovered:** 2026-02-23
**Resolved:** 2026-02-23
**Affects:** Any client using ListObjectsV1 with a delimiter (aws CLI `s3api list-objects`, boto3 `list_objects`)
**Resolution:** Changed V1 listing path to return `storage.InternalResult` (same as V2), wiring `CommonPrefixes`, `IsTruncated`, `NextMarker`, and `Marker` through all three layers

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

1. **Storage layer** — `storage/storage.go:ListObjects` returned only `[]s3types.Object`,
   discarding the `CommonPrefixes` already computed by `listInternal`:
   ```go
   func (s *Storage) ListObjects(...) ([]s3types.Object, error) {
       result, _ := s.listInternal(...)
       return result.Objects, nil  // CommonPrefixes silently dropped
   }
   ```

2. **Service layer** — `service/s3/s3.go:ListObjects` returned `[]s3types.Object`
   instead of a structured result. CommonPrefixes were never forwarded.

3. **HTTP handler** — `internal/http/api/s3/bucket.go:ListObjects` built the response
   without `CommonPrefixes`, `IsTruncated`, or `NextMarker`, and never passed the `marker`
   query parameter down to the service.

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

## Resolution

Fixed all three layers to mirror the working V2 path:

### 1. `internal/persistence/storage/storage.go`

`ListObjects` now returns `InternalResult` instead of `[]s3types.Object`, and accepts
a `marker` parameter passed through as `startAt` to `listInternal`:

```go
func (s *Storage) ListObjects(ctx context.Context, bucket, prefix, marker, delimiter string, maxKeys int) (InternalResult, error) {
    return s.listInternal(ctx, bucket, prefix, marker, delimiter, maxKeys, false)
}
```

### 2. `internal/service/s3/types.go`

Added `Marker` field to `ListObjectsRequest` (was a TODO):

```go
type ListObjectsRequest struct {
    Bucket    string
    Prefix    string
    Marker    string
    Delimiter string
    MaxKeys   int
}
```

### 3. `internal/service/s3/s3.go`

`ListObjects` now returns `storage.InternalResult`:

```go
func (s *Service) ListObjects(ctx context.Context, req *ListObjectsRequest) (storage.InternalResult, error) {
    return s.storage.ListObjects(ctx, req.Bucket, req.Prefix, req.Marker, req.Delimiter, req.MaxKeys)
}
```

### 4. `internal/http/api/s3/bucket.go`

Handler now passes `marker` into the request and populates `CommonPrefixes`, `IsTruncated`,
and `NextMarker` in the response:

```go
response := s3types.ListBucketResult{
    ...
    Marker:         marker,
    NextMarker:     result.NextMarker,
    IsTruncated:    result.IsTruncated,
    Contents:       filteredObjects,
    CommonPrefixes: result.CommonPrefixes,
}
```

## Testing

Confirmed in: `scripts/validate-setup.sh` (section 8, first two assertions)

```bash
aws --endpoint-url http://localhost:9000 --output json \
  s3api list-objects --bucket alpha --delimiter "/"
# CommonPrefixes now present in response
```

## Related Issues

- Bug #007: ListObjects MaxKeys ignored (same code path, V1 listing)
- ListObjectsV2 (`api/s3/bucket.go:ListObjectsV2`) served as the reference implementation

## Files Changed

- `internal/persistence/storage/storage.go` — `ListObjects` returns `InternalResult`, accepts `marker`
- `internal/service/s3/types.go` — added `Marker` field to `ListObjectsRequest`
- `internal/service/s3/s3.go` — `ListObjects` returns `storage.InternalResult`
- `internal/http/api/s3/bucket.go` — handler populates `CommonPrefixes`, `IsTruncated`, `NextMarker`; passes `marker`
