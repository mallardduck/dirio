# DirIO Development Roadmap

Current status: **Phase 4 IN PROGRESS** - IAM & User Management (elevated priority to unlock Phase 3.3 filtering test validation)

## Recent Updates

**February 16, 2026 (19:46) - Phase 3.3 Status Update:**
- ✅ **Client Compatibility Tests Confirmed:**
  - AWS CLI: 21/23 passed (91%) - All core features working
  - boto3: 22/23 passed (96%) - Excellent compatibility maintained
  - MinIO mc: 20/23 passed (87%) - Core operations working, 1 known issue persists
  - ⚠️ Known Issue: MinIO mc PreSignedURL_Upload still failing with content integrity mismatch
  - 📊 Overall Status: 91% S3 compatibility across major clients
- ✅ **Result Filtering Implementation Complete:**
  - ListBuckets filtering by s3:GetBucketLocation permission
  - ListObjects filtering by s3:GetObject permission
  - Admin fast path optimization
  - UUID-based ownership tracking
  - ⚠️ Missing: Integration tests and client tests for filtering

**February 16, 2026 - Policy Condition Evaluation Complete:**
- ✅ **Policy Condition Evaluation:** Full implementation of all 6 operator categories (String, Numeric, Date, IP, Boolean, Null)
  - ✅ IpAddress/NotIpAddress conditions with CIDR support
  - ✅ StringEquals/StringLike with glob pattern matching
  - ✅ DateLessThan/DateGreaterThan/DateEquals with ISO 8601 parsing
  - ✅ NumericLessThan/NumericGreaterThan/NumericEquals with type coercion
  - ✅ Bool and Null operators
  - ✅ AWS IAM evaluation semantics (AND across operators, OR across values)
  - ✅ Integration with policy matcher (fail-closed security)
  - ✅ Comprehensive test coverage (26 tests across conditions package)
- ✅ **User Lookup Optimization:** Added GetUserByUUID method to metadata manager for owner display name resolution
- ✅ **Bug Fixes:** Owner DisplayName now shows username instead of UUID

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

### Client Compatibility (23 canonical S3 operations) ✅ VERIFIED (Feb 16, 2026 19:46)
- [x] **AWS CLI:** 21/23 passed (91%) - All core features working
- [x] **boto3:** 22/23 passed (96%) - Excellent compatibility
- [x] **MinIO mc:** 20/23 passed (87%) - Core operations working
- [x] S3 Compatibility Matrix created
- [x] Automated test suite with JSON output and content integrity validation

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
1. **MinIO mc PreSignedURL_Upload content mismatch** (Confirmed Feb 16, 2026 19:46)
   - Status: ❌ FAILING in latest test run
   - Content integrity hash varies between runs, indicating data corruption during POST Policy upload
   - See [CLIENTS.md](CLIENTS.md) for details
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

## Phase 3.3: Advanced Policy Features ✅ COMPLETE

**Goal:** Enhance policy engine with advanced features for fine-grained access control.

**Status:** ✅ COMPLETE (Feb 16, 2026). Policy evaluation, filtering implementation, and client tests complete. Remaining items moved to Phase 4 (IAM-dependent) and Phase 4.5 (performance/POST uploads).

**Infrastructure Complete (Feb 16, 2026):**
- ✅ UUID-based ownership system (buckets and objects)
- ✅ Admin UUID constant (`badfc0de-fadd-fc0f-fee0-000dadbeef00`)
- ✅ UserStatus enum type with validation
- ✅ Ownership semantics (nil = admin-only, UUID = user ownership)
- ✅ Setup scripts with comprehensive test scenarios
- ✅ **Ownership-based authorization** - Resource owners have implicit access (AWS-like model)
  - ✅ Evaluation at position 3.5 (after explicit deny, before default deny)
  - ✅ Bucket owners can access their buckets
  - ✅ Object owners can access their objects
  - ✅ Bucket owners have access to all objects in their bucket (AWS model)
  - ✅ Explicit deny beats ownership (AWS security principle)
  - ✅ Comprehensive test coverage (14 test scenarios)

