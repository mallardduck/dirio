# DirIO Development Roadmap

Current status: **Phase 3.2 COMPLETE** - All critical S3 features implemented!

## Recent Updates

**February 16, 2026 - Phase 3.2 Complete:**
- ✅ **Core S3 Features:** Multipart upload, pre-signed URLs, CopyObject, range requests, object tagging
- ✅ **Test Framework:** Structured JSON output with content integrity validation (MD5 hashes)
- ✅ **Client Compatibility:** AWS CLI (91%), boto3 (96%), MinIO mc (87%) - 23 canonical operations tested
- ✅ **Bug Fixes:** ListObjectsV2 pagination & delimiter, chunked encoding, MinIO mc DELETE operations
- 📁 **Known Issues:** See [bugs/](bugs/) for tracking (1 minor issue: MinIO mc PreSignedURL_Upload)

## Phase 1: MVP Core ✅

- [x] Project structure and HTTP server setup
- [x] Storage backend interface and metadata manager
- [x] API handlers (skeleton) and MinIO import logic
- [x] Integration tests for bucket, object, and ListObjectsV2 operations
- [x] Basic logging and error handling

## Phase 1.5: Configuration & Service Discovery ✅

### Configuration Framework
- [x] Cobra CLI structure, Viper config management
- [x] Support CLI flags, ENV vars, YAML config (`~/.dirio/config.yaml`, `/etc/dirio/config.yaml`)
- [x] Global config system (`internal/config/`) with validation

### mDNS Service Discovery
- [x] Service registration with auto IP detection (`internal/mdns/mdns.go`)
- [x] Multi-instance support: `{service}-{hostname}.local` format
- [x] Graceful shutdown with SIGINT/SIGTERM handling

### Domain-Aware URL Generation
- [x] CanonicalDomain configuration with Host header detection
- [x] URL generation helpers for internal vs canonical domains

### Testing
- [x] MinIO import, mDNS discovery, URL generation, config precedence tests

## Phase 2: Authentication, Security & MinIO Import ✅

### Authentication
- [x] AWS Signature V4 authentication with request ID and access logging
- [x] Authentication middleware, tested with AWS CLI

### Security Enhancement
- [x] go-billy filesystem abstraction layer (`internal/path/`)
- [x] R/O FS for MinIO metadata, R/W FS for DirIO metadata
- [x] Refactored all stdlib file operations to use go-billy

### Improved MinIO Imports
- [x] Parse MinIO Created timestamps and fs.json metadata
- [x] Import all bucket metadata (NotificationConfig, LifecycleConfig, etc.)
- [x] Tested with MinIO 2019 and 2022 formats
- [x] Custom metadata support (x-amz-meta-*, Cache-Control, etc.)
- **Decision:** Skip bitrot/checksums (not in MinIO FS mode, rely on filesystem) 

## Phase 2.5: Client Testing & Validation ✅

**Goal:** Test with real S3 clients, document compatibility, drive Phase 3 priorities.

### Test Framework
- [x] Test framework with structured JSON output and content integrity validation (MD5)
- [x] Generic S3 setup scripts for any endpoint (`scripts/s3-generic-setup.sh` & `.ps1`)

### Client Compatibility (23 canonical S3 operations)
- [x] **AWS CLI:** 21/23 passed (91%) - All core features working
- [x] **boto3:** 22/23 passed (96%) - Excellent compatibility
- [x] **MinIO mc:** 20/23 passed (87%) - Core operations working
- [x] S3 Compatibility Matrix created

**📊 Detailed Results:** See [CLIENTS.md](CLIENTS.md) for complete compatibility matrix

## Phase 2.75: Configuration Architecture ✅

**Goal:** Separate data config from app config for data portability.

### Data Directory Config (`internal/dataconfig`)
- [x] `DataConfig` structure for `.dirio/config.json` (region, credentials, compression, WORM, storage class)
- [x] Import MinIO config (2019 and 2022 formats)
- [x] Data config takes precedence, CLI provides initial values for new directories
- [x] Support both data config admin AND CLI admin credentials simultaneously

**Philosophy:** Data config travels with data and takes precedence; app config controls tool behavior locally.

## Known Issues / Questions

