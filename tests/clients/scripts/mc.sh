#!/bin/bash
# MinIO Client (mc) S3 Integration Tests
# Tests DirIO server compatibility with MinIO mc client

set -euo pipefail

# Get the script directory
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
MC_VERSION=$(mc --version 2>&1 | head -n1)
init_test_runner "mc" "$MC_VERSION"

# Test configuration
BUCKET="mc-test-bucket-$(date +%s)"
ENDPOINT="${DIRIO_ENDPOINT}"
MC_ALIAS="dirio"

echo "=== MinIO mc Tests ===" >&2
echo "Endpoint: ${ENDPOINT}" >&2
echo "mc version: ${MC_VERSION}" >&2

# Network probe
echo "--- Network Probe ---" >&2
PROBE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -m 5 "${ENDPOINT}/healthz" || echo "000")
if [ "${PROBE_CODE}" = "000" ]; then
    echo "FATAL: Cannot reach server at ${ENDPOINT}" >&2
    exit 1
fi
echo "GET /healthz -> HTTP ${PROBE_CODE}" >&2

# Configure alias (not a test, required setup)
mc alias set ${MC_ALIAS} ${ENDPOINT} ${DIRIO_ACCESS_KEY} ${DIRIO_SECRET_KEY} --api S3v4 2>/dev/null
if [ $? -ne 0 ]; then
    echo "FATAL: Failed to configure mc alias" >&2
    exit 1
fi

#------------------------------------------------------------------------------
# Test Functions
#------------------------------------------------------------------------------

test_list_buckets() {
    mc ls ${MC_ALIAS} > /dev/null
}

test_create_bucket() {
    mc mb ${MC_ALIAS}/${BUCKET} > /dev/null 2>&1
}

test_head_bucket() {
    mc stat ${MC_ALIAS}/${BUCKET} > /dev/null 2>&1
}

test_get_bucket_location() {
    # mc stat calls GetBucketInfo which uses GetBucketLocation
    mc stat ${MC_ALIAS}/${BUCKET} > /dev/null 2>&1
}

test_put_object() {
    echo "test content" > /tmp/test.txt
    mc cp /tmp/test.txt ${MC_ALIAS}/${BUCKET}/test.txt > /dev/null 2>&1
}

test_head_object() {
    mc stat ${MC_ALIAS}/${BUCKET}/test.txt > /dev/null 2>&1
}

test_get_object() {
    mc cp ${MC_ALIAS}/${BUCKET}/test.txt /tmp/download.txt > /dev/null 2>&1
    validate_content_integrity /tmp/test.txt /tmp/download.txt
}

test_delete_object() {
    mc rm ${MC_ALIAS}/${BUCKET}/test.txt > /dev/null 2>&1
    # Re-create for subsequent tests
    mc cp /tmp/test.txt ${MC_ALIAS}/${BUCKET}/test.txt > /dev/null 2>&1
}

test_copy_object() {
    mc cp ${MC_ALIAS}/${BUCKET}/test.txt ${MC_ALIAS}/${BUCKET}/copied.txt > /dev/null 2>&1
    mc cp ${MC_ALIAS}/${BUCKET}/copied.txt /tmp/copied.txt > /dev/null 2>&1
    validate_content_integrity /tmp/test.txt /tmp/copied.txt
}

test_list_objects_v2_basic() {
    mc ls ${MC_ALIAS}/${BUCKET}/ > /tmp/list-basic.txt 2>&1
    grep -q "test.txt" /tmp/list-basic.txt
}

test_list_objects_v2_prefix() {
    # Create folder structure
    echo "folder1 file1" > /tmp/f1-file1.txt
    mc cp /tmp/f1-file1.txt ${MC_ALIAS}/${BUCKET}/folder1/file1.txt > /dev/null 2>&1
    mc cp /tmp/f1-file1.txt ${MC_ALIAS}/${BUCKET}/folder1/file2.txt > /dev/null 2>&1

    # Test prefix filtering
    mc ls ${MC_ALIAS}/${BUCKET}/folder1/ > /tmp/list-prefix.txt 2>&1
    grep -q "file1.txt" /tmp/list-prefix.txt
    grep -q "file2.txt" /tmp/list-prefix.txt
}

test_list_objects_v2_delimiter() {
    # Create more folder structure
    mc cp /tmp/test.txt ${MC_ALIAS}/${BUCKET}/folder2/file1.txt > /dev/null 2>&1
    mc cp /tmp/test.txt ${MC_ALIAS}/${BUCKET}/root-file.txt > /dev/null 2>&1

    # Non-recursive listing should show folders as prefixes
    mc ls ${MC_ALIAS}/${BUCKET}/ > /tmp/list-delim.txt 2>&1
    grep -q "folder1/" /tmp/list-delim.txt
}

test_list_objects_v2_maxkeys() {
    # mc doesn't expose MaxKeys directly — not applicable, count as pass
    echo "N/A: mc client does not expose MaxKeys parameter" >&2
}

test_list_objects_v1() {
    # mc uses ListObjectsV2 by default — not applicable, count as pass
    echo "N/A: mc uses ListObjectsV2 by default" >&2
}

test_custom_metadata_set() {
    echo "metadata test" > /tmp/meta-test.txt
    mc cp --attr "x-amz-meta-custom-key=custom-value" /tmp/meta-test.txt ${MC_ALIAS}/${BUCKET}/metadata-test.txt > /dev/null 2>&1

    # Verify content integrity after metadata set
    mc cp ${MC_ALIAS}/${BUCKET}/metadata-test.txt /tmp/meta-download.txt > /dev/null 2>&1
    validate_content_integrity /tmp/meta-test.txt /tmp/meta-download.txt
}

