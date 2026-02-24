#!/usr/bin/env bash
set -euo pipefail

# -----------------------
# Config
# -----------------------
MINIO_IMAGE="minio/minio:RELEASE.2022-10-24T18-35-07Z"
MC_IMAGE="minio/mc:RELEASE.2022-10-22T03-39-29Z"

MINIO_CONTAINER="minio-2022"
MINIO_ROOT_USER="minioadmin"
MINIO_ROOT_PASSWORD="minioadmin"
MINIO_PORT="9004"
DATA_DIR="$(pwd)/minio-data-2022"

OBJECT_SIZE=65536

# Users
ALICE_USER="alice"
ALICE_PASS="alicepass1234"

BOB_USER="bob"
BOB_PASS="bobpass1234"

CHARLIE_USER="charlie"
CHARLIE_PASS="charliepass1234"

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
validate_secret "${CHARLIE_USER}" "${CHARLIE_PASS}"

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
  -p "$((MINIO_PORT + 1)):$((MINIO_PORT + 1))" \
  -e MINIO_ROOT_USER="${MINIO_ROOT_USER}" \
  -e MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD}" \
  -v "${DATA_DIR}:/data" \
  "${MINIO_IMAGE}" server /data --console-address ":$((MINIO_PORT + 1))"

echo "⏳ Waiting for MinIO..."
sleep 5

# -----------------------
# mc root auth
# -----------------------
export MC_HOST_local="http://${MINIO_ROOT_USER}:${MINIO_ROOT_PASSWORD}@127.0.0.1:${MINIO_PORT}"

# -----------------------
# Create buckets
# -----------------------
for bucket in alpha beta gamma delta; do
  docker run --rm --network host -e MC_HOST_local "${MC_IMAGE}" mb "local/${bucket}"
done

# -----------------------
# Create users
# -----------------------
docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin user add local "${ALICE_USER}" "${ALICE_PASS}"

docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin user add local "${BOB_USER}" "${BOB_PASS}"

docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin user add local "${CHARLIE_USER}" "${CHARLIE_PASS}"

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

