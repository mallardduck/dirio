#!/usr/bin/env bash
set -euo pipefail

# -----------------------
# Benchmark Data Seeding Script
# -----------------------
# Seeds a bucket with a large number of objects across several prefix patterns,
# providing realistic data for ListObjects performance profiling.
#
# Usage:
#   S3_ENDPOINT=http://localhost:9000 \
#   S3_ACCESS_KEY=dirio-admin \
#   S3_SECRET_KEY=dirio-admin-secret \
#   ./seed-large-bucket.sh
#
# Optional environment variables:
#   S3_ALIAS        - mc alias name (default: "bench")
#   BUCKET_NAME     - Bucket to create and seed (default: "bench-large")
#   OBJECT_COUNT    - Total number of objects to create (default: 10000)
#   OBJECT_SIZE     - Size of each object in bytes (default: 1024)
#   PARALLELISM     - mc upload concurrency (default: 20)
#   SKIP_CLEANUP    - Set to "true" to keep temp files after seeding (default: false)

# -----------------------
# Config
# -----------------------
S3_ALIAS="${S3_ALIAS:-bench}"
S3_ENDPOINT="${S3_ENDPOINT:-}"
S3_ACCESS_KEY="${S3_ACCESS_KEY:-}"
S3_SECRET_KEY="${S3_SECRET_KEY:-}"
BUCKET_NAME="${BUCKET_NAME:-bench-large}"
OBJECT_COUNT="${OBJECT_COUNT:-10000}"
OBJECT_SIZE="${OBJECT_SIZE:-1024}"
PARALLELISM="${PARALLELISM:-20}"
SKIP_CLEANUP="${SKIP_CLEANUP:-false}"

# -----------------------
# Validation
# -----------------------
if [ -z "${S3_ENDPOINT}" ]; then
  echo "❌ Error: S3_ENDPOINT is required"
  echo ""
  echo "Usage:"
  echo "  S3_ENDPOINT=http://localhost:9000 \\"
  echo "  S3_ACCESS_KEY=dirio-admin \\"
  echo "  S3_SECRET_KEY=dirio-admin-secret \\"
  echo "  $0"
  echo ""
  echo "Optional variables:"
  echo "  S3_ALIAS=bench          # mc alias name"
  echo "  BUCKET_NAME=bench-large # bucket to seed"
  echo "  OBJECT_COUNT=10000      # total objects to create"
  echo "  OBJECT_SIZE=1024        # bytes per object (default: 1KB)"
  echo "  PARALLELISM=20          # upload concurrency"
  exit 1
fi

if [ -z "${S3_ACCESS_KEY}" ] || [ -z "${S3_SECRET_KEY}" ]; then
  echo "❌ Error: S3_ACCESS_KEY and S3_SECRET_KEY are required"
  exit 1
fi

if ! command -v mc &>/dev/null; then
  echo "❌ Error: mc (MinIO client) is required but not found in PATH"
  echo "  Install: https://min.io/docs/minio/linux/reference/minio-mc.html"
  exit 1
fi

# -----------------------
# Prefix distribution
# -----------------------
# Objects are split across four prefix patterns to stress different
# listing scenarios: flat, two independent prefixes, and deep nesting.
#
#   flat/          40% — no hierarchy, baseline scan
#   prefix-a/      20% — isolated prefix filter
#   prefix-b/      20% — isolated prefix filter
#   deep/a/b/c/    20% — deep hierarchy, delimiter grouping

FLAT_COUNT=$(( OBJECT_COUNT * 40 / 100 ))
ALPHA_COUNT=$(( OBJECT_COUNT * 20 / 100 ))
BETA_COUNT=$(( OBJECT_COUNT * 20 / 100 ))
DEEP_COUNT=$(( OBJECT_COUNT - FLAT_COUNT - ALPHA_COUNT - BETA_COUNT ))