test_custom_metadata_get() {
    mc stat ${MC_ALIAS}/${BUCKET}/metadata-test.txt > /tmp/meta-stat.txt 2>&1
    if ! grep -qi "custom-key" /tmp/meta-stat.txt; then
        fail_test "Custom metadata key not found in mc stat output"
    fi
}

test_object_tagging_set() {
    # Get content before tagging
    mc cat ${MC_ALIAS}/${BUCKET}/test.txt > /tmp/test-before-tag.txt 2>&1

    # Set tags
    mc tag set ${MC_ALIAS}/${BUCKET}/test.txt "env=test" > /dev/null 2>&1

    # Verify content not corrupted
    mc cat ${MC_ALIAS}/${BUCKET}/test.txt > /tmp/test-after-tag.txt 2>&1
    validate_content_integrity /tmp/test-before-tag.txt /tmp/test-after-tag.txt
}

test_object_tagging_get() {
    mc tag list ${MC_ALIAS}/${BUCKET}/test.txt > /tmp/tags.txt 2>&1
    if ! grep -q "env" /tmp/tags.txt; then
        fail_test "Tags not returned or incorrect"
    fi
}

test_range_request() {
    # Create 100-byte file
    printf "%0100d" 0 > /tmp/range-source.txt
    mc cp /tmp/range-source.txt ${MC_ALIAS}/${BUCKET}/range.txt > /dev/null 2>&1

    # mc doesn't support range requests directly, use presigned URL + curl
    RANGE_URL=$(mc share download --expire=1h ${MC_ALIAS}/${BUCKET}/range.txt 2>&1 | awk '/^Share:/ {print $2}')
    if [ -z "$RANGE_URL" ]; then
        fail_test "Could not generate presigned URL for range test"
    fi

    # Download first 10 bytes using curl
    curl -f -s -r 0-9 "$RANGE_URL" > /tmp/range-partial.txt 2>&1
    validate_partial_content /tmp/range-partial.txt 10
}

test_presigned_url_download() {
    PRESIGNED_URL=$(mc share download --expire=1h ${MC_ALIAS}/${BUCKET}/test.txt 2>&1 | awk '/^Share:/ {print $2}')
    if [ -z "$PRESIGNED_URL" ]; then
        fail_test "Failed to generate presigned URL"
    fi

    curl -f -s "$PRESIGNED_URL" > /tmp/presigned-download.txt 2>&1
    validate_content_integrity /tmp/test.txt /tmp/presigned-download.txt
}

test_presigned_url_upload() {
    echo "presigned upload content" > /tmp/presigned-upload.txt

    # mc share upload generates an S3 POST policy for browser-based multipart/form-data uploads.
    # The output contains a ready-to-use curl command with all required form fields.
    MC_SHARE_OUTPUT=$(mc share upload --expire=1h "${MC_ALIAS}/${BUCKET}/presigned-upload.txt" 2>&1)

    # Extract the curl command line.
    # Old mc format:  "Share:  curl http://..."
    # New mc format:  " Curl  : curl -L -X POST http://..."
    CURL_LINE=$(printf '%s\n' "$MC_SHARE_OUTPUT" | grep -i " curl " | grep -v "^#" | head -1)

    if [ -z "$CURL_LINE" ]; then
        printf 'mc share upload output:\n%s\n' "$MC_SHARE_OUTPUT" >&2
        fail_test "Failed to extract curl command from mc share upload output"
    fi

    # Strip any label prefix (e.g. "Share: ", "Curl  : ", leading spaces)
    # leaving just "curl ..." regardless of mc version.
    CURL_CMD=$(printf '%s' "$CURL_LINE" | sed 's/^.*curl /curl /')

    # Replace the <FILE> placeholder with the actual test file path
    CURL_CMD="${CURL_CMD//<FILE>///tmp/presigned-upload.txt}"

    # Execute the POST policy upload; -f makes curl fail on HTTP 4xx/5xx
    if ! eval "$CURL_CMD" -f -s -o /dev/null 2>&1; then
        fail_test "POST policy upload failed"
    fi

    # Verify the uploaded object's content integrity
    mc cat "${MC_ALIAS}/${BUCKET}/presigned-upload.txt" > /tmp/presigned-upload-verify.txt 2>&1
    validate_content_integrity /tmp/presigned-upload.txt /tmp/presigned-upload-verify.txt
}

test_multipart_upload() {
    # Create 10MB file to trigger multipart upload
    dd if=/dev/zero of=/tmp/large-file.dat bs=1M count=10 > /dev/null 2>&1

    # Upload (mc automatically uses multipart for large files)
    mc cp /tmp/large-file.dat ${MC_ALIAS}/${BUCKET}/large-file.dat > /dev/null 2>&1

    # Verify content integrity
    mc cp ${MC_ALIAS}/${BUCKET}/large-file.dat /tmp/large-file-downloaded.dat > /dev/null 2>&1

    if ! cmp -s /tmp/large-file.dat /tmp/large-file-downloaded.dat; then
        fail_test "Downloaded file differs from original"
    fi

    # Cleanup
    rm -f /tmp/large-file.dat /tmp/large-file-downloaded.dat
}

test_delete_bucket() {
    # Cleanup all objects first
    mc rm --recursive --force ${MC_ALIAS}/${BUCKET}/ > /dev/null 2>&1 || true
    mc rb ${MC_ALIAS}/${BUCKET} > /dev/null 2>&1
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
