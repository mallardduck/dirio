# DirIO Development Roadmap

Current status: **Phase 2 Complete - Ready for Client Testing**

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

## Phase 2: Authentication, Security Enhance & Improved MinIO Imports

### Authentication ✅
- [x] Add request ID generation
- [x] Add access logging
- [x] Add authentication middleware
- [x] Implement AWS Signature V4 authentication
- [x] Test with AWS CLI

### Security Enhance
- [x] Import github.com/go-git/go-billy
- [x] Create an internal path package to make using go-billy easier
  - It should provide FS helpers that will be helpful for how we read/write to buckets
  - There should be a R/O FS exposed to access and read the Minio metadata.
  - There should be a R/W FS exposed for DirIO metadata
  - A helper to get FS for specific buckets
  - A helper to get "root data dir" for creating new bucket Dirs
- [x] Refactor and replace all stdlib os.Open (and similar FS) calls with new go-billy based pkg

### Improved MinIO Imports ✅
- [x] Parse MinIO's Created timestamp in import
- [x] Per-object metadata import and storage (fs.json)
  - [x] Parse fs.json files during import
  - [x] Store custom metadata (x-amz-meta-*, Cache-Control, Content-Disposition, etc.)
  - [x] Return custom metadata in GetObject/HeadObject responses
  - [x] Accept and store custom metadata in PutObject requests
- [x] Ensure all minio metadata files and data has been audited for parity in DirIO
  - [x] Import additional bucket metadata fields (NotificationConfig, LifecycleConfig, ObjectLockConfig, VersioningConfig, EncryptionConfig, TaggingConfig, QuotaConfig, ReplicationConfig, BucketTargetsConfig)
  - [x] Tested with both MinIO 2019 and 2022 to understand FS mode evolution
  - **Decision:** Skip bitrot/checksums - never implemented in MinIO FS mode, rely on underlying filesystem (ZFS/Btrfs/RAID)
  - All metadata now imported and stored in versioned compact JSON format, ready for future features 

## Phase 2.5: Client Testing & Validation

**Goal:** Test with real S3 clients, document what works/fails, use failures to drive Phase 3 priorities.

### Test Framework Setup
- [ ] Create `tests/clients/` directory with test scripts
- [ ] Document baseline: what currently works with basic operations

### Client Compatibility Testing
- [ ] Test with AWS CLI - basic CRUD operations
  - Expect: some operations work, others may fail on edge cases
- [ ] Test with boto3 (Python) - programmatic access patterns
  - Expect: may reveal missing headers, error format issues
- [ ] Test with MinIO client (mc) - migration compatibility
  - Expect: should mostly work given MinIO import support
- [ ] Create S3 Compatibility Matrix (document ✅ ❌ ⚠️ for each feature/client)

### Real-World Scenarios
- [ ] Test migration from actual MinIO instance
- [ ] Test mDNS discovery from other machines on LAN
- [ ] Test behind reverse proxy (nginx) with canonical domain

**Output:** Prioritized list of missing features based on real client needs

## Phase 3: Essential S3 Features

**Prioritize based on Phase 2.5 findings. Default priorities below:**

### High Priority (Core S3 compatibility)
- [ ] Range requests for GetObject (resumable downloads, video streaming)
- [ ] ListObjects pagination with max-keys and continuation tokens
- [ ] Pre-signed URLs (temporary access sharing)
- [ ] Copy object (CopyObject API)

### Medium Priority
- [ ] Multipart upload support (large files >5GB)
- [ ] Object tagging (metadata beyond x-amz-meta)

### Lower Priority
- [ ] Bucket Policies (parse and validate)
- [ ] Bucket Policies (enforce public-read)
- [ ] Bucket Policies (complex policy statements)

## Phase 3.5: Stability & Performance

### Performance Optimization
- [ ] Metadata caching strategy (based on profiling)
- [ ] Optimize ListObjects for large buckets
- [ ] Memory profiling and leak detection

### Stability
- [ ] Concurrent access testing
- [ ] Error handling audit across all API handlers
- [ ] Load testing with large files
- [ ] Load testing with many small files

