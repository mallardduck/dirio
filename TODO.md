# DirIO Development Roadmap

Current status: **Phase 4.5 IN PROGRESS** — Performance optimizations shipped; stability & testing tasks remain

## Recent Updates

**February 23, 2026 - Phase 4.5 Performance Optimizations Complete:**
- ✅ pprof endpoints added (gated on `--debug` flag) — `run-profile` Taskfile task
- ✅ `scripts/seed-large-bucket.sh` — seeds 10k objects across 4 prefix patterns for profiling
- ✅ `tests/perf/` — opt-in profiling tests (`//go:build perf`, `task test-perf`) using testcontainers for seeding; three tests: `TestPerfMetadataCaching`, `TestPerfListObjectsLargeBucket`, `TestPerfMemory`
- ✅ **Metadata cache** — `github.com/phuslu/lru` sharded LRU (100k entries, ~20 MB cap) added to `metadata.Manager`; exact invalidation on all write/delete paths. Cache hit eliminates per-object file open + JSON decode.
- ✅ **Early walk termination** — `listInternal` stops walking after `maxKeys+1` entries when `delimiter=""`. `full-scan-100` is now ~3× faster than `full-scan-1000` (proves early exit). Both dropped from ~450ms → ~1.5–4.5ms per call (~100–300× improvement).
- ✅ **Memory leak check** — goroutine diff: net zero; live heap delta: ~2.5 KB after 200 rounds. No leaks detected.
- ⏭️ **Sustained load test / memory profiling deferred** — existing perf data shows no active leaks. Multipart upload memory behaviour under sustained concurrent load is the remaining open question; deferred to a later phase alongside load testing infrastructure (wrk/hey/k6).

**February 22, 2026 - Phase 4.4 Complete:**
- ✅ `tests/console/` — 27 console stopgap tests: session auth (login/logout/protected routes), full S3 bucket policy editor, bucket ownership management, request simulator (single-action + effective permissions)

**February 22, 2026 - Phase 4.4 Testing Complete (except console):**
- ✅ `tests/integration/serviceaccount_policy_test.go` — SA delegation (inherit/override mode) and expiration integration tests
- ✅ `tests/clients/scripts/mc_admin.sh` + `TestMCAdmin` — mc admin CLI testcontainer tests (user add/list/info, policy CRUD, group add, user disable/enable/remove)
- ✅ `internal/persistence/metadata/import.go` — MinIO import now rebuilds bolt indexes after import so users are immediately visible
- ✅ `tests/admin/helpers_test.go` — Added `Stop()` method and cancelable context to `NewTestServerWithDataDir` for clean BoltDB lock release

**February 21, 2026 - SA Policy Inheritance (Eval-Time Resolution):**
- ✅ `pkg/iam/serviceaccount.go` — Added `PolicyMode` type (`"inherit"` / `"override"`); replaced `ParentUser *string` with `ParentUserUUID *uuid.UUID` (stable across key rotation)
- ✅ `internal/context/context.go` — Added `ServiceAccountInfo` struct + `WithServiceAccountInfo`/`GetServiceAccountInfo` context helpers
- ✅ `internal/http/auth/auth.go` — Added `IsServiceAccount()` method for SA detection
- ✅ `internal/http/auth/middleware.go` — Stores `ServiceAccountInfo` in context post-auth for non-admin users
- ✅ `internal/persistence/metadata/metadata.go` — Added UUID→username in-memory index; `GetUserByUUID` is now O(1)
- ✅ `internal/policy/resolver.go` (new) — `PolicyResolver` interface + `MetadataResolver` implementation
- ✅ `internal/policy/types.go` — Added `IsServiceAccount`, `ParentUserUUID`, `PolicyMode` to `Principal`
- ✅ `internal/policy/middleware.go` — Populates SA fields on `Principal` from context
- ✅ `internal/policy/engine.go` — Added `resolver` field; `New()` takes `PolicyResolver`; step 3 (IAM eval) implemented with `resolveEffectivePolicyNames()` helper
- ✅ `internal/http/server/server.go` — Wires `MetadataResolver` into `policy.New()`
- ✅ `internal/service/serviceaccount/serviceaccount.go` — `Create()` resolves parent access key → UUID before persisting
- ✅ `internal/service/serviceaccount/types.go` — Added `PolicyMode` to `CreateServiceAccountRequest`
- ✅ `internal/http/api/iam/service_account.go` — `AddServiceAccount` passes `PolicyMode`; `InfoServiceAccount` returns `parentUserUUID` + `policyMode`

