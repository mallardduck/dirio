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
- [x] Create generic S3 setup scripts for any endpoint (`scripts/s3-generic-setup.sh` & `.ps1`)
  - Can point at any S3 API (DirIO, MinIO, AWS, etc.)
  - Uses mc (MinIO client) to create buckets, objects, metadata
  - Useful for creating consistent test state regardless of client

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

**📊 Detailed Results:** See [CLIENTS.md](CLIENTS.md) for complete compatibility matrix, test results, and known issues

## Phase 2.75: Configuration Architecture Refactoring ✅ (In Progress)

**Goal:** Separate data directory configuration from application configuration for proper data portability.

### Data Directory Config (`internal/dataconfig`)
- [x] Create `DataConfig` structure for `.dirio/config.json`
- [x] Support region, credentials, compression, WORM, storage class
- [x] Import MinIO config (both 2019 and 2022 formats)
- [x] Save DataConfig during MinIO import
- [x] Init logic: CLI flags provide initial values for new data directories
- [ ] Load logic: Data config takes precedence, warn when CLI differs (region only)
- [ ] Support both data config admin AND CLI admin credentials simultaneously
- [ ] Refactor Settings to remove data-level configs (keep only app-level)
- [x] Migration for existing installations
- [ ] Update documentation and examples

### Configuration Management TODOs
- [ ] **Add explicit config update command** - Allow updating data config values explicitly
  - `dirio config set region us-west-2`
  - `dirio config set compression.enabled true`
  - Currently: must manually edit `.dirio/config.json` or re-import
- [ ] **API rate limits** - Add to DataConfig for per-data-directory rate limiting
- [ ] **Storage path configurations** - Consider if paths should be configurable per data directory
- [ ] **Validation strategy** - Experiment with different approaches for invalid/missing configs (see inline TODO in `internal/dataconfig/dataconfig.go`)
  - Option A: Fail fast (current)
  - Option B: Merge with defaults
  - Option C: Warn and use defaults

### Configuration Philosophy
- **Data Config** (`.dirio/config.json`): Controls how data must be handled, travels with data, takes precedence
- **App Config** (CLI flags/ENV/YAML): Controls how tool runs, local preferences
- **Credentials Strategy**: Support both data config admin (official) and CLI admin (temporary/alternative) simultaneously
- **Region Updates**: CLI flags ignored if data config exists (log warning), require explicit update command

## Known Issues / Questions

1. ~~Need to test msgpack decoding of MinIO Created timestamp~~ ✅ Resolved in Phase 2
2. ~~Should we store per-object metadata separately or rely on fs.json import?~~ ✅ Resolved - using fs.json
3. Need to decide on object metadata caching strategy → Phase 3.5
4. Need to implement proper ETag calculation for multipart uploads → Phase 3 (Medium Priority)
5. Virtual-hosted-style buckets will require DNS wildcard or mDNS wildcard → Phase N+
6. Admin CLI and Web UI will need app-level audit logging beyond HTTP middleware → Phase 7
7. ~~Need to decide data vs app config architecture~~ ✅ Resolved - Phase 2.75 (split into dataconfig + app config)
8. **🚨 Bug #001: AWS SigV4 Chunked Encoding Corruption** - See [bugs/001-chunked-encoding-corruption.md](bugs/001-chunked-encoding-corruption.md) and [CLIENTS.md](CLIENTS.md) for detailed impact

## Phase 3: Essential S3 Features

**📊 For detailed client compatibility status, see [CLIENTS.md](CLIENTS.md)**

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

### Recommended Priority Order

Based on client testing results (see [CLIENTS.md](CLIENTS.md)):

