#!/usr/bin/env bash
set -euo pipefail

# -----------------------
# MinIO 2019→2022 Import Script
# -----------------------
# This script imports data created by minio-2019-setup.sh into a 2022 MinIO instance.
# This allows testing both the original 2019 FS format and the migrated 2022 format.
#
# Prerequisites: Run minio-2019-setup.sh first to create minio-data-2019/
#
# What this does:
# 1. Starts a 2022 MinIO instance
# 2. Copies the 2019 data directory as the starting point
# 3. Lets 2022 MinIO migrate/upgrade the data format
# 4. Recreates users/policies using 2022 syntax (MinIO won't auto-migrate IAM)
# 5. Result: A 2022-compatible version of the 2019 test data

# -----------------------
# Config
# -----------------------
MINIO_IMAGE="minio/minio:RELEASE.2022-10-24T18-35-07Z"
MC_IMAGE="minio/mc:RELEASE.2022-10-22T03-39-29Z"

MINIO_CONTAINER="minio-2022-import"
MINIO_ROOT_USER="minioadmin"
MINIO_ROOT_PASSWORD="minioadmin"
MINIO_PORT="9002"  # Different port to avoid conflicts with both 2019 and standalone
SOURCE_DATA_DIR="$(pwd)/minio-data-2019"
DEST_DATA_DIR="$(pwd)/minio-data-2022-import"

# Users (must match 2019 setup)
ALICE_USER="alice"
ALICE_PASS="alicepass1234"

BOB_USER="bob"
BOB_PASS="bobpass1234"

# Test user with multiple policies
CHARLIE_USER="charlie"
CHARLIE_PASS="charliepass1234"

# -----------------------
# Validation
# -----------------------
if [ ! -d "${SOURCE_DATA_DIR}" ]; then
  echo "❌ Source data directory not found: ${SOURCE_DATA_DIR}"
  echo "   Please run minio-2019-setup.sh first to create the 2019 data."
  exit 1
fi

echo "✓ Found 2019 data directory: ${SOURCE_DATA_DIR}"

# -----------------------
# Cleanup
# -----------------------
echo "🧹 Cleaning up old import container and data..."
docker rm -f "${MINIO_CONTAINER}" >/dev/null 2>&1 || true
[ -d "${DEST_DATA_DIR}" ] && rm -rf "${DEST_DATA_DIR}" || true

# -----------------------
# Copy 2019 data as starting point
# -----------------------
echo "📦 Copying 2019 data to new directory..."
cp -r "${SOURCE_DATA_DIR}" "${DEST_DATA_DIR}"
[ -d "${DEST_DATA_DIR}/.dirio" ] && rm -rf "${DEST_DATA_DIR}/.dirio"
echo "✓ Copied ${SOURCE_DATA_DIR} → ${DEST_DATA_DIR}"

# -----------------------
# Start MinIO 2022
# -----------------------
echo "🚀 Starting MinIO 2022 with imported 2019 data..."
docker run -d \
  --name "${MINIO_CONTAINER}" \
  -p "${MINIO_PORT}:9000" \
  -e MINIO_ROOT_USER="${MINIO_ROOT_USER}" \
  -e MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD}" \
  -v "${DEST_DATA_DIR}:/data" \
  "${MINIO_IMAGE}" server /data

echo "⏳ Waiting for MinIO to migrate data and start (up to 30 seconds)..."
sleep 10

# -----------------------
# mc root auth
# -----------------------
export MC_HOST_minio2022="http://${MINIO_ROOT_USER}:${MINIO_ROOT_PASSWORD}@127.0.0.1:${MINIO_PORT}"

# Test connection
echo "🔌 Testing connection..."
for i in {1..15}; do
  if docker run --rm --network host -e MC_HOST_minio2022 "${MC_IMAGE}" ls minio2022 >/dev/null 2>&1; then
    echo "✓ Connected to MinIO 2022"
    break
  fi
  if [ $i -eq 15 ]; then
    echo "❌ Failed to connect to MinIO"
    echo "Container logs:"
    docker logs "${MINIO_CONTAINER}"
    exit 1
  fi
  echo "  Attempt $i/15..."
  sleep 2
done

# -----------------------
# Verify buckets were migrated
# -----------------------
echo "📦 Verifying migrated buckets..."
docker run --rm --network host -e MC_HOST_minio2022 "${MC_IMAGE}" ls minio2022

# -----------------------
# Recreate IAM policies (MinIO doesn't auto-migrate these)
# -----------------------
echo "📋 Recreating IAM policies for 2022..."
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

docker run --rm --network host -e MC_HOST_minio2022 \
  -v /tmp/alpha-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add minio2022 alpha-rw /policy.json

docker run --rm --network host -e MC_HOST_minio2022 \
  -v /tmp/beta-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add minio2022 beta-rw /policy.json

# -----------------------
# Recreate users (2022 syntax: no policy in add command)
# -----------------------
echo "👥 Recreating users..."
docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" admin user add minio2022 "${ALICE_USER}" "${ALICE_PASS}"

docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" admin user add minio2022 "${BOB_USER}" "${BOB_PASS}"