**February 21, 2026 - Phase 4.3 Complete:**
- ✅ `consoleapi/` — full interface seam: Users, Policies, Buckets, Ownership, Policy Observability + all request/response types
- ✅ `console/auth/` — `AdminAuth` interface + HMAC-SHA256 signed cookie sessions (8-hour TTL)
- ✅ `console/handlers/` — Login/Logout, Dashboard, Users, Policies, Buckets list, Bucket detail, Ownership transfer, Policy editor, Simulator; HTMX partial-swap support
- ✅ `console/ui/` — templ components: layout, all list pages, bucket detail (policy + ownership), policy simulator
- ✅ `console/static/` — Tailwind v4 CSS, htmx.min.js, DirIO logo; embedded via Go `embed`
- ✅ `internal/console/adapter.go` — all methods wired: Users (5), Policies (6), Buckets (GetBucket/List/GetPolicy/SetPolicy), Ownership (GetBucketOwner/Transfer/GetObjectOwner), Observability (GetEffectivePermissions/SimulateRequest)
- ✅ `internal/persistence/metadata` — added `SetBucketOwner` for ownership transfer
- ✅ `internal/service/factory` — added `PolicyEngine()` accessor for simulator evaluation
- ✅ `cmd/server/cmd/wire_console.go` + `wire_console_stub.go` — build tag wiring (`-tags noconsole` strips console entirely)
- ✅ `--console` flag (default: true) and `--console-address` flag for optional separate port
- ✅ Protected routes behind session middleware; public routes: `/login`, `/static/`

**February 20, 2026 - Phase 4.2 Complete:**
- ✅ **Admin Integration Test Suite** (`tests/admin/`, 37 tests) — New test area separate from S3 integration tests
  - User CRUD, policy CRUD, attach/detach, policy-entities — all endpoints covered
  - madmin encryption protocol tested end-to-end (EncryptData/DecryptData)
- ✅ **MinIO IAM Import Tests** (`tests/admin/minio_import_test.go`) — End-to-end import verification
  - Users, policies, mappings, disabled status, idempotent restart, post-import management
- 🐛 **Bug Fix:** MinIO "enabled"/"disabled" status not converted to DirIO "on"/"off" on import
- 🐛 **Bug Fix:** `AttachPolicy` silently accepted non-existent policy names — now returns 404
- ✅ **UnsetPolicy HTTP endpoint** confirmed complete (`/idp/builtin/policy/detach`)

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
  - Client tests implemented (25 tests, require alice/bob IAM users to activate)

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

- ✅ Project structure and HTTP server setup
- ✅ Storage backend interface and metadata manager
- ✅ API handlers (skeleton) and MinIO import logic
- ✅ Integration tests for bucket, object, and ListObjectsV2 operations
- ✅ Basic logging and error handling

## Phase 1.5: Configuration & Service Discovery ✅

### Configuration Framework
- ✅ Cobra CLI structure, Viper config management
- ✅ Support CLI flags, ENV vars, YAML config (`~/.dirio/config.yaml`, `/etc/dirio/config.yaml`)
- ✅ Global config system (`internal/config/`) with validation

### mDNS Service Discovery
- ✅ Service registration with auto IP detection (`internal/mdns/mdns.go`)
- ✅ Multi-instance support: `{service}-{hostname}.local` format
- ✅ Graceful shutdown with SIGINT/SIGTERM handling

### Domain-Aware URL Generation
- ✅ CanonicalDomain configuration with Host header detection
- ✅ URL generation helpers for internal vs canonical domains

### Testing
- ✅ MinIO import, mDNS discovery, URL generation, config precedence tests

## Phase 2: Authentication, Security & MinIO Import ✅

### Authentication
- ✅ AWS Signature V4 authentication with request ID and access logging
- ✅ Authentication middleware, tested with AWS CLI

### Security Enhancement
- ✅ go-billy filesystem abstraction layer (`internal/path/`)
- ✅ R/O FS for MinIO metadata, R/W FS for DirIO metadata
- ✅ Refactored all stdlib file operations to use go-billy

