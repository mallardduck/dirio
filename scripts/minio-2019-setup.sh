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
#
# Usage:
#   ./minio-2019-setup.sh
#
# Optional environment variables:
#   OBJECT_SIZE          - Size of test objects in bytes (default: 65536)
#   SETUP_POLICY_TESTS   - Set to "true" to create advanced policy test scenarios (default: true)

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

OBJECT_SIZE="${OBJECT_SIZE:-65536}"
SETUP_POLICY_TESTS="${SETUP_POLICY_TESTS:-true}"

# Users
ALICE_USER="alice"
ALICE_PASS="alicepass1234"

BOB_USER="bob"
BOB_PASS="bobpass1234"

CHARLIE_USER="charlie"
CHARLIE_PASS="charliepass1234"

# -----------------------
# Validation
# -----------------------
# Check if docker is installed
if ! command -v docker &> /dev/null; then
  echo "❌ Error: Docker is not installed"
  echo ""
  echo "Install Docker:"
  echo "  Visit https://docs.docker.com/get-docker/"
  exit 1
fi

echo "✓ Docker found: $(which docker)"

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
echo "📦 Creating test buckets..."
for bucket in alpha beta gamma delta; do
  if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" ls "minio2019/${bucket}" >/dev/null 2>&1; then
    echo "  ⚠️  Bucket '${bucket}' already exists, skipping"
  else
    docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" mb "minio2019/${bucket}"
    echo "  ✓ Created bucket '${bucket}'"
  fi
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

# Check if alpha-rw policy exists, create if not
if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" admin policy info minio2019 alpha-rw >/dev/null 2>&1; then
  echo "  ⚠️  Policy 'alpha-rw' already exists, skipping"
elif docker run --rm --network host -e MC_HOST_minio2019 \
  -v /tmp/alpha-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add minio2019 alpha-rw /policy.json 2>/dev/null; then
  echo "  ✓ Created policy 'alpha-rw'"
else
  echo "  ⚠️  Failed to create policy 'alpha-rw'"
fi

# Check if beta-rw policy exists, create if not
if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" admin policy info minio2019 beta-rw >/dev/null 2>&1; then
  echo "  ⚠️  Policy 'beta-rw' already exists, skipping"
elif docker run --rm --network host -e MC_HOST_minio2019 \
  -v /tmp/beta-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add minio2019 beta-rw /policy.json 2>/dev/null; then
  echo "  ✓ Created policy 'beta-rw'"
else
  echo "  ⚠️  Failed to create policy 'beta-rw'"
fi

# Check if delta-rw policy exists, create if not
if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" admin policy info minio2019 delta-rw >/dev/null 2>&1; then
  echo "  ⚠️  Policy 'delta-rw' already exists, skipping"
elif docker run --rm --network host -e MC_HOST_minio2019 \
  -v /tmp/delta-rw.json:/policy.json \
  "${MC_IMAGE}" admin policy add minio2019 delta-rw /policy.json 2>/dev/null; then
  echo "  ✓ Created policy 'delta-rw'"
else
  echo "  ⚠️  Failed to create policy 'delta-rw'"
fi

# -----------------------
# Create users with policies (2019 syntax: requires POLICYNAME as 4th arg)
# -----------------------
echo "👥 Creating users with policies..."
# 2019 syntax: mc admin user add TARGET ACCESSKEY SECRETKEY POLICYNAME

# Check if alice user exists, create if not
if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" admin user info minio2019 "${ALICE_USER}" >/dev/null 2>&1; then
  echo "  ⚠️  User '${ALICE_USER}' already exists, skipping"
elif docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" admin user add minio2019 "${ALICE_USER}" "${ALICE_PASS}" alpha-rw 2>/dev/null; then
  echo "  ✓ Created user '${ALICE_USER}' with policy 'alpha-rw'"
else
  echo "  ⚠️  Failed to create user '${ALICE_USER}'"
fi

# Check if bob user exists, create if not
if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" admin user info minio2019 "${BOB_USER}" >/dev/null 2>&1; then
  echo "  ⚠️  User '${BOB_USER}' already exists, skipping"
