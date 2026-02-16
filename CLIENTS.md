# S3 Client Compatibility

DirIO's compatibility status with major S3 clients.

**Last Updated:** February 16, 2026
**Test Framework:** Structured JSON output with automated feature matrix
**Test Location:** `tests/clients/`

---

## Quick Summary

**Overall S3 Compatibility:** 21/23 core operations working (91%)

| Client | Tested | Passed | Failed | Skipped | Pass Rate |
|--------|--------|--------|--------|---------|-----------|
| AWS CLI | 23 | 21 | 0 | 2 | 91% |
| boto3 | 23 | 22 | 0 | 1 | 96% |
| MinIO mc | 23 | 20 | 1 | 2 | 87% |

---

## Feature Compatibility Matrix

All clients test the same 23 canonical S3 operations defined in `tests/clients/features.yaml`.

### Bucket Operations

| Feature | AWS CLI | boto3 | MinIO mc | Notes |
|---------|---------|-------|----------|-------|
| ListBuckets | ✅ | ✅ | ✅ | List all buckets |
| CreateBucket | ✅ | ✅ | ✅ | Create new bucket |
| HeadBucket | ✅ | ✅ | ✅ | Check bucket existence |
| GetBucketLocation | ✅ | ✅ | ✅ | Get bucket region |
| DeleteBucket | ✅ | ✅ | ✅ | Delete empty bucket |

### Object Operations

| Feature | AWS CLI | boto3 | MinIO mc | Notes |
|---------|---------|-------|----------|-------|
| PutObject | ✅ | ✅ | ✅ | Upload object |
| GetObject | ✅ | ✅ | ✅ | Download object with content verification |
| HeadObject | ✅ | ✅ | ✅ | Get object metadata |
| DeleteObject | ✅ | ✅ | ✅ | Delete object |
| CopyObject | ✅ | ✅ | ✅ | Server-side copy with content verification |

### Listing Operations