# Create test user for multi-policy attachment
docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" admin user add minio2022 "${CHARLIE_USER}" "${CHARLIE_PASS}"

# -----------------------
# Attach policies to users (2022 syntax: separate step)
# -----------------------
echo "🔗 Attaching policies to users..."
docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" admin policy set minio2022 alpha-rw user="${ALICE_USER}"

docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" admin policy set minio2022 beta-rw user="${BOB_USER}"

# Attach MULTIPLE policies to charlie (testing multi-policy support)
# NOTE: Multiple policies must be comma-separated in a single call
echo "🧪 Testing multi-policy attachment for user '${CHARLIE_USER}'..."
docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" admin policy set minio2022 alpha-rw,beta-rw user="${CHARLIE_USER}"

# -----------------------
# Recreate public-read bucket policies
# -----------------------
echo "🌐 Recreating public-read policies..."
docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" anonymous set download minio2022/gamma

docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" anonymous set download minio2022/beta

# -----------------------
# List all objects to verify migration
# -----------------------
echo
echo "📋 Verifying migrated objects..."
echo
echo "Alpha bucket:"
docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" ls --recursive minio2022/alpha | head -20
echo
echo "Beta bucket:"
docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" ls --recursive minio2022/beta | head -20
echo
echo "Gamma bucket:"
docker run --rm --network host -e MC_HOST_minio2022 \
  "${MC_IMAGE}" ls --recursive minio2022/gamma | head -20

# -----------------------
# Check metadata migration
# -----------------------
echo
echo "🔍 Examining migrated metadata structure..."
echo
echo "Format file:"
cat "${DEST_DATA_DIR}/.minio.sys/format.json" 2>/dev/null || echo "(no format.json)"
echo
echo "Bucket metadata:"
find "${DEST_DATA_DIR}/.minio.sys/buckets" -name "*.json" 2>/dev/null | head -10 || true

# -----------------------
# Done
# -----------------------
echo
echo "✅ MinIO 2019→2022 import complete!"
echo
echo "Container: ${MINIO_CONTAINER}"
echo "Port: ${MINIO_PORT}"
echo "Data: ${DEST_DATA_DIR}"
echo
echo "Original 2019 data: ${SOURCE_DATA_DIR} (unchanged)"
echo "Migrated 2022 data: ${DEST_DATA_DIR} (format auto-upgraded by MinIO)"
echo
echo "Users (recreated for 2022 IAM):"
echo "  alice   / alicepass1234   → alpha-rw policy"
echo "  bob     / bobpass1234     → beta-rw policy"
echo "  charlie / charliepass1234 → alpha-rw + beta-rw (multi-policy test)"
echo
echo "Buckets (migrated from 2019):"
echo "  alpha → folder structure, metadata objects, simulated tagged objects"
echo "  beta  → public-read, copied objects"
echo "  gamma → public-read, large files"
echo
echo "Migrated objects include:"
echo "  - Basic objects (alice-object.bin, bob-object.bin, public-object.bin)"
echo "  - Folder structures (folder1/file1.txt, folder2/file1.txt, etc.)"
echo "  - Objects with standard metadata (Content-Type, Content-Encoding, Content-Language)"
echo "  - Objects with custom metadata (x-amz-meta-author, x-amz-meta-project, etc.)"
echo "  - Simulated tagged objects (using x-amz-meta-* from 2019)"
echo "  - Server-side copies (alice-copy.bin, copied-from-alpha.txt)"
echo "  - Large multipart uploads (large-file.dat - 10MB)"
echo
echo "What changed during migration:"
echo "  - Object data: unchanged (same files in same locations)"
echo "  - Bucket policies: migrated automatically"
echo "  - IAM users/policies: recreated manually (not auto-migrated)"
echo "  - Metadata format: upgraded from 2019 FS to 2022 FS format"
echo "  - .minio.sys structure: updated to 2022 layout"
echo
echo "2022 MC features now available (vs 2019):"
echo "  ✓ mc tag - object tagging support"
echo "  ✓ mc version enable - bucket versioning"
echo "  ✓ mc ilm - lifecycle management policies"
echo "  ✓ mc anonymous - improved public access control"
echo "  ✓ Better metadata syntax (semicolon separator, complex values)"
echo
echo "Next steps:"
echo "  1. Compare metadata formats: diff -r ${SOURCE_DATA_DIR}/.minio.sys ${DEST_DATA_DIR}/.minio.sys"
echo "  2. Test 2022 features (tagging, versioning) on migrated data"
echo "  3. Import both datasets into DirIO to test import compatibility"
echo "  4. Verify DirIO correctly handles both 2019 and 2022 metadata formats"
echo
echo "Running containers:"
echo "  - MinIO 2019: minio-2019 on port 9001 (if still running)"
echo "  - MinIO 2022 Import: ${MINIO_CONTAINER} on port ${MINIO_PORT}"
echo "  - MinIO 2022 Standalone: minio-old on port 9000 (if running)"
echo
echo "To stop: docker rm -f ${MINIO_CONTAINER}"
echo "To remove data: rm -rf ${DEST_DATA_DIR}"
