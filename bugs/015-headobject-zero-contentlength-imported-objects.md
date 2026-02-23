# Bug #015: HeadObject Returns ContentLength=0 for MinIO-Imported Objects

**Status:** Open
**Priority:** High
**Discovered:** 2026-02-23
**Affects:** HeadObject on any object whose metadata was imported from MinIO (all imported objects)

## Summary

`HeadObject` returns `ContentLength: 0` for objects imported from a MinIO data directory.
The underlying object files are intact and the correct size is returned by `GetObject`, but
`HeadObject` reads size from the stored metadata record which is set to 0 at import time.

## Evidence

```
━━━ 11. Large File (10MB Multipart Upload) ━━━
  ✗ alpha/large-file.dat: 0 bytes (expected ≥10485760)
  ✗ gamma/large-public.dat: missing or wrong size (got '0')
```

`GetObject` (section 4) returns the correct size for the same bucket:
```
  ✓ alpha/alice-object.bin: GetObject returned 65536 bytes
```

This confirms the data files are intact; the bug is in metadata only.

## Root Cause Analysis

The chain that causes size=0:

### 1. MinIO `fs.json` has no reliable size field

`internal/minio/types.go:163` — the `ObjectMetadata` struct mirrors MinIO's `fs.json`:
```go
type ObjectMetadata struct {
    Version  string            `json:"version"`
    Checksum ChecksumInfo      `json:"checksum"`
    Meta     map[string]string `json:"meta"`
}

type ChecksumInfo struct {
    Algorithm string   `json:"algorithm"`
    BlockSize int      `json:"blocksize"`  // often 0 in practice
    Hashes    []string `json:"hashes"`
}
```
MinIO's `fs.json` does not store the total object size. `BlockSize` is a checksum chunk
size, not the object size, and is frequently 0 in real data.

### 2. Import sets `Size: 0` unconditionally

`internal/persistence/metadata/import.go:170`:
```go
dirioMeta := &ObjectMetadata{
    Version:        ObjectMetadataVersion,
    ContentType:    minioMeta.Meta["content-type"],
    ETag:           minioMeta.Meta["etag"],
    CustomMetadata: make(map[string]string),
    // Size is never set → defaults to 0
}
```

### 3. Storage layer returns stored size as-is

`internal/persistence/storage/object.go:GetObjectMetadata` (line 299):
```go
meta, err := s.metadata.GetObjectMetadata(ctx, bucket, key)
if err != nil {
    // fallback to info.Size() only when metadata is MISSING
    meta = &metadata.ObjectMetadata{Size: info.Size(), ...}
}
return meta, nil  // if metadata exists (even with Size=0), no fallback
```

`GetObject` (line 99) correctly uses `info.Size()` directly and is unaffected.
`HeadObject` calls `GetObjectMetadata`, which hits the broken path.

## Impact

**Functionality:**
- `HeadObject` (`s3api head-object`) reports wrong size for all imported objects
- Clients using HeadObject to check file size before downloading get incorrect data
- S3 sync tools that use HeadObject for size comparison will behave incorrectly

**Clients Affected:**
- ❌ AWS CLI `s3api head-object --query ContentLength`
- ❌ boto3 `head_object()['ContentLength']`
- ✅ `GetObject` (reads size from file stat, unaffected)
- ✅ `ListObjects` / `ListObjectsV2` (reads size from file stat, unaffected)

**Workarounds:**
- None for HeadObject callers

## Proposed Fix

Two options — both may be needed in combination:

### Option A: Fallback in storage layer (defensive fix, handles existing imported data)

In `internal/persistence/storage/object.go:GetObjectMetadata`, fall back to `info.Size()`
when stored size is 0:
```go
meta, err := s.metadata.GetObjectMetadata(ctx, bucket, key)
if err != nil || meta.Size == 0 {
    // No metadata or size missing — derive from file stat
    if meta == nil { meta = &metadata.ObjectMetadata{...} }
    meta.Size = info.Size()
}
return meta, nil
```

### Option B: Set size during import (fix at the source)

In `internal/persistence/metadata/import.go`, stat the actual object file to get size,
or skip setting Size so that option A's fallback takes over naturally.

Option A alone is sufficient for already-imported data and handles future imports where
MinIO metadata genuinely has no size. Option B prevents silent 0-size metadata records
from accumulating.

## Testing

Confirmed in: `scripts/validate-setup.sh` (section 11)

```bash
aws --endpoint-url http://localhost:9000 s3api head-object \
  --bucket alpha --key large-file.dat \
  --query 'ContentLength' --output text
# Returns 0 — should return 10485760
```

## Related Issues

- Bug #005: Multipart content corruption — same multipart objects are affected
- The fix touches `internal/persistence/storage/object.go` — same file as GetObject range logic

## Files to Investigate

- `internal/persistence/storage/object.go:299` — `GetObjectMetadata` missing fallback
- `internal/persistence/metadata/import.go:170` — import never sets `Size`
- `internal/minio/types.go:163` — `ObjectMetadata` has no size field