elif docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" admin user add minio2019 "${BOB_USER}" "${BOB_PASS}" beta-rw 2>/dev/null; then
  echo "  ✓ Created user '${BOB_USER}' with policy 'beta-rw'"
else
  echo "  ⚠️  Failed to create user '${BOB_USER}'"
fi

# Check if charlie user exists, create if not
# Charlie gets delta-rw initially; because MinIO 2019 MC only supports one policy per user.
if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" admin user info minio2019 "${CHARLIE_USER}" >/dev/null 2>&1; then
  echo "  ⚠️  User '${CHARLIE_USER}' already exists, skipping"
elif docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" admin user add minio2019 "${CHARLIE_USER}" "${CHARLIE_PASS}" delta-rw 2>/dev/null; then
  echo "  ✓ Created user '${CHARLIE_USER}' with initial policy 'alpha-rw'"
else
  echo "  ⚠️  Failed to create user '${CHARLIE_USER}'"
fi

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
echo "📤 Uploading basic test objects..."
upload_object () {
  local bucket=$1
  local name=$2
  local quiet=${3:-false}

  tmpfile="$(mktemp)"
  head -c "${OBJECT_SIZE}" /dev/urandom > "${tmpfile}"

  docker run --rm --network host \
    -e MC_HOST_minio2019 \
    -v "${tmpfile}:/file.bin:ro" \
    "${MC_IMAGE}" \
    cp /file.bin "minio2019/${bucket}/${name}" >/dev/null 2>&1

  if [ "${quiet}" != "true" ]; then
    echo "  ✓ Uploaded ${bucket}/${name}"
  fi

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
# Advanced Features Testing
# -----------------------
echo "🔬 Testing advanced FS mode features..."

# Test versioning (if supported in 2019 FS mode)
echo "  Testing versioning..."
if docker run --rm --network host -e MC_HOST_minio2019 \
  "${MC_IMAGE}" version enable "minio2019/alpha" >/dev/null 2>&1; then
  echo "  ✓ Versioning enabled on bucket 'alpha'"
else
  echo "  ⚠️  Versioning not available in 2019 FS mode"
fi

# Upload object with custom metadata (2019 syntax: comma separator, simple values)
echo "  Testing custom metadata..."
tmpfile="$(mktemp)"
head -c "${OBJECT_SIZE}" /dev/urandom > "${tmpfile}"

# Note: 2019 MC has trouble with values containing '=' or complex syntax
# Use simple key=value pairs only
if docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/file.bin:ro" \
  "${MC_IMAGE}" \
  cp --attr "x-amz-meta-author=TestUser,x-amz-meta-project=DirIO,x-amz-meta-version=1" \
  /file.bin "minio2019/alpha/metadata-test.bin" >/dev/null 2>&1; then
  echo "  ✓ Uploaded alpha/metadata-test.bin (with custom metadata)"
else
  echo "  ⚠️  Failed to upload object with custom metadata"
fi

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
  cp /tagging-test.txt "minio2019/alpha/tagging-test.txt" >/dev/null 2>&1
echo "  ✓ Uploaded alpha/tagging-test.txt"
rm -f "${tmpfile}"

# Note: Object tagging (mc tag) was not available in 2019
# We'll use custom metadata instead to simulate tagging
echo "  ⚠️  Object tagging (mc tag) not available in 2019 - using custom metadata instead"
tmpfile="$(mktemp)"
echo "simulated tagged content" > "${tmpfile}"
if docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/tagged.txt:ro" \
  "${MC_IMAGE}" \
  cp --attr "x-amz-meta-environment=test,x-amz-meta-project=dirio,x-amz-meta-version=1.0" \
  /tagged.txt "minio2019/alpha/simulated-tagged.txt" >/dev/null 2>&1; then
  echo "  ✓ Uploaded alpha/simulated-tagged.txt (using metadata as tag simulation)"
else
  echo "  ⚠️  Failed to upload simulated tagged object"
fi
rm -f "${tmpfile}"

# -----------------------
# Multipart Upload (large file) Testing
# -----------------------
echo "📦 Uploading large file for multipart testing..."
# Create a 10MB file for multipart upload testing
largefile="$(mktemp)"
dd if=/dev/zero of="${largefile}" bs=1M count=10 2>/dev/null

if docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${largefile}:/large-file.dat:ro" \
  "${MC_IMAGE}" \
  cp /large-file.dat "minio2019/alpha/large-file.dat" >/dev/null 2>&1; then
  echo "  ✓ Uploaded alpha/large-file.dat (10MB, likely multipart)"
else
  echo "  ⚠️  Multipart upload failed (not supported in 2019 FS mode?)"
fi

if docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${largefile}:/large-file.dat:ro" \
  "${MC_IMAGE}" \
  cp /large-file.dat "minio2019/gamma/large-public.dat" >/dev/null 2>&1; then
  echo "  ✓ Uploaded gamma/large-public.dat (10MB, likely multipart)"
else
  echo "  ⚠️  Multipart upload failed (not supported in 2019 FS mode?)"
fi

rm -f "${largefile}"

# -----------------------
# Additional Metadata Variations (2019 syntax: comma separator, simple values)
# -----------------------
echo "🔖 Uploading objects with various metadata..."

# Object with Content-Type only
tmpfile="$(mktemp)"
echo '{"test": "json data"}' > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/data.json:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=application/json" \
  /data.json "minio2019/alpha/data.json" >/dev/null 2>&1
echo "  ✓ Uploaded alpha/data.json (Content-Type: application/json)"
rm -f "${tmpfile}"

# Object with Content-Type and custom metadata
tmpfile="$(mktemp)"
cat > "${tmpfile}" <<'HTML'
<!DOCTYPE html>
<html>
<head><title>DirIO Public Read Test</title></head>
<body>
  <h1>DirIO S3 Public Read &#10003;</h1>
  <p>If you can read this page anonymously, the public-read bucket policy is working.</p>
  <p><strong>Bucket:</strong> gamma &nbsp;|&nbsp; <strong>Object:</strong> index.html</p>
</body>
</html>
HTML
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/index.html:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=text/html,x-amz-meta-page=index" \
  /index.html "minio2019/gamma/index.html" >/dev/null 2>&1
echo "  ✓ Uploaded gamma/index.html (Content-Type: text/html, browser smoke-test page)"
rm -f "${tmpfile}"

# Object with Content-Encoding
tmpfile="$(mktemp)"
echo "compressed data" | gzip > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/data.gz:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=application/gzip,Content-Encoding=gzip" \
  /data.gz "minio2019/beta/data.gz" >/dev/null 2>&1
echo "  ✓ Uploaded beta/data.gz (Content-Encoding: gzip)"
rm -f "${tmpfile}"

# Object with multiple custom metadata fields (x-amz-meta-*)
tmpfile="$(mktemp)"
echo "user data" > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/userdata.txt:ro" \
  "${MC_IMAGE}" \
  cp --attr "x-amz-meta-user-id=12345,x-amz-meta-department=engineering,x-amz-meta-uploaded-by=alice" \
  /userdata.txt "minio2019/alpha/userdata.txt" >/dev/null 2>&1
echo "  ✓ Uploaded alpha/userdata.txt (multiple custom metadata fields)"
rm -f "${tmpfile}"

# Object with Content-Language
tmpfile="$(mktemp)"
echo "Bonjour le monde" > "${tmpfile}"
docker run --rm --network host \
  -e MC_HOST_minio2019 \
  -v "${tmpfile}:/french.txt:ro" \
  "${MC_IMAGE}" \
  cp --attr "Content-Type=text/plain,Content-Language=fr" \
  /french.txt "minio2019/alpha/french.txt" >/dev/null 2>&1
echo "  ✓ Uploaded alpha/french.txt (Content-Language: fr)"
rm -f "${tmpfile}"

# -----------------------
# Advanced Policy Test Scenarios (Phase 3.3)
# -----------------------
if [ "${SETUP_POLICY_TESTS}" = "true" ]; then
  echo ""
  echo "🔒 Setting up advanced policy test scenarios..."
  echo ""

  # -----------------------
  # 1. Conditional Policy Testing
  # -----------------------
  echo "📋 Creating buckets for conditional policy tests..."

  # Create policy-test buckets
  for bucket in policy-ip-test policy-time-test policy-string-test policy-numeric-test; do
    if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" ls "minio2019/${bucket}" >/dev/null 2>&1; then
      echo "  ⚠️  Bucket '${bucket}' already exists, skipping"
    else
      docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" mb "minio2019/${bucket}" >/dev/null 2>&1
      echo "  ✓ Created bucket '${bucket}'"
    fi
  done

  # Upload test objects to policy buckets
  echo "📤 Uploading objects for conditional policy tests..."
  upload_object policy-ip-test ip-restricted.txt true
  upload_object policy-time-test time-restricted.txt true
  upload_object policy-string-test useragent-restricted.txt true
  upload_object policy-numeric-test size-restricted.txt true
  echo "  ✓ Uploaded test objects to policy buckets"

  # Create example conditional policies
  echo "📝 Creating example conditional policy documents..."

  # IP-based condition policy
  cat > /tmp/policy-ip-condition.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject"],
      "Resource": ["arn:aws:s3:::policy-ip-test/*"],
      "Condition": {
        "IpAddress": {
          "aws:SourceIp": ["192.168.1.0/24", "10.0.0.0/8"]
        }
      }
    }
  ]
}
EOF
  echo "  ✓ Created policy-ip-condition.json (allows GetObject only from specific IPs)"

  # Date-based condition policy
  cat > /tmp/policy-time-condition.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject"],
      "Resource": ["arn:aws:s3:::policy-time-test/*"],
      "Condition": {
        "DateGreaterThan": {
          "aws:CurrentTime": "2026-01-01T00:00:00Z"
        },
        "DateLessThan": {
          "aws:CurrentTime": "2026-12-31T23:59:59Z"
        }
      }
    }
  ]
}
EOF
  echo "  ✓ Created policy-time-condition.json (allows GetObject only during 2026)"

  # String condition policy (UserAgent)
  cat > /tmp/policy-string-condition.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject"],
      "Resource": ["arn:aws:s3:::policy-string-test/*"],
      "Condition": {
        "StringLike": {
          "aws:UserAgent": ["aws-cli/*", "boto3/*"]
        }
      }
    }
  ]
}
EOF
  echo "  ✓ Created policy-string-condition.json (allows GetObject only for aws-cli/boto3)"

  # Numeric condition policy (object size)
  cat > /tmp/policy-numeric-condition.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:PutObject"],
      "Resource": ["arn:aws:s3:::policy-numeric-test/*"],
      "Condition": {
        "NumericLessThan": {
          "s3:content-length": 10485760
        }
      }
    }
  ]
}
EOF
  echo "  ✓ Created policy-numeric-condition.json (allows PutObject only for files < 10MB)"

  # -----------------------
  # 2. NotAction/NotResource/NotPrincipal Testing
  # -----------------------
  echo ""
  echo "📋 Creating buckets for NotAction/NotResource tests..."

  for bucket in policy-notaction-test policy-notresource-test; do
    if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" ls "minio2019/${bucket}" >/dev/null 2>&1; then
      echo "  ⚠️  Bucket '${bucket}' already exists, skipping"
    else
      docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" mb "minio2019/${bucket}" >/dev/null 2>&1
      echo "  ✓ Created bucket '${bucket}'"
    fi
  done

  # Upload test objects
  echo "📤 Uploading objects for NotAction/NotResource tests..."
  upload_object policy-notaction-test readonly.txt true
  upload_object policy-notresource-test protected-file.txt true
  upload_object policy-notresource-test unprotected-file.txt true
  echo "  ✓ Uploaded test objects"

  # Create NotAction policy (deny everything except GetObject)
  cat > /tmp/policy-notaction.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "NotAction": ["s3:DeleteObject", "s3:DeleteBucket"],
      "Resource": [
        "arn:aws:s3:::policy-notaction-test",
        "arn:aws:s3:::policy-notaction-test/*"
      ]
    }
  ]
}
EOF
  echo "  ✓ Created policy-notaction.json (allows everything except delete operations)"

  # Create NotResource policy (protect specific files)
  cat > /tmp/policy-notresource.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Deny",
      "Action": ["s3:DeleteObject"],
      "NotResource": ["arn:aws:s3:::policy-notresource-test/unprotected-*"]
    }
  ]
}
EOF
  echo "  ✓ Created policy-notresource.json (deny delete except for unprotected-* files)"

  # -----------------------
  # 3. Policy Variables Testing
  # -----------------------
  echo ""
  echo "📋 Creating buckets for policy variable tests..."

  if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" ls "minio2019/policy-variables-test" >/dev/null 2>&1; then
    echo "  ⚠️  Bucket 'policy-variables-test' already exists, skipping"
  else
    docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" mb "minio2019/policy-variables-test" >/dev/null 2>&1
    echo "  ✓ Created bucket 'policy-variables-test'"
  fi

  # Create user-specific folders
  echo "📤 Creating user-specific folder structure..."
  upload_object policy-variables-test alice/private-file.txt true
  upload_object policy-variables-test alice/data.json true
  upload_object policy-variables-test bob/private-file.txt true
  upload_object policy-variables-test bob/data.json true
  upload_object policy-variables-test shared/public-file.txt true
  echo "  ✓ Created user-specific folders"

  # Create policy with ${aws:username} variable
  cat > /tmp/policy-username-variable.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject"],
      "Resource": ["arn:aws:s3:::policy-variables-test/${aws:username}/*"]
    },
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject"],
      "Resource": ["arn:aws:s3:::policy-variables-test/shared/*"]
    }
  ]
}
EOF
  echo "  ✓ Created policy-username-variable.json (allows access to own prefix + shared)"

  # Create policy with multiple variables
  cat > /tmp/policy-multiple-variables.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject"],
      "Resource": ["arn:aws:s3:::policy-variables-test/*"],
      "Condition": {
        "IpAddress": {
          "aws:SourceIp": "${aws:SourceIp}"
        },
        "StringEquals": {
          "s3:prefix": "${aws:username}/"
        }
      }
    }
  ]
}
EOF
  echo "  ✓ Created policy-multiple-variables.json (combines variables in conditions)"

  # -----------------------
  # 4. ListBuckets/ListObjects Filtering Testing
  # -----------------------
  echo ""
  echo "📋 Creating buckets for result filtering tests..."

  for bucket in filter-alice-only filter-bob-only filter-shared; do
    if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" ls "minio2019/${bucket}" >/dev/null 2>&1; then
      echo "  ⚠️  Bucket '${bucket}' already exists, skipping"
    else
      docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" mb "minio2019/${bucket}" >/dev/null 2>&1
      echo "  ✓ Created bucket '${bucket}'"
    fi
  done

  # Upload objects to filtering test buckets
  echo "📤 Uploading objects for filtering tests..."
  for i in {1..20}; do
    upload_object filter-alice-only "alice-file-${i}.txt" true
  done
  echo "  ✓ Uploaded 20 objects to filter-alice-only"

  for i in {1..20}; do
    upload_object filter-bob-only "bob-file-${i}.txt" true
  done
  echo "  ✓ Uploaded 20 objects to filter-bob-only"

  for i in {1..20}; do
    upload_object filter-shared "shared-file-${i}.txt" true
  done
  echo "  ✓ Uploaded 20 objects to filter-shared"

  # Create bucket with mixed permissions (some objects readable, some not)
  if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" ls "minio2019/filter-mixed-perms" >/dev/null 2>&1; then
    echo "  ⚠️  Bucket 'filter-mixed-perms' already exists, skipping"
  else
    docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" mb "minio2019/filter-mixed-perms" >/dev/null 2>&1
    echo "  ✓ Created bucket 'filter-mixed-perms'"
  fi

  # Create objects with different prefixes for partial permissions
  echo "📤 Creating objects with different permission prefixes..."
  upload_object filter-mixed-perms public/file1.txt true
  upload_object filter-mixed-perms public/file2.txt true
  upload_object filter-mixed-perms private/file1.txt true
  upload_object filter-mixed-perms private/file2.txt true
  upload_object filter-mixed-perms restricted/file1.txt true
  upload_object filter-mixed-perms restricted/file2.txt true
  echo "  ✓ Created mixed-permission object structure"

  # Create policy for partial bucket access (prefix-based)
  cat > /tmp/policy-prefix-filter.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": ["arn:aws:s3:::filter-mixed-perms"],
      "Condition": {
        "StringLike": {
          "s3:prefix": ["public/*", ""]
        }
      }
    },
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject"],
      "Resource": ["arn:aws:s3:::filter-mixed-perms/public/*"]
    }
  ]
}
EOF
  echo "  ✓ Created policy-prefix-filter.json (allows listing/reading only public/* prefix)"

  # Create policy for partial bucket list access
  cat > /tmp/policy-bucket-filter.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:ListBucket", "s3:GetObject"],
      "Resource": [
        "arn:aws:s3:::filter-alice-only",
        "arn:aws:s3:::filter-alice-only/*",
        "arn:aws:s3:::filter-shared",
        "arn:aws:s3:::filter-shared/*"
      ]
    }
  ]
}
EOF
  echo "  ✓ Created policy-bucket-filter.json (alice can only see 2 of 4 filter buckets)"

  # -----------------------
  # 5. POST Policy Upload Testing
  # -----------------------
  echo ""
  echo "📋 Creating POST upload policy examples..."

  # Create bucket for POST uploads
  if docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" ls "minio2019/post-upload-test" >/dev/null 2>&1; then
    echo "  ⚠️  Bucket 'post-upload-test' already exists, skipping"
  else
    docker run --rm --network host -e MC_HOST_minio2019 "${MC_IMAGE}" mb "minio2019/post-upload-test" >/dev/null 2>&1
    echo "  ✓ Created bucket 'post-upload-test'"
  fi

  # Create example POST upload policy
  cat > /tmp/post-upload-policy.json <<'EOF'
{
  "expiration": "2026-12-31T23:59:59Z",
  "conditions": [
    {"bucket": "post-upload-test"},
    ["starts-with", "$key", "uploads/"],
    {"acl": "private"},
    ["content-length-range", 0, 10485760],
    ["starts-with", "$Content-Type", "image/"]
  ]
}
EOF
  echo "  ✓ Created post-upload-policy.json (example browser upload policy)"

  # Create HTML form example
  cat > /tmp/post-upload-form.html <<'EOF'
