# DirIO Development Roadmap

Current status: **Phase 4.5 complete** — Phases 1–4.5 done; next up is Phase 5 (production readiness) and Phase 8 (extended console UI).

> 📋 Completed work log: [docs/CHANGELOG.md](docs/CHANGELOG.md)

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

**Goal:** Implement hybrid IAM combining S3-native authorization (COMPLETE) with MinIO-compatible admin API (COMPLETE) for multi-user scenarios.

**Architecture:** Hybrid approach combining best of S3 and MinIO (see [docs/IAM-ARCHITECTURE.md](docs/IAM-ARCHITECTURE.md))
- **S3 API layer:** Bucket policies with S3 actions/resources, AWS-standard conditions/variables, UUID-based ownership ✅ COMPLETE
- **MinIO Admin API layer:** User/policy CRUD operations via `mc admin` commands ✅ COMPLETE
- **Shared backend:** Unified IAM metadata in `.dirio/iam/` supporting both APIs ✅ COMPLETE

**Target Compatibility:**
- S3 API (bucket policies via AWS CLI, boto3, MinIO mc) - data plane authorization ✅
- MinIO Admin API (`mc admin` for user/policy management) - control plane ✅
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
- [x] Concurrent access testing
- [x] Error handling audit across all API handlers
- [x] Load testing with large files and many small files

## Phase 5: Observability & Health

**Goal:** Give DirIO the instrumentation it needs to run reliably in production — visibility into what's happening, proof that it's healthy, and a lightweight audit trail out of the box.

### Health Checks
- [x] **Health endpoint** (`GET /health`) — returns 200 + JSON status; used by load balancers, Docker health checks, and basic monitoring
- [x] **Readiness probe** (`GET /health/ready`) — checks BoltDB is open and storage directory is accessible; returns 503 if not ready
- [x] **Liveness probe** (`GET /health/live`) — confirms the process is alive and not deadlocked; always 200 if reachable

### Metrics
- [ ] **Prometheus metrics endpoint** (`GET /metrics`) — request count by method/status, error rate, latency histograms (p50/p95/p99), metadata cache hit ratio, active connections, BoltDB size

### Structured Access Log
- [ ] **Structured access log to stdout** — one JSON line per S3/admin/console request: timestamp, user (or `"anonymous"`), service (s3/admin/console), bucket, object, action, allow/deny decision, source IP, request ID, latency ms
  - Always on, zero body capture, minimal allocations — suitable for direct ingestion by Loki, CloudWatch, Datadog, etc.
  - Configurable format: `json` (default) or `logfmt` via `--log-format` flag

## Phase 6: Full HTTP Audit Logging

**Goal:** Production-grade audit trail for compliance and debugging. Builds on the Phase 5 structured access log — this phase adds body capture, configurable verbosity levels, non-blocking I/O, and a UI to browse logs.

**Distinction from Phase 5 access log:** Phase 5 logs one line per request (who/what/allow-deny). This phase adds full request/response bodies, streaming to external destinations, and tooling to query the log.

### Middleware
- [ ] Non-blocking audit log writer with bounded queue (no request latency impact)
- [ ] Log levels: `0`=off, `1`=access only (Phase 5 baseline), `2`=headers, `3`=headers + request body, `4`=headers + both bodies
- [ ] Minimize allocations in hot path — avoid capturing body unless level ≥ 3
- [ ] Configurable output destination: file, stdout, or HTTP endpoint (e.g. vector, fluentd)
- [ ] Log rotation support (size-based and time-based)

### Configuration
- [ ] `audit.level` config key + `--audit-level` flag
- [ ] `audit.output` config key (stdout / file path / HTTP endpoint)
- [ ] `audit.max_body_bytes` — cap body capture size (default 4KB)

### Observability
- [ ] Document the two-tier log model: Phase 5 access log (always on, lightweight) vs Phase 6 audit log (configurable, heavy)

## Phase 7: Deployment & Operations

**Goal:** Validate and document DirIO in real deployment scenarios. Establish the dual-port mode as the recommended production topology, harden operational tooling, and confirm the MinIO migration path end-to-end.

### Deployment Modes

Both single-port and dual-port modes are supported and maintained. **Dual-port is the recommended production mode.**

**Single-port mode** (current default): S3, admin API, and console all share one port, distinguished by path prefix. Simple to set up; useful for embedded/dev deployments. The trade-off is path-based muxing overhead and more complex routing rules.

**Dual-port mode** (recommended for production): S3 data plane on a dedicated port (e.g. `:9000`), admin + console control plane on a separate port (e.g. `:9001`). Each service gets its own router with no path-prefix logic. Enables clean DNS separation (e.g. `s3.myserver.local` → `:9000`, `admin.myserver.local` → `:9001` via nginx or mDNS) and simplifies firewall rules — S3 traffic never touches the admin port.

- [ ] **Document deployment modes** — write `docs/DEPLOYMENT.md` covering single-port vs dual-port, when to use each, example configs for both, and mDNS/DNS routing for dual-port
- [ ] **nginx reference configs** — document `proxy_pass` examples for both modes: S3 path-routed on single port, and split-port with separate `server {}` blocks; include TLS termination, Host header preservation, and pre-signed URL considerations
- [ ] **Docker Compose example** — single service, dual-port exposed, bind-mounted data directory; suitable as a quickstart template

