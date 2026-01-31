# DirIO Development Roadmap

Current status: **Phase 2.5 - Client Testing & Bug Discovery**

**📁 Known Issues:** See [bugs/](bugs/) directory for detailed bug reports and tracking

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
- [x] Multi-instance support: mDNS name format `{service}-{hostname}.local` (e.g., `dirio-s3-macbook.local`)
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
- [x] Create `tests/clients/` directory with test scripts
- [x] Document baseline: what currently works with basic operations

### Client Compatibility Testing
- [x] Test with AWS CLI - basic CRUD operations
  - **Result:** 11/11 passed - 100% success rate (via testcontainers-go, Jan 31, 2026)
  - Core CRUD operations all work
  - High-level s3 commands (cp upload/download) work
  - HeadBucket returns x-amz-bucket-region header
  - DeleteObject and DeleteBucket both working
- [x] Test with boto3 (Python) - programmatic access patterns
  - **Result:** 13/21 passed - 62% success rate (via testcontainers-go, Jan 31, 2026)
  - Core CRUD operations all work
  - GetBucketLocation, ListObjectsV1, ListObjectsV2 (basic/prefix) working
  - Custom metadata set works, get returns wrong key case
  - Failed: delimiter (0 prefixes), max-keys (returns all 5), range requests (returns full 100 bytes), CopyObject (0-byte file), pre-signed URLs (403), multipart (405), object tagging (corrupts object content with XML)
- [x] Test with MinIO client (mc) - migration compatibility
  - **Result:** 12/14 passed - 85.7% success rate (via testcontainers-go, Jan 31, 2026)
  - **Major improvement:** Object operations now working! PutObject, HeadObject, GetObject all pass
  - ✅ Working: alias, ListBuckets, CreateBucket, HeadBucket, GetBucketLocation, PutObject (mc put/cp), HeadObject (mc stat), GetObject (mc cp/cat), ListObjectsV2
  - ❌ Failed: DeleteObject (405 Method Not Allowed), DeleteBucket (bucket not empty - expected if DeleteObject fails)
- [x] Create S3 Compatibility Matrix (document ✅ ❌ ⚠️ for each feature/client)

### Real-World Scenarios
- [x] Test mDNS discovery from other machines on LAN
  - After removing lots of wrapper code it works on external machines

**Output:** Prioritized list of missing features based on real client needs

## Phase 3: Essential S3 Features

**Prioritize based on Phase 2.5 findings:**

### High Priority (Core S3 compatibility)
- [x] Fix GetBucketLocation for MinIO mc (Critical - unblocks mc client) - ✅ Fixed routing + added x-amz-bucket-region to HeadBucket
- [ ] CommonPrefixes in ListObjectsV2 (delimiter support) - ⚠️ Partially implemented, works in integration tests but fails with boto3 client (returns 0 CommonPrefixes)
- [ ] ListObjects pagination with max-keys and continuation tokens
- [ ] Range requests for GetObject (resumable downloads, video streaming)
- [ ] Pre-signed URLs (temporary access sharing)
- [ ] CopyObject (x-amz-copy-source header) - NOT implemented, creates empty file

### Medium Priority
- [ ] Multipart upload support (large files >5GB)
- [ ] Fix custom metadata key case in responses
- [x] Object tagging - ✅ Already works!

### Lower Priority
- [ ] Bucket Policies (parse and validate)
- [ ] Bucket Policies (enforce public-read)
- [ ] Bucket Policies (complex policy statements)

### Real-World Scenarios
- [ ] Test migration from actual MinIO instance
- [ ] Test behind reverse proxy (nginx) with canonical domain

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

### Optional Minio Compatibility Layer
Using "Core + Sidecar" approach:

1. **The Core (Port 9000)**: Keep this 100% strictly S3 compatible. No custom headers, no weird endpoints. This ensures rclone, boto3, and cyberduck never get confused.
2. **The Management API (Port 9001)**: Put `datausageinfo`, `health`, and `user-management` here. This separates **Data Plane** (S3) from **Control Plane** (Admin).

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

**Updated: January 31, 2026 - Latest test run via testcontainers-go**

