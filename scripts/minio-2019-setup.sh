#!/usr/bin/env bash
set -euo pipefail

# -----------------------
# MinIO 2019 FS Mode Test Setup
# -----------------------
# This script uses MinIO from April 2019 to test the full FS mode feature set
# before MinIO pivoted to focus on distributed/erasure-coded deployments.
#
# Goal: Create comprehensive test data to understand what metadata features
# were actually implemented in FS mode during its "golden era".

# -----------------------
# Config
# -----------------------
MINIO_IMAGE="minio/minio:RELEASE.2019-04-23T23-50-36Z"
MC_IMAGE="minio/mc:RELEASE.2019-04-24T00-09-41Z"

MINIO_CONTAINER="minio-2019"
# In 2019, MinIO used different env var names
MINIO_ACCESS_KEY="minioadmin"
MINIO_SECRET_KEY="minioadmin"
MINIO_PORT="9001"  # Different port to avoid conflicts
DATA_DIR="$(pwd)/minio-data-2019"

OBJECT_SIZE=65536

# Users
ALICE_USER="alice"
ALICE_PASS="alicepass1234"

BOB_USER="bob"
BOB_PASS="bobpass1234"

# -----------------------
# Password validation
# -----------------------
validate_secret () {
  local name=$1
  local secret=$2
  local len=${#secret}
  if (( len < 8 || len > 40 )); then
    echo "❌ Secret for user '${name}' must be 8-40 characters (got ${len})"
    exit 1
  fi
}

validate_secret "${ALICE_USER}" "${ALICE_PASS}"
validate_secret "${BOB_USER}" "${BOB_PASS}"

# -----------------------
# Cleanup
# -----------------------
set -x
echo "🧹 Cleaning up old containers and data..."
docker rm -f "${MINIO_CONTAINER}" >/dev/null 2>&1 || true
[ -d "${DATA_DIR}" ] && rm -rf "${DATA_DIR}" || true
mkdir -p "${DATA_DIR}"

set +x

# Note: In 2019, MinIO defaulted to FS mode for single-disk setups
# No need to pre-create format.json - let MinIO create it naturally

# -----------------------
# Start MinIO 2019
# -----------------------
echo "🚀 Starting MinIO 2019..."
docker run -d \
  --name "${MINIO_CONTAINER}" \
  -p "${MINIO_PORT}:9000" \
  -e MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY}" \
  -e MINIO_SECRET_KEY="${MINIO_SECRET_KEY}" \
  -v "${DATA_DIR}:/data" \
  "${MINIO_IMAGE}" server /data

echo "⏳ Waiting for MinIO (up to 30 seconds)..."
# 2019 MinIO doesn't have /minio/health/live endpoint, so just wait and test bucket creation
sleep 10

# -----------------------
# mc root auth
# -----------------------
export MC_HOST_minio2019="http://${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@127.0.0.1:${MINIO_PORT}"

# Test connection with a simple command
echo "🔌 Testing connection..."
for i in {1..10}; do
  if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" ls minio2019 >/dev/null 2>&1; then
    echo "✓ Connected to MinIO"
    break
  fi
  if [ $i -eq 10 ]; then
    echo "❌ Failed to connect to MinIO"
    docker logs "${MINIO_CONTAINER}"
    exit 1
  fi
  sleep 2
done

# -----------------------
# Create buckets
# -----------------------
echo "📦 Creating buckets..."
for bucket in alpha beta gamma; do
  docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" mb "minio2019/${bucket}"
done

# -----------------------
# Create policies first (before users)
# -----------------------
echo "📋 Creating IAM policies..."
cat > /tmp/alpha-rw.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:*"],
      "Resource": [
        "arn:aws:s3:::alpha",
        "arn:aws:s3:::alpha/*"
      ]
    }
  ]
}
EOF

cat > /tmp/beta-rw.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:*"],
      "Resource": [
        "arn:aws:s3:::beta",
        "arn:aws:s3:::beta/*"
      ]
    }
  ]
}
EOF

docker run --rm --network host -e MC_HOST_minio2019 \
  -v /tmp/alpha-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add minio2019 alpha-rw /policy.json

docker run --rm --network host -e MC_HOST_minio2019 \
  -v /tmp/beta-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add minio2019 beta-rw /policy.json

# -----------------------
# Create users with policies (2019 syntax: requires POLICYNAME as 4th arg)
# -----------------------
echo "👥 Creating users with policies..."
# 2019 syntax: mc admin user add TARGET ACCESSKEY SECRETKEY POLICYNAME
docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" admin user add minio2019 "${ALICE_USER}" "${ALICE_PASS}" alpha-rw

docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" admin user add minio2019 "${BOB_USER}" "${BOB_PASS}" beta-rw

# -----------------------
# Public-read buckets (2019 syntax: no "set" subcommand)
# -----------------------
echo "🌐 Setting public-read on gamma and beta..."
# 2019 syntax: mc policy download TARGET (not "mc policy set download")
docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" policy download "minio2019/gamma"

docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" policy download "minio2019/beta"

# -----------------------
# Upload basic objects
# -----------------------
echo "📤 Uploading test objects..."
upload_object () {
  local bucket=$1
  local name=$2

  tmpfile="$(mktemp)"
  head -c "${OBJECT_SIZE}" /dev/urandom > "${tmpfile}"

  docker run --rm --network host \
    -e MC_HOST_minio2019 \
    -v "${tmpfile}:/file.bin:ro" \
    "${MC_IMAGE}" \
    cp /file.bin "minio2019/${bucket}/${name}"

  rm -f "${tmpfile}"
}

upload_object alpha alice-object.bin
upload_object beta bob-object.bin
upload_object gamma public-object.bin

# -----------------------
# Folder Structure Objects (for delimiter/prefix testing)
# -----------------------
echo "📁 Creating folder structure for ListObjects delimiter testing..."
upload_object alpha folder1/file1.txt
upload_object alpha folder1/file2.txt
upload_object alpha folder1/subfolder/deep.txt
upload_object alpha folder2/file1.txt
upload_object alpha root-file.txt

upload_object beta prefix/test.txt
upload_object beta prefix/data/nested.txt
upload_object beta other/file.txt

# -----------------------
# Advanced Features Testing
# -----------------------
echo "🔬 Testing advanced FS mode features..."

# Test versioning (if supported in 2019 FS mode)
echo "  Testing versioning..."
docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" version enable "minio2019/alpha" || \
  echo "  ⚠️  Versioning not available in 2019 FS mode"

# Upload object with custom metadata (2019 syntax: comma separator, simple values)
echo "  Testing custom metadata..."
tmpfile="$(mktemp)"
head -c "${OBJECT_SIZE}" /dev/urandom > "${tmpfile}"

# Note: 2019 MC has trouble with values containing '=' or complex syntax
# Use simple key=value pairs only
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/file.bin:ro" \
  "${MC_IMAGE}" \
  cp --attr "x-amz-meta-author=TestUser,x-amz-meta-project=DirIO,x-amz-meta-version=1" \
  /file.bin "minio2019/alpha/metadata-test.bin"

rm -f "${tmpfile}"

# Note: Lifecycle policies (mc ilm) not available in 2019 FS mode

# -----------------------
# CopyObject (server-side copy) Testing
# -----------------------
echo "📋 Testing CopyObject (server-side copy)..."
docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" \
  cp "minio2019/alpha/alice-object.bin" "minio2019/alpha/alice-copy.bin" || \
  echo "  ⚠️  Server-side copy might not work in 2019 FS mode"

docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" \
  cp "minio2019/alpha/folder1/file1.txt" "minio2019/beta/copied-from-alpha.txt" || \
  echo "  ⚠️  Cross-bucket copy might not work in 2019 FS mode"

# -----------------------
# Object Tagging Testing
# -----------------------
echo "🏷️  Testing Object Tagging..."
# First create a dedicated object for tagging
tmpfile="$(mktemp)"
echo "tagging test content" > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/tagging-test.txt:ro" \
  "${MC_IMAGE}" \
  cp /tagging-test.txt "minio2019/alpha/tagging-test.txt"
rm -f "${tmpfile}"

# Note: Object tagging (mc tag) was not available in 2019
# We'll use custom metadata instead to simulate tagging
echo "  ⚠️  Object tagging (mc tag) not available in 2019 - using custom metadata instead"
tmpfile="$(mktemp)"
echo "simulated tagged content" > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/tagged.txt:ro" \
  "${MC_IMAGE}" \
  cp --attr "x-amz-meta-environment=test,x-amz-meta-project=dirio,x-amz-meta-version=1.0" \
  /tagged.txt "minio2019/alpha/simulated-tagged.txt"
rm -f "${tmpfile}"

# -----------------------
# Multipart Upload (large file) Testing
# -----------------------
echo "📦 Testing Multipart Upload (large file >5MB)..."
# Create a 10MB file for multipart upload testing
largefile="$(mktemp)"
dd if=/dev/zero of="${largefile}" bs=1M count=10 2>/dev/null

docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${largefile}:/large-file.dat:ro" \
  "${MC_IMAGE}" \
  cp /large-file.dat "minio2019/alpha/large-file.dat" || \
  echo "  ⚠️  Multipart upload might not work in 2019 FS mode"

docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${largefile}:/large-file.dat:ro" \
  "${MC_IMAGE}" \
  cp /large-file.dat "minio2019/gamma/large-public.dat" || \
  echo "  ⚠️  Multipart upload might not work in 2019 FS mode"

rm -f "${largefile}"

# -----------------------
# Additional Metadata Variations (2019 syntax: comma separator, simple values)
# -----------------------
echo "🔖 Creating objects with various metadata combinations..."

# Object with Content-Type only
tmpfile="$(mktemp)"
echo '{"test": "json data"}' > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/data.json:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=application/json" \
  /data.json "minio2019/alpha/data.json"
rm -f "${tmpfile}"

# Object with Content-Type and custom metadata
tmpfile="$(mktemp)"
echo "<html><body>Test</body></html>" > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/index.html:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=text/html,x-amz-meta-page=index" \
  /index.html "minio2019/gamma/index.html"
rm -f "${tmpfile}"

# Object with Content-Encoding
tmpfile="$(mktemp)"
echo "compressed data" | gzip > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/data.gz:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=application/gzip,Content-Encoding=gzip" \
  /data.gz "minio2019/beta/data.gz"
rm -f "${tmpfile}"

# Object with multiple custom metadata fields (x-amz-meta-*)
tmpfile="$(mktemp)"
echo "user data" > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/userdata.txt:ro" \
  "${MC_IMAGE}" \
  cp --attr "x-amz-meta-user-id=12345,x-amz-meta-department=engineering,x-amz-meta-uploaded-by=alice" \
  /userdata.txt "minio2019/alpha/userdata.txt"
rm -f "${tmpfile}"

# Object with Content-Language
tmpfile="$(mktemp)"
echo "Bonjour le monde" > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/french.txt:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=text/plain,Content-Language=fr" \
  /french.txt "minio2019/alpha/french.txt"
rm -f "${tmpfile}"

# -----------------------
# Examine what was created
# -----------------------
echo
echo "🔍 Examining created metadata files..."
echo
echo "Bucket metadata files:"
find "${DATA_DIR}/.minio.sys/buckets" -name ".metadata.bin" -o -name "*.json" 2>/dev/null | head -20 || true
echo
echo "Config files:"
find "${DATA_DIR}/.minio.sys/config" -type f 2>/dev/null | head -20 || true
echo
echo "Sample fs.json (if exists):"
find "${DATA_DIR}/.minio.sys/buckets" -name "fs.json" | head -1 | xargs cat 2>/dev/null || echo "(none found yet)"

# -----------------------
# Done
# -----------------------
echo
echo "✅ MinIO 2019 FS mode test setup complete!"
echo
echo "Container: ${MINIO_CONTAINER}"
echo "Port: ${MINIO_PORT}"
echo "Data: ${DATA_DIR}"
echo
echo "Users:"
echo "  alice / alicepass1234 → (attempted) RW on bucket alpha"
echo "  bob   / bobpass1234   → (attempted) RW on bucket beta"
echo
echo "Buckets:"
echo "  alpha → folder structure, metadata objects, simulated tagged objects"
echo "  beta  → public-read, copied objects"
echo "  gamma → public-read, large files"
echo
echo "Created objects include:"
echo "  - Basic objects (alice-object.bin, bob-object.bin, public-object.bin)"
echo "  - Folder structures (folder1/file1.txt, folder2/file1.txt, etc.)"
echo "  - Objects with standard metadata (Content-Type, Content-Encoding, Content-Language)"
echo "  - Objects with custom metadata (x-amz-meta-author, x-amz-meta-project, etc.)"
echo "  - Simulated tagged objects (using x-amz-meta-* instead of real tags)"
echo "  - Server-side copies (alice-copy.bin, copied-from-alpha.txt)"
echo "  - Large multipart uploads (large-file.dat - 10MB)"
echo
echo "2019 MinIO MC limitations found:"
echo "  - No 'mc tag' command (used x-amz-meta-* custom metadata instead)"
echo "  - No 'mc version enable' command"
echo "  - No 'mc ilm' (lifecycle) command"
echo "  - Metadata separator: comma (,) not semicolon (;)"
echo "  - Metadata values with '=' fail (e.g., Cache-Control=max-age=3600)"
echo "  - Content-Disposition with filename parameters fails"
echo
echo "Next steps:"
echo "  1. Examine ${DATA_DIR}/.minio.sys/ to see what metadata was created"
echo "  2. Compare with modern MinIO data to identify differences"
echo "  3. Import this data into DirIO to test compatibility"
echo "  4. Run DirIO S3 API tests against imported data to verify correctness"
echo
echo "To stop: docker rm -f ${MINIO_CONTAINER}"