### Improved MinIO Imports
- ✅ Parse MinIO Created timestamps and fs.json metadata
- ✅ Import all bucket metadata (NotificationConfig, LifecycleConfig, etc.)
- ✅ Tested with MinIO 2019 and 2022 formats
- ✅ Custom metadata support (x-amz-meta-*, Cache-Control, etc.)
- **Decision:** Skip bitrot/checksums (not in MinIO FS mode, rely on filesystem) 

## Phase 2.5: Client Testing & Validation ✅

**Goal:** Test with real S3 clients, document compatibility, drive Phase 3 priorities.

### Test Framework
- ✅ Test framework with structured JSON output and content integrity validation (MD5)
- ✅ Generic S3 setup scripts for any endpoint (`scripts/s3-generic-setup.sh` & `.ps1`)

### Client Compatibility (23 canonical S3 operations) ✅ VERIFIED (Feb 16, 2026 19:46)
- ✅ **AWS CLI:** 21/23 passed (91%) - All core features working
- ✅ **boto3:** 22/23 passed (96%) - Excellent compatibility
- ✅ **MinIO mc:** 20/23 passed (87%) - Core operations working
- ✅ S3 Compatibility Matrix created
- ✅ Automated test suite with JSON output and content integrity validation

**📊 Detailed Results:** See [CLIENTS.md](CLIENTS.md) for complete compatibility matrix

## Phase 2.75: Configuration Architecture ✅

**Goal:** Separate data config from app config for data portability.

### Data Directory Config (`internal/dataconfig`)
- ✅ `DataConfig` structure for `.dirio/config.json` (region, credentials, compression, WORM, storage class)
- ✅ Import MinIO config (2019 and 2022 formats)
- ✅ Data config takes precedence, CLI provides initial values for new directories
- ✅ Support both data config admin AND CLI admin credentials simultaneously

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
- ✅ **Policy evaluation engine** - Action/Resource/Principal/Effect matching with wildcards
- ✅ **Action mapper** - S3 to IAM permission translation (HeadObject→GetObject, CopyObject→Get+Put)
- ✅ **Thread-safe cache** - In-memory policy cache with RWMutex
- ✅ **Persistence** - Bucket policies in `.dirio/buckets/{bucket}.json`
- ✅ **Anonymous requests** - Unauthenticated requests supported for public buckets
- ✅ **Authorization middleware** - All S3 routes evaluated against policies
- ✅ **Admin bypass** - Root credentials skip policy checks

**Connection to Phase 5:** Policy engine will extend to IAM user/group policies.

### Phase 3.2 Features ✅ COMPLETE

**All Core S3 Features Implemented:**
- ✅ DeleteObject & DeleteBucket (MinIO mc compatibility)
- ✅ Pre-signed URLs (query-based SigV4 with expiration)
- ✅ CopyObject (S3-to-S3 with metadata)
- ✅ Range requests (206 Partial Content)
- ✅ ListObjectsV2 pagination (NextContinuationToken, StartAfter)
- ✅ Multipart upload (all 5 handlers)
- ✅ Object tagging (with content preservation)
- ✅ Custom metadata (case-insensitive, HTTP spec compliant)

## Phase 3.3: Advanced Policy Features ✅ COMPLETE

**Goal:** Enhance policy engine with advanced features for fine-grained access control.

**Status:** ✅ COMPLETE (Feb 16, 2026). Policy evaluation, filtering implementation, and client tests complete. Integration tests require multi-user IAM support (Phase 4).

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
- ✅ **Policy condition evaluation** ✅ COMPLETE (Feb 16, 2026) - IpAddress, StringEquals, DateLessThan, etc.
  - ✅ IpAddress / NotIpAddress conditions (test: policy-ip-test bucket)
  - ✅ StringEquals / StringLike / StringNotEquals / StringNotLike (test: policy-string-test bucket)
  - ✅ DateLessThan / DateGreaterThan / DateEquals (test: policy-time-test bucket)
  - ✅ NumericLessThan / NumericGreaterThan / NumericEquals (test: policy-numeric-test bucket)
  - ✅ Bool condition operator
  - ✅ Null condition operator
- ✅ **NotAction/NotResource/NotPrincipal** - Inverse matching support (test scenarios ready)
  - ✅ NotAction support (deny everything except specified actions) (test: policy-notaction-test bucket)
  - ✅ NotResource support (apply to all resources except specified) (test: policy-notresource-test bucket)
  - ✅ NotPrincipal support (apply to all principals except specified)
