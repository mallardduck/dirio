# DirIO Development Changelog

A running log of completed work by date, moved here from the main TODO roadmap to keep it focused on what remains.

---

## February 23, 2026 — Phase 4.5 Performance Optimizations Complete

- ✅ pprof endpoints added (gated on `--debug` flag) — `run-profile` Taskfile task
- ✅ `scripts/seed-large-bucket.sh` — seeds 10k objects across 4 prefix patterns for profiling
- ✅ `tests/perf/` — opt-in profiling tests (`//go:build perf`, `task test-perf`) using testcontainers for seeding; three tests: `TestPerfMetadataCaching`, `TestPerfListObjectsLargeBucket`, `TestPerfMemory`
- ✅ **Metadata cache** — `github.com/phuslu/lru` sharded LRU (100k entries, ~20 MB cap) added to `metadata.Manager`; exact invalidation on all write/delete paths. Cache hit eliminates per-object file open + JSON decode.
- ✅ **Early walk termination** — `listInternal` stops walking after `maxKeys+1` entries when `delimiter=""`. `full-scan-100` is now ~3× faster than `full-scan-1000` (proves early exit). Both dropped from ~450ms → ~1.5–4.5ms per call (~100–300× improvement).
- ✅ **Memory leak check** — goroutine diff: net zero; live heap delta: ~2.5 KB after 200 rounds. No leaks detected.
- ⏭️ **Sustained load test / memory profiling deferred** — existing perf data shows no active leaks. Multipart upload memory behaviour under sustained concurrent load is the remaining open question; deferred to a later phase alongside load testing infrastructure (wrk/hey/k6).

---

## February 22, 2026 — Phase 4.4 Complete

- ✅ `tests/console/` — 27 console stopgap tests: session auth (login/logout/protected routes), full S3 bucket policy editor, bucket ownership management, request simulator (single-action + effective permissions)

## February 22, 2026 — Phase 4.4 Testing Complete (except console)

- ✅ `tests/integration/serviceaccount_policy_test.go` — SA delegation (inherit/override mode) and expiration integration tests
- ✅ `tests/clients/scripts/mc_admin.sh` + `TestMCAdmin` — mc admin CLI testcontainer tests (user add/list/info, policy CRUD, group add, user disable/enable/remove)
- ✅ `internal/persistence/metadata/import.go` — MinIO import now rebuilds bolt indexes after import so users are immediately visible
- ✅ `tests/admin/helpers_test.go` — Added `Stop()` method and cancelable context to `NewTestServerWithDataDir` for clean BoltDB lock release

---

## February 21, 2026 — SA Policy Inheritance (Eval-Time Resolution)

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

---

## February 21, 2026 — Phase 4.3 Complete

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

---

## February 20, 2026 — Phase 4.2 Complete

- ✅ **Admin Integration Test Suite** (`tests/admin/`, 37 tests) — New test area separate from S3 integration tests
  - User CRUD, policy CRUD, attach/detach, policy-entities — all endpoints covered
  - madmin encryption protocol tested end-to-end (EncryptData/DecryptData)
- ✅ **MinIO IAM Import Tests** (`tests/admin/minio_import_test.go`) — End-to-end import verification
  - Users, policies, mappings, disabled status, idempotent restart, post-import management
- 🐛 **Bug Fix:** MinIO "enabled"/"disabled" status not converted to DirIO "on"/"off" on import
- 🐛 **Bug Fix:** `AttachPolicy` silently accepted non-existent policy names — now returns 404
- ✅ **UnsetPolicy HTTP endpoint** confirmed complete (`/idp/builtin/policy/detach`)

---

## February 16, 2026 — Phase 3.3 Status Update

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

## February 16, 2026 — Policy Condition Evaluation Complete

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

## February 16, 2026 — Phase 3.2 Complete

- ✅ **Core S3 Features:** Multipart upload, pre-signed URLs, CopyObject, range requests, object tagging
- ✅ **Test Framework:** Structured JSON output with content integrity validation (MD5 hashes)
- ✅ **Client Compatibility:** AWS CLI (91%), boto3 (96%), MinIO mc (87%) - 23 canonical operations tested
- ✅ **Bug Fixes:** ListObjectsV2 pagination & delimiter, chunked encoding, MinIO mc DELETE operations
- 📁 **Known Issues:** See [bugs/](../bugs/) for tracking (1 minor issue: MinIO mc PreSignedURL_Upload)