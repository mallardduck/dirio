#!/bin/bash
set +e

# Cleanup handler for signals
cleanup() {
  echo "Received signal, cleaning up..."
  exit 130
}
trap cleanup SIGINT SIGTERM

PASSED=0
FAILED=0

pass() { echo "PASS: $1"; PASSED=$((PASSED+1)); }
fail() { echo "FAIL: $1 - $2"; FAILED=$((FAILED+1)); }

BUCKET="awscli-test-bucket-$(date +%s)"
ENDPOINT="${DIRIO_ENDPOINT}"
AWS="aws --endpoint-url ${ENDPOINT}"

echo "=== AWS CLI Tests ==="
echo "Endpoint: ${ENDPOINT}"

# Network probe — plain curl, no AWS CLI.  Proves the container can reach the
# server and that we are talking to a real DirIO instance.
echo "--- Network Probe ---"
PROBE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -m 5 "${ENDPOINT}/healthz")
if [ "${PROBE_CODE}" = "000" ]; then
  echo "  FATAL: Cannot reach server at ${ENDPOINT}"
  exit 1
fi
echo "  GET /healthz            -> HTTP ${PROBE_CODE}"
QP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -m 5 "${ENDPOINT}/healthz?probe=1")
echo "  GET /healthz?probe=1    -> HTTP ${QP_CODE}"

# ListBuckets
$AWS s3api list-buckets && pass "ListBuckets" || fail "ListBuckets"

# CreateBucket
$AWS s3api create-bucket --bucket ${BUCKET} && pass "CreateBucket" || fail "CreateBucket"

# HeadBucket
$AWS s3api head-bucket --bucket ${BUCKET} && pass "HeadBucket" || fail "HeadBucket"

# GetBucketLocation - verify server returns bucket location metadata
$AWS s3api get-bucket-location --bucket ${BUCKET} --output json > /tmp/location.json
if [ $? -eq 0 ] && grep -q "LocationConstraint" /tmp/location.json; then
  pass "GetBucketLocation"
else
  fail "GetBucketLocation" "missing LocationConstraint in response"
fi

# PutObject
echo "test content" > /tmp/test.txt
$AWS s3api put-object --bucket ${BUCKET} --key test.txt --body /tmp/test.txt && pass "PutObject" || fail "PutObject"

# PutObject with custom metadata
echo "metadata test" > /tmp/meta-test.txt
$AWS s3api put-object --bucket ${BUCKET} --key meta-test.txt --body /tmp/meta-test.txt --metadata custom-key=custom-value
if [ $? -eq 0 ]; then
  pass "Custom metadata (set)"
else
  fail "Custom metadata (set)" "put-object with metadata failed"
fi

# HeadObject
$AWS s3api head-object --bucket ${BUCKET} --key test.txt && pass "HeadObject" || fail "HeadObject"

# Custom metadata (get) - verify metadata in HeadObject response AND content not corrupted
$AWS s3api head-object --bucket ${BUCKET} --key meta-test.txt > /tmp/meta-head.txt
$AWS s3api get-object --bucket ${BUCKET} --key meta-test.txt /tmp/meta-download.txt
if grep -qi "custom-key" /tmp/meta-head.txt && diff -q /tmp/meta-test.txt /tmp/meta-download.txt >/dev/null 2>&1; then
  pass "Custom metadata (get)"
else
  fail "Custom metadata (get)" "metadata not returned or content corrupted"
fi

# GetObject
$AWS s3api get-object --bucket ${BUCKET} --key test.txt /tmp/download.txt
if [ $? -eq 0 ] && diff -q /tmp/test.txt /tmp/download.txt >/dev/null 2>&1; then
  pass "GetObject"
else
  fail "GetObject" "download failed or content mismatch"
fi

# Range request - create 100-byte file with known content
printf "%0100d" 0 > /tmp/range-source.txt
$AWS s3api put-object --bucket ${BUCKET} --key range.txt --body /tmp/range-source.txt

# Request only first 10 bytes
$AWS s3api get-object --bucket ${BUCKET} --key range.txt --range bytes=0-9 /tmp/range-partial.txt
PARTIAL_SIZE=$(wc -c < /tmp/range-partial.txt | tr -d ' ')
if [ "${PARTIAL_SIZE}" = "10" ]; then
  pass "Range request"
