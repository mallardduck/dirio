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
docker rm -f "${MINIO_CONTAINER}" >/dev/null 2>&1 || true
rm -rf "${DATA_DIR}"
mkdir -p "${DATA_DIR}/.minio.sys"

# -----------------------
# Force legacy FS mode
# -----------------------
cat > "${DATA_DIR}/.minio.sys/format.json" <<'EOF'
{"version":"1","format":"fs","id":"legacy-fs-mode","fs":{"version":"2"}}
EOF

# -----------------------
# Start MinIO
# -----------------------
docker run -d \
  --name "${MINIO_CONTAINER}" \
  -p "${MINIO_PORT}:9000" \
  -e MINIO_ROOT_USER="${MINIO_ROOT_USER}" \
  -e MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD}" \
  -v "${DATA_DIR}:/data" \
  "${MINIO_IMAGE}" server /data

echo "⏳ Waiting for MinIO..."
sleep 5

# -----------------------
# mc root auth
# -----------------------
export MC_HOST_local="http://${MINIO_ROOT_USER}:${MINIO_ROOT_PASSWORD}@127.0.0.1:${MINIO_PORT}"

# -----------------------
# Create buckets
# -----------------------
for bucket in alpha beta gamma; do
  docker run --rm --network host -e MC_HOST_local "${MC_IMAGE}" mb "local/${bucket}"
done

# -----------------------
# Create users
# -----------------------
docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin user add local "${ALICE_USER}" "${ALICE_PASS}"

docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin user add local "${BOB_USER}" "${BOB_PASS}"

# -----------------------
# Create policies
# -----------------------
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

docker run --rm --network host -e MC_HOST_local \
  -v /tmp/alpha-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add local alpha-rw /policy.json

docker run --rm --network host -e MC_HOST_local \
  -v /tmp/beta-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add local beta-rw /policy.json

# -----------------------
# Attach policies
# -----------------------
docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin policy set local alpha-rw user="${ALICE_USER}"

docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin policy set local beta-rw user="${BOB_USER}"

# -----------------------
# Public-read buckets
# -----------------------
docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" anonymous set download local/gamma

docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" anonymous set download local/beta

# -----------------------
# Upload objects (after users/policies)
# -----------------------
upload_object () {
  local bucket=$1
  local name=$2

  tmpfile="$(mktemp)"
  head -c "${OBJECT_SIZE}" /dev/urandom > "${tmpfile}"

  docker run --rm --network host \
    -e MC_HOST_local \
    -v "${tmpfile}:/file.bin:ro" \
    "${MC_IMAGE}" \
    cp /file.bin "local/${bucket}/${name}"

  rm -f "${tmpfile}"
}

upload_object alpha alice-object.bin
upload_object beta bob-object.bin
upload_object gamma public-object.bin

# -----------------------
# Done
# -----------------------
echo
echo "✅ Advanced legacy FS MinIO setup complete"
echo
echo "Users:"
echo "  alice / alicepass → RW on bucket alpha"
echo "  bob   / bobpass   → RW on bucket beta"
echo
echo "Buckets:"
echo "  alpha → private (alice)"
echo "  beta  → private (bob)"
echo "  gamma → public-read (anonymous)"
echo
echo "Disk layout example:"
echo "  minio-data/alpha/alice-object.bin"
echo "  minio-data/beta/bob-object.bin"
echo "  minio-data/gamma/public-object.bin"
