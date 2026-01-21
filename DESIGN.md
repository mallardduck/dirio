# Design Philosophy

DirIO is built on a simple principle: **objects should be files**.

## Core Tenets

### 1. Filesystem is the Source of Truth

Objects live in `buckets/bucket-name/path/to/object`. That's where your data is. Everything else (metadata, indexes, caches) is derived state that can be regenerated.

If you delete `.metadata/`, DirIO should still work. It might be slower, but the data is there.

### 2. No Encoding, No Chunking

When you PUT an object, it's written to disk as-is. When you GET it, you read that file. No base64, no multipart reassembly, no deduplication tricks.

This makes debugging trivial: `ls` and `cat` work as expected.

### 3. Minimal Metadata

Metadata lives in `.metadata/` as JSON files. Structure:

```
.metadata/
├── users.json              # All users, one file
├── buckets/
│   └── mybucket.json      # Per-bucket: owner, policy, created timestamp
└── .import-state          # MinIO import tracking
```

Object metadata (ETag, content-type) can be stored per-object if needed, but by default we calculate on-the-fly from the filesystem.

### 4. Import, Don't Migrate

When you point DirIO at MinIO data, we don't "migrate" it. We import the metadata into `.metadata/` and leave `.minio.sys/` alone.

Why? Because you might need to switch back. Or debug. Or compare. The cost is a few extra kilobytes.

### 5. Single Binary, No Dependencies

DirIO is one Go binary. No Redis, no PostgreSQL, no external services. Just:
- The binary
- A data directory
- TCP port 9000

## Non-Goals

### We Will Not Support

- **Distributed mode**: Use actual S3 or MinIO if you need clustering
- **Replication**: Use filesystem-level tools (rsync, ZFS send/recv, etc.)
- **Versioning**: Keep it simple; this is Phase N work if ever
- **Advanced IAM**: Basic access keys and bucket policies only

### We Will Not Optimize For

- **Billions of objects**: If you have that scale, use a real object store
- **Sub-millisecond latency**: This is for homelabs, not production SaaS
- **Maximum throughput**: Filesystem I/O is the bottleneck by design

## Architecture Decisions

### Why Go?

- MinIO is Go, so we can reference their code
- Single static binary for easy deployment
- Good stdlib for HTTP and filesystem I/O
- Fast enough for homelab workloads

### Why JSON for Metadata?

- Human-readable for debugging
- Easy to edit manually if needed
- Go stdlib marshaling is fine for our scale
- We're not optimizing for metadata read speed

### Why Not SQLite?

SQLite would be great, but adds dependency complexity. For 10-100 buckets and 1000-100k objects, JSON files are fine.

If we hit performance issues, we can add SQLite as an optional backend later.

### Why msgpack for MinIO Import?

MinIO stores bucket metadata as msgpack (`.metadata.bin`). We need to decode it once during import. After that, we never touch msgpack again—our format is JSON.

## Implementation Principles

### Fail Loud

If something goes wrong, log it clearly and return an error. Don't silently degrade or make assumptions.

Example: If we can't read bucket metadata, return 500, don't pretend the bucket doesn't exist.

### Idempotent Operations

PUT the same object twice? Same result. Import MinIO data twice? Same result. Creating an existing bucket? Error, not silent success.

### Filesystem Writes are Scary

Always write to a temp file first, then atomically rename. Never truncate/overwrite in place.

```go
// Good
tmp := filepath.Join(dir, ".tmp-"+uuid.New().String())
ioutil.WriteFile(tmp, data, 0644)
os.Rename(tmp, finalPath)

// Bad
os.OpenFile(finalPath, os.O_TRUNC|os.O_WRONLY, 0644)
```

### No Global State

Everything should be configurable and testable. Pass dependencies explicitly, don't rely on `init()` functions or global variables.

## Testing Strategy

### Unit Tests

Test each package in isolation. Mock filesystem operations if needed.

### Integration Tests

Spin up a real server, use AWS CLI to test operations end-to-end.

### Migration Tests

Keep a snapshot of MinIO data in `testdata/` and verify import works correctly.

## Performance Expectations

For a typical homelab NAS (4-core CPU, spinning disks):

- **Small objects (<1MB)**: 50-200 req/sec
- **Large objects (>100MB)**: Limited by disk I/O, ~100-500 MB/sec
- **List operations**: 1000-10000 objects/sec

These are rough estimates. Real performance depends on disk speed, CPU, and workload.

## Future Work

Things we might add later (Phase 3+):

- **Multipart uploads**: Required for large files from some clients
- **Pre-signed URLs**: Useful for browser uploads
- **Range requests**: For streaming video/audio
- **Object tagging**: Nice-to-have for organization
- **Bucket versioning**: Complex but requested feature

Things we probably won't add:

- **S3 Select**: Too complex for our use case
- **Glacier storage tiers**: Filesystem is already "one tier"
- **Cross-region replication**: Not applicable to single-node
- **Lambda triggers**: Out of scope entirely

## Contributing Philosophy

If you're adding a feature, ask:

1. Does this make the common case simpler?
2. Can it be done without external dependencies?
3. Is the code easy to understand?
4. Does it align with "filesystem-first"?

If yes to all four, it's probably a good fit.
