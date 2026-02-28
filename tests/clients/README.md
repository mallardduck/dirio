# S3 Client Compatibility Tests

This directory contains comprehensive S3 API compatibility tests for DirIO across three major S3 clients.

## Test Structure

### Test Framework
- **Framework:** testcontainers-go with Docker containers
- **Language:** Go test framework (`go test`)
- **Test Files:** `clients_test.go`, `sanity_test.go`
- **Scripts:** Embedded from `scripts/` directory via `go:embed`
- **Libraries:** Shared test framework and validators in `lib/`
- **Output Format:** Structured JSON with dual output (human-readable + machine-parseable)

### Test Clients

Each client runs in an isolated Docker container:

1. **AWS CLI** (`amazon/aws-cli:2.15.0`)
2. **boto3** (`python:3.11-slim`)
3. **MinIO mc** (`minio/mc:latest`)

### Framework Architecture

```
lib/
├── test_framework.sh    # Bash test runner with JSON output
├── test_framework.py    # Python test runner with JSON output
├── validators.sh        # Bash validation functions
└── validators.py        # Python validation functions
```

**Key Features:**
- **Dual Output:** Human-readable progress (stderr) + JSON results (stdout)
- **Standardized Validation:** Shared validators for content integrity, metadata, etc.
- **Consistent Coverage:** All clients test the same 23 canonical S3 operations
- **Automated Aggregation:** JSON results compiled into markdown tables

## Running Tests

```bash
# Run all client tests
go test -v ./tests/clients

# Run specific client test
go test -v -run TestAWSCLI ./tests/clients
go test -v -run TestBoto3 ./tests/clients
go test -v -run TestMinIOMC ./tests/clients

# Run sanity checks
go test -v -run TestSanityCheck ./tests/clients
```

## Test Coverage

All three test suites test the same **23 canonical S3 operations** defined in `features.yaml`:

### Bucket Operations (5)
1. ListBuckets
2. CreateBucket
3. HeadBucket
4. GetBucketLocation
5. DeleteBucket

### Object Operations (5)
6. PutObject
7. GetObject
8. HeadObject
9. DeleteObject
10. CopyObject

### Listing Operations (5)
11. ListObjectsV2_Basic
12. ListObjectsV2_Prefix
13. ListObjectsV2_Delimiter
14. ListObjectsV2_MaxKeys (optional)
15. ListObjectsV1 (optional)

### Metadata Operations (4)
16. CustomMetadata_Set
17. CustomMetadata_Get
18. ObjectTagging_Set
19. ObjectTagging_Get

### Advanced Features (4)
20. RangeRequest
21. PreSignedURL_Download
22. PreSignedURL_Upload (optional)
23. MultipartUpload

**Note:** Optional tests are skipped if the client doesn't support the feature (e.g., AWS CLI v2 uses ListObjectsV2 exclusively)

## Test Validation

All tests include comprehensive validation:

### Content Verification
- **GetObject**: Verify downloaded content matches uploaded content
- **CopyObject**: Verify copied content matches original
- **Pre-signed URLs**: Verify download via URL matches object content
- **Multipart Upload**: Verify assembled content matches concatenated parts
- **Object Tagging**: Verify content NOT corrupted after tagging operations
- **Custom Metadata**: Verify content NOT corrupted when metadata is added

### Structure Verification
- **ListObjectsV2**: Verify expected objects appear in listings
- **ListObjectsV2 (prefix)**: Verify only objects with prefix are returned
- **ListObjectsV2 (delimiter)**: Verify CommonPrefixes field present
- **GetBucketLocation**: Verify LocationConstraint field in response
- **Custom Metadata**: Verify metadata keys returned in HeadObject

### Exit Code Validation
All operations check both:
1. Command exit code (success/failure)
2. Content integrity (where applicable)

## JSON Output Format

Each test script outputs structured JSON to stdout:

```json
{
  "meta": {
    "client": "awscli",
    "version": "aws-cli/2.15.0",
    "test_run_id": "1708089600",
    "duration_ms": 5300
  },
  "results": [
    {
      "feature": "PutObject",
      "category": "object_operations",
      "status": "pass",
      "duration_ms": 450,
      "message": "",
      "details": {
        "validation_type": "content_integrity"
      }
    }
  ],
  "summary": {
    "total": 23,
    "passed": 22,
    "failed": 1,
    "skipped": 0
  }
}
```

**Status values:** `pass`, `fail`, `skip`

## Aggregated Results

The `aggregate_results.py` script compiles JSON from all clients into a markdown report:

```markdown
# Client Test Results

## Summary
| Client | Total | Passed | Failed | Skipped | Duration |
|--------|-------|--------|--------|---------|----------|
| awscli | 23    | 22     | 1      | 0       | 5.30s    |

## Feature Support Matrix
| Feature              | awscli | boto3 | mc  |
|----------------------|--------|-------|-----|
| ListBuckets          | ✅     | ✅    | ✅  |
| PutObject            | ❌     | ✅    | ✅  |

## Failed Tests
### awscli
- **PutObject**: Content mismatch after upload
```

Reports are generated automatically in `results/REPORT.md`

## Test Scripts

### `scripts/awscli.sh`
- **Tests:** 23 (canonical set)
- **Language:** Bash
- **Framework:** test_framework.sh
- **Validation:** Standardized validators (MD5 hashes, metadata checks)

### `scripts/boto3.py`
- **Tests:** 23 (canonical set)
- **Language:** Python
- **Framework:** test_framework.py
- **Validation:** Standardized validators (MD5 hashes, metadata checks)