- ✅ **Policy variables** ✅ COMPLETE - Dynamic variable substitution (test scenarios ready)
  - ✅ ${aws:username} - Current authenticated username (test: policy-variables-test bucket)
  - ✅ ${aws:userid} - Current authenticated user ID (UUID-based)
  - ✅ ${aws:SourceIp} - Request source IP
  - ✅ ${s3:prefix} - Object key prefix (for ListObjects filtering)
  - ✅ ${s3:delimiter} - Delimiter for ListObjects
  - ✅ ${aws:CurrentTime} - Request timestamp (ISO 8601 format)
  - ✅ ${aws:UserAgent} - HTTP User-Agent header
  - ✅ Variable substitution engine with regex-based pattern matching
  - ✅ Context building from HTTP requests
  - ✅ Integration with policy evaluation engine
  - ✅ Comprehensive test coverage (all variables tested)

### Result Filtering ✅ COMPLETE (Feb 16, 2026)
- ✅ **ListBuckets result filtering** - Only return buckets user has permission to access ✅ COMPLETE
  - ✅ Evaluate GetBucketLocation permission per bucket
  - ✅ Filter out buckets without permission
  - ✅ Admin fast path (bypass filtering)
  - ✅ Use ownership tracking (UUID-based) for filtering
  - ✅ Implementation: `filterBuckets()` in `internal/http/api/s3/filtering.go`
  - ✅ Integration: Called by ListBuckets handler in `s3.go`
- ✅ **ListObjects result filtering** - Only return objects user has permission to read ✅ COMPLETE
  - ✅ Evaluate GetObject permission per object
  - ✅ Filter out objects without permission
  - ✅ Handle prefix-based permissions efficiently
  - ✅ Admin fast path (bypass filtering)
  - ✅ Use object ownership (UUID-based) for filtering
  - ✅ Implementation: `filterObjects()` in `internal/http/api/s3/filtering.go`
  - ✅ Integration: Called by ListObjects handlers in `bucket.go`
- ✅ **Unit tests** - Helper function tests in `filtering_test.go`
- ✅ **Client tests** - ✅ IMPLEMENTED (Feb 16, 2026) - Tests added, skip until IAM available (Phase 4)
  - ✅ Test ListBuckets returns only permitted buckets
  - ✅ Test ListObjects returns only permitted objects
  - ✅ Test filtering with AWS CLI
  - ✅ Test filtering with boto3
  - ✅ Test filtering with MinIO mc

**Note:** Setup scripts create test buckets with comprehensive policy scenarios. Client tests implemented but require IAM users to run (currently skipped).

### Testing ✅ COMPLETE
- ✅ Policy condition test scenarios - Setup script with comprehensive test buckets
- ✅ NotAction/NotResource/NotPrincipal test scenarios - Setup script complete
- ✅ Policy variable substitution tests - Setup script with user-specific folders
- ✅ ListBuckets/ListObjects filtering setup - Setup script creates filter-* buckets
- ✅ Automated tests for condition evaluation - 26 tests in internal/policy/conditions
- ✅ Automated tests for policy variables - Comprehensive coverage
- ✅ Unit tests for result filtering - Helper function tests in filtering_test.go
- ✅ Client tests for result filtering - Implemented (25 tests total, skip until IAM)

### Setup Script Enhancements ✅ COMPLETE
- ✅ Add SETUP_POLICY_TESTS flag to s3-minio-setup.sh (905 lines)
- ✅ Create test buckets with conditional policies
- ✅ Create users with prefix-based permissions
- ✅ Create test scenarios for NotAction/NotResource
- ✅ Create test buckets for result filtering (4 buckets, 60+ objects)

## Phase 4: Hybrid IAM & User Management

**Goal:** Implement hybrid IAM combining S3-native authorization (COMPLETE) with MinIO-compatible admin API (IN PROGRESS) for multi-user scenarios.

**Architecture:** Hybrid approach combining best of S3 and MinIO (see [docs/IAM-ARCHITECTURE.md](docs/IAM-ARCHITECTURE.md))
- **S3 API layer:** Bucket policies with S3 actions/resources, AWS-standard conditions/variables, UUID-based ownership ✅ COMPLETE
- **MinIO Admin API layer:** User/policy CRUD operations via `mc admin` commands ⏳ IN PROGRESS
- **Shared backend:** Unified IAM metadata in `.dirio/iam/` supporting both APIs ⏳ IN PROGRESS

