#!/bin/bash
# AWS CLI compatibility tests for DirIO
#
# Prerequisites:
#   - AWS CLI v2 installed (brew install awscli)
#   - DirIO server running (or use run_tests.sh)
#
# Usage:
#   ./test_awscli.sh

# Don't use set -e as we expect some tests to fail for compatibility testing

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/config.sh"

print_header "AWS CLI Compatibility Tests"

# Check prerequisites
if ! check_command aws; then
    echo "AWS CLI not found. Install with: brew install awscli"
    exit 1
fi

echo "AWS CLI version: $(aws --version)"
echo "Endpoint: ${DIRIO_ENDPOINT}"
echo "Test bucket: ${TEST_BUCKET}"

# Configure AWS CLI for this test
export AWS_ACCESS_KEY_ID="${DIRIO_ACCESS_KEY}"
export AWS_SECRET_ACCESS_KEY="${DIRIO_SECRET_KEY}"
export AWS_DEFAULT_REGION="${DIRIO_REGION}"

# Common AWS CLI options
AWS_OPTS="--endpoint-url ${DIRIO_ENDPOINT}"

# ============================================================================
# Bucket Operations
# ============================================================================

print_test "ListBuckets (empty)"
if aws s3api list-buckets ${AWS_OPTS} 2>&1; then
    print_pass "ListBuckets (empty)"
else
    print_fail "ListBuckets (empty)" "$?"
fi

print_test "CreateBucket"
if aws s3api create-bucket --bucket "${TEST_BUCKET}" ${AWS_OPTS} 2>&1; then
    print_pass "CreateBucket"
else
    print_fail "CreateBucket" "$?"
fi

print_test "ListBuckets (with bucket)"
OUTPUT=$(aws s3api list-buckets ${AWS_OPTS} 2>&1)
if echo "$OUTPUT" | grep -q "${TEST_BUCKET}"; then
    print_pass "ListBuckets (with bucket)"
else
    print_fail "ListBuckets (with bucket)" "Bucket not found in list"
fi

print_test "HeadBucket"
if aws s3api head-bucket --bucket "${TEST_BUCKET}" ${AWS_OPTS} 2>&1; then
    print_pass "HeadBucket"
else
    print_fail "HeadBucket" "$?"
fi

# ============================================================================
# Object Operations
# ============================================================================

print_test "PutObject (simple)"
echo "${TEST_OBJECT_CONTENT}" > /tmp/dirio-test-upload.txt
if aws s3api put-object --bucket "${TEST_BUCKET}" --key "${TEST_OBJECT_KEY}" --body /tmp/dirio-test-upload.txt ${AWS_OPTS} 2>&1; then
    print_pass "PutObject (simple)"
else
    print_fail "PutObject (simple)" "$?"
fi

print_test "HeadObject"
if aws s3api head-object --bucket "${TEST_BUCKET}" --key "${TEST_OBJECT_KEY}" ${AWS_OPTS} 2>&1; then
    print_pass "HeadObject"
else
    print_fail "HeadObject" "$?"
fi

print_test "GetObject"
rm -f /tmp/dirio-test-download.txt
if aws s3api get-object --bucket "${TEST_BUCKET}" --key "${TEST_OBJECT_KEY}" /tmp/dirio-test-download.txt ${AWS_OPTS} 2>&1; then
    DOWNLOADED=$(cat /tmp/dirio-test-download.txt)
    if [ "${DOWNLOADED}" = "${TEST_OBJECT_CONTENT}" ]; then
        print_pass "GetObject"
    else
        print_fail "GetObject" "Content mismatch"
    fi
else
    print_fail "GetObject" "$?"
fi

print_test "PutObject (with custom metadata)"
if aws s3api put-object --bucket "${TEST_BUCKET}" --key "metadata-test.txt" --body /tmp/dirio-test-upload.txt \
    --metadata "custom-key=custom-value,another-key=another-value" \
    --content-type "text/plain" \
    --cache-control "max-age=3600" ${AWS_OPTS} 2>&1; then
    print_pass "PutObject (with custom metadata)"