### `scripts/mc.sh`
- **Tests:** 23 (canonical set)
- **Language:** Bash
- **Framework:** test_framework.sh
- **Validation:** Standardized validators (MD5 hashes, metadata checks)

## Naming Conventions

Test names are standardized across all three clients:

- `"ListBuckets"` - Basic operation
- `"GetBucketLocation"` - Metadata operation
- `"ListObjectsV2 (basic)"` - Operation variant in parentheses
- `"ListObjectsV2 (prefix)"` - Another variant
- `"Custom metadata (set)"` - Multi-step operation split by action
- `"Custom metadata (get)"` - Corresponding retrieval operation
- `"Range request"` - Singular form
- `"Multipart upload"` - Lowercase compound words
- `"Object tagging"` - Combined set+get with content verification
- `"Pre-signed URL"` - Singular form

## Known Issues

All three clients exhibit the same failures, confirming these are DirIO bugs:

1. **Range Requests** - Returns full content instead of requested range
2. **CopyObject** - Creates empty file or fails
3. **Pre-signed URLs** - Returns 403 Forbidden
4. **Multipart Upload** - Returns 405 Method Not Allowed (AWS CLI/boto3 only; mc works)
5. **Object Tagging** - Corrupts object content with XML tags

See [CLIENTS.md](../../docs/CLIENTS.md) for detailed compatibility matrix and bug tracking.

## Sanity Tests

Defensive tests ensure we're not getting false positives:

### `TestSanityCheck_FailingServer`
- Mock server returns HTTP 500 for all requests
- All clients should fail all operations
- Confirms tests detect actual failures

### `TestSanityCheck_DumbSuccessServer`
- Mock server returns HTTP 200 with empty bodies
- All clients should fail due to invalid responses
- Confirms tests validate response content, not just status codes

### Known Limitation (Windows)
**Status:** Sanity tests currently fail on Windows due to Docker Desktop networking + Windows Firewall.

- ✅ **Architecture**: Sanity tests use the exact same test scripts, so all 21 operations ARE validated
- ✅ **Server**: Mock server correctly binds to `0.0.0.0:port`
- ❌ **Issue**: Windows Firewall blocks Docker containers from reaching `host.docker.internal:port`
- ✅ **Workaround**: Use fixed port (18080) instead of random ports, or test on Linux/macOS

See `README_SANITY_TESTS.md` for detailed analysis and solutions.

## Adding New Tests

To add a new S3 operation test:

1. **Add to features.yaml** with name, category, priority, and validation type
2. **Implement in all three scripts** (`awscli.sh`, `boto3.py`, `mc.sh`):
   - Create `test_<feature_name>()` function
   - Use `run_test()` or `runner.register_test()` with exact feature name from YAML
   - Use standardized validators (`validate_content_integrity`, etc.)
3. **Use identical test names** across all clients (must match features.yaml)
4. **Follow framework patterns**:
   - Bash: `run_test "FeatureName" "category" "validation" test_function_name`
   - Python: `runner.register_test("FeatureName", "category", "validation", test_function)`
5. **Update this README** with new operation
6. **Re-run aggregation** to update feature matrix

### Example: Adding DeleteObjectTagging

**1. Add to features.yaml:**
```yaml
metadata_operations:
  - name: DeleteObjectTagging
    priority: optional
    validation: exit_code
    description: Remove all tags from an object
```

**2. Implement in awscli.sh:**
```bash
test_delete_object_tagging() {
    $AWS s3api delete-object-tagging --bucket ${BUCKET} --key test.txt > /dev/null
}

run_test "DeleteObjectTagging" "metadata_operations" "exit_code" test_delete_object_tagging
```

**3. Implement in boto3.py and mc.sh** similarly.

## Test Environment

### Container Setup
- **Network:** host.docker.internal for container-to-host communication
- **Timeouts:** 2-5 minutes per client test
- **Cleanup:** Automatic via testcontainers (removes containers after test)
- **Isolation:** Each test gets fresh bucket with timestamp suffix
- **Libraries:** Framework files written to `/tmp` before test execution

### Environment Variables
Tests receive:
- `DIRIO_ENDPOINT` - DirIO server URL
- `DIRIO_ACCESS_KEY` - Test admin access key
- `DIRIO_SECRET_KEY` - Test admin secret key
- `DIRIO_REGION` - AWS region (us-east-1)

## Architecture

```
tests/clients/
├── README.md                  # This file
├── clients_test.go            # Main test orchestration (embeds + runs in containers)
├── sanity_test.go             # Defensive validation tests
├── features.yaml              # Canonical S3 feature set (23 operations)
├── lib/
│   ├── README.md              # Framework API documentation
│   ├── test_framework.sh      # Bash test runner
│   ├── test_framework.py      # Python test runner
│   ├── validators.sh          # Bash validators
│   └── validators.py          # Python validators
└── scripts/
    ├── awscli.sh              # AWS CLI test script
    ├── boto3.py               # boto3 test script
    ├── mc.sh                  # MinIO mc test script
    └── aggregate_results.py   # JSON → Markdown aggregator
```

## Validation Philosophy

**Exit codes alone are insufficient.** Every read/write operation must verify content integrity.

Example: Object tagging operations return 200 OK but corrupt the object by replacing its content with XML tags. Without content verification, this would appear as a passing test.

Our tests use three-tier validation:
1. **Exit code** - Operation succeeded
2. **Content** - Data integrity preserved
3. **Structure** - Response contains expected fields

This defensive approach has successfully identified multiple critical bugs that would have been missed with status-code-only validation.