### Operational Validation
- [ ] **End-to-end MinIO migration test** — export data from a real MinIO instance, import into DirIO, verify all objects, metadata, and IAM (users/policies/mappings) are intact
- [ ] **Reverse proxy integration test** — run DirIO behind nginx in dual-port mode; verify `mc`, AWS CLI, and boto3 all work correctly including pre-signed URLs and chunked uploads
- [ ] **Sustained load test** — multipart uploads under concurrent load using wrk/hey/k6; confirm no heap growth over time (builds on Phase 4.5 memory profiling baseline)

### Configuration Tooling
- [ ] **`dirio config set` subcommand** — update data config values without manually editing `.dirio/config.json` (e.g. `dirio config set region us-west-2`, `dirio config set compression.enabled true`); print current config via `dirio config show`

## Phase 8: Web Console — Extended Features

**Foundation built in Phase 4.3 (auth, IAM views, policy editor, simulator, ownership management). This phase covers the S3 data plane UI and IAM management forms — making DirIO fully operable without a terminal for day-to-day tasks.**

### S3 Data Browser
- [ ] **Bucket browser** — list objects with prefix navigation (folder-style), sortable by name/size/date
- [ ] **Object detail** — view metadata, tags, owner, ETag; download button; copy pre-signed URL
- [ ] **Upload interface** — drag-and-drop file upload with progress bar; multipart for large files; uses POST policy or pre-signed PUT
- [ ] **Object actions** — delete object, copy object (within/across buckets), set tags

### IAM Management Forms
- [ ] **User CRUD forms** — create/edit/delete users directly in console (currently requires `mc admin`)
- [ ] **Policy CRUD forms** — create/edit named policies with JSON editor + validation (currently requires `mc admin` or the raw policy editor)
- [ ] **Service account management** — create/revoke service accounts, view expiry, manage policy mode
- [ ] **Group management UI** — create groups, assign members, attach policies

### Audit Log Viewer (depends on Phase 6)
- [ ] Filterable log stream in console — filter by user, bucket, action, allow/deny, time range
- [ ] Export filtered log to CSV/JSON

## Phase 9: DirIO Client

**Goal:** A first-party CLI client for DirIO that covers the operations no existing tool handles well — DirIO-specific features, scripting-friendly output, and a single binary that doesn't require `mc` or AWS CLI to be installed.

**Design principle:** Don't replicate what `mc` and AWS CLI already do well. Focus on DirIO-specific operations and convenience wrappers that make scripting and automation easy. Standard S3 operations (upload/download/sync) are included because having them in one tool is practical, but they are not the primary motivation.

### S3 Operations
- [ ] `dirio ls [bucket[/prefix]]` — list buckets or objects; JSON and table output modes
- [ ] `dirio cp <src> <dst>` — upload/download/copy; supports `s3://` URIs; multipart for large files
- [ ] `dirio sync <src> <dst>` — sync local directory to/from bucket; `--delete` flag

### DirIO-Specific Operations (no equivalent in `mc` or AWS CLI)
- [ ] `dirio ownership get <bucket[/object]>` — show current owner
- [ ] `dirio ownership transfer <bucket> <user>` — transfer bucket ownership
- [ ] `dirio simulate <user> <bucket> <action>` — run the policy simulator from CLI; returns allow/deny + reason
- [ ] `dirio sa create <parent-user> [--policy override] [--expires <duration>]` — create service account with policy mode
- [ ] `dirio sa list [user]` — list service accounts

### IAM Convenience (wraps `mc admin` equivalents for single-binary workflows)
- [ ] `dirio iam user create/list/delete/enable/disable`
- [ ] `dirio iam policy create/list/attach/detach`

### Configuration & Auth
- [ ] `dirio config init` — interactive setup for endpoint, credentials, and default bucket
- [ ] Named profiles (similar to AWS CLI `~/.aws/credentials`)
- [ ] Respect `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` env vars for drop-in compatibility

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

Priority docs — these are the highest-value items for any external user of DirIO:

- [ ] **Migration guide from MinIO** — extract the MinIO import section from README into `docs/MIGRATION.md`; expand with step-by-step walkthrough, data layout comparison, IAM import details, known differences, and a "what doesn't migrate" section. Designed to grow as the app matures.
- [ ] **S3 API compliance status** — which operations are supported, which are intentionally omitted, known deviations from AWS S3 behavior; should reference CLIENTS.md
- [ ] **Configuration guide** (CLI/ENV/YAML) — all flags, env vars, and config file keys in one place; data config vs app config distinction

Reference docs (lower urgency):
- [ ] API documentation (internal — endpoint list with request/response shapes)
- [ ] mDNS setup and troubleshooting
- [ ] Reverse proxy setup guide (nginx examples; will come out of Phase 7 deployment work)
- [ ] Troubleshooting guide
- [ ] Performance tuning guide

Already complete:
- [x] Client compatibility guide — [CLIENTS.md](CLIENTS.md)
- [x] IAM/Admin API architecture — [docs/IAM-ARCHITECTURE.md](docs/IAM-ARCHITECTURE.md)
- [x] Console architecture — [docs/CONSOLE-ARCHITECTURE.md](docs/CONSOLE-ARCHITECTURE.md)
- [x] Completed work log — [docs/CHANGELOG.md](docs/CHANGELOG.md)