| Feature                   | AWS CLI | boto3 | MinIO mc | Notes                                                 | Priority |
|---------------------------|---------|-------|----------|-------------------------------------------------------|----------|
| CreateBucket              | ✅       | ✅     | ✅        | mc: via `mc mb`                                       | High     |
| DeleteBucket              | ✅       | ✅     | ❌        | mc: 405 Method Not Allowed (mc rb)                    | High     |
| ListBuckets               | ✅       | ✅     | ✅        | mc: works                                             | High     |
| HeadBucket                | ✅       | ✅     | ✅        | mc: via `stat --no-list`; returns x-amz-bucket-region | High     |
| GetBucketLocation         | ✅       | ✅     | ✅        | mc: via `stat`                                        | High     |
| PutObject                 | ✅       | ✅     | ✅        | mc: mc put/cp both work                               | High     |
| GetObject                 | ✅       | ✅     | ✅        | mc: mc cp/cat both work                               | High     |
| HeadObject                | ✅       | ✅     | ✅        | mc: mc stat works                                     | High     |
| DeleteObject              | ✅       | ✅     | ❌        | mc: 405 Method Not Allowed (mc rm)                    | High     |
| ListObjectsV2 (basic)     | ✅       | ✅     | ✅        | mc: mc ls works                                       | High     |
| ListObjectsV2 (prefix)    | ❓       | ✅     | ✅        | mc: mc ls prefix/ works                               | High     |
| ListObjectsV2 (delimiter) | ❓       | ❌     | ✅        | boto3: returns 0 CommonPrefixes; mc: shows folders    | High     |
| ListObjectsV2 (recursive) | ❓       | ❓     | ✅        | mc: mc ls -r works                                    | Medium   |
| ListObjectsV2 (max-keys)  | ❓       | ❌     | ❓        | boto3: MaxKeys parameter ignored, returns all 5       | Medium   |
| ListObjectsV1             | ❓       | ✅     | ❓        | boto3: works                                          | Medium   |
| Range Requests            | ❓       | ❌     | ❌        | Returns full content instead of range                 | High     |
| Custom Metadata (set)     | ❓       | ✅     | ✅        | mc: mc cp --attr works                                | Medium   |
| Custom Metadata (get)     | ❓       | ⚠️    | ❌        | boto3: wrong key case; mc: not returned in stat       | Medium   |
| Pre-signed URLs (down)    | ❓       | ❌     | ❌        | mc: mc share download fails                           | Medium   |
| Pre-signed URLs (up)      | ❓       | ❓     | ❌        | mc: mc share upload fails                             | Medium   |
| CopyObject                | ❓       | ❌     | ❌        | Creates 0-byte file; mc: mc cp s3-to-s3 fails         | Medium   |
| Multipart Upload          | ❓       | ❌     | ⚠️       | boto3: 405; mc: uploads but corrupts content (+14KB)  | High     |
| Object Tagging (set)      | ❓       | ❌     | ⚠️       | boto3+mc: operation succeeds but corrupts content      | High     |
| Object Tagging (get)      | ❓       | ❌     | ⚠️       | boto3+mc: returns tags but object content is XML       | High     |

Legend: ✅ Works | ❌ Fails | ⚠️ Partial | ❓Untested

### Key Findings from Phase 2.5 Testing

**Latest Test Run:** January 31, 2026 via `go test -v ./tests/clients/...`

**Test Framework:** testcontainers-go running Docker containers for each client. Tests refactored to use canonical scripts from `tests/clients/scripts/` via go:embed.

**Defensive Testing (Jan 31, 2026):** Comprehensive content verification added to all client tests to prevent false positives:
- ✅ **Object Tagging tests:** Verify content before and after tagging - EXPOSED BUG: tags replace object content
- ✅ **Multipart Upload tests:** Download and verify byte-for-byte content integrity - EXPOSED BUG: downloaded file 14KB larger than original
- ✅ **GetObject tests:** Verify exact content matches expected data - EXPOSED BUG: chunked encoding markers in content
- These defensive checks revealed that AWS SigV4 chunked transfer encoding headers are being written directly to object files, corrupting data

**Sanity Testing & Defensive Checks:** Added comprehensive validation to prevent false positives:
- ✅ **FailingServer test:** Returns 500 errors - all clients correctly fail
- ✅ **DumbSuccessServer test:** Returns 200 OK with empty responses - all clients correctly fail
- ✅ **Defensive boto3 checks:** Added content validation to detect query parameter routing issues:
  - GetBucketLocation: Verify response contains `LocationConstraint` field
  - ListObjectsV2: Verify actual object keys in results
  - Custom Metadata: Verify object content not corrupted by metadata operations
  - Object Tagging: Verify object content not overwritten by tagging XML
  - Multipart Upload: Verify assembled content matches expected parts