echo "========================================="
echo "  DirIO Benchmark Seeding"
echo "========================================="
echo "  Endpoint:    ${S3_ENDPOINT}"
echo "  Bucket:      ${BUCKET_NAME}"
echo "  Total objs:  ${OBJECT_COUNT}"
echo "  Object size: ${OBJECT_SIZE} bytes"
echo "  Parallelism: ${PARALLELISM}"
echo ""
echo "  Prefix breakdown:"
echo "    flat/         ${FLAT_COUNT} objects"
echo "    prefix-a/     ${ALPHA_COUNT} objects"
echo "    prefix-b/     ${BETA_COUNT} objects"
echo "    deep/a/b/c/   ${DEEP_COUNT} objects"
echo "========================================="

# -----------------------
# Set up mc alias
# -----------------------
mc alias set "${S3_ALIAS}" "${S3_ENDPOINT}" "${S3_ACCESS_KEY}" "${S3_SECRET_KEY}" >/dev/null 2>&1
echo "✅ mc alias configured"

# -----------------------
# Create bucket
# -----------------------
if mc ls "${S3_ALIAS}/${BUCKET_NAME}" >/dev/null 2>&1; then
  echo "ℹ️  Bucket '${BUCKET_NAME}' already exists — seeding into existing bucket"
else
  mc mb "${S3_ALIAS}/${BUCKET_NAME}" >/dev/null
  echo "✅ Bucket '${BUCKET_NAME}' created"
fi

# -----------------------
# Build local temp directory
# -----------------------
TMPDIR_SEED=$(mktemp -d)
if [ "${SKIP_CLEANUP}" != "true" ]; then
  trap "rm -rf ${TMPDIR_SEED}" EXIT
fi

echo ""
echo "📁 Generating ${OBJECT_COUNT} seed files (${OBJECT_SIZE} bytes each)..."

# Create one random seed file and copy it — avoids N calls to dd/urandom.
SEED_FILE="${TMPDIR_SEED}/_seed"
dd if=/dev/urandom of="${SEED_FILE}" bs="${OBJECT_SIZE}" count=1 2>/dev/null

seed_prefix() {
  local dir="$1"
  local count="$2"
  mkdir -p "${TMPDIR_SEED}/${dir}"
  for i in $(seq -f "%07g" 1 "${count}"); do
    cp "${SEED_FILE}" "${TMPDIR_SEED}/${dir}/obj-${i}"
  done
  echo "  ✓ ${count} files prepared in ${dir}/"
}

seed_prefix "flat"        "${FLAT_COUNT}"
seed_prefix "prefix-a"   "${ALPHA_COUNT}"
seed_prefix "prefix-b"   "${BETA_COUNT}"

# Deep nesting: split evenly across four sub-paths so delimiter tests
# exercise grouping at the third level too.
DEEP_EACH=$(( DEEP_COUNT / 4 ))
DEEP_REM=$(( DEEP_COUNT - DEEP_EACH * 4 ))
seed_prefix "deep/a/b/c" "${DEEP_EACH}"
seed_prefix "deep/a/b/d" "${DEEP_EACH}"
seed_prefix "deep/a/x"   "${DEEP_EACH}"
seed_prefix "deep/b"     $(( DEEP_EACH + DEEP_REM ))

echo ""
echo "⬆️  Uploading to ${S3_ALIAS}/${BUCKET_NAME} (parallelism=${PARALLELISM})..."

upload_prefix() {
  local dir="$1"
  local remote="$2"
  mc cp --recursive "${TMPDIR_SEED}/${dir}/" "${S3_ALIAS}/${BUCKET_NAME}/${remote}" \
    --disable-multipart \
    --quiet \
    --concurrent-upload-parts "${PARALLELISM}"
  echo "  ✓ ${dir}/ → ${remote}"
}

upload_prefix "flat"        "flat/"
upload_prefix "prefix-a"   "prefix-a/"
upload_prefix "prefix-b"   "prefix-b/"
upload_prefix "deep"        "deep/"

echo ""
echo "✅ Seeding complete."
echo ""
echo "Verify:"
echo "  mc ls --recursive ${S3_ALIAS}/${BUCKET_NAME} | wc -l"
echo ""
echo "Start profiling:"
echo "  task run-profile"
echo "  # in another terminal:"
echo "  go tool pprof http://localhost:9000/debug/pprof/heap"
echo "  go tool pprof http://localhost:9000/debug/pprof/profile   # 30s CPU sample"