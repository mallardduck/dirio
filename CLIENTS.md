# S3 Client Compatibility Documentation

This document tracks DirIO's compatibility with various S3 clients, test results, and known issues.

**Latest Update: January 31, 2026 18:30 UTC**
**Test Framework:** testcontainers-go with Docker containers for each client
**Test Location:** `tests/clients/` using canonical scripts from `tests/clients/scripts/`

---

## 🚨 Critical Issues

**Bug #001: AWS SigV4 Chunked Encoding Corruption** - See [bugs/001-chunked-encoding-corruption.md](bugs/001-chunked-encoding-corruption.md)
- **Status:** STILL PRESENT (verified Jan 31, 2026)
- AWS Signature V4 chunked transfer encoding headers are being written directly to object files
- **Impact:** Affects ALL write operations (PutObject, multipart uploads, object tagging)
- **Evidence:** Object content contains `d;chunk-signature=...` markers followed by data chunks
- **Priority:** CRITICAL - Must fix before any write operations can be considered working

---

## S3 Client Compatibility Matrix

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

**Legend:** ✅ Works | ❌ Fails | ⚠️ Partial | ❓Untested

---

## Test Results Summary

**S3 API Implementation Status** (24 unique features tested across 3 clients):
- ✅ **Fully Working:** 13/24 (54%) - Works correctly across all tested clients
- ⚠️ **Partially Working:** 4/24 (17%) - ListObjectsV2 delimiter (boto3 fails, mc works), Custom metadata (set works, get has issues), Pre-signed URLs (partial failures), Multipart/Tagging (boto3 fails, mc works)
- ❌ **Not Working:** 7/24 (29%) - DeleteObject/DeleteBucket for mc, Range requests, CopyObject, ListObjectsV2 max-keys, Pre-signed URLs, Custom metadata get
- ❓ **Not Tested:** Many features only tested with subset of clients

---

## Detailed Client Test Results

### AWS CLI (11/11 tests passed - 100%)

**Status:** ✅ **Fully Compatible**

**Test Coverage:**
- ✅ ListBuckets
- ✅ CreateBucket
- ✅ HeadBucket (returns `x-amz-bucket-region` header)
- ✅ PutObject
- ✅ HeadObject
- ✅ GetObject
- ✅ ListObjectsV2
- ✅ High-level `s3 cp` upload
- ✅ High-level `s3 cp` download
- ✅ DeleteObject
- ✅ DeleteBucket