<!DOCTYPE html>
<html>
<head>
  <title>S3 POST Upload Example</title>
</head>
<body>
  <h1>S3 POST Upload Test</h1>
  <form action="http://localhost:9001/post-upload-test" method="post" enctype="multipart/form-data">
    <input type="hidden" name="key" value="uploads/${filename}">
    <input type="hidden" name="acl" value="private">
    <input type="hidden" name="Content-Type" value="image/jpeg">
    <input type="hidden" name="policy" value="BASE64_ENCODED_POLICY">
    <input type="hidden" name="x-amz-algorithm" value="AWS4-HMAC-SHA256">
    <input type="hidden" name="x-amz-credential" value="CREDENTIALS">
    <input type="hidden" name="x-amz-date" value="DATE">
    <input type="hidden" name="x-amz-signature" value="SIGNATURE">
    <input type="file" name="file" accept="image/*">
    <input type="submit" value="Upload">
  </form>
  <p>Note: This is an example form. Policy, credentials, and signature must be generated server-side.</p>
</body>
</html>
EOF
  echo "  ✓ Created post-upload-form.html (example HTML form for browser uploads)"

  # Create restrictive POST policy (size limits, content type)
  cat > /tmp/post-upload-restrictive.json <<'EOF'
{
  "expiration": "2026-12-31T23:59:59Z",
  "conditions": [
    {"bucket": "post-upload-test"},
    ["starts-with", "$key", "images/"],
    {"acl": "public-read"},
    ["content-length-range", 1024, 5242880],
    {"Content-Type": "image/jpeg"},
    {"x-amz-meta-uploaded-by": "browser-form"}
  ]
}
EOF
  echo "  ✓ Created post-upload-restrictive.json (restrictive POST policy with size/type limits)"

  # -----------------------
  # Summary of policy test artifacts
  # -----------------------
  echo ""
  echo "✅ Advanced policy test setup complete!"
  echo ""
  echo "📁 Policy Test Artifacts Created:"
  echo ""
  echo "Conditional Policy Examples:"
  echo "  - /tmp/policy-ip-condition.json (IP-based access)"
  echo "  - /tmp/policy-time-condition.json (time-based access)"
  echo "  - /tmp/policy-string-condition.json (UserAgent matching)"
  echo "  - /tmp/policy-numeric-condition.json (file size limits)"
  echo ""
  echo "NotAction/NotResource Examples:"
  echo "  - /tmp/policy-notaction.json (allow all except delete)"
  echo "  - /tmp/policy-notresource.json (protect specific files)"
  echo ""
  echo "Policy Variable Examples:"
  echo "  - /tmp/policy-username-variable.json (user-specific prefixes)"
  echo "  - /tmp/policy-multiple-variables.json (multiple variables)"
  echo ""
  echo "Result Filtering Examples:"
  echo "  - /tmp/policy-prefix-filter.json (prefix-based ListObjects filtering)"
  echo "  - /tmp/policy-bucket-filter.json (partial ListBuckets access)"
  echo ""
  echo "POST Upload Examples:"
  echo "  - /tmp/post-upload-policy.json (basic browser upload)"
  echo "  - /tmp/post-upload-restrictive.json (restrictive upload policy)"
  echo "  - /tmp/post-upload-form.html (HTML form example)"
  echo ""
  echo "📦 Test Buckets Created:"
  echo "  - policy-ip-test (for IP condition testing)"
  echo "  - policy-time-test (for date/time condition testing)"
  echo "  - policy-string-test (for string matching testing)"
  echo "  - policy-numeric-test (for numeric condition testing)"
  echo "  - policy-notaction-test (for NotAction testing)"
  echo "  - policy-notresource-test (for NotResource testing)"
  echo "  - policy-variables-test (for policy variable substitution)"
  echo "  - filter-alice-only (for ListBuckets filtering - alice only)"
  echo "  - filter-bob-only (for ListBuckets filtering - bob only)"
  echo "  - filter-shared (for ListBuckets filtering - shared)"
  echo "  - filter-mixed-perms (for ListObjects prefix-based filtering)"
  echo "  - post-upload-test (for POST upload testing)"
  echo ""
