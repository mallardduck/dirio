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

## Phase 1.5: Configuration & Service Discovery

### Configuration Framework
- [ ] Add spf13/cobra for CLI command structure
- [ ] Add spf13/viper for configuration management
- [ ] Define configuration structure (ServerConfig)
- [ ] Support CLI flags, ENV vars, and YAML config file
- [ ] Default config locations (`~/.dirio/config.yaml`, `/etc/dirio/config.yaml`)
- [ ] Config validation and sensible defaults

### mDNS Service Discovery
- [ ] Add github.com/hashicorp/mdns dependency
- [ ] Implement mDNS service registration
- [ ] Default mDNS name: `dirio-s3.local` (configurable)
- [ ] Graceful mDNS shutdown on server stop

### Domain-Aware URL Generation
- [ ] Add CanonicalDomain configuration option
- [ ] Implement request domain detection (Host header)
- [ ] Build URL generation helpers (internal vs canonical)
- [ ] Update API responses to use appropriate domain
- [ ] Mock/test domain-aware URL generation
- [ ] Document virtual-hosted-style bucket support for future (Phase 3)

### Testing
- [ ] Test MinIO import with real data
- [ ] Test mDNS registration and discovery
- [ ] Test URL generation with different Host headers
- [ ] Test config loading from CLI/ENV/file with precedence

## Phase 2: Authentication, Security & Audit Logging

### Authentication
- [ ] Implement AWS Signature V4 authentication
- [ ] Add authentication middleware
- [ ] Test with AWS CLI
- [ ] Add request ID generation
- [ ] Add access logging

### HTTP Audit Logging
- [ ] Design audit log middleware (streaming, queue-based)
- [ ] Implement log levels (0=off, 1=headers, 2=headers+req body, 3=headers+both bodies)
- [ ] Non-blocking audit log writer with queue
- [ ] Minimize memory allocation in middleware
- [ ] Audit log configuration (level, output destination)
- [ ] Audit log rotation support
- [ ] Document distinction: HTTP audit log vs full app audit log

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

### Virtual-Hosted-Style Buckets (Future)
- [ ] Support `bucket.domain.com` style addressing
- [ ] Subdomain routing logic
- [ ] Update URL generation for virtual-hosted style
- [ ] DNS/mDNS considerations for wildcard subdomains

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
- [ ] Graceful shutdown improvements
- [ ] Log rotation for application logs
- [ ] Admin commands via CLI (will need separate audit consideration)

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
- [ ] (Note: UI actions will need audit logging separate from HTTP middleware)

## Known Issues / Questions

1. Need to test msgpack decoding of MinIO Created timestamp
2. Should we store per-object metadata separately or rely on fs.json import?
3. Need to decide on object metadata caching strategy
4. Need to implement proper ETag calculation for multipart uploads (future)
5. Virtual-hosted-style buckets will require DNS wildcard or mDNS wildcard (investigate feasibility)
6. Admin CLI and Web UI will need app-level audit logging beyond HTTP middleware

## Testing Checklist

- [ ] Test with AWS CLI
- [ ] Test with boto3 (Python)
- [ ] Test with MinIO client (mc)
- [ ] Test migration from actual MinIO instance
- [ ] Test mDNS discovery from other machines on LAN
- [ ] Test behind reverse proxy (nginx) with canonical domain
- [ ] Load testing with large files
- [ ] Load testing with many small files
- [ ] Concurrent access testing

## Documentation

- [ ] API documentation
- [ ] Migration guide from MinIO
- [ ] Configuration guide (CLI/ENV/YAML)
- [ ] mDNS setup and troubleshooting
- [ ] Reverse proxy setup guide (nginx examples)
- [ ] Audit logging guide
- [ ] Troubleshooting guide
- [ ] Performance tuning guide