#!/usr/bin/env bash
set -euo pipefail

# -----------------------
# Generic S3 API Setup Script
# -----------------------
# This script sets up test data on ANY S3-compatible API endpoint.
# It uses the MinIO client (mc) which works with any S3 API.
#
# Usage:
#   S3_ENDPOINT=http://localhost:8080 \
#   S3_ACCESS_KEY=admin \
#   S3_SECRET_KEY=password123 \
#   ./s3-generic-setup.sh
#
# Optional environment variables:
#   S3_ALIAS       - mc alias name (default: "target")
#   S3_REGION      - AWS region if needed (default: "us-east-1")
#   OBJECT_SIZE    - Size of test objects in bytes (default: 65536)
#   SKIP_USERS     - Set to "true" to skip user/policy creation (default: false)
#   SKIP_POLICIES  - Set to "true" to skip bucket policies (default: false)

# -----------------------
# Config
# -----------------------
S3_ALIAS="${S3_ALIAS:-target}"
S3_ENDPOINT="${S3_ENDPOINT:-}"
S3_ACCESS_KEY="${S3_ACCESS_KEY:-}"
S3_SECRET_KEY="${S3_SECRET_KEY:-}"
S3_REGION="${S3_REGION:-us-east-1}"
OBJECT_SIZE="${OBJECT_SIZE:-65536}"
SKIP_USERS="${SKIP_USERS:-false}"
SKIP_POLICIES="${SKIP_POLICIES:-false}"

# Users (for IAM creation, if supported)
ALICE_USER="alice"
ALICE_PASS="alicepass1234"
BOB_USER="bob"
BOB_PASS="bobpass1234"

# -----------------------
# Validation
# -----------------------
if [ -z "${S3_ENDPOINT}" ]; then
  echo "❌ Error: S3_ENDPOINT is required"
  echo ""
  echo "Usage:"
  echo "  S3_ENDPOINT=http://localhost:8080 \\"
  echo "  S3_ACCESS_KEY=admin \\"
  echo "  S3_SECRET_KEY=password123 \\"
  echo "  $0"
  echo ""
  echo "Optional variables:"
  echo "  S3_ALIAS=target        # mc alias name"
  echo "  S3_REGION=us-east-1    # AWS region"
  echo "  OBJECT_SIZE=65536      # Test object size in bytes"
  echo "  SKIP_USERS=true        # Skip IAM user creation"
  echo "  SKIP_POLICIES=true     # Skip bucket policy creation"
  exit 1
fi

if [ -z "${S3_ACCESS_KEY}" ]; then
  echo "❌ Error: S3_ACCESS_KEY is required"
  exit 1
fi

if [ -z "${S3_SECRET_KEY}" ]; then
  echo "❌ Error: S3_SECRET_KEY is required"
  exit 1
fi

# Check if mc is installed
if ! command -v mc &> /dev/null; then
  echo "❌ Error: MinIO client 'mc' is not installed"
  echo ""
  echo "Install mc:"
  echo "  # macOS"
  echo "  brew install minio/stable/mc"
  echo ""
  echo "  # Linux"
  echo "  wget https://dl.min.io/client/mc/release/linux-amd64/mc"
  echo "  chmod +x mc"
  echo "  sudo mv mc /usr/local/bin/"
  echo ""
  echo "  # Windows"
  echo "  # Download from https://dl.min.io/client/mc/release/windows-amd64/mc.exe"
  exit 1
fi

echo "✓ MinIO client (mc) found: $(which mc)"

# -----------------------
# Setup mc alias
# -----------------------
echo "🔧 Configuring mc alias '${S3_ALIAS}'..."
mc alias set "${S3_ALIAS}" "${S3_ENDPOINT}" "${S3_ACCESS_KEY}" "${S3_SECRET_KEY}" --api S3v4

# Test connection
echo "🔌 Testing connection..."
if ! mc ls "${S3_ALIAS}" >/dev/null 2>&1; then
  echo "❌ Failed to connect to S3 endpoint"
  echo "   Endpoint: ${S3_ENDPOINT}"
  echo "   Alias: ${S3_ALIAS}"
  mc ls "${S3_ALIAS}" || true
  exit 1
fi

echo "✓ Connected to ${S3_ENDPOINT}"

# -----------------------
# Create buckets
# -----------------------
echo "📦 Creating test buckets..."
for bucket in alpha beta gamma; do
  if mc ls "${S3_ALIAS}/${bucket}" >/dev/null 2>&1; then
    echo "  ⚠️  Bucket '${bucket}' already exists, skipping"
  else
    mc mb "${S3_ALIAS}/${bucket}" --region="${S3_REGION}"
    echo "  ✓ Created bucket '${bucket}'"
  fi