else
  fail "Range request" "expected 10 bytes, got ${PARTIAL_SIZE}"
fi

# ListObjectsV2 (basic)
$AWS s3api list-objects-v2 --bucket ${BUCKET} > /tmp/list-basic.txt
if [ $? -eq 0 ] && grep -q "test.txt" /tmp/list-basic.txt; then
  pass "ListObjectsV2 (basic)"
else
  fail "ListObjectsV2 (basic)" "test.txt not found in list"
fi

# Setup folder structure for prefix tests
echo "folder1 file1" > /tmp/f1-file1.txt
echo "folder1 file2" > /tmp/f1-file2.txt
$AWS s3api put-object --bucket ${BUCKET} --key folder1/file1.txt --body /tmp/f1-file1.txt
$AWS s3api put-object --bucket ${BUCKET} --key folder1/file2.txt --body /tmp/f1-file2.txt

# ListObjectsV2 with prefix
$AWS s3api list-objects-v2 --bucket ${BUCKET} --prefix folder1/ > /tmp/list-prefix.txt
if [ $? -eq 0 ] && grep -q "folder1/file1.txt" /tmp/list-prefix.txt && grep -q "folder1/file2.txt" /tmp/list-prefix.txt; then
  pass "ListObjectsV2 (prefix)"
else
  fail "ListObjectsV2 (prefix)" "expected objects not found"
fi

# ListObjectsV2 with delimiter - should show folder prefixes
$AWS s3api list-objects-v2 --bucket ${BUCKET} --delimiter / > /tmp/list-delim.txt
if [ $? -eq 0 ] && grep -q "CommonPrefixes" /tmp/list-delim.txt; then
  pass "ListObjectsV2 (delimiter)"
else
  fail "ListObjectsV2 (delimiter)" "CommonPrefixes not found"
fi

# CopyObject - copy test.txt to copied.txt
$AWS s3api copy-object --copy-source ${BUCKET}/test.txt --bucket ${BUCKET} --key copied.txt 2>&1
if [ $? -eq 0 ]; then
  # Verify copied content matches original
  $AWS s3api get-object --bucket ${BUCKET} --key copied.txt /tmp/copied.txt
  if diff -q /tmp/test.txt /tmp/copied.txt >/dev/null 2>&1; then
    pass "CopyObject"
  else
    fail "CopyObject" "content mismatch after copy"
  fi
else
  fail "CopyObject" "copy operation failed"
fi