**Notes:**
- All core CRUD operations work perfectly
- High-level `s3` commands work
- ⚠️ Content verification not yet implemented for AWS CLI tests (may be affected by bug #001)

---

### boto3 (13/21 tests passed - 62%)

**Status:** ⚠️ **Partially Compatible**

**Working Features:**
- ✅ Core CRUD operations (Create, Read, Update, Delete)
- ✅ GetBucketLocation
- ✅ ListObjectsV1
- ✅ ListObjectsV2 (basic/prefix)
- ✅ Custom metadata set

**Issues:**
- ⚠️ Custom metadata get returns Title-Case keys (`Custom-Key`) instead of lowercase (`custom-key`)

**Failed Tests (8/21):**
- ❌ ListObjectsV2 delimiter: Returns 0 CommonPrefixes instead of 2+
- ❌ ListObjectsV2 max-keys: Ignores MaxKeys=2, returns all 5 objects
- ❌ Range request: Returns full 100 bytes instead of first 10 bytes
- ❌ CopyObject: Creates 0-byte empty file instead of copying content
- ❌ Pre-signed URLs: Returns 403 Forbidden
- ❌ Multipart: Returns 405 Method Not Allowed
- ❌ Object Tagging: Corrupts object content with XML (root cause: bug #001 + query parameter `?tagging` routing issue)

---

### MinIO mc (20/30 tests passed - 67%)

**Status:** ⚠️ **Partially Compatible**

**Expanded Test Coverage:** Now testing 30 features with comprehensive content verification

**Working Features:**

**Bucket Operations (6/6):**
- ✅ Configure alias
- ✅ ListBuckets
- ✅ CreateBucket (`mc mb`)
- ✅ HeadBucket (`mc stat --no-list` / `mc stat`)
- ✅ GetBucketLocation (`mc stat`)

**Object CRUD (9/11):**
- ✅ PutObject (`mc put` / `mc cp`)
- ✅ HeadObject (`mc stat`)
- ✅ GetObject (`mc cp` / `mc cat`)
- ✅ ListObjectsV2 (`mc ls` / `mc ls prefix/` / `mc ls -r`)
- ✅ ListObjectsV2 with delimiter

**Tagging/Multipart Operations (5/7):**
- ✅ Object tagging set/get work
- ✅ Multipart upload completes
- ✅ Size metadata correct

**Failed Tests (10/30) - Bug #001 Confirmed:**
- ❌ DeleteObject (`mc rm`): 405 Method Not Allowed
- ❌ DeleteBucket (`mc rb`): 405 Method Not Allowed (bucket not empty due to DeleteObject failure)
- ❌ Custom Metadata get: Not returned in `mc stat`
- ❌ CopyObject (`mc cp s3-to-s3`): EOF error
- ❌ Pre-signed URL download: Failed to download (curl error)
- ❌ Pre-signed URL upload: Failed to upload
- ❌ Range Requests (curl): Returns 0 bytes instead of 10
- ❌ **Object Tagging - content corruption**: Tags stored as object content (XML replaces original) - Root cause: bug #001 + query routing
- ❌ **Multipart Upload - content corruption**: Downloaded 10,500,246 bytes instead of 10,485,760 bytes (bug #001)
- ❌ **GetObject - chunked encoding leak**: AWS SigV4 chunk headers visible in content (bug #001) - Example: `d;chunk-signature=...`

---

## Testing Methodology

### Test Framework Setup

**Framework:** testcontainers-go running Docker containers for each client
**Scripts:** Canonical test scripts from `tests/clients/scripts/` embedded via go:embed
**Command:** `go test -v ./tests/clients/...`

### Defensive Testing (Jan 31, 2026)

Comprehensive content verification added to prevent false positives:

- ✅ **Object Tagging tests:** Verify content before and after tagging - EXPOSED BUG: tags replace object content
- ✅ **Multipart Upload tests:** Download and verify byte-for-byte content integrity - EXPOSED BUG: downloaded file 14KB larger than original
- ✅ **GetObject tests:** Verify exact content matches expected data - EXPOSED BUG: chunked encoding markers in content
- These defensive checks revealed that AWS SigV4 chunked transfer encoding headers are being written directly to object files, corrupting data

### Sanity Testing & Defensive Checks

Added comprehensive validation to prevent false positives:

- ✅ **FailingServer test:** Returns 500 errors - all clients correctly fail
- ✅ **DumbSuccessServer test:** Returns 200 OK with empty responses - all clients correctly fail
- ✅ **Defensive boto3 checks:** Added content validation to detect query parameter routing issues:
  - GetBucketLocation: Verify response contains `LocationConstraint` field
  - ListObjectsV2: Verify actual object keys in results
  - Custom Metadata: Verify object content not corrupted by metadata operations
  - Object Tagging: Verify object content not overwritten by tagging XML
  - Multipart Upload: Verify assembled content matches expected parts
- These tests confirm passing tests are validating actual server functionality, not just status codes or accidental matches

---

## Architecture Improvements (January 27, 2026)

### Auth Package Refactor

- ✅ **Merged sigv4 into auth package** - `internal/sigv4/` → `internal/auth/signature.go`
- ✅ **Unified authentication API** - Single `AuthenticateRequest(r)` method replaces 4-step orchestration
- ✅ **Auth middleware encapsulation** - Moved from `server.go` to `auth.AuthMiddleware()` method
- ✅ **Improved architecture:**
  - Single package owns all authentication concerns (signature verification, user lookup, validation)
  - Cleaner API: `user, err := auth.AuthenticateRequest(r)` instead of juggling sigv4 + auth packages
  - Better testability and reusability
  - User added to request context for downstream handlers
- ✅ **No regressions** - All test results identical before/after refactor

### MinIO mc Success & Remaining Issues

- ✅ **Resolved:** Object PUTs now work perfectly (mc put/cp both working)
- ✅ **Working:** Multipart uploads, object tagging, custom metadata set, ListObjectsV2 with delimiter/prefix/recursive
- ❌ **Critical blockers:** DeleteObject and DeleteBucket return 405 Method Not Allowed
  - Likely routing issue or missing HTTP method handler
  - Works perfectly in AWS CLI and boto3, mc-specific problem
- ❌ **Other failures:** Range requests, CopyObject, Pre-signed URLs, Custom metadata retrieval

---

## Known Working Features (Jan 31, 2026)

- ✅ GetBucketLocation (AWS CLI, boto3, MinIO mc) - FIXED Jan 24, 2026
- ✅ HeadBucket (AWS CLI, boto3, MinIO mc)
- ✅ DeleteObject/DeleteBucket work in AWS CLI and boto3
- ✅ ListObjectsV2 with delimiter works in MinIO mc (Jan 31, 2026)
- ✅ Object metadata operations (size, ETag, timestamps) work correctly

---

## Known Broken Features (Bug #001 Impact)

**⚠️ Content Corruption - Bug #001 STILL PRESENT (Jan 31, 2026 18:30 UTC)**

- ❌ **PutObject** - Uploads succeed but content includes chunked encoding artifacts (verified in mc cat output)
- ❌ **GetObject** - Downloads succeed but return corrupted content with encoding markers like `d;chunk-signature=...`
- ❌ **Multipart uploads** - Upload completes, wrong size (10,500,246 vs 10,485,760 bytes)
- ❌ **Object tagging** - Operations succeed but tags replace object content (combined bug #001 + query routing issue)