else
  echo ""
  echo "⏭️  Skipping advanced policy tests (set SETUP_POLICY_TESTS=true to enable)"
fi

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
echo "  alice   / alicepass1234   → alpha-rw (single policy)"
echo "  bob     / bobpass1234     → beta-rw  (single policy)"
echo "  charlie / charliepass1234 → delta-rw (single policy in 2019; setup for multi-policy in 2022 update)"
echo
echo "Buckets:"
echo "  alpha → folder structure, metadata objects, simulated tagged objects"
echo "  beta  → public-read, copied objects"
echo "  gamma → public-read, large files"
echo "  delta → charlie's private bucket (multi-policy test)"
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
echo "Environment variables:"
echo "  OBJECT_SIZE=${OBJECT_SIZE}       # Test object size in bytes"
echo "  SETUP_POLICY_TESTS=${SETUP_POLICY_TESTS}  # Advanced policy test scenarios"
echo
echo "Next steps:"
echo "  1. Examine ${DATA_DIR}/.minio.sys/ to see what metadata was created"
echo "  2. Compare with modern MinIO data to identify differences"
echo "  3. Import this data into DirIO to test compatibility"
echo "  4. Run DirIO S3 API tests against imported data to verify correctness"
echo
echo "To setup advanced policy test scenarios (Phase 3.3):"
echo "  SETUP_POLICY_TESTS=true ./$(basename "$0")"
echo
echo "To stop: docker rm -f ${MINIO_CONTAINER}"
