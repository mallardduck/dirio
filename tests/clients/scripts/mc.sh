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

BUCKET="mc-test-bucket-$(date +%s)"
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

# HeadBucket
mc stat ${MC_ALIAS}/${BUCKET} 2>&1
if [ $? -eq 0 ]; then
  pass "HeadBucket (mc stat)"
else
  fail "HeadBucket (mc stat)"
fi

# GetBucketLocation (mc stat calls GetBucketInfo which uses GetBucketLocation)
mc stat ${MC_ALIAS}/${BUCKET} 2>&1
if [ $? -eq 0 ]; then
  pass "GetBucketLocation (mc stat)"
else
  fail "GetBucketLocation (mc stat)"
fi

# PutObject
echo "test content" > /tmp/test.txt
mc put /tmp/test.txt ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "PutObject (mc put upload)"
else
  fail "PutObject (mc put upload)"
fi

# PutObject (via mc cp)
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

# ListObjectsV2 (basic)
mc ls ${MC_ALIAS}/${BUCKET}/ 2>&1
if [ $? -eq 0 ]; then
  pass "ListObjectsV2 (mc ls)"
else
  fail "ListObjectsV2 (mc ls)"
fi

# ListObjectsV2 with prefix
echo "prefix test" > /tmp/prefix-test.txt
mc cp /tmp/prefix-test.txt ${MC_ALIAS}/${BUCKET}/prefix/test.txt 2>&1 >/dev/null
mc ls ${MC_ALIAS}/${BUCKET}/prefix/ 2>&1 | grep -q "test.txt"
if [ $? -eq 0 ]; then
  pass "ListObjectsV2 with prefix (mc ls prefix/)"
else
  fail "ListObjectsV2 with prefix (mc ls prefix/)" "Object not found in prefix listing"
fi

# ListObjectsV2 with delimiter (non-recursive to show folders)
# Create folder structure
mc cp /tmp/test.txt ${MC_ALIAS}/${BUCKET}/folder1/file1.txt 2>&1 >/dev/null
mc cp /tmp/test.txt ${MC_ALIAS}/${BUCKET}/folder1/file2.txt 2>&1 >/dev/null
mc cp /tmp/test.txt ${MC_ALIAS}/${BUCKET}/folder2/file1.txt 2>&1 >/dev/null
mc cp /tmp/test.txt ${MC_ALIAS}/${BUCKET}/root-file.txt 2>&1 >/dev/null
# Non-recursive listing should show folders as prefixes
mc ls ${MC_ALIAS}/${BUCKET}/ 2>&1 | grep -q "folder1/"
if [ $? -eq 0 ]; then
  pass "ListObjectsV2 with delimiter (mc ls shows folders)"
else
  fail "ListObjectsV2 with delimiter (mc ls shows folders)" "folder1/ not shown as prefix"
fi

# ListObjectsV2 recursive (delimiter empty)
mc ls --recursive ${MC_ALIAS}/${BUCKET}/ 2>&1 | grep -q "folder1/file1.txt"
if [ $? -eq 0 ]; then
  pass "ListObjectsV2 recursive (mc ls -r)"
else
  fail "ListObjectsV2 recursive (mc ls -r)" "Recursive listing failed"
fi

# Custom Metadata (set)
echo "metadata test" > /tmp/metadata-test.txt
mc cp --attr "x-amz-meta-custom-key=custom-value;Cache-Control=max-age=3600" /tmp/metadata-test.txt ${MC_ALIAS}/${BUCKET}/metadata-test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "Custom Metadata set (mc cp --attr)"
else
  fail "Custom Metadata set (mc cp --attr)" "Failed to upload with custom metadata"
fi

# Custom Metadata (get)
mc stat ${MC_ALIAS}/${BUCKET}/metadata-test.txt 2>&1 | grep -q "custom-key"
if [ $? -eq 0 ]; then
  pass "Custom Metadata get (mc stat shows metadata)"
else
  fail "Custom Metadata get (mc stat shows metadata)" "Custom metadata not returned"
fi

# CopyObject (server-side copy)
mc cp ${MC_ALIAS}/${BUCKET}/test.txt ${MC_ALIAS}/${BUCKET}/test-copy.txt 2>&1
if [ $? -eq 0 ]; then
  # Verify the copy exists and has same content
  mc cat ${MC_ALIAS}/${BUCKET}/test-copy.txt 2>&1 | grep -q "test content"
  if [ $? -eq 0 ]; then
    pass "CopyObject (mc cp s3-to-s3)"
  else
    fail "CopyObject (mc cp s3-to-s3)" "Copied file has wrong content"
  fi