done

# -----------------------
# Create IAM users and policies (if not skipped)
# -----------------------
if [ "${SKIP_USERS}" != "true" ]; then
  echo "👥 Creating IAM users and policies..."
  echo "  (This may fail if the S3 API doesn't support IAM)"

  # Create policies
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

  # Try to create policies (may fail on non-MinIO S3 APIs)
  if mc admin policy add "${S3_ALIAS}" alpha-rw /tmp/alpha-rw.json 2>/dev/null; then
    echo "  ✓ Created policy 'alpha-rw'"
  else
    echo "  ⚠️  Failed to create policy 'alpha-rw' (IAM not supported?)"
  fi

  if mc admin policy add "${S3_ALIAS}" beta-rw /tmp/beta-rw.json 2>/dev/null; then
    echo "  ✓ Created policy 'beta-rw'"
  else
    echo "  ⚠️  Failed to create policy 'beta-rw' (IAM not supported?)"
  fi

  # Try to create users (may fail on non-MinIO S3 APIs)
  if mc admin user add "${S3_ALIAS}" "${ALICE_USER}" "${ALICE_PASS}" 2>/dev/null; then
    echo "  ✓ Created user '${ALICE_USER}'"
    mc admin policy set "${S3_ALIAS}" alpha-rw "user=${ALICE_USER}" 2>/dev/null || \
      echo "  ⚠️  Failed to attach policy to '${ALICE_USER}'"
  else
    echo "  ⚠️  Failed to create user '${ALICE_USER}' (IAM not supported?)"
  fi

  if mc admin user add "${S3_ALIAS}" "${BOB_USER}" "${BOB_PASS}" 2>/dev/null; then
    echo "  ✓ Created user '${BOB_USER}'"
    mc admin policy set "${S3_ALIAS}" beta-rw "user=${BOB_USER}" 2>/dev/null || \
      echo "  ⚠️  Failed to attach policy to '${BOB_USER}'"
  else
    echo "  ⚠️  Failed to create user '${BOB_USER}' (IAM not supported?)"
  fi

  rm -f /tmp/alpha-rw.json /tmp/beta-rw.json
else
  echo "⏭️  Skipping IAM user creation (SKIP_USERS=true)"
fi

# -----------------------
# Set bucket policies (if not skipped)
# -----------------------
if [ "${SKIP_POLICIES}" != "true" ]; then
  echo "🌐 Setting bucket policies..."

  # Try to set public-read on gamma and beta
  if mc anonymous set download "${S3_ALIAS}/gamma" 2>/dev/null; then
    echo "  ✓ Set public-read on bucket 'gamma'"
  else
    echo "  ⚠️  Failed to set public-read on 'gamma' (bucket policies not supported?)"
  fi

  if mc anonymous set download "${S3_ALIAS}/beta" 2>/dev/null; then
    echo "  ✓ Set public-read on bucket 'beta'"
  else
    echo "  ⚠️  Failed to set public-read on 'beta' (bucket policies not supported?)"
  fi
else
  echo "⏭️  Skipping bucket policies (SKIP_POLICIES=true)"
fi

# -----------------------
# Upload basic objects
# -----------------------
echo "📤 Uploading basic test objects..."
upload_object () {
  local bucket=$1
  local name=$2

  tmpfile="$(mktemp)"
  head -c "${OBJECT_SIZE}" /dev/urandom > "${tmpfile}"

  mc cp "${tmpfile}" "${S3_ALIAS}/${bucket}/${name}"
  echo "  ✓ Uploaded ${bucket}/${name}"

  rm -f "${tmpfile}"
}

upload_object alpha alice-object.bin
upload_object beta bob-object.bin
upload_object gamma public-object.bin

# -----------------------
# Create folder structure
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
# Upload objects with metadata
# -----------------------
echo "🔖 Uploading objects with various metadata..."

# Object with custom metadata
tmpfile="$(mktemp)"
head -c "${OBJECT_SIZE}" /dev/urandom > "${tmpfile}"
mc cp --attr "x-amz-meta-author=TestUser;x-amz-meta-project=DirIO;x-amz-meta-version=1" \
  "${tmpfile}" "${S3_ALIAS}/alpha/metadata-test.bin"
echo "  ✓ Uploaded alpha/metadata-test.bin (with custom metadata)"
rm -f "${tmpfile}"

# Object with Content-Type
tmpfile="$(mktemp)"
echo '{"test": "json data"}' > "${tmpfile}"
mc cp --attr "Content-Type=application/json" \
  "${tmpfile}" "${S3_ALIAS}/alpha/data.json"