### Active Issues
1. MinIO mc PreSignedURL_Upload content mismatch - see [CLIENTS.md](CLIENTS.md) for details
2. Object metadata caching strategy → Phase 3.5
3. ETag calculation for multipart uploads → Phase 3.5

### Design Decisions (Deferred)
- Virtual-hosted-style buckets (DNS/mDNS wildcard) → Phase N+
- App-level audit logging for Admin/Web UI → Phase 7

**📋 Resolved Issues:** 11 bugs fixed in Phase 3.2 - see [bugs/fixed/](bugs/fixed/) directory

## Phase 3: Essential S3 Features

**📊 For detailed client compatibility status, see [CLIENTS.md](CLIENTS.md)**

**Prioritize based on Phase 2.5 findings:**

### Policy Engine Foundation ✅ COMPLETE (Feb 2026)

**Goal:** Comprehensive policy system for public bucket access and IAM groundwork.

**Status:** Fully implemented and integrated. Public bucket access working!

#### Core Components
- [x] **Policy evaluation engine** - Action/Resource/Principal/Effect matching with wildcards
- [x] **Action mapper** - S3 to IAM permission translation (HeadObject→GetObject, CopyObject→Get+Put)
- [x] **Thread-safe cache** - In-memory policy cache with RWMutex
- [x] **Persistence** - Bucket policies in `.dirio/buckets/{bucket}.json`
- [x] **Anonymous requests** - Unauthenticated requests supported for public buckets
- [x] **Authorization middleware** - All S3 routes evaluated against policies
- [x] **Admin bypass** - Root credentials skip policy checks

**Connection to Phase 5:** Policy engine will extend to IAM user/group policies.

### Phase 3.2 Features ✅ COMPLETE

**All Core S3 Features Implemented:**
- [x] DeleteObject & DeleteBucket (MinIO mc compatibility)
- [x] Pre-signed URLs (query-based SigV4 with expiration)
- [x] CopyObject (S3-to-S3 with metadata)
- [x] Range requests (206 Partial Content)
- [x] ListObjectsV2 pagination (NextContinuationToken, StartAfter)
- [x] Multipart upload (all 5 handlers)
- [x] Object tagging (with content preservation)
- [x] Custom metadata (case-insensitive, HTTP spec compliant)

### Remaining Work

**Phase 3.5+ (Deferred):**
- [ ] **ListBuckets/ListObjects result filtering** - Filter results per-item based on permissions
- [ ] **Policy condition evaluation** - IpAddress, StringEquals, DateLessThan, etc.
- [ ] **NotAction/NotResource/NotPrincipal** - Inverse matching support
- [ ] **Policy variables** - ${aws:username}, ${aws:userid}, etc.
- [ ] **POST Policy Uploads** - Browser-based form uploads (optional MinIO feature for `mc share upload`)

## Phase 3.5: Stability & Performance

### Performance Optimization
- [ ] Metadata caching strategy (based on profiling)
- [ ] Optimize ListObjects for large buckets
- [ ] Memory profiling and leak detection

### Stability & Testing
- [ ] Concurrent access testing
- [ ] Error handling audit across all API handlers
- [ ] Load testing with large files and many small files
- [ ] Test migration from actual MinIO instance
- [ ] Test behind reverse proxy (nginx) with canonical domain

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


## Phase 5: MinIO-Style IAM & User Management

**Goal:** Implement MinIO-compatible user, service account, group, and policy management for multi-user scenarios.

**Strategy:** MinIO-style IAM (see [docs/MINIO-IAM-SUPPORT.md](docs/MINIO-IAM-SUPPORT.md)) - focused on self-hosted operational needs, NOT AWS IAM compatibility.

**Compatibility Target:**
- ✅ MinIO Admin API (subset)
- ✅ `mc admin` commands (partial - core user/policy management)
- ✅ Custom DirIO CLI (full functionality)
- ❌ `aws iam` CLI (explicitly not supported)
- ❌ Terraform AWS provider (explicitly not supported)

### User Management
- [ ] AddUser - Create new user with credentials (MinIO Admin API)
- [ ] RemoveUser - Delete user
- [ ] ListUsers - List all users
- [ ] GetUserInfo - Get user details and policies
- [ ] EnableUser - Enable disabled user
- [ ] DisableUser - Disable user (soft delete)
- [ ] SetUserStatus - Change user enabled/disabled state

