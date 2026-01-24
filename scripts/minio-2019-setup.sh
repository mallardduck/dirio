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
echo "🧹 Cleaning up old containers and data..."
docker rm -f "${MINIO_CONTAINER}" >/dev/null 2>&1 || true
rm -rf "${DATA_DIR}"
mkdir -p "${DATA_DIR}"

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
# Create users (2019 syntax)
# -----------------------
echo "👥 Creating users..."
# In 2019, command might be different - try both formats
docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" admin user add minio2019 "${ALICE_USER}" "${ALICE_PASS}" || \
  echo "⚠️  User creation might use different syntax in 2019"

docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" admin user add minio2019 "${BOB_USER}" "${BOB_PASS}" || \
  echo "⚠️  User creation might use different syntax in 2019"

# -----------------------
# Create policies
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
  "${MC_IMAGE}" admin policy add minio2019 alpha-rw /policy.json || \
  echo "⚠️  Policy creation syntax might differ in 2019"

docker run --rm --network host -e MC_HOST_minio2019 \
  -v /tmp/beta-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add minio2019 beta-rw /policy.json || \
  echo "⚠️  Policy creation syntax might differ in 2019"

# -----------------------
# Attach policies (2019 syntax might differ)
# -----------------------
echo "🔗 Attaching policies to users..."
docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" admin policy set minio2019 alpha-rw user="${ALICE_USER}" || \
  echo "⚠️  Policy attachment syntax might differ in 2019"

docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" admin policy set minio2019 beta-rw user="${BOB_USER}" || \
  echo "⚠️  Policy attachment syntax might differ in 2019"

# -----------------------
# Public-read buckets
# -----------------------
echo "🌐 Setting public-read on gamma and beta..."
docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" policy set download "minio2019/gamma" || \
  echo "⚠️  Public bucket policy syntax might differ in 2019"

docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" policy set download "minio2019/beta" || \
  echo "⚠️  Public bucket policy syntax might differ in 2019"

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
# Advanced Features Testing
# -----------------------
echo "🔬 Testing advanced FS mode features..."

# Test versioning (if supported in 2019 FS mode)
echo "  Testing versioning..."
docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" version enable "minio2019/alpha" || \
  echo "  ⚠️  Versioning not available in 2019 FS mode"

# Upload object with custom metadata
echo "  Testing custom metadata..."
tmpfile="$(mktemp)"
head -c "${OBJECT_SIZE}" /dev/urandom > "${tmpfile}"

docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/file.bin:ro" \
  "${MC_IMAGE}" \
  cp --attr "Cache-Control=max-age=3600;Content-Disposition=attachment;x-amz-meta-author=TestUser;x-amz-meta-project=DirIO" \
  /file.bin "minio2019/alpha/metadata-test.bin" || \
  echo "  ⚠️  Custom metadata syntax might differ in 2019"

rm -f "${tmpfile}"

# Test lifecycle policy (if supported)
echo "  Testing lifecycle policies..."
cat > /tmp/lifecycle.json <<'EOF'
{
  "Rules": [
    {
      "ID": "expire-old",
      "Status": "Enabled",
      "Expiration": {
        "Days": 30
      }
    }
  ]
}
EOF

docker run --rm --network host -e MC_HOST_minio2019 \
  -v /tmp/lifecycle.json:/lifecycle.json \
  "${MC_IMAGE}" \
  ilm import "minio2019/beta" < /tmp/lifecycle.json || \
  echo "  ⚠️  Lifecycle policies not available in 2019 FS mode"

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
echo "  alpha → (attempted) versioning enabled"
echo "  beta  → (attempted) lifecycle policy"
echo "  gamma → (attempted) public-read"
echo
echo "Next steps:"
echo "  1. Examine ${DATA_DIR}/.minio.sys/ to see what metadata was created"
echo "  2. Compare with modern MinIO data to identify differences"
echo "  3. Import this data into DirIO to test compatibility"
echo
echo "To stop: docker rm -f ${MINIO_CONTAINER}"