else
    print_fail "PutObject (with custom metadata)" "$?"
fi

print_test "HeadObject (verify custom metadata)"
OUTPUT=$(aws s3api head-object --bucket "${TEST_BUCKET}" --key "metadata-test.txt" ${AWS_OPTS} 2>&1)
if echo "$OUTPUT" | grep -qi "custom-key"; then
    print_pass "HeadObject (verify custom metadata)"
else
    print_fail "HeadObject (verify custom metadata)" "Metadata not returned"
    echo "$OUTPUT"
fi

# ============================================================================
# ListObjects Operations
# ============================================================================

# Create more objects for list tests
for i in 1 2 3; do
    aws s3api put-object --bucket "${TEST_BUCKET}" --key "folder/file${i}.txt" --body /tmp/dirio-test-upload.txt ${AWS_OPTS} > /dev/null 2>&1
done

print_test "ListObjectsV2 (basic)"
if aws s3api list-objects-v2 --bucket "${TEST_BUCKET}" ${AWS_OPTS} 2>&1; then
    print_pass "ListObjectsV2 (basic)"
else
    print_fail "ListObjectsV2 (basic)" "$?"
fi

print_test "ListObjectsV2 (with prefix)"
OUTPUT=$(aws s3api list-objects-v2 --bucket "${TEST_BUCKET}" --prefix "folder/" ${AWS_OPTS} 2>&1)
if echo "$OUTPUT" | grep -q "file1.txt"; then
    print_pass "ListObjectsV2 (with prefix)"
else
    print_fail "ListObjectsV2 (with prefix)" "Files not found with prefix"
fi

print_test "ListObjectsV2 (with delimiter)"
OUTPUT=$(aws s3api list-objects-v2 --bucket "${TEST_BUCKET}" --delimiter "/" ${AWS_OPTS} 2>&1)
if echo "$OUTPUT" | grep -q "CommonPrefixes"; then
    print_pass "ListObjectsV2 (with delimiter)"
else
    print_fail "ListObjectsV2 (with delimiter)" "CommonPrefixes not found"
    echo "$OUTPUT"
fi

print_test "ListObjectsV2 (with max-keys)"
OUTPUT=$(aws s3api list-objects-v2 --bucket "${TEST_BUCKET}" --max-keys 2 ${AWS_OPTS} 2>&1)
if echo "$OUTPUT" | grep -q "KeyCount"; then
    print_pass "ListObjectsV2 (with max-keys)"
else
    print_fail "ListObjectsV2 (with max-keys)" "$?"
fi

print_test "ListObjectsV1"
if aws s3api list-objects --bucket "${TEST_BUCKET}" ${AWS_OPTS} 2>&1; then
    print_pass "ListObjectsV1"
else
    print_fail "ListObjectsV1" "$?"
fi

# ============================================================================
# S3 High-Level Commands (uses s3:// syntax)
# ============================================================================

print_test "s3 ls (list buckets)"
if aws s3 ls ${AWS_OPTS} 2>&1 | grep -q "${TEST_BUCKET}"; then
    print_pass "s3 ls (list buckets)"
else
    print_fail "s3 ls (list buckets)" "$?"
fi

print_test "s3 ls (list objects in bucket)"
if aws s3 ls "s3://${TEST_BUCKET}/" ${AWS_OPTS} 2>&1; then
    print_pass "s3 ls (list objects in bucket)"
else
    print_fail "s3 ls (list objects in bucket)" "$?"
fi

print_test "s3 cp (upload)"
echo "High level upload test" > /tmp/dirio-test-hl-upload.txt
if aws s3 cp /tmp/dirio-test-hl-upload.txt "s3://${TEST_BUCKET}/hl-test.txt" ${AWS_OPTS} 2>&1; then
    print_pass "s3 cp (upload)"