1. **🚨 Fix AWS SigV4 Chunked Encoding Handling** (CRITICAL - Bug #001)
2. **Fix DeleteObject for MinIO mc** (Critical - 405 Method Not Allowed)
3. **Fix DeleteBucket for MinIO mc** (Critical - 405 Method Not Allowed)
4. **Object Tagging** - Query parameter routing needed
5. **Multipart Upload for boto3** - Proper multipart assembly
6. **CommonPrefixes in ListObjectsV2** - delimiter support for boto3
7. **ListObjectsV2 max-keys/pagination** - MaxKeys parameter ignored
8. **Range requests** - Returns full content instead of range
9. **CopyObject** - Creates 0-byte file instead of copying
10. **Pre-signed URL validation** - Returns 403 Forbidden
11. **Fix custom metadata key case** - boto3 returns wrong case, mc doesn't return it

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

## Phase 5: IAM & User Management

**Goal:** Implement AWS IAM-compatible user, role, group, and policy management for multi-user scenarios.

### User/Group/Role Management
- [ ] CreateUser - Create new IAM user with credentials
- [ ] DeleteUser - Remove IAM user
- [ ] ListUsers - List all IAM users
- [ ] GetUser - Get IAM user details
- [ ] UpdateUser - Modify IAM user properties
- [ ] CreateGroup - Create user group
- [ ] DeleteGroup - Remove user group
- [ ] ListGroups - List all groups
- [ ] AddUserToGroup - Add user to group
- [ ] RemoveUserFromGroup - Remove user from group
- [ ] CreateRole - Create IAM role (for future assume-role support)
- [ ] DeleteRole - Remove IAM role
- [ ] ListRoles - List all roles

### Policy Management
- [ ] CreatePolicy - Define new access policy (JSON-based)
- [ ] DeletePolicy - Remove policy
- [ ] ListPolicies - List all policies
- [ ] GetPolicy - Retrieve policy document
- [ ] AttachUserPolicy - Attach policy to user
- [ ] DetachUserPolicy - Detach policy from user
- [ ] AttachGroupPolicy - Attach policy to group
- [ ] DetachGroupPolicy - Detach policy from group
- [ ] AttachRolePolicy - Attach policy to role
- [ ] DetachRolePolicy - Detach policy from role
- [ ] PutUserPolicy - Inline policy for user
- [ ] PutGroupPolicy - Inline policy for group
- [ ] PutRolePolicy - Inline policy for role

### Access Key Management
- [ ] CreateAccessKey - Generate new access key pair for user
- [ ] DeleteAccessKey - Revoke access key
- [ ] ListAccessKeys - List user's access keys
- [ ] UpdateAccessKey - Enable/disable access key

### Authorization & Audit
- [ ] GetAccountAuthorizationDetails - Retrieve all IAM resources
- [ ] SimulatePrincipalPolicy - Test policy evaluation (debugging)
- [ ] Policy evaluation engine - Enforce policies on S3 operations

### Storage & Data Model
- [ ] Design IAM metadata storage structure (.dirio/iam/)
- [ ] User metadata schema (credentials, policies, group memberships)
- [ ] Group metadata schema (policies, members)
- [ ] Role metadata schema (policies, trust policies)
- [ ] Policy metadata schema (JSON policy documents)
- [ ] Access key storage and rotation

### API Design Decision
- [ ] **Option A:** Separate IAM API endpoint (e.g., port 9001, `/iam/*`) - Cleaner separation
- [ ] **Option B:** Query parameter based (e.g., `/?Action=CreateUser`) - AWS-compatible
- [ ] **Option C:** RESTful routes (e.g., `/api/iam/users`) - Modern, easier to implement
- **Decision needed:** Which API style to implement?

### Integration with Existing Auth
- [ ] Refactor auth package to support multiple users (currently single admin)
- [ ] Policy-based bucket access control
- [ ] Policy-based object access control
- [ ] Integrate with existing SigV4 authentication

### Testing
- [ ] Unit tests for IAM operations
- [ ] Integration tests with AWS CLI IAM commands
- [ ] Policy evaluation test suite
- [ ] Multi-user access scenarios

## Phase 6: Client CLI (Low Priority)

- [ ] List buckets command
- [ ] Upload/download commands
- [ ] Sync command
- [ ] Configuration management
- [ ] IAM management commands (create-user, attach-policy, etc.)

## Phase 7: Advanced Features & Audit Logging

### HTTP Audit Logging
- [ ] Design audit log middleware (streaming, queue-based)
- [ ] Implement log levels (0=off, 1=headers, 2=headers+req body, 3=headers+both bodies)
- [ ] Non-blocking audit log writer with queue
- [ ] Minimize memory allocation in middleware
- [ ] Audit log configuration (level, output destination)
- [ ] Audit log rotation support
- [ ] Document distinction: HTTP audit log vs full app audit log

## Phase 8: Web UI (Lowest Priority)

- [ ] Basic file browser
- [ ] Upload interface
- [ ] User management UI (IAM users, groups, roles)
- [ ] Bucket policy editor
- [ ] IAM policy editor and tester
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

---

## Documentation

- [ ] API documentation
- [ ] Migration guide from MinIO
- [ ] Configuration guide (CLI/ENV/YAML)
- [x] Client compatibility guide - See [CLIENTS.md](CLIENTS.md)
- [ ] mDNS setup and troubleshooting
- [ ] Reverse proxy setup guide (nginx examples)
- [ ] S3 API compliance status
- [ ] Troubleshooting guide
- [ ] Performance tuning guide