- These tests confirm passing tests are validating actual server functionality, not just status codes or accidental matches

**S3 API Implementation Status (24 unique features tested across 3 clients):**
- ✅ **Fully Working:** 13/24 (54%) - Works correctly across all tested clients
- ⚠️ **Partially Working:** 4/24 (17%) - ListObjectsV2 delimiter (boto3 fails, mc works), Custom metadata (set works, get has issues), Pre-signed URLs (partial failures), Multipart/Tagging (boto3 fails, mc works)
- ❌ **Not Working:** 7/24 (29%) - DeleteObject/DeleteBucket for mc, Range requests, CopyObject, ListObjectsV2 max-keys, Pre-signed URLs, Custom metadata get
- ❓ **Not Tested:** Many features only tested with subset of clients

**Client Test Results:**

**AWS CLI (11/11 tests passed - 100% | via testcontainers-go, Jan 31, 2026):**
- ✅ All core CRUD operations work perfectly
- ✅ High-level `s3` commands (cp upload/download) work
- ✅ HeadBucket returns `x-amz-bucket-region` header
- ✅ DeleteObject and DeleteBucket both working
- Tests cover: ListBuckets, CreateBucket, HeadBucket, PutObject, HeadObject, GetObject, ListObjectsV2, s3 cp upload, s3 cp download, DeleteObject, DeleteBucket

**boto3 (13/21 tests passed - 62% | via testcontainers-go, Jan 31, 2026):**
- ✅ Core CRUD operations all work (Create, Read, Update, Delete)
- ✅ GetBucketLocation works correctly
- ✅ ListObjectsV1 and ListObjectsV2 (basic/prefix) work
- ✅ Custom metadata set works
- ⚠️ Custom metadata get returns Title-Case keys ('Custom-Key') instead of lowercase ('custom-key')
- ❌ **Failed tests (8/21):**
  - ListObjectsV2 delimiter: Returns 0 CommonPrefixes instead of 2+
  - ListObjectsV2 max-keys: Ignores MaxKeys=2, returns all 5 objects
  - Range request: Returns full 100 bytes instead of first 10 bytes
  - CopyObject: Creates 0-byte empty file instead of copying content
  - Pre-signed URLs: Returns 403 Forbidden
  - Multipart: Returns 405 Method Not Allowed
  - Object Tagging: Corrupts object content with XML (query parameter `?tagging` ignored)

**MinIO mc (20/30 tests passed - 67% | via testcontainers-go, Jan 31, 2026):**
- ✅ **Expanded test coverage:** Now testing 30 features with comprehensive content verification
- ✅ **Bucket operations (6/6):** Configure alias, ListBuckets, CreateBucket (mc mb), HeadBucket (mc stat --no-list/stat), GetBucketLocation (mc stat)
- ✅ **Object CRUD (9/11):** PutObject (mc put/cp), HeadObject (mc stat), GetObject (mc cp/cat), ListObjectsV2 (mc ls/ls prefix/ls -r), ListObjectsV2 with delimiter
- ✅ **Tagging/Multipart operations (5/7):** Object tagging set/get work, multipart upload completes, size metadata correct
- ❌ **Failed tests (10/30):**
  - DeleteObject (mc rm): 405 Method Not Allowed
  - DeleteBucket (mc rb): 405 Method Not Allowed (bucket not empty due to DeleteObject failure)
  - Custom Metadata get: Not returned in mc stat
  - CopyObject (mc cp s3-to-s3): EOF error
  - Pre-signed URL download: Failed to download (curl error)
  - Pre-signed URL upload: Failed to upload
  - Range Requests (curl): Returns 0 bytes instead of 10
  - **Object Tagging - content corruption**: Tags stored as object content (XML replaces original)
  - **Multipart Upload - content corruption**: Downloaded 10,500,246 bytes instead of 10,485,760 bytes (chunked encoding artifacts)
  - **GetObject - chunked encoding leak**: AWS SigV4 chunk headers visible in object content

### Architecture Improvements (January 27, 2026):

