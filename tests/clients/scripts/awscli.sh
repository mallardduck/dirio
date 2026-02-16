#!/bin/bash
# AWS CLI S3 Integration Tests
# Tests DirIO server compatibility with AWS CLI

set -euo pipefail

# Get the script directory (works in containers where this is passed inline)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source test framework - handle both container and local execution
if [ -f "$SCRIPT_DIR/../lib/test_framework.sh" ]; then
    source "$SCRIPT_DIR/../lib/test_framework.sh"
    source "$SCRIPT_DIR/../lib/validators.sh"
elif [ -f "/tmp/test_framework.sh" ]; then
    source /tmp/test_framework.sh
    source /tmp/validators.sh
else
    echo "ERROR: Cannot find test_framework.sh" >&2
    exit 1
fi

# Initialize test runner
AWS_VERSION=$(aws --version 2>&1 | head -n1)
init_test_runner "awscli" "$AWS_VERSION"

# Test configuration
BUCKET="awscli-test-bucket-$(date +%s)"
ENDPOINT="${DIRIO_ENDPOINT}"
AWS="aws --endpoint-url ${ENDPOINT}"

echo "=== AWS CLI Tests ===" >&2
echo "Endpoint: ${ENDPOINT}" >&2
echo "AWS CLI: ${AWS_VERSION}" >&2

# Network probe
echo "--- Network Probe ---" >&2
PROBE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -m 5 "${ENDPOINT}/healthz" || echo "000")
if [ "${PROBE_CODE}" = "000" ]; then
    echo "FATAL: Cannot reach server at ${ENDPOINT}" >&2
    exit 1
fi
echo "GET /healthz -> HTTP ${PROBE_CODE}" >&2

#------------------------------------------------------------------------------
# Test Functions
#------------------------------------------------------------------------------

test_list_buckets() {
    $AWS s3api list-buckets > /dev/null
}

test_create_bucket() {
    $AWS s3api create-bucket --bucket ${BUCKET} > /dev/null
}

test_head_bucket() {
    $AWS s3api head-bucket --bucket ${BUCKET}
}

test_get_bucket_location() {
    $AWS s3api get-bucket-location --bucket ${BUCKET} --output json > /tmp/location.json
    grep -q "LocationConstraint" /tmp/location.json
}

test_put_object() {
    echo "test content" > /tmp/test.txt
    $AWS s3api put-object --bucket ${BUCKET} --key test.txt --body /tmp/test.txt > /dev/null
}

test_head_object() {
    $AWS s3api head-object --bucket ${BUCKET} --key test.txt > /dev/null
}

test_get_object() {
    $AWS s3api get-object --bucket ${BUCKET} --key test.txt /tmp/download.txt > /dev/null
    validate_content_integrity /tmp/test.txt /tmp/download.txt
}

test_delete_object() {
    $AWS s3api delete-object --bucket ${BUCKET} --key test.txt > /dev/null
    # Re-create for subsequent tests
    $AWS s3api put-object --bucket ${BUCKET} --key test.txt --body /tmp/test.txt > /dev/null
}

test_copy_object() {
    $AWS s3api copy-object --copy-source ${BUCKET}/test.txt --bucket ${BUCKET} --key copied.txt > /dev/null
    $AWS s3api get-object --bucket ${BUCKET} --key copied.txt /tmp/copied.txt > /dev/null
    validate_content_integrity /tmp/test.txt /tmp/copied.txt
}

test_list_objects_v2_basic() {
    $AWS s3api list-objects-v2 --bucket ${BUCKET} > /tmp/list-basic.txt
    grep -q "test.txt" /tmp/list-basic.txt
}

test_list_objects_v2_prefix() {
    # Setup folder structure
    echo "folder1 file1" > /tmp/f1-file1.txt
    echo "folder1 file2" > /tmp/f1-file2.txt
    $AWS s3api put-object --bucket ${BUCKET} --key folder1/file1.txt --body /tmp/f1-file1.txt > /dev/null
    $AWS s3api put-object --bucket ${BUCKET} --key folder1/file2.txt --body /tmp/f1-file2.txt > /dev/null

    # Test prefix filtering
    $AWS s3api list-objects-v2 --bucket ${BUCKET} --prefix folder1/ > /tmp/list-prefix.txt
    grep -q "folder1/file1.txt" /tmp/list-prefix.txt
    grep -q "folder1/file2.txt" /tmp/list-prefix.txt
}

test_list_objects_v2_delimiter() {
    $AWS s3api list-objects-v2 --bucket ${BUCKET} --delimiter / > /tmp/list-delim.txt
    grep -q "CommonPrefixes" /tmp/list-delim.txt
}

