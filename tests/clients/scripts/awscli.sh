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

# PutObject
echo "test content" > /tmp/test.txt
$AWS s3api put-object --bucket ${BUCKET} --key test.txt --body /tmp/test.txt && pass "PutObject" || fail "PutObject"

# HeadObject
$AWS s3api head-object --bucket ${BUCKET} --key test.txt && pass "HeadObject" || fail "HeadObject"

# GetObject
$AWS s3api get-object --bucket ${BUCKET} --key test.txt /tmp/download.txt && pass "GetObject" || fail "GetObject"

# ListObjectsV2
$AWS s3api list-objects-v2 --bucket ${BUCKET} && pass "ListObjectsV2" || fail "ListObjectsV2"

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