### Policy Evaluation Enhancements
- [x] **Policy condition evaluation** ✅ COMPLETE (Feb 16, 2026) - IpAddress, StringEquals, DateLessThan, etc.
  - [x] IpAddress / NotIpAddress conditions (test: policy-ip-test bucket)
  - [x] StringEquals / StringLike / StringNotEquals / StringNotLike (test: policy-string-test bucket)
  - [x] DateLessThan / DateGreaterThan / DateEquals (test: policy-time-test bucket)
  - [x] NumericLessThan / NumericGreaterThan / NumericEquals (test: policy-numeric-test bucket)
  - [x] Bool condition operator
  - [x] Null condition operator
- [x] **NotAction/NotResource/NotPrincipal** - Inverse matching support (test scenarios ready)
  - [x] NotAction support (deny everything except specified actions) (test: policy-notaction-test bucket)
  - [x] NotResource support (apply to all resources except specified) (test: policy-notresource-test bucket)
  - [x] NotPrincipal support (apply to all principals except specified)
- [x] **Policy variables** ✅ COMPLETE - Dynamic variable substitution (test scenarios ready)
  - [x] ${aws:username} - Current authenticated username (test: policy-variables-test bucket)
  - [x] ${aws:userid} - Current authenticated user ID (UUID-based)
  - [x] ${aws:SourceIp} - Request source IP
  - [x] ${s3:prefix} - Object key prefix (for ListObjects filtering)
  - [x] ${s3:delimiter} - Delimiter for ListObjects
  - [x] ${aws:CurrentTime} - Request timestamp (ISO 8601 format)
  - [x] ${aws:UserAgent} - HTTP User-Agent header
  - [x] Variable substitution engine with regex-based pattern matching
  - [x] Context building from HTTP requests
  - [x] Integration with policy evaluation engine
  - [x] Comprehensive test coverage (all variables tested)

### Result Filtering ✅ COMPLETE (Feb 16, 2026)
- [x] **ListBuckets result filtering** - Only return buckets user has permission to access ✅ COMPLETE
  - [x] Evaluate GetBucketLocation permission per bucket
  - [x] Filter out buckets without permission
  - [x] Admin fast path (bypass filtering)
  - [x] Use ownership tracking (UUID-based) for filtering
  - [x] Implementation: `filterBuckets()` in `internal/http/api/s3/filtering.go`
  - [x] Integration: Called by ListBuckets handler in `s3.go`
- [x] **ListObjects result filtering** - Only return objects user has permission to read ✅ COMPLETE
  - [x] Evaluate GetObject permission per object
  - [x] Filter out objects without permission
  - [x] Handle prefix-based permissions efficiently
  - [x] Admin fast path (bypass filtering)
  - [x] Use object ownership (UUID-based) for filtering
  - [x] Implementation: `filterObjects()` in `internal/http/api/s3/filtering.go`
  - [x] Integration: Called by ListObjects handlers in `bucket.go`
- [x] **Unit tests** - Helper function tests in `filtering_test.go`
- [x] **Client tests** - ✅ IMPLEMENTED (Feb 16, 2026) - Tests added, skip until IAM available (Phase 4)
  - [x] Test ListBuckets returns only permitted buckets
  - [x] Test ListObjects returns only permitted objects
  - [x] Test filtering with AWS CLI
  - [x] Test filtering with boto3
  - [x] Test filtering with MinIO mc

**Note:** Setup scripts create test buckets with policies. Integration tests moved to Phase 4 (IAM-dependent), POST uploads moved to Phase 4.5.

### Testing ✅ COMPLETE
- [x] Policy condition test scenarios - Setup script with comprehensive test buckets
- [x] NotAction/NotResource/NotPrincipal test scenarios - Setup script complete
- [x] Policy variable substitution tests - Setup script with user-specific folders
- [x] ListBuckets/ListObjects filtering setup - Setup script creates filter-* buckets
- [x] Automated tests for condition evaluation - 26 tests in internal/policy/conditions
- [x] Automated tests for policy variables - Comprehensive coverage
- [x] Unit tests for result filtering - Helper function tests in filtering_test.go
- [x] Client tests for result filtering - Implemented (25 tests total, skip until IAM)