test_list_objects_v2_maxkeys() {
    # Test pagination with max-keys
    $AWS s3api list-objects-v2 --bucket ${BUCKET} --max-items 2 > /tmp/list-maxkeys.txt
    # Should have NextToken or limited results
    if ! grep -q "NextToken" /tmp/list-maxkeys.txt; then
        # Alternative: count number of keys returned (should be <= 2)
        KEY_COUNT=$(grep -c "\"Key\":" /tmp/list-maxkeys.txt || echo 0)
        if [ "$KEY_COUNT" -gt 2 ]; then
            fail_test "MaxKeys not respected: found $KEY_COUNT keys"
        fi
    fi
}

test_list_objects_v1() {
    # AWS CLI v2 defaults to ListObjectsV2, skip V1 test
    skip_test "AWS CLI v2 uses ListObjectsV2 by default"
}

test_custom_metadata_set() {
    echo "metadata test" > /tmp/meta-test.txt
    $AWS s3api put-object --bucket ${BUCKET} --key meta-test.txt --body /tmp/meta-test.txt \
        --metadata custom-key=custom-value > /dev/null
    # Verify content integrity after metadata set
    $AWS s3api get-object --bucket ${BUCKET} --key meta-test.txt /tmp/meta-download.txt > /dev/null
    validate_content_integrity /tmp/meta-test.txt /tmp/meta-download.txt
}

test_custom_metadata_get() {
    $AWS s3api head-object --bucket ${BUCKET} --key meta-test.txt > /tmp/meta-head.txt
    if ! grep -qi "custom-key" /tmp/meta-head.txt; then
        fail_test "Custom metadata key not found in HeadObject response"
    fi
    if ! grep -qi "custom-value" /tmp/meta-head.txt; then
        fail_test "Custom metadata value not found in HeadObject response"
    fi
}

test_object_tagging_set() {
    # Get hash before tagging
    $AWS s3api get-object --bucket ${BUCKET} --key test.txt /tmp/test-before-tag.txt > /dev/null
    HASH_BEFORE=$(compute_hash /tmp/test-before-tag.txt)

    # Put tags
    $AWS s3api put-object-tagging --bucket ${BUCKET} --key test.txt \
        --tagging 'TagSet=[{Key=env,Value=test}]' > /dev/null

    # Verify content not corrupted
    $AWS s3api get-object --bucket ${BUCKET} --key test.txt /tmp/test-after-tag.txt > /dev/null
    HASH_AFTER=$(compute_hash /tmp/test-after-tag.txt)

    if [ "$HASH_BEFORE" != "$HASH_AFTER" ]; then
        fail_test "CRITICAL: object content corrupted after tagging"
    fi
}

test_object_tagging_get() {
    $AWS s3api get-object-tagging --bucket ${BUCKET} --key test.txt > /tmp/tags.txt
    if ! grep -q "env" /tmp/tags.txt || ! grep -q "test" /tmp/tags.txt; then
        fail_test "Tags not returned or incorrect"
    fi
}

test_range_request() {
    # Create 100-byte file
    printf "%0100d" 0 > /tmp/range-source.txt
    $AWS s3api put-object --bucket ${BUCKET} --key range.txt --body /tmp/range-source.txt > /dev/null

    # Request first 10 bytes
    $AWS s3api get-object --bucket ${BUCKET} --key range.txt --range bytes=0-9 /tmp/range-partial.txt > /dev/null
    validate_partial_content /tmp/range-partial.txt 10
}