## Phase 4: Production Readiness & Operations

### Monitoring & Health (Elevated - needed for production)
- [ ] Health check endpoint
- [ ] Metrics endpoint (Prometheus format)
- [ ] Readiness vs liveness probes

### Operational Tools
- [ ] Graceful shutdown improvements (if needed)
- [ ] Admin commands via CLI (minimal set, needs audit consideration)

### Deferred Operational Features
- [ ] Log rotation for application logs (OS/container can handle)
- [ ] HTTP Audit Logging (complex, lower value - see Phase 6)

## Phase 5: Client CLI (Low Priority)

- [ ] List buckets command
- [ ] Upload/download commands
- [ ] Sync command
- [ ] Configuration management

## Phase 6: Advanced Features & Audit Logging

### HTTP Audit Logging
- [ ] Design audit log middleware (streaming, queue-based)
- [ ] Implement log levels (0=off, 1=headers, 2=headers+req body, 3=headers+both bodies)
- [ ] Non-blocking audit log writer with queue
- [ ] Minimize memory allocation in middleware
- [ ] Audit log configuration (level, output destination)
- [ ] Audit log rotation support
- [ ] Document distinction: HTTP audit log vs full app audit log

## Phase 7: Web UI (Lowest Priority)

- [ ] Basic file browser
- [ ] Upload interface
- [ ] User management UI
- [ ] Bucket policy editor
- [ ] (Note: UI actions will need audit logging separate from HTTP middleware)

## Phase N+: Any future work

### Virtual-Hosted-Style Buckets (Future)
- [ ] Support `bucket.domain.com` style addressing
- [ ] Subdomain routing logic
- [ ] Update URL generation for virtual-hosted style
- [ ] DNS/mDNS considerations for wildcard subdomains
- [ ] Document virtual-hosted-style bucket support and configuration

## Known Issues / Questions

1. ~~Need to test msgpack decoding of MinIO Created timestamp~~ ✅ Resolved in Phase 2
2. ~~Should we store per-object metadata separately or rely on fs.json import?~~ ✅ Resolved - using fs.json
3. Need to decide on object metadata caching strategy → Phase 3.5
4. Need to implement proper ETag calculation for multipart uploads → Phase 3 (Medium Priority)
5. Virtual-hosted-style buckets will require DNS wildcard or mDNS wildcard → Phase N+
6. Admin CLI and Web UI will need app-level audit logging beyond HTTP middleware → Phase 6/7

## S3 Client Compatibility Matrix

**To be filled in during Phase 2.5 testing**

| Feature | AWS CLI | boto3 | MinIO mc | Notes | Priority |
|---------|---------|-------|----------|-------|----------|
| CreateBucket | ? | ? | ? | | High |
| DeleteBucket | ? | ? | ? | | High |
| ListBuckets | ? | ? | ? | | High |
| PutObject | ? | ? | ? | | High |
| GetObject | ? | ? | ? | | High |
| HeadObject | ? | ? | ? | | High |
| DeleteObject | ? | ? | ? | | High |
| ListObjectsV2 (basic) | ? | ? | ? | | High |
| ListObjectsV2 (pagination) | ? | ? | ? | | High |
| Range Requests | ? | ? | ? | | High |
| Custom Metadata | ? | ? | ? | x-amz-meta-* headers | Medium |
| Pre-signed URLs | ? | ? | ? | | Medium |
| CopyObject | ? | ? | ? | | Medium |
| Multipart Upload | ? | ? | ? | | Medium |
| Object Tagging | ? | ? | ? | | Low |

Legend: ✅ Works | ❌ Fails | ⚠️ Partial | ? Untested

## Documentation

- [ ] API documentation
- [ ] Migration guide from MinIO
- [ ] Configuration guide (CLI/ENV/YAML)
- [ ] Client compatibility guide (AWS CLI, boto3, mc) - populate from Phase 2.5
- [ ] mDNS setup and troubleshooting
- [ ] Reverse proxy setup guide (nginx examples)
- [ ] S3 API compliance status
- [ ] Troubleshooting guide
- [ ] Performance tuning guide