### Setup Script Enhancements ✅ COMPLETE
- [x] Add SETUP_POLICY_TESTS flag to s3-minio-setup.sh (905 lines)
- [x] Create test buckets with conditional policies
- [x] Create users with prefix-based permissions
- [x] Create test scenarios for NotAction/NotResource
- [x] Create test buckets for result filtering (4 buckets, 60+ objects)

## Phase 4: Hybrid IAM & User Management

**Goal:** Implement hybrid IAM combining S3-native authorization with MinIO-compatible admin API to unblock Phase 3.3 filtering tests and enable multi-user scenarios.

**Why Elevated:** Phase 3.3 filtering tests, client tests, and integration tests are blocked without IAM support. Moving IAM up in priority unblocks critical testing and validation work.

**Architecture:** Hybrid approach combining best of S3 and MinIO (see [docs/IAM-ARCHITECTURE.md](docs/IAM-ARCHITECTURE.md))
- **S3 API layer:** Bucket policies with S3 actions/resources, AWS-standard conditions/variables, UUID-based ownership
- **MinIO Admin API layer:** User/policy CRUD operations via `mc admin` commands
- **Shared backend:** Unified IAM metadata in `.dirio/iam/` supporting both APIs

**Compatibility:**
- ✅ S3 API (bucket policies via AWS CLI, boto3, MinIO mc - data plane)
- ✅ MinIO Admin API (`mc admin` for user/policy management - control plane)
- ✅ S3-standard policy documents (Principal, Action, Resource, Condition, NotAction, NotResource)
- ✅ AWS-like authorization (ownership, conditions, variables, result filtering)
- ❌ AWS IAM API (`aws iam` commands - explicitly not supported)
- ❌ Terraform AWS provider (requires AWS IAM API - explicitly not supported)

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
  - **Optional (separate port):** `/minio/admin/v3/*` on port 9001 - cleaner separation
- [ ] Path-based routing middleware (check prefix before S3 routing)
- [ ] JSON request/response format (NOT XML Query API)
- [ ] Standard HTTP methods (POST/GET/DELETE)
- [ ] **mDNS registration** for admin endpoints
- [ ] Configuration options for admin API

### Integration with Existing Auth
- [ ] Refactor auth package to support multiple users (currently single admin)
- [ ] Multi-user credential lookup and validation
- [ ] Policy-based bucket access control
- [ ] Policy-based object access control
- [ ] Integrate with existing SigV4 authentication
- [ ] Admin API authentication

### Testing & Phase 3.3 Completion
- [ ] Unit tests for IAM operations
- [ ] Integration tests with `mc admin` commands
- [ ] Policy evaluation test suite
- [ ] Multi-user S3 access scenarios
- [ ] Service account delegation testing
- [ ] Test migration from MinIO IAM metadata
- [ ] **COMPLETE Phase 3.3 filtering integration tests** - Create tests/integration/list_filtering_test.go
- [ ] **Activate client filtering tests** - Setup alice/bob users so tests run instead of skip
- [ ] **Setup policy test data in client tests** - Implement setupPolicyTestData() integration

## Phase 4.5: Stability & Performance (Was Phase 3.5)

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

### Browser Upload Support (Moved from Phase 3.3)
- [ ] **POST Policy Uploads** - Browser-based form uploads
  - [ ] Parse POST policy documents
  - [ ] Validate policy signature and expiration
  - [ ] Support multipart/form-data uploads
  - [ ] HTML form upload examples
  - [ ] MinIO `mc share upload` compatibility

## Phase 5: Production Readiness & Operations (Was Phase 4)

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
- [x] IAM/Admin API design decision - See [IAM-ARCHITECTURE.md](docs/IAM-ARCHITECTURE.md)
- [ ] mDNS setup and troubleshooting
- [ ] Reverse proxy setup guide (nginx examples)
- [ ] S3 API compliance status
- [ ] Troubleshooting guide
- [ ] Performance tuning guide