### Service Account Management
- [ ] AddServiceAccount - Create service account for a user (long-lived credentials)
- [ ] RemoveServiceAccount - Delete service account
- [ ] ListServiceAccounts - List service accounts for a user
- [ ] GetServiceAccountInfo - Get service account details
- [ ] UpdateServiceAccount - Update service account policy/description

### Group Management
- [ ] AddGroup - Create user group
- [ ] RemoveGroup - Delete group
- [ ] ListGroups - List all groups
- [ ] GetGroupInfo - Get group details and members
- [ ] GroupAdd - Add user(s) to group
- [ ] GroupRemove - Remove user(s) from group

### Policy Management
- [ ] AddPolicy - Create new policy (JSON document, S3-style actions/resources)
- [ ] RemovePolicy - Delete policy
- [ ] ListPolicies - List all policies
- [ ] GetPolicy - Retrieve policy document
- [ ] SetPolicy - Attach policy to user or group
- [ ] UnsetPolicy - Detach policy from user or group
- [ ] Policy evaluation engine - Enforce policies on S3 operations

### Access Key Management
- [ ] User access keys (access key ID + secret key pairs)
- [ ] Key rotation support
- [ ] Multiple keys per user support
- [ ] Key enable/disable (without deletion)

### Storage & Data Model
- [ ] Design IAM metadata storage structure (.dirio/iam/)
- [ ] User metadata schema (access keys, enabled status, policies, group memberships)
- [ ] Group metadata schema (policies, members)
- [ ] Service account metadata schema (parent user, policies, description)
- [ ] Policy metadata schema (JSON policy documents with S3 actions/resources)
- [ ] Secure credential storage (encrypted access keys)

### API Design
- [ ] **MinIO Admin API** - REST-based endpoints, configurable port strategy
  - **Default (same port):** `/minio/admin/v3/*` on port 9000 - full `mc` compatibility
    - POST `/minio/admin/v3/add-user`
    - GET `/minio/admin/v3/list-users`
    - POST `/minio/admin/v3/set-user-policy`
    - etc.
  - **Optional (separate port):** `/minio/admin/v3/*` on port 9001 - cleaner separation
  - Path-based routing middleware (check prefix before S3 routing)
- [ ] **Port binding:** If separate admin port, bind to same address as S3 port (both behind proxy typically)
- [ ] **mDNS registration:**
  - Same port: Single mDNS record for S3 API (admin accessible at same endpoint)
  - Separate port: Register TWO mDNS services:
    - `{mdns-name}-s3.{hostname}.local` → port 9000 (S3 API)
    - `{mdns-name}-admin.{hostname}.local` → port 9001 (Admin API)
- [ ] JSON request/response format (NOT XML Query API)
- [ ] Standard HTTP methods (POST/GET/DELETE)
- [ ] Configuration options:
  ```yaml
  admin_api:
    enabled: true
    port: 9000        # Same port (default), or separate (e.g., 9001)
    path_prefix: "/minio/admin/v3"  # MinIO compatible path
  ```

### Integration with Existing Auth
- [ ] Refactor auth package to support multiple users (currently single admin)
- [ ] Multi-user credential lookup and validation
- [ ] Policy-based bucket access control (read/write/list)
- [ ] Policy-based object access control
- [ ] Integrate with existing SigV4 authentication for S3 operations
- [ ] Admin API authentication (separate or same credentials?)

### Testing
- [ ] Unit tests for IAM operations
- [ ] Integration tests with `mc admin` commands (user, policy, group operations)
- [ ] Policy evaluation test suite (allow/deny scenarios)
- [ ] Multi-user S3 access scenarios (user A can't access user B's buckets)
- [ ] Service account delegation testing
- [ ] Test migration from MinIO IAM metadata

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
- [ ] IAM/Admin API design decision - See [MINIO-IAM-SUPPORT.md](docs/MINIO-IAM-SUPPORT.md)
- [ ] mDNS setup and troubleshooting
- [ ] Reverse proxy setup guide (nginx examples)
- [ ] S3 API compliance status
- [ ] Troubleshooting guide
- [ ] Performance tuning guide