else
  fail "CopyObject (mc cp s3-to-s3)" "Copy operation failed"
fi

# Pre-signed URLs (download)
PRESIGNED_URL=$(mc share download --expire=1h ${MC_ALIAS}/${BUCKET}/test.txt 2>&1 | grep -o 'https\?://[^ ]*' | head -1)
if [ -n "$PRESIGNED_URL" ]; then
  # Try to download using the pre-signed URL
  curl -f -s "$PRESIGNED_URL" > /tmp/presigned-download.txt 2>&1
  if [ $? -eq 0 ] && grep -q "test content" /tmp/presigned-download.txt; then
    pass "Pre-signed URL download (mc share download)"
  else
    fail "Pre-signed URL download (mc share download)" "Failed to download via pre-signed URL"
  fi
else
  fail "Pre-signed URL download (mc share download)" "Failed to generate pre-signed URL"
fi

# Pre-signed URLs (upload)
UPLOAD_URL=$(mc share upload --expire=1h ${MC_ALIAS}/${BUCKET}/presigned-upload.txt 2>&1 | grep -o 'curl.*' | sed 's/.*-X PUT //' | grep -o 'https\?://[^ ]*' | head -1)
if [ -n "$UPLOAD_URL" ]; then
  echo "presigned upload content" > /tmp/presigned-upload.txt
  curl -f -s -X PUT -T /tmp/presigned-upload.txt "$UPLOAD_URL" 2>&1 >/dev/null
  if [ $? -eq 0 ]; then
    # Verify the uploaded file
    mc cat ${MC_ALIAS}/${BUCKET}/presigned-upload.txt 2>&1 | grep -q "presigned upload content"
    if [ $? -eq 0 ]; then
      pass "Pre-signed URL upload (mc share upload)"
    else
      fail "Pre-signed URL upload (mc share upload)" "Uploaded file not found or wrong content"
    fi
  else
    fail "Pre-signed URL upload (mc share upload)" "Failed to upload via pre-signed URL"
  fi
else
  fail "Pre-signed URL upload (mc share upload)" "Failed to generate upload URL"
fi

# Object Tagging (set)
mc tag set ${MC_ALIAS}/${BUCKET}/test.txt "key1=value1&key2=value2" 2>&1
if [ $? -eq 0 ]; then
  pass "Object Tagging set (mc tag set)"
else
  fail "Object Tagging set (mc tag set)" "Failed to set tags"
fi

# Object Tagging (get)
mc tag list ${MC_ALIAS}/${BUCKET}/test.txt 2>&1 | grep -q "key1"
if [ $? -eq 0 ]; then
  pass "Object Tagging get (mc tag list)"
else
  fail "Object Tagging get (mc tag list)" "Tags not returned or incorrect"
fi

# Multipart Upload (large file >5MB)
dd if=/dev/zero of=/tmp/large-file.dat bs=1M count=10 2>/dev/null
mc cp /tmp/large-file.dat ${MC_ALIAS}/${BUCKET}/large-file.dat 2>&1
if [ $? -eq 0 ]; then
  # Verify file size
  SIZE=$(mc stat ${MC_ALIAS}/${BUCKET}/large-file.dat 2>&1 | grep "Size" | awk '{print $3}')
  if [ "$SIZE" = "10" ]; then
    pass "Multipart Upload (mc cp large file)"
  else
    fail "Multipart Upload (mc cp large file)" "File size mismatch: expected 10 MiB, got $SIZE"
  fi
else
  fail "Multipart Upload (mc cp large file)" "Upload failed"
fi
rm -f /tmp/large-file.dat

# Range Requests (partial download)
# mc doesn't have direct range request support, but we can test via curl with mc share
RANGE_URL=$(mc share download --expire=1h ${MC_ALIAS}/${BUCKET}/metadata-test.txt 2>&1 | grep -o 'https\?://[^ ]*' | head -1)
if [ -n "$RANGE_URL" ]; then
  # Download only first 10 bytes
  PARTIAL=$(curl -f -s -r 0-9 "$RANGE_URL" 2>&1)
  if [ $? -eq 0 ] && [ ${#PARTIAL} -eq 10 ]; then
    pass "Range Requests (curl with Range header)"
  else
    fail "Range Requests (curl with Range header)" "Expected 10 bytes, got ${#PARTIAL}"
  fi
else
  fail "Range Requests (curl with Range header)" "Could not generate URL for range test"
fi

# DeleteObject
mc rm ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "DeleteObject (mc rm)"
else
  fail "DeleteObject (mc rm)"
fi

# DeleteObject (cleanup all test objects)
mc rm --recursive --force ${MC_ALIAS}/${BUCKET}/ 2>&1 >/dev/null

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