# Pre-signed URL - generate URL and download via curl
PRESIGNED_URL=$($AWS s3 presign s3://${BUCKET}/test.txt --expires-in 300 2>&1)
if [ $? -eq 0 ]; then
  # Download via presigned URL using curl
  curl -s -f -o /tmp/presigned-download.txt "${PRESIGNED_URL}"
  if [ $? -eq 0 ] && diff -q /tmp/test.txt /tmp/presigned-download.txt >/dev/null 2>&1; then
    pass "Pre-signed URL"
  else
    fail "Pre-signed URL" "download failed or content mismatch"
  fi
else
  fail "Pre-signed URL" "URL generation failed"
fi

# Multipart upload - create 2 parts and assemble
echo "part1 content" > /tmp/part1.txt
echo "part2 content" > /tmp/part2.txt

# Initiate
CREATE_RESP=$($AWS s3api create-multipart-upload --bucket ${BUCKET} --key multipart.txt --output json 2>&1)
if [ $? -ne 0 ]; then
  fail "Multipart upload" "create failed"
else
  # Extract UploadId - try jq first, fallback to grep/sed
  if command -v jq >/dev/null 2>&1; then
    UPLOAD_ID=$(echo "$CREATE_RESP" | jq -r '.UploadId')
  else
    UPLOAD_ID=$(echo "$CREATE_RESP" | grep -o '"UploadId"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)".*/\1/')
  fi

  if [ -z "$UPLOAD_ID" ]; then
    fail "Multipart upload" "could not parse UploadId"
  else
    # Upload part 1
    PART1_RESP=$($AWS s3api upload-part --bucket ${BUCKET} --key multipart.txt --upload-id "$UPLOAD_ID" --part-number 1 --body /tmp/part1.txt --output json)
    if command -v jq >/dev/null 2>&1; then
      ETAG1=$(echo "$PART1_RESP" | jq -r '.ETag')
    else
      ETAG1=$(echo "$PART1_RESP" | grep -o '"ETag"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)".*/\1/')
    fi

    # Upload part 2
    PART2_RESP=$($AWS s3api upload-part --bucket ${BUCKET} --key multipart.txt --upload-id "$UPLOAD_ID" --part-number 2 --body /tmp/part2.txt --output json)
    if command -v jq >/dev/null 2>&1; then
      ETAG2=$(echo "$PART2_RESP" | jq -r '.ETag')
    else
      ETAG2=$(echo "$PART2_RESP" | grep -o '"ETag"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)".*/\1/')
    fi

    # Complete multipart
    COMPLETE_JSON="{\"Parts\":[{\"PartNumber\":1,\"ETag\":$ETAG1},{\"PartNumber\":2,\"ETag\":$ETAG2}]}"
    $AWS s3api complete-multipart-upload --bucket ${BUCKET} --key multipart.txt --upload-id "$UPLOAD_ID" --multipart-upload "$COMPLETE_JSON" 2>&1

    if [ $? -eq 0 ]; then
      # Verify assembled content
      $AWS s3api get-object --bucket ${BUCKET} --key multipart.txt /tmp/mp-downloaded.txt
      cat /tmp/part1.txt /tmp/part2.txt > /tmp/mp-expected.txt
      if diff -q /tmp/mp-expected.txt /tmp/mp-downloaded.txt >/dev/null 2>&1; then
        pass "Multipart upload"
      else
        fail "Multipart upload" "content mismatch after assembly"
      fi
    else
      fail "Multipart upload" "complete operation failed"
    fi
  fi
fi

# Object tagging - CRITICAL: verify content not corrupted
# Get original content hash first
$AWS s3api get-object --bucket ${BUCKET} --key test.txt /tmp/test-before-tag.txt
HASH_BEFORE=$(md5sum /tmp/test-before-tag.txt 2>/dev/null | awk '{print $1}' || md5 -q /tmp/test-before-tag.txt)

# Put tags
$AWS s3api put-object-tagging --bucket ${BUCKET} --key test.txt --tagging 'TagSet=[{Key=env,Value=test}]' 2>&1
if [ $? -eq 0 ]; then
  # Get tags back
  $AWS s3api get-object-tagging --bucket ${BUCKET} --key test.txt > /tmp/tags.txt
  if grep -q "env" /tmp/tags.txt && grep -q "test" /tmp/tags.txt; then
    # Verify object content NOT corrupted
    $AWS s3api get-object --bucket ${BUCKET} --key test.txt /tmp/test-after-tag.txt
    HASH_AFTER=$(md5sum /tmp/test-after-tag.txt 2>/dev/null | awk '{print $1}' || md5 -q /tmp/test-after-tag.txt)
    if [ "$HASH_BEFORE" = "$HASH_AFTER" ]; then
      pass "Object tagging"
    else
      fail "Object tagging" "CRITICAL: object content corrupted after tagging"
    fi
  else
    fail "Object tagging" "tags not returned"
  fi
else
  fail "Object tagging" "put operation failed"
fi

# s3 cp upload
$AWS s3 cp /tmp/test.txt s3://${BUCKET}/hl-test.txt && pass "s3 cp upload" || fail "s3 cp upload"

# s3 cp download
$AWS s3 cp s3://${BUCKET}/hl-test.txt /tmp/hl-download.txt && pass "s3 cp download" || fail "s3 cp download"

# DeleteObject
$AWS s3api delete-object --bucket ${BUCKET} --key test.txt && pass "DeleteObject" || fail "DeleteObject"

# Cleanup and DeleteBucket
$AWS s3 rm s3://${BUCKET} --recursive 2>/dev/null || true
$AWS s3api delete-bucket --bucket ${BUCKET} && pass "DeleteBucket" || fail "DeleteBucket"

echo ""
echo "=== Summary ==="
echo "Passed: ${PASSED}"
echo "Failed: ${FAILED}"
if [ ${FAILED} -eq 0 ]; then
  echo "All tests passed"
  exit 0
else
  exit 1
fi