| Feature | AWS CLI | boto3 | MinIO mc | Notes |
|---------|---------|-------|----------|-------|
| ListObjectsV2_Basic | ✅ | ✅ | ✅ | List all objects |
| ListObjectsV2_Prefix | ✅ | ✅ | ✅ | Filter by prefix |
| ListObjectsV2_Delimiter | ✅ | ✅ | ✅ | Hierarchical listing with CommonPrefixes |
| ListObjectsV2_MaxKeys | ✅ | ✅ | ⏭️ | Pagination support (mc doesn't expose) |
| ListObjectsV1 | ⏭️ | ✅ | ⏭️ | Legacy API (clients use V2 by default) |

### Metadata Operations

| Feature | AWS CLI | boto3 | MinIO mc | Notes |
|---------|---------|-------|----------|-------|
| CustomMetadata_Set | ✅ | ✅ | ✅ | Set x-amz-meta-* headers with content verification |
| CustomMetadata_Get | ✅ | ✅ | ✅ | Retrieve custom metadata (case-insensitive) |
| ObjectTagging_Set | ✅ | ✅ | ✅ | Set object tags with content preservation verified |
| ObjectTagging_Get | ✅ | ✅ | ✅ | Get object tags |

### Advanced Features

| Feature | AWS CLI | boto3 | MinIO mc | Notes |
|---------|---------|-------|----------|-------|
| RangeRequest | ✅ | ✅ | ✅ | Partial content (206) with byte-exact verification |
| PreSignedURL_Download | ✅ | ✅ | ✅ | Generate GET URL with content verification |
| PreSignedURL_Upload | ⏭️ | ⏭️ | ❌ | AWS CLI/boto3 skip (complex setup), mc fails with content mismatch |
| MultipartUpload | ✅ | ✅ | ✅ | Large file uploads with full content integrity verification |

**Legend:** ✅ Pass | ❌ Fail | ⏭️ Skip | ➖ N/A

---

## Known Issues

### Active Bugs

**MinIO mc PreSignedURL_Upload - Content Mismatch**
- **Status:** ❌ FAILING
- **Clients Affected:** MinIO mc only
- **Error:** Content integrity check failed (hash mismatch: expected `ec9eadb8b71af4c664405284ac9323de`, got `f840e1434e8ff5782497a5c5b1b8a922`)
- **Impact:** Pre-signed PUT URLs return different content than what was uploaded
- **Note:** This is a real bug found by the new content integrity validation

### Optional Features (Intentionally Skipped)

- **ListObjectsV1:** Legacy API, modern clients use ListObjectsV2 by default (AWS CLI, mc)
- **PreSignedURL_Upload:** AWS CLI and boto3 skip due to complex test setup requirements
- **ListObjectsV2_MaxKeys:** MinIO mc doesn't expose pagination parameter at CLI level

---

## Test Details

### Content Integrity Validation

All data operations verify content integrity using MD5 hashes:
- ✅ PutObject/GetObject - Round-trip verification
- ✅ CopyObject - Copied content matches original
- ✅ MultipartUpload - Assembled content matches parts
- ✅ PreSignedURL - Downloads via URL match object data
- ✅ Metadata/Tagging - Content not corrupted by metadata operations

### Test Architecture

```
tests/clients/
├── features.yaml              # 23 canonical S3 operations
├── lib/
│   ├── test_framework.sh      # Bash test runner
│   ├── test_framework.py      # Python test runner
│   ├── validators.sh          # Content integrity validators
│   └── validators.py          # Python validators
├── scripts/
│   ├── awscli.sh              # AWS CLI tests (23 operations)
│   ├── boto3.py               # boto3 tests (23 operations)
│   ├── mc.sh                  # MinIO mc tests (23 operations)
│   └── aggregate_results.py   # JSON → Markdown reporter
└── clients_test.go            # Go test orchestration (testcontainers)
```

### Running Tests

```bash
# Run with testcontainers (cross-platform)
go test -v ./tests/clients

# View detailed results
cat tests/clients/results/REPORT.md

# View raw JSON
cat tests/clients/results/awscli.json
cat tests/clients/results/boto3.json
cat tests/clients/results/mc.json
```

---

## Client-Specific Notes

### AWS CLI

- Uses AWS CLI v2 with S3 API v4 signatures
- Tests core `s3api` commands
- Skips ListObjectsV1 (uses V2 by default)
- May skip PreSignedURL_Upload (presign command doesn't easily support PUT)

### boto3 (AWS SDK for Python)

- Python 3.12+ with boto3
- Most comprehensive test coverage
- Supports both ListObjectsV1 and V2
- Full multipart upload support

### MinIO mc

- MinIO client with high-level commands
- Tests `mc` commands: mb, rb, ls, cp, stat, tag, share
- Skips ListObjectsV1 (uses V2 by default)
- Skips MaxKeys (doesn't expose pagination parameter)
- PreSignedURL_Upload uses POST Policy (different S3 feature)

---

## Recent Changes

### February 16, 2026 - Test Framework Refactoring

**Major Changes:**
- ✅ Unified test framework with JSON output
- ✅ All clients test same 23 canonical operations
- ✅ Automated feature matrix generation
- ✅ Standardized content integrity validation
- ✅ Consistent test coverage across all clients

**Benefits:**
- Feature parity visible at a glance
- Automated reporting (no manual log inspection)
- Shared validation logic reduces false positives
- Easy to add new tests consistently

---

## How to Update This Document

After running tests:

1. **Run tests:** `go test -v ./tests/clients`
2. **Check results:** `cat tests/clients/results/REPORT.md`
3. **Update Quick Summary table** with pass/fail counts
4. **Update Feature Matrix** by copying from REPORT.md (replace TBD with ✅/❌/⏭️)
5. **Update Known Issues** section with any new failures
6. **Commit changes** with test evidence

The `aggregate_results.py` script generates the feature matrix automatically, making it easy to keep this document current.
