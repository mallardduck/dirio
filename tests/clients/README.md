# S3 Client Compatibility Tests

This directory contains comprehensive S3 API compatibility tests for DirIO across three major S3 clients.

## Test Structure

### Test Framework
- **Framework:** testcontainers-go with Docker containers
- **Language:** Go test framework (`go test`)
- **Test Files:** `clients_test.go`, `sanity_test.go`
- **Scripts:** Embedded from `scripts/` directory via `go:embed`

### Test Clients

Each client runs in an isolated Docker container:

1. **AWS CLI** (`amazon/aws-cli:2.15.0`)
2. **boto3** (`python:3.11-slim`)
3. **MinIO mc** (`minio/mc:latest`)

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

All three test suites follow the same structure and test the same S3 operations:

### Core Operations (Tested by all clients)
1. Network Probe
2. ListBuckets
3. CreateBucket
4. HeadBucket
5. GetBucketLocation
6. PutObject
7. Custom Metadata (set)
8. Custom Metadata (get)
9. HeadObject
10. GetObject
11. Range Requests
12. ListObjectsV2 (basic)
13. ListObjectsV2 (prefix)
14. ListObjectsV2 (delimiter)
15. CopyObject
16. Pre-signed URLs
17. Multipart Upload
18. Object Tagging
19. DeleteObject
20. DeleteBucket

### Client-Specific Operations
- **boto3**: ListObjectsV2 (max-keys), ListObjectsV1
- **AWS CLI**: High-level `s3 cp` commands
- **MinIO mc**: Multiple command variants (mc put/cp, mc stat variants, mc ls -r)

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

## Test Scripts

### `scripts/awscli.sh`
- **Lines:** 259
- **Tests:** 21
- **Pass Rate:** 76.2% (16/21)
- **Language:** Bash
- **Validation:** Exit codes + content verification with `diff`

### `scripts/boto3.py`
- **Lines:** 361
- **Tests:** 21
- **Pass Rate:** 71.4% (15/21)
- **Language:** Python
- **Validation:** Assertions + content checks with byte comparison

### `scripts/mc.sh`
- **Lines:** 340
- **Tests:** 30 (includes command variants)
- **Pass Rate:** 73.3% (22/30)
- **Language:** Bash
- **Validation:** Exit codes + content verification with `grep`/`diff`/`cmp`

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

See [CLIENTS.md](../../CLIENTS.md) for detailed compatibility matrix and bug tracking.

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

1. **Add to all three scripts** (`awscli.sh`, `boto3.py`, `mc.sh`)
2. **Use identical test names** across all clients
3. **Include content verification** where applicable
4. **Follow existing patterns**:
   - Exit code check first
   - Content validation second
   - Descriptive failure messages
5. **Update this README** with new operation
6. **Update CLIENTS.md** compatibility matrix

## Test Environment

### Container Setup
- **Network:** host.docker.internal for container-to-host communication
- **Timeouts:** 2-5 minutes per client test
- **Cleanup:** Automatic via testcontainers (removes containers after test)
- **Isolation:** Each test gets fresh bucket with timestamp suffix

### Environment Variables
Tests receive:
- `DIRIO_ENDPOINT` - DirIO server URL
- `DIRIO_ACCESS_KEY` - Test admin access key
- `DIRIO_SECRET_KEY` - Test admin secret key
- `DIRIO_REGION` - AWS region (us-east-1)

## Architecture

```
tests/clients/
├── README.md              # This file
├── clients_test.go        # Main test orchestration
├── sanity_test.go         # Defensive validation tests
└── scripts/
    ├── awscli.sh         # AWS CLI test script
    ├── boto3.py          # boto3 test script
    └── mc.sh             # MinIO mc test script
```

## Validation Philosophy

**Exit codes alone are insufficient.** Every read/write operation must verify content integrity.

Example: Object tagging operations return 200 OK but corrupt the object by replacing its content with XML tags. Without content verification, this would appear as a passing test.

Our tests use three-tier validation:
1. **Exit code** - Operation succeeded
2. **Content** - Data integrity preserved
3. **Structure** - Response contains expected fields

This defensive approach has successfully identified multiple critical bugs that would have been missed with status-code-only validation.
