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

BUCKET="test-bucket-$(date +%s)"
ENDPOINT="${DIRIO_ENDPOINT}"
MC_ALIAS="dirio"

echo "=== MinIO mc Tests ==="
echo "Endpoint: ${ENDPOINT}"

# Configure alias
mc alias set ${MC_ALIAS} ${ENDPOINT} ${DIRIO_ACCESS_KEY} ${DIRIO_SECRET_KEY} --api S3v4 2>/dev/null
if [ $? -eq 0 ]; then
  pass "Configure alias"
else
  fail "Configure alias"
  exit 1
fi

# ListBuckets
mc ls ${MC_ALIAS} && pass "ListBuckets" || fail "ListBuckets"

# CreateBucket
mc mb ${MC_ALIAS}/${BUCKET} 2>&1
if [ $? -eq 0 ]; then
  pass "CreateBucket (mc mb)"
else
  fail "CreateBucket (mc mb)"
fi

# HeadBucket (requires --no-list flag)
mc stat --no-list ${MC_ALIAS}/${BUCKET} 2>&1
if [ $? -eq 0 ]; then
  pass "HeadBucket (mc stat --no-list)"
else
  fail "HeadBucket (mc stat --no-list)"
fi

# PutObject
echo "test content" > /tmp/test.txt
mc cp /tmp/test.txt ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "PutObject (mc cp upload)"
else
  fail "PutObject (mc cp upload)"
fi

# HeadObject
mc stat ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "HeadObject (mc stat)"
else
  fail "HeadObject (mc stat)"
fi

# GetObject (download)
mc cp ${MC_ALIAS}/${BUCKET}/test.txt /tmp/download.txt 2>&1
if [ $? -eq 0 ]; then
  pass "GetObject (mc cp download)"
else
  fail "GetObject (mc cp download)"
fi

# GetObject (cat)
mc cat ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "GetObject (mc cat)"
else
  fail "GetObject (mc cat)"
fi

# ListObjectsV2
mc ls ${MC_ALIAS}/${BUCKET}/ 2>&1
if [ $? -eq 0 ]; then
  pass "ListObjectsV2 (mc ls)"
else
  fail "ListObjectsV2 (mc ls)"
fi

# DeleteObject
mc rm ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "DeleteObject (mc rm)"
else
  fail "DeleteObject (mc rm)"
fi

# DeleteBucket
mc rb ${MC_ALIAS}/${BUCKET} 2>&1
if [ $? -eq 0 ]; then
  pass "DeleteBucket (mc rb)"
else
  fail "DeleteBucket (mc rb)"
fi

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