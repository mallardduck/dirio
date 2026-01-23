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
- [x] Test bucket operations (create, list, delete) - Integration tests in `tests/integration/bucket_test.go`
- [x] Test object operations (put, get, head, delete) - Integration tests in `tests/integration/object_test.go`
- [x] Test ListObjectsV2 with various parameters - Integration tests in `tests/integration/list_objects_test.go`
- [x] Add basic logging

## Phase 1.5: Configuration & Service Discovery

### Configuration Framework
- [x] Add spf13/cobra for CLI command structure
- [x] Add spf13/viper for configuration management
- [x] Define configuration structure (ServerConfig)
- [x] Support CLI flags, ENV vars, and YAML config file
- [x] Default config locations (`~/.dirio/config.yaml`, `/etc/dirio/config.yaml`)
- [x] Global config values system similar to [SCC-Operator internal/config](https://github.com/rancher/scc-operator/tree/main/internal/config) (minus ConfigMap support) - Implemented in `internal/config/`
- [x] Config validation and sensible defaults - Settings.Validate() in `internal/config/config.go`

### mDNS Service Discovery ✅
- Q: How do we know the IP to use for mDNS record?
  - A: Use the "outbound IP" method: create a UDP connection to 8.8.8.8:80 (doesn't send packets) and get the local address the OS would use. Fallback: enumerate network interfaces and pick first non-loopback IPv4. See `internal/mdns/ip.go`.
- Q: Assume we must support simple ":9000" port binding - how do we look up IP?
  - A: Same approach - `GetLocalIP()` in `internal/mdns/ip.go` auto-detects the appropriate IP address.
- [x] Add github.com/hashicorp/mdns dependency
- [x] Implement mDNS service registration - `internal/mdns/mdns.go`
- [x] Multi-instance support: mDNS name format `{service}.{hostname}.local` (e.g., `dirio-s3.macbook.local`)
  - Allows multiple DirIO instances to coexist on the same network
  - `--mdns-name` flag configures service name (default: `dirio-s3`)
  - `--mdns-hostname` flag overrides hostname component (default: system hostname)
  - Advertised as: `{mdns-name}.{mdns-hostname}.local`
- [x] Graceful mDNS shutdown on server stop - integrated with signal handling in `internal/server/server.go`
- [x] Graceful HTTP server shutdown with SIGINT/SIGTERM handling

### Domain-Aware URL Generation ✅
- [x] Add CanonicalDomain configuration option
- [x] Implement request domain detection (Host header)
- [x] Build URL generation helpers (internal vs canonical)
- [x] Update API responses to use appropriate domain
- [x] Mock/test domain-aware URL generation

### Testing
- [x] Test MinIO import with real data - Comprehensive tests in `internal/minio/import_test.go`
- [x] Test mDNS registration and discovery - Unit tests in `internal/mdns/mdns_test.go`
- [x] Test URL generation with different Host headers - Tests in `internal/urlbuilder/urlbuilder_test.go`
- [x] Test config loading from CLI/ENV/file with precedence - Tests in `internal/config/config_test.go`

## Phase 2: Authentication, Security & Audit Logging

### Authentication
- [ ] Add request ID generation
- [ ] Add access logging
- [ ] Add authentication middleware
- [ ] Implement AWS Signature V4 authentication
- [ ] Test with AWS CLI

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
- [ ] ListObjects pagination (max-keys parameter)

### Virtual-Hosted-Style Buckets (Future)
- [ ] Support `bucket.domain.com` style addressing
- [ ] Subdomain routing logic
- [ ] Update URL generation for virtual-hosted style
- [ ] DNS/mDNS considerations for wildcard subdomains
- [ ] Document virtual-hosted-style bucket support and configuration

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