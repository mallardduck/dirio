#!/usr/bin/env bash
set -euo pipefail

# -----------------------
# Config
# -----------------------
MINIO_IMAGE="minio/minio:RELEASE.2022-10-24T18-35-07Z"
MC_IMAGE="minio/mc:RELEASE.2022-10-22T03-39-29Z"

MINIO_CONTAINER="minio-old"
MINIO_ROOT_USER="minioadmin"
MINIO_ROOT_PASSWORD="minioadmin"
MINIO_PORT="9000"
DATA_DIR="$(pwd)/minio-data"

# Buckets + files to create
BUCKETS=("alpha" "beta" "gamma")

# Object size (64 KiB)
OBJECT_SIZE=65536

# -----------------------
# Cleanup (idempotent)
# -----------------------
docker rm -f "${MINIO_CONTAINER}" >/dev/null 2>&1 || true
rm -rf "${DATA_DIR}"
mkdir -p "${DATA_DIR}/.minio.sys"

# -----------------------
# Force legacy FS mode
# -----------------------
cat > "${DATA_DIR}/.minio.sys/format.json" <<'EOF'
{"version":"1","format":"fs","id":"avoid-going-into-snsd-mode-legacy-is-fine-with-me","fs":{"version":"2"}}
EOF

# -----------------------
# Start MinIO (legacy FS mode)
# -----------------------
docker run -d \
  --name "${MINIO_CONTAINER}" \
  -p "${MINIO_PORT}:9000" \
  -e MINIO_ROOT_USER="${MINIO_ROOT_USER}" \
  -e MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD}" \
  -v "${DATA_DIR}:/data" \
  "${MINIO_IMAGE}" server /data

echo "⏳ Waiting for MinIO to be ready..."
sleep 5

# -----------------------
# mc auth via env
# -----------------------
export MC_HOST_local="http://${MINIO_ROOT_USER}:${MINIO_ROOT_PASSWORD}@127.0.0.1:${MINIO_PORT}"

# -----------------------
# Create buckets + files
# -----------------------
for bucket in "${BUCKETS[@]}"; do
  echo "📦 Creating bucket: ${bucket}"
  docker run --rm --network host \
    -e MC_HOST_local \
    "${MC_IMAGE}" mb "local/${bucket}"

  for i in {1..3}; do
    tmpfile="$(mktemp)"
    head -c "${OBJECT_SIZE}" /dev/urandom > "${tmpfile}"

    docker run --rm --network host \
      -e MC_HOST_local \
      -v "${tmpfile}:/file.bin:ro" \
      "${MC_IMAGE}" \
      cp /file.bin "local/${bucket}/object-${i}.bin"

    rm -f "${tmpfile}"
  done
done

# -----------------------
# Done
# -----------------------
echo
echo "✅ MinIO running in *legacy FS mode*"
echo "   Endpoint: http://localhost:${MINIO_PORT}"
echo "   Data dir: ${DATA_DIR}"
echo
echo "🧪 Buckets:"
docker run --rm --network host \
  -e MC_HOST_local \
  "${MC_IMAGE}" ls local