echo "  ✓ Uploaded alpha/data.json (Content-Type: application/json)"
rm -f "${tmpfile}"

# Object with Content-Type and custom metadata
tmpfile="$(mktemp)"
echo "<html><body>Test</body></html>" > "${tmpfile}"
mc cp --attr "Content-Type=text/html;x-amz-meta-page=index" \
  "${tmpfile}" "${S3_ALIAS}/gamma/index.html"
echo "  ✓ Uploaded gamma/index.html (Content-Type + custom metadata)"
rm -f "${tmpfile}"

# Object with Content-Encoding
tmpfile="$(mktemp)"
echo "compressed data" | gzip > "${tmpfile}"
mc cp --attr "Content-Type=application/gzip;Content-Encoding=gzip" \
  "${tmpfile}" "${S3_ALIAS}/beta/data.gz"
echo "  ✓ Uploaded beta/data.gz (Content-Encoding: gzip)"
rm -f "${tmpfile}"

# Object with multiple custom metadata fields
tmpfile="$(mktemp)"
echo "user data" > "${tmpfile}"
mc cp --attr "x-amz-meta-user-id=12345;x-amz-meta-department=engineering;x-amz-meta-uploaded-by=alice" \
  "${tmpfile}" "${S3_ALIAS}/alpha/userdata.txt"
echo "  ✓ Uploaded alpha/userdata.txt (multiple custom metadata fields)"
rm -f "${tmpfile}"

# -----------------------
# Test CopyObject (server-side copy)
# -----------------------
echo "📋 Testing CopyObject (server-side copy)..."
if mc cp "${S3_ALIAS}/alpha/alice-object.bin" "${S3_ALIAS}/alpha/alice-copy.bin" 2>/dev/null; then
  echo "  ✓ Server-side copy: alpha/alice-object.bin → alpha/alice-copy.bin"
else
  echo "  ⚠️  Server-side copy failed (not supported?)"
fi

if mc cp "${S3_ALIAS}/alpha/folder1/file1.txt" "${S3_ALIAS}/beta/copied-from-alpha.txt" 2>/dev/null; then
  echo "  ✓ Cross-bucket copy: alpha/folder1/file1.txt → beta/copied-from-alpha.txt"
else
  echo "  ⚠️  Cross-bucket copy failed (not supported?)"
fi

# -----------------------
# Upload large file (multipart)
# -----------------------
echo "📦 Uploading large file for multipart testing..."
largefile="$(mktemp)"
dd if=/dev/zero of="${largefile}" bs=1M count=10 2>/dev/null
mc cp "${largefile}" "${S3_ALIAS}/alpha/large-file.dat"
echo "  ✓ Uploaded alpha/large-file.dat (10MB, likely multipart)"
rm -f "${largefile}"

# -----------------------
# List created objects
# -----------------------
echo ""
echo "📋 Listing created objects..."
echo ""
echo "Alpha bucket:"
mc ls --recursive "${S3_ALIAS}/alpha" | head -20
echo ""
echo "Beta bucket:"
mc ls --recursive "${S3_ALIAS}/beta" | head -20
echo ""
echo "Gamma bucket:"
mc ls --recursive "${S3_ALIAS}/gamma" | head -20

# -----------------------
# Done
# -----------------------
echo ""
echo "✅ Generic S3 setup complete!"
echo ""
echo "Endpoint: ${S3_ENDPOINT}"
echo "Alias: ${S3_ALIAS}"
echo "Region: ${S3_REGION}"
echo ""
echo "Buckets created:"
echo "  alpha → private (alice if IAM supported)"
echo "  beta  → public-read (if bucket policies supported)"
echo "  gamma → public-read (if bucket policies supported)"
echo ""
echo "Objects created:"
echo "  - Basic objects (alice-object.bin, bob-object.bin, public-object.bin)"
echo "  - Folder structures (folder1/file1.txt, folder2/file1.txt, etc.)"
echo "  - Objects with standard metadata (Content-Type, Content-Encoding)"
echo "  - Objects with custom metadata (x-amz-meta-*)"
echo "  - Server-side copies (alice-copy.bin, copied-from-alpha.txt)"
echo "  - Large file for multipart testing (large-file.dat - 10MB)"
echo ""
echo "To test with different credentials:"
echo "  mc alias set test-alice ${S3_ENDPOINT} alice alicepass1234"
echo "  mc ls test-alice/alpha"
echo ""
echo "To remove the alias:"
echo "  mc alias rm ${S3_ALIAS}"
echo ""