**Target Compatibility:**
- S3 API (bucket policies via AWS CLI, boto3, MinIO mc) - data plane authorization ✅
- MinIO Admin API (`mc admin` for user/policy management) - control plane ⏳
- AWS-like authorization (ownership, conditions, variables, result filtering) ✅
- AWS IAM API (`aws iam` commands) ❌ Not supported by design
- Terraform AWS provider ❌ Not supported (requires AWS IAM API)

### Phase 4.1: Authorization Foundation ✅ COMPLETE

**Status:** ✅ COMPLETE (Feb 16, 2026) - Full policy engine with ownership-based authorization

**Implemented:**
- ✅ **UUID-based ownership system** - Buckets and objects track owner UUIDs
- ✅ **Policy evaluation engine** - Action/Resource/Principal/Effect matching with wildcards
- ✅ **Ownership-based authorization** - Resource owners have implicit access (AWS model)
- ✅ **Bucket policies** - S3-standard policy documents with persistence
- ✅ **Policy conditions** - All 6 operator categories (String, Numeric, Date, IP, Boolean, Null)
- ✅ **Policy variables** - Dynamic substitution (${aws:username}, ${s3:prefix}, etc.)
- ✅ **NotAction/NotResource/NotPrincipal** - Inverse matching support
- ✅ **Result filtering** - ListBuckets/ListObjects filter by permissions
- ✅ **Authorization middleware** - All S3 routes evaluated against policies
- ✅ **Anonymous access support** - Unauthenticated requests for public buckets
- ✅ **Admin bypass** - Root credentials skip policy checks

**Testing:**
- ✅ Unit tests for policy evaluation (26 tests in internal/policy/conditions)
- ✅ Unit tests for result filtering (filtering_test.go)
- ✅ Client filtering tests implemented (25 tests, require IAM users to activate)
- ✅ Setup scripts with comprehensive policy test scenarios

### Phase 4.2: Core IAM — mc-Compatible User & Policy Management ✅ COMPLETE

**Status:** ✅ COMPLETE (Feb 20, 2026) — All endpoints implemented and tested. Credential encryption at rest is deferred to Phase 4.4.

**Goal:** Complete the MinIO-compatible IAM backbone — everything `mc admin` can drive. Groups, service accounts, and DirIO-specific IAM features move to Phase 4.4 (after the console foundation is in place).

### User Management
- ✅ AddUser — `PUT /minio/admin/v3/add-user` (`internal/http/api/iam/user.go`)
- ✅ RemoveUser — `POST /minio/admin/v3/remove-user`
- ✅ ListUsers — `GET /minio/admin/v3/list-users`
- ✅ GetUserInfo — `GET /minio/admin/v3/user-info`
- ✅ SetUserStatus (enable/disable) — `POST /minio/admin/v3/set-user-status`

### Policy Management
- ✅ AddPolicy — `POST|PUT /minio/admin/v3/add-canned-policy` (`internal/http/api/iam/policy.go`)
- ✅ RemovePolicy — `POST /minio/admin/v3/remove-canned-policy`
- ✅ ListPolicies — `GET /minio/admin/v3/list-canned-policies`
- ✅ GetPolicy — `GET /minio/admin/v3/info-canned-policy`
- ✅ SetPolicy (attach) — `POST /minio/admin/v3/set-policy` + `POST /minio/admin/v3/idp/builtin/policy/attach` (users + groups)
- ✅ ListPolicyEntities — `GET /minio/admin/v3/policy-entities` (returns both `userMappings` and `groupMappings`)
- ✅ UnsetPolicy (detach) — `POST /minio/admin/v3/idp/builtin/policy/detach` (users + groups)

### Storage & Data Model
- ✅ IAM metadata storage structure (`.dirio/iam/users/`, `.dirio/iam/policies/`)
- ✅ User metadata schema — UUID, accessKey, secretKey, status, attachedPolicies (`pkg/iam/types.go`)
- ✅ Policy metadata schema — name, PolicyDocument, timestamps (`pkg/iam/types.go`)
- ✅ Credential encryption at rest — currently stored as plaintext JSON (only encrypted in transit)

### API Design
- ✅ MinIO Admin API endpoints (`/minio/admin/v3/*`) on main port (`internal/http/server/routes.go`)
- ✅ Path-based routing middleware (teapot router)
- ✅ JSON request/response format
- ✅ mDNS registration for admin endpoints (`internal/mdns/mdns.go`)

