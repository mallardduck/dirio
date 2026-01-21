# DirIO Development Roadmap

Current status: **Phase 1 - MVP Scaffold Complete**

## Phase 1: MVP Core ✅ (Scaffolded)

### Completed (Scaffold)
- [x] Project structure
- [x] Basic HTTP server setup
- [x] Storage backend interface
- [x] Metadata manager
- [x] API handlers (skeleton)
- [x] MinIO import logic (skeleton)

### Remaining for Phase 1
- [x] Fix compilation errors
- [x] Implement missing storage error types in API handlers
- [x] Add go.sum file (run `go mod tidy`)
- [x] Test basic server startup
- [ ] Test bucket operations (create, list, delete)
- [ ] Test object operations (put, get, head, delete)
- [ ] Test ListObjectsV2 with various parameters
- [x] Add basic logging
- [ ] Test MinIO import with real data

## Phase 2: Authentication & Security

- [ ] Implement AWS Signature V4 authentication
- [ ] Add authentication middleware
- [ ] Test with AWS CLI
- [ ] Add request ID generation
- [ ] Add access logging

## Phase 3: Advanced Features

### Bucket Policies
- [ ] Parse and validate S3 bucket policies
- [ ] Enforce public-read policies
- [ ] Support more complex policy statements

### Additional S3 Operations
- [ ] Multipart upload support
- [ ] Pre-signed URLs
- [ ] Range requests for GetObject
- [ ] Copy object
- [ ] Object tagging

### Metadata
- [ ] Per-object metadata storage (if needed)
- [ ] Parse MinIO's Created timestamp in import
- [ ] Store custom metadata headers

### Performance
- [ ] Add caching for metadata
- [ ] Optimize ListObjects for large buckets
- [ ] Add concurrent request handling tests

## Phase 4: Operations & Monitoring

- [ ] Health check endpoint
- [ ] Metrics endpoint (Prometheus?)
- [ ] Configuration file support (YAML)
- [ ] Graceful shutdown
- [ ] Log rotation

## Phase 5: Client CLI (Low Priority)

- [ ] List buckets command
- [ ] Upload/download commands
- [ ] Sync command
- [ ] Configuration management

## Phase 6: Web UI (Lowest Priority)

- [ ] Basic file browser
- [ ] Upload interface
- [ ] User management UI
- [ ] Bucket policy editor

## Known Issues / Questions

1. Need to test msgpack decoding of MinIO Created timestamp
2. Should we store per-object metadata separately or rely on fs.json import?
3. Need to decide on object metadata caching strategy
4. Need to implement proper ETag calculation for multipart uploads (future)

## Testing Checklist

- [ ] Test with AWS CLI
- [ ] Test with boto3 (Python)
- [ ] Test with MinIO client (mc)
- [ ] Test migration from actual MinIO instance
- [ ] Load testing with large files
- [ ] Load testing with many small files
- [ ] Concurrent access testing

## Documentation

- [ ] API documentation
- [ ] Migration guide from MinIO
- [ ] Troubleshooting guide
- [ ] Performance tuning guide