cat > /tmp/delta-rw.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:*"],
      "Resource": [
        "arn:aws:s3:::delta",
        "arn:aws:s3:::delta/*"
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

docker run --rm --network host -e MC_HOST_local \
  -v /tmp/delta-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add local delta-rw /policy.json

# -----------------------
# Attach policies
# -----------------------
docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin policy set local alpha-rw user="${ALICE_USER}"

docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin policy set local beta-rw user="${BOB_USER}"

# charlie: delta-rw directly, alpha-rw via alpha-users group (multi-policy test)
docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin policy set local delta-rw user="${CHARLIE_USER}"

docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin group add local alpha-users "${CHARLIE_USER}"

docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" admin policy set local alpha-rw group=alpha-users

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
upload_object delta charlie-object.bin

# -----------------------
# Folder Structure Objects (for delimiter/prefix testing)
# -----------------------
echo "📁 Creating folder structure for ListObjects testing..."
upload_object alpha folder1/file1.txt
upload_object alpha folder1/file2.txt
upload_object alpha folder1/subfolder/deep.txt
upload_object alpha folder2/file1.txt
upload_object alpha root-file.txt

upload_object beta prefix/test.txt
upload_object beta prefix/data/nested.txt
upload_object beta other/file.txt

# -----------------------
# Objects with custom x-amz-meta-* metadata
# -----------------------
echo "🔬 Uploading objects with custom metadata..."

# Custom metadata (author, project, version)
tmpfile="$(mktemp)"
head -c "${OBJECT_SIZE}" /dev/urandom > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_local \
  -v "${tmpfile}:/file.bin:ro" \
  "${MC_IMAGE}" \
  cp --attr "x-amz-meta-author=TestUser;x-amz-meta-project=DirIO;x-amz-meta-version=1" \
  /file.bin "local/alpha/metadata-test.bin" >/dev/null 2>&1
echo "  ✓ Uploaded alpha/metadata-test.bin (custom x-amz-meta-* fields)"
rm -f "${tmpfile}"

# Simulated tagging via metadata (mc tag not in 2022 FS mode for this dataset)
tmpfile="$(mktemp)"
echo "simulated tagged content" > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_local \
  -v "${tmpfile}:/tagged.txt:ro" \
  "${MC_IMAGE}" \
  cp --attr "x-amz-meta-environment=test;x-amz-meta-project=dirio;x-amz-meta-version=1.0" \
  /tagged.txt "local/alpha/simulated-tagged.txt" >/dev/null 2>&1
echo "  ✓ Uploaded alpha/simulated-tagged.txt (metadata-simulated tags)"
rm -f "${tmpfile}"

# Multiple custom metadata fields
tmpfile="$(mktemp)"
echo "user data" > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_local \
  -v "${tmpfile}:/userdata.txt:ro" \
  "${MC_IMAGE}" \
  cp --attr "x-amz-meta-user-id=12345;x-amz-meta-department=engineering;x-amz-meta-uploaded-by=alice" \
  /userdata.txt "local/alpha/userdata.txt" >/dev/null 2>&1
echo "  ✓ Uploaded alpha/userdata.txt (multiple custom metadata fields)"
rm -f "${tmpfile}"

# -----------------------
# Objects with standard content headers
# -----------------------
echo "🔖 Uploading objects with content headers..."

# JSON content type
tmpfile="$(mktemp)"
echo '{"test": "json data"}' > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_local \
  -v "${tmpfile}:/data.json:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=application/json" \
  /data.json "local/alpha/data.json" >/dev/null 2>&1
echo "  ✓ Uploaded alpha/data.json (Content-Type: application/json)"
rm -f "${tmpfile}"

# Gzip content encoding
tmpfile="$(mktemp)"
echo "compressed data" | gzip > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_local \
  -v "${tmpfile}:/data.gz:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=application/gzip;Content-Encoding=gzip" \
  /data.gz "local/beta/data.gz" >/dev/null 2>&1
echo "  ✓ Uploaded beta/data.gz (Content-Type: application/gzip, Content-Encoding: gzip)"
rm -f "${tmpfile}"

# Content-Language
tmpfile="$(mktemp)"
echo "Bonjour le monde" > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_local \
  -v "${tmpfile}:/french.txt:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=text/plain;Content-Language=fr" \
  /french.txt "local/alpha/french.txt" >/dev/null 2>&1
echo "  ✓ Uploaded alpha/french.txt (Content-Language: fr)"
rm -f "${tmpfile}"

# -----------------------
# Large file (multipart upload)
# -----------------------
echo "📦 Uploading large file for multipart testing..."
largefile="$(mktemp)"
dd if=/dev/zero of="${largefile}" bs=1M count=10 2>/dev/null
docker run --rm --network host \
  -e MC_HOST_local \
  -v "${largefile}:/large-file.dat:ro" \
  "${MC_IMAGE}" \
  cp /large-file.dat "local/alpha/large-file.dat" >/dev/null 2>&1
echo "  ✓ Uploaded alpha/large-file.dat (10MB)"
rm -f "${largefile}"

# -----------------------
# Server-side copies (CopyObject)
# -----------------------
echo "📋 Testing CopyObject (server-side copy)..."
docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" \
  cp "local/alpha/alice-object.bin" "local/alpha/alice-copy.bin" || \
  echo "  ⚠️  Same-bucket copy failed"

docker run --rm --network host -e MC_HOST_local \
  "${MC_IMAGE}" \
  cp "local/alpha/folder1/file1.txt" "local/beta/copied-from-alpha.txt" || \
  echo "  ⚠️  Cross-bucket copy failed"
echo "  ✓ Server-side copies complete"

# -----------------------
# Additional public objects (gamma — anonymous-accessible)
# -----------------------
tmpfile="$(mktemp)"
cat > "${tmpfile}" <<'HTML'
<!DOCTYPE html>
<html>
<head><title>DirIO Public Read Test</title></head>
<body>
  <h1>DirIO S3 Public Read ✓</h1>
  <p>If you can read this page anonymously, the public-read bucket policy is working.</p>
  <p><strong>Bucket:</strong> gamma &nbsp;|&nbsp; <strong>Object:</strong> index.html</p>
</body>
</html>
HTML
docker run --rm --network host \
  -e MC_HOST_local \
  -v "${tmpfile}:/index.html:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=text/html" /index.html "local/gamma/index.html"
echo "  ✓ Uploaded gamma/index.html (Content-Type: text/html, browser smoke-test page)"
rm -f "${tmpfile}"

largefile="$(mktemp)"
dd if=/dev/zero of="${largefile}" bs=1M count=10 2>/dev/null
docker run --rm --network host \
  -e MC_HOST_local \
  -v "${largefile}:/large-public.dat:ro" \
  "${MC_IMAGE}" \
  cp /large-public.dat "local/gamma/large-public.dat"
echo "  ✓ Uploaded gamma/large-public.dat (10MB, anonymously readable)"
rm -f "${largefile}"

# -----------------------
# Done
# -----------------------
echo
echo "✅ Advanced legacy FS MinIO setup complete"
echo
echo "Users:"
echo "  alice   / alicepass1234   → alpha-rw (direct)"
echo "  bob     / bobpass1234     → beta-rw (direct)"
echo "  charlie / charliepass1234 → delta-rw (direct) + alpha-rw (via alpha-users group)"
echo
echo "Buckets:"
echo "  alpha → private (alice + charlie via group)"
echo "  beta  → private (bob)"
echo "  gamma → public-read (anonymous)"
echo "  delta → private (charlie)"
echo
echo "Objects created:"
echo "  alpha → alice-object.bin, alice-copy.bin, folder1/file1.txt, folder1/file2.txt,"
echo "          folder1/subfolder/deep.txt, folder2/file1.txt, root-file.txt,"
echo "          metadata-test.bin (x-amz-meta-*), simulated-tagged.txt (x-amz-meta-*),"
echo "          userdata.txt (x-amz-meta-*), data.json (Content-Type), french.txt (Content-Language),"
echo "          large-file.dat (10MB)"
echo "  beta  → bob-object.bin, prefix/test.txt, prefix/data/nested.txt, other/file.txt,"
echo "          data.gz (Content-Encoding: gzip), copied-from-alpha.txt (server-side copy)"
echo "  gamma → public-object.bin, index.html (Content-Type: text/html), large-public.dat (10MB)"
echo "  delta → charlie-object.bin"