**Auth Package Refactor:**
- ✅ **Merged sigv4 into auth package** - `internal/sigv4/` → `internal/auth/signature.go`
- ✅ **Unified authentication API** - Single `AuthenticateRequest(r)` method replaces 4-step orchestration
- ✅ **Auth middleware encapsulation** - Moved from `server.go` to `auth.AuthMiddleware()` method
- ✅ **Improved architecture**:
  - Single package owns all authentication concerns (signature verification, user lookup, validation)
  - Cleaner API: `user, err := auth.AuthenticateRequest(r)` instead of juggling sigv4 + auth packages
  - Better testability and reusability
  - User added to request context for downstream handlers
- ✅ **No regressions** - All test results identical before/after refactor

**MinIO mc Success & Remaining Issues:**
- ✅ **Resolved:** Object PUTs now work perfectly (mc put/cp both working)
- ✅ **Working:** Multipart uploads, object tagging, custom metadata set, ListObjectsV2 with delimiter/prefix/recursive
- ❌ **Critical blockers:** DeleteObject and DeleteBucket return 405 Method Not Allowed
  - Likely routing issue or missing HTTP method handler
  - Works perfectly in AWS CLI and boto3, mc-specific problem
- ❌ **Other failures:** Range requests, CopyObject, Pre-signed URLs, Custom metadata retrieval

### CRITICAL BUGS DISCOVERED (January 31, 2026):

**📁 See `bugs/` directory for detailed bug reports**

**🚨 #001: AWS SigV4 Chunked Encoding Bug** - [bugs/001-chunked-encoding-corruption.md](bugs/001-chunked-encoding-corruption.md)
- AWS Signature V4 chunked transfer encoding headers are being written directly to object files
- Evidence: Object content contains `15;chunk-signature=...` markers followed by data chunks
- Impact: Affects ALL write operations (PutObject, multipart uploads, object tagging)
- Multipart uploads: Downloaded file is 10,500,246 bytes instead of 10,485,760 bytes (extra 14,486 bytes of encoding artifacts)
- Object tagging: Tags XML replaces object content instead of being stored separately
- GetObject: Returns chunked encoding markers in content, corrupting data
- **This is the root cause of multiple "false positive" test results**
- **Priority: CRITICAL - Must fix before any write operations can be considered working**

### Recommended Priority for Phase 3 (based on findings):

1. **🚨 Fix AWS SigV4 Chunked Encoding Handling** (CRITICAL - Root cause corrupting ALL write operations. Chunked encoding headers leaking into object content)
2. **Fix DeleteObject for MinIO mc** (Critical - mc rm returns 405, blocking complete mc workflow. Works in AWS CLI/boto3, likely routing or method handling issue)
3. **Fix DeleteBucket for MinIO mc** (Critical - mc rb returns 405, similar to DeleteObject issue)
4. **Object Tagging** - Query parameter routing needed - After fixing chunked encoding bug, implement proper `?tagging` routing
5. **Multipart Upload for boto3** - After fixing chunked encoding bug, implement proper multipart assembly
6. **CommonPrefixes in ListObjectsV2** (delimiter support for boto3) - boto3 returns 0 CommonPrefixes, but mc shows folders correctly
7. **ListObjectsV2 max-keys/pagination** - MaxKeys parameter ignored, returns all 5 objects instead of 2
8. **Range requests** - Returns full content instead of requested range (blocks video streaming, resumable downloads, fails in boto3 and mc)
9. **CopyObject** - Creates 0-byte empty file instead of copying content (fails in boto3 and mc)
10. **Pre-signed URL validation** - Returns 403 Forbidden (fails in boto3 and mc)
11. **Fix custom metadata key case and retrieval** - boto3 returns 'Custom-Key' instead of 'custom-key', mc doesn't return it at all

**Confirmed Working (Jan 31, 2026):**
- ✅ GetBucketLocation (AWS CLI, boto3, MinIO mc) - FIXED Jan 24, 2026
- ✅ HeadBucket (AWS CLI, boto3, MinIO mc)
- ✅ DeleteObject/DeleteBucket work in AWS CLI and boto3
- ✅ ListObjectsV2 with delimiter works in MinIO mc (Jan 31, 2026)
- ✅ Object metadata operations (size, ETag, timestamps) work correctly

**Partially Working (operations succeed but content corrupted by chunked encoding bug):**
- ⚠️ PutObject - Uploads succeed but content includes chunked encoding artifacts
- ⚠️ GetObject - Downloads succeed but return corrupted content with encoding markers
- ⚠️ Multipart uploads - Upload completes, wrong size (10,500,246 vs 10,485,760 bytes)
- ⚠️ Object tagging - Operations succeed but tags replace object content

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