test_presigned_url_download() {
    PRESIGNED_URL=$($AWS s3 presign s3://${BUCKET}/test.txt --expires-in 300 2>&1)
    curl -s -f -o /tmp/presigned-download.txt "${PRESIGNED_URL}"
    validate_content_integrity /tmp/test.txt /tmp/presigned-download.txt
}

test_presigned_url_upload() {
    # AWS CLI presign doesn't easily support upload URLs
    skip_test "AWS CLI presign does not support upload URLs easily"
}

test_multipart_upload() {
    echo "part1 content" > /tmp/part1.txt
    echo "part2 content" > /tmp/part2.txt

    # Initiate
    CREATE_RESP=$($AWS s3api create-multipart-upload --bucket ${BUCKET} --key multipart.txt --output json 2>&1)

    # Extract UploadId
    if command -v jq >/dev/null 2>&1; then
        UPLOAD_ID=$(echo "$CREATE_RESP" | jq -r '.UploadId')
    else
        UPLOAD_ID=$(echo "$CREATE_RESP" | grep -o '"UploadId"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)".*/\1/')
    fi

    if [ -z "$UPLOAD_ID" ]; then
        fail_test "Could not parse UploadId"
    fi

    # Upload parts
    PART1_RESP=$($AWS s3api upload-part --bucket ${BUCKET} --key multipart.txt \
        --upload-id "$UPLOAD_ID" --part-number 1 --body /tmp/part1.txt 2>&1)
    ETAG1=$(echo "$PART1_RESP" | grep -i '"ETag":' | sed -E 's/.*"ETag"[[:space:]]*:[[:space:]]*"\\+"([^\\]+)\\+".*/\1/' | tr -d '\n\r')

    PART2_RESP=$($AWS s3api upload-part --bucket ${BUCKET} --key multipart.txt \
        --upload-id "$UPLOAD_ID" --part-number 2 --body /tmp/part2.txt 2>&1)
    ETAG2=$(echo "$PART2_RESP" | grep -i '"ETag":' | sed -E 's/.*"ETag"[[:space:]]*:[[:space:]]*"\\+"([^\\]+)\\+".*/\1/' | tr -d '\n\r')

    # Complete
    COMPLETE_JSON="{\"Parts\":[{\"PartNumber\":1,\"ETag\":\"\\\"$ETAG1\\\"\"},{\"PartNumber\":2,\"ETag\":\"\\\"$ETAG2\\\"\"}]}"
    $AWS s3api complete-multipart-upload --bucket ${BUCKET} --key multipart.txt \
        --upload-id "$UPLOAD_ID" --multipart-upload "$COMPLETE_JSON" > /dev/null

    # Verify
    $AWS s3api get-object --bucket ${BUCKET} --key multipart.txt /tmp/mp-downloaded.txt > /dev/null
    cat /tmp/part1.txt /tmp/part2.txt > /tmp/mp-expected.txt
    validate_content_integrity /tmp/mp-expected.txt /tmp/mp-downloaded.txt
}

test_delete_bucket() {
    # Cleanup all objects first
    $AWS s3 rm s3://${BUCKET} --recursive 2>/dev/null || true
    $AWS s3api delete-bucket --bucket ${BUCKET}
}

#------------------------------------------------------------------------------
# Run All Tests
#------------------------------------------------------------------------------

run_test "ListBuckets" "bucket_operations" "exit_code" test_list_buckets
run_test "CreateBucket" "bucket_operations" "exit_code" test_create_bucket
run_test "HeadBucket" "bucket_operations" "exit_code" test_head_bucket
run_test "GetBucketLocation" "bucket_operations" "exit_code" test_get_bucket_location
run_test "PutObject" "object_operations" "content_integrity" test_put_object
run_test "HeadObject" "object_operations" "exit_code" test_head_object
run_test "GetObject" "object_operations" "content_integrity" test_get_object
run_test "DeleteObject" "object_operations" "exit_code" test_delete_object
run_test "CopyObject" "object_operations" "content_integrity" test_copy_object
run_test "ListObjectsV2_Basic" "listing_operations" "exit_code" test_list_objects_v2_basic
run_test "ListObjectsV2_Prefix" "listing_operations" "exit_code" test_list_objects_v2_prefix
run_test "ListObjectsV2_Delimiter" "listing_operations" "exit_code" test_list_objects_v2_delimiter
run_test "ListObjectsV2_MaxKeys" "listing_operations" "exit_code" test_list_objects_v2_maxkeys
run_test "ListObjectsV1" "listing_operations" "exit_code" test_list_objects_v1
run_test "CustomMetadata_Set" "metadata_operations" "content_integrity" test_custom_metadata_set
run_test "CustomMetadata_Get" "metadata_operations" "metadata" test_custom_metadata_get
run_test "ObjectTagging_Set" "metadata_operations" "content_integrity" test_object_tagging_set
run_test "ObjectTagging_Get" "metadata_operations" "exit_code" test_object_tagging_get
run_test "RangeRequest" "advanced_features" "partial_content" test_range_request
run_test "PreSignedURL_Download" "advanced_features" "content_integrity" test_presigned_url_download
run_test "PreSignedURL_Upload" "advanced_features" "content_integrity" test_presigned_url_upload
run_test "MultipartUpload" "advanced_features" "content_integrity" test_multipart_upload
run_test "DeleteBucket" "bucket_operations" "exit_code" test_delete_bucket

# Output JSON results
finalize_test_runner

# Exit with appropriate code
if [ $TEST_FAILED -gt 0 ]; then
    exit 1
else
    exit 0
fi