else
    print_fail "s3 cp (upload)" "$?"
fi

print_test "s3 cp (download)"
rm -f /tmp/dirio-test-hl-download.txt
if aws s3 cp "s3://${TEST_BUCKET}/hl-test.txt" /tmp/dirio-test-hl-download.txt ${AWS_OPTS} 2>&1; then
    print_pass "s3 cp (download)"
else
    print_fail "s3 cp (download)" "$?"
fi

# ============================================================================
# Range Requests (if supported)
# ============================================================================

print_test "GetObject (range request)"
# Create a larger file for range testing
dd if=/dev/zero bs=1024 count=100 2>/dev/null | tr '\0' 'A' > /tmp/dirio-test-large.txt
aws s3api put-object --bucket "${TEST_BUCKET}" --key "large-file.txt" --body /tmp/dirio-test-large.txt ${AWS_OPTS} > /dev/null 2>&1
if aws s3api get-object --bucket "${TEST_BUCKET}" --key "large-file.txt" --range "bytes=0-1023" /tmp/dirio-test-range.txt ${AWS_OPTS} 2>&1; then
    SIZE=$(wc -c < /tmp/dirio-test-range.txt | tr -d ' ')
    if [ "$SIZE" = "1024" ]; then
        print_pass "GetObject (range request)"
    else
        print_fail "GetObject (range request)" "Expected 1024 bytes, got ${SIZE}"
    fi
else
    print_fail "GetObject (range request)" "$?"
fi

# ============================================================================
# Copy Object (if supported)
# ============================================================================

print_test "CopyObject"
if aws s3api copy-object --bucket "${TEST_BUCKET}" --key "copied-object.txt" --copy-source "${TEST_BUCKET}/${TEST_OBJECT_KEY}" ${AWS_OPTS} 2>&1; then
    print_pass "CopyObject"
else
    print_fail "CopyObject" "$?"
fi

# ============================================================================
# Pre-signed URLs (if supported)
# ============================================================================

print_test "Presign URL (GET)"
PRESIGNED=$(aws s3 presign "s3://${TEST_BUCKET}/${TEST_OBJECT_KEY}" --expires-in 300 ${AWS_OPTS} 2>&1)
if [ $? -eq 0 ] && echo "$PRESIGNED" | grep -q "http"; then
    # Try to fetch the presigned URL
    if curl -sf "$PRESIGNED" > /dev/null 2>&1; then
        print_pass "Presign URL (GET)"
    else
        print_fail "Presign URL (GET)" "URL generated but fetch failed"
    fi
else
    print_fail "Presign URL (GET)" "$PRESIGNED"
fi

# ============================================================================
# Delete Operations (cleanup)
# ============================================================================

print_test "DeleteObject"
if aws s3api delete-object --bucket "${TEST_BUCKET}" --key "${TEST_OBJECT_KEY}" ${AWS_OPTS} 2>&1; then
    print_pass "DeleteObject"
else
    print_fail "DeleteObject" "$?"
fi

print_test "DeleteObject (verify deletion)"
if ! aws s3api head-object --bucket "${TEST_BUCKET}" --key "${TEST_OBJECT_KEY}" ${AWS_OPTS} 2>&1; then
    print_pass "DeleteObject (verify deletion)"
else
    print_fail "DeleteObject (verify deletion)" "Object still exists"
fi

# Clean up remaining objects before bucket deletion
aws s3 rm "s3://${TEST_BUCKET}" --recursive ${AWS_OPTS} > /dev/null 2>&1 || true

print_test "DeleteBucket"
if aws s3api delete-bucket --bucket "${TEST_BUCKET}" ${AWS_OPTS} 2>&1; then
    print_pass "DeleteBucket"
else
    print_fail "DeleteBucket" "$?"
fi

# ============================================================================
# Cleanup temp files
# ============================================================================
rm -f /tmp/dirio-test-*.txt

print_summary