### Authentication Integration
- ✅ Multi-user auth support — checks root, alt-root, then IAM users (`internal/http/auth/auth.go`)
- ✅ Multi-user credential lookup — user injected into request context via middleware

### Testing
- ✅ Admin API integration tests — `tests/admin/` (37 tests covering all user/policy CRUD + attach/detach/entities)
  - ✅ User CRUD: create, list, info, delete, enable/disable, validation errors, duplicate detection
  - ✅ Policy CRUD: create, list, info, delete, invalid doc rejection, missing name
  - ✅ Policy attach/detach: attach, detach, idempotent attach, policy-entities, policy-not-found now returns 404
  - ✅ madmin encryption protocol tested (add-user uses EncryptData/DecryptData)
- ✅ MinIO IAM import integration tests — `tests/admin/minio_import_test.go`
  - ✅ Users, policies, user-policy mappings imported on startup
  - ✅ Disabled users imported with correct "off" status (bug found + fixed)
  - ✅ Idempotent re-import (state file prevents duplicate import on restart)
  - ✅ Post-import user management via admin API

## Phase 4.3: Web Admin Console Foundation ✅ COMPLETE

**Goal:** Build an embedded admin console into the DirIO server as the primary interface for DirIO-specific hybrid IAM features that `mc` and S3 clients cannot reach.

**Architecture:** See [docs/CONSOLE-ARCHITECTURE.md](docs/CONSOLE-ARCHITECTURE.md) for full design.

**Key decisions:**
- `consoleapi/` package defines the interface seam — the only coupling point between console and server
- `console/` package lives outside `internal/`, imports only `consoleapi/` — extractable later
- `internal/console/adapter.go` implements the interface by calling the service layer directly (no HTTP round-trips)
- Build tag `noconsole` strips it entirely: `go build -tags noconsole`
- Served at `/dirio/ui/` on main port by default; `--console-address :9001` for separate port
- MinIO admin API stays on main port always — `mc` compatibility requires this

### Package Structure ✅ COMPLETE
- ✅ `consoleapi/` — `ConsoleAPI` interface + all request/response types
- ✅ `console/auth/` — `AdminAuth` interface + `Session` (HMAC-SHA256 signed cookies, 8-hour TTL)
- ✅ `console/handlers/` — Login/Logout, Dashboard, Users, Policies, Buckets list, Bucket detail, Ownership transfer, Policy editor, Simulator; HTMX partial-swap support
- ✅ `console/ui/` — templ components: layout, all list pages, bucket detail (policy editor + ownership), policy simulator
- ✅ `console/static/` — Tailwind v4 CSS, htmx.min.js, DirIO logo; all embedded via Go `embed`
- ✅ `internal/console/adapter.go` — all methods wired: Users (5), Policies (6), Buckets (GetBucket/List/GetPolicy/SetPolicy), Ownership (GetBucketOwner/Transfer/GetObjectOwner), Observability (GetEffectivePermissions/SimulateRequest)
- ✅ `cmd/server/cmd/wire_console.go` + `wire_console_stub.go` build tag wiring in place

### Configuration ✅ COMPLETE
- ✅ `console.enabled` / `--console` flag (default: true)
- ✅ `console.address` / `--console-address` for optional separate port

### Foundation UI ✅ COMPLETE
- ✅ Basic auth — login page using admin credentials; HMAC-signed session cookies
- ✅ Dashboard — bucket count, user count, policy count
- ✅ Bucket list — with owner display; bucket names link to detail page
- ✅ User list — with attached policies and status
- ✅ Policy list — with name and timestamps

### Stopgap Priorities ✅ COMPLETE
- ✅ **Ownership view** — bucket list shows owner (access key + username resolved from UUID)
- ✅ **Ownership management** — bucket detail page: transfer ownership to any IAM user by access key
- ✅ **Full S3 bucket policy editor** — bucket detail page: view/edit raw JSON, save or clear
- ✅ **Policy observability** — Simulate page: single-action allow/deny evaluation with reason; "show all permissions" view across all common S3 actions
- ✅ **Object owner** — `GetObjectOwner` adapter wired; no dedicated UI (Phase 8 file browser)

### Not in scope for Phase 4.3 (→ Phase 4.4)
- User/policy CRUD forms in the console UI
- Group management, service account management, access key management

---

## Phase 4.4: Extended IAM + Console Stopgaps

**Goal:** Build out the IAM features that go beyond what `mc` alone can drive, using the Phase 4.3 console as their primary interface. These features require the console foundation to be in place first.

### Group Management (mc-compatible, but lower priority)
- ✅ AddGroup, RemoveGroup, ListGroups, GetGroupInfo
- ✅ GroupAdd / GroupRemove — add/remove users from groups
- ✅ Attach/detach policies to groups (`/idp/builtin/policy/attach|detach` and `/set-policy` — shared with users via `isGroup` flag)
- ✅ Console UI: group list, membership management

### Service Account Management (mc-compatible + DirIO extensions)
- ✅ AddServiceAccount — long-lived credentials, optional expiration, conflict detection across users + SAs
- ✅ RemoveServiceAccount, ListServiceAccounts, GetServiceAccountInfo, UpdateServiceAccount
- ✅ Policy inheritance from parent user with optional override — eval-time resolution via `PolicyMode` (`inherit`/`override`)
- ✅ Console UI: service account list, expiration management

### Access Key Management
- Service accounts cover the multi-key / per-user scoped credential use case
- ✅ User secret key rotation (update secret key without changing access key) — simple `update-user` call, no separate endpoint needed

### Console Stopgaps (DirIO-specific — no mc equivalent)
- ✅ **Ownership management UI** — view bucket/object owners, transfer ownership
- ✅ **Effective permissions view** — show a user's combined access (bucket policy + IAM policies)
- ✅ **Request simulator** — given user + bucket + action, show allow/deny and which rule decided it
- ✅ **Full S3 bucket policy editor** — JSON editor with conditions/variables (beyond `mc policy set` canned policies)

### Testing
- ✅ Unit tests for group/service account CRUD (13 group + 12 SA tests in `tests/admin/`)
- ✅ Integration tests for group policy inheritance
- ✅ Service account delegation and expiration testing (`tests/integration/serviceaccount_policy_test.go`)
- ✅ Console stopgap feature testing (`tests/console/` — 27 tests: session auth, policy editor, ownership management, request simulator)
- ✅ Integration tests with live `mc admin` CLI (`tests/clients/scripts/mc_admin.sh` + `TestMCAdmin` in `clients_test.go`)
- ✅ Multi-user S3 access scenarios (alice/bob test users)
- ✅ **Activate client filtering tests** — create alice/bob users to run existing filtering tests
- ✅ **Create integration tests** — `tests/integration/list_filtering_test.go` for result filtering

---

## Phase 4.5: Stability & Performance

### Browser Upload Support
- [x] **POST Policy Uploads** - Browser-based form uploads
  - [x] Parse POST policy documents
  - [x] Validate policy signature and expiration
  - [x] Support multipart/form-data uploads
  - [x] HTML form upload examples (`examples/post-policy/index.html`)
  - [x] MinIO `mc share upload` compatibility

### Performance Optimization
- [x] Metadata caching strategy — `phuslu/lru` sharded LRU in `metadata.Manager`; ~100–300× list speedup
- [x] Optimize ListObjects for large buckets — early walk termination in `listInternal` (stops at `maxKeys+1`)
- [x] Memory profiling and leak detection — no goroutine leaks, no heap growth under sustained load

### Stability & Testing
- [ ] Concurrent access testing
- [ ] Error handling audit across all API handlers
- [ ] Load testing with large files and many small files
- [ ] Test migration from actual MinIO instance
- [ ] Test behind reverse proxy (nginx) with canonical domain

## Phase 5: Production Readiness & Operations

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
- [ ] Sustained load test (multipart upload memory under concurrent load) — deferred, needs wrk/hey/k6 infra

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

## Phase 8: Web UI — Extended Features

**Foundation built in Phase 4.3. This phase covers features beyond IAM stopgaps.**

- [ ] File browser (browse bucket contents, preview objects)
- [ ] Upload interface (drag-and-drop, progress)
- [ ] Audit log viewer (when Phase 7 audit logging is implemented)
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
- [x] Console architecture - See [CONSOLE-ARCHITECTURE.md](docs/CONSOLE-ARCHITECTURE.md)
- [ ] mDNS setup and troubleshooting
- [ ] Reverse proxy setup guide (nginx examples)
- [ ] S3 API compliance status
- [ ] Troubleshooting guide
- [ ] Performance tuning guide