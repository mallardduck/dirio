#!/usr/bin/env bash
set -euo pipefail

# -----------------------
# Generic S3 API Setup Script
# -----------------------
# This script sets up test data on ANY S3-compatible API endpoint.
# It uses the MinIO client (mc) which works with any S3 API.
#
# Usage:
#   S3_ENDPOINT=http://localhost:9000 \
#   S3_ACCESS_KEY=dirio-admin \
#   S3_SECRET_KEY=dirio-admin-secret \
#   ./s3-minio-setup.sh
#
# Optional environment variables:
#   S3_ALIAS         - mc alias name (default: "target")
#   S3_REGION        - AWS region if needed (default: "us-east-1")
#   OBJECT_SIZE      - Size of test objects in bytes (default: 65536)
#   SKIP_USERS       - Set to "true" to skip user/policy creation (default: false)
#   SKIP_POLICIES    - Set to "true" to skip bucket policies (default: false)
#   SETUP_POLICY_TESTS - Set to "true" to create advanced policy test scenarios (default: false)

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
SETUP_POLICY_TESTS="${SETUP_POLICY_TESTS:-false}"

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
  echo "  S3_ALIAS=target           # mc alias name"
  echo "  S3_REGION=us-east-1       # AWS region"
  echo "  OBJECT_SIZE=65536         # Test object size in bytes"
  echo "  SKIP_USERS=true           # Skip IAM user creation"
  echo "  SKIP_POLICIES=true        # Skip bucket policy creation"
  echo "  SETUP_POLICY_TESTS=true   # Create advanced policy test scenarios (Phase 3.3)"
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
# Parse MC Version
# -----------------------
# mc --version outputs: "mc version RELEASE.2024-01-15T08-23-05Z (commit-id: ...)"
# We need to extract the date portion (YYYY-MM-DD)
MC_VERSION_INFO=$(mc --version 2>/dev/null || echo "unknown")
MC_VERSION_DATE=""

if [[ "${MC_VERSION_INFO}" =~ RELEASE\.([0-9]{4})-([0-9]{2})-([0-9]{2}) ]]; then
  MC_VERSION_DATE="${BASH_REMATCH[1]}-${BASH_REMATCH[2]}-${BASH_REMATCH[3]}"
  echo "✓ Detected mc version date: ${MC_VERSION_DATE}"
else
  echo "⚠️  Could not parse mc version date, using latest syntax"
  MC_VERSION_DATE="9999-99-99"  # Use latest syntax as fallback
fi

# -----------------------
# Compatibility Functions
# -----------------------
# mc admin policy command changed from 'add' to 'create' on 2023-03-20
# See: https://github.com/minio/mc/blob/master/RELEASE.md

mc_admin_policy_create() {
  local alias=$1
  local policy_name=$2
  local policy_file=$3

  if [[ "${MC_VERSION_DATE}" < "2023-03-20" ]]; then
    mc admin policy add "${alias}" "${policy_name}" "${policy_file}"
  else
    mc admin policy create "${alias}" "${policy_name}" "${policy_file}"
  fi
}

mc_admin_policy_attach() {
  local alias=$1
  local policy_name=$2
  local user_spec=$3

  if [[ "${MC_VERSION_DATE}" < "2023-03-20" ]]; then
    mc admin policy set "${alias}" "${policy_name}" "${user_spec}"
  else
    mc admin policy attach "${alias}" "${policy_name}" --user="${user_spec#user=}"
  fi
}


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
  echo "  (This may fail if the MinIO Admin API doesn't support IAM)"

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

  # Check if alpha-rw policy exists, create if not
  if mc admin policy info "${S3_ALIAS}" alpha-rw >/dev/null 2>&1; then
    echo "  ⚠️  Policy 'alpha-rw' already exists, skipping"
  elif mc_admin_policy_create "${S3_ALIAS}" alpha-rw /tmp/alpha-rw.json 2>/dev/null; then
    echo "  ✓ Created policy 'alpha-rw'"
  else
    echo "  ⚠️  Failed to create policy 'alpha-rw' (IAM not supported?)"
  fi

  # Check if beta-rw policy exists, create if not
  if mc admin policy info "${S3_ALIAS}" beta-rw >/dev/null 2>&1; then
    echo "  ⚠️  Policy 'beta-rw' already exists, skipping"
  elif mc_admin_policy_create "${S3_ALIAS}" beta-rw /tmp/beta-rw.json 2>/dev/null; then
    echo "  ✓ Created policy 'beta-rw'"
  else
    echo "  ⚠️  Failed to create policy 'beta-rw' (IAM not supported?)"
  fi

  # Try to create users (may fail on non-MinIO S3 APIs)
  # Check if alice user exists, create if not
  if mc admin user info "${S3_ALIAS}" "${ALICE_USER}" >/dev/null 2>&1; then
    echo "  ⚠️  User '${ALICE_USER}' already exists, skipping"
  elif mc admin user add "${S3_ALIAS}" "${ALICE_USER}" "${ALICE_PASS}" 2>/dev/null; then
    echo "  ✓ Created user '${ALICE_USER}'"
    mc_admin_policy_attach "${S3_ALIAS}" alpha-rw "user=${ALICE_USER}" 2>/dev/null || \
      echo "  ⚠️  Failed to attach policy to '${ALICE_USER}'"
  else
    echo "  ⚠️  Failed to create user '${ALICE_USER}' (IAM not supported?)"
  fi

  # Check if bob user exists, create if not
  if mc admin user info "${S3_ALIAS}" "${BOB_USER}" >/dev/null 2>&1; then
    echo "  ⚠️  User '${BOB_USER}' already exists, skipping"
  elif mc admin user add "${S3_ALIAS}" "${BOB_USER}" "${BOB_PASS}" 2>/dev/null; then
    echo "  ✓ Created user '${BOB_USER}'"
    mc_admin_policy_attach "${S3_ALIAS}" beta-rw "user=${BOB_USER}" 2>/dev/null || \
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
    if mc ls "${S3_ALIAS}/${bucket}" >/dev/null 2>&1; then
      echo "  ⚠️  Bucket '${bucket}' already exists, skipping"
    else
      mc mb "${S3_ALIAS}/${bucket}" --region="${S3_REGION}"
      echo "  ✓ Created bucket '${bucket}'"
    fi
  done

  # Upload test objects to policy buckets
  echo "📤 Uploading objects for conditional policy tests..."
  upload_object policy-ip-test ip-restricted.txt
  upload_object policy-time-test time-restricted.txt
  upload_object policy-string-test useragent-restricted.txt
  upload_object policy-numeric-test size-restricted.txt

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
    if mc ls "${S3_ALIAS}/${bucket}" >/dev/null 2>&1; then
      echo "  ⚠️  Bucket '${bucket}' already exists, skipping"
    else
      mc mb "${S3_ALIAS}/${bucket}" --region="${S3_REGION}"
      echo "  ✓ Created bucket '${bucket}'"
    fi
  done

  # Upload test objects
  echo "📤 Uploading objects for NotAction/NotResource tests..."
  upload_object policy-notaction-test readonly.txt
  upload_object policy-notresource-test protected-file.txt
  upload_object policy-notresource-test unprotected-file.txt

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

  if mc ls "${S3_ALIAS}/policy-variables-test" >/dev/null 2>&1; then
    echo "  ⚠️  Bucket 'policy-variables-test' already exists, skipping"
  else
    mc mb "${S3_ALIAS}/policy-variables-test" --region="${S3_REGION}"
    echo "  ✓ Created bucket 'policy-variables-test'"
  fi

  # Create user-specific folders
  echo "📤 Creating user-specific folder structure..."
  upload_object policy-variables-test alice/private-file.txt
  upload_object policy-variables-test alice/data.json
  upload_object policy-variables-test bob/private-file.txt
  upload_object policy-variables-test bob/data.json
  upload_object policy-variables-test shared/public-file.txt

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
    if mc ls "${S3_ALIAS}/${bucket}" >/dev/null 2>&1; then
      echo "  ⚠️  Bucket '${bucket}' already exists, skipping"
    else
      mc mb "${S3_ALIAS}/${bucket}" --region="${S3_REGION}"
      echo "  ✓ Created bucket '${bucket}'"
    fi
  done

  # Upload objects to filtering test buckets
  echo "📤 Uploading objects for filtering tests..."
  for i in {1..20}; do
    upload_object filter-alice-only "alice-file-${i}.txt" >/dev/null 2>&1
  done
  echo "  ✓ Uploaded 20 objects to filter-alice-only"

  for i in {1..20}; do
    upload_object filter-bob-only "bob-file-${i}.txt" >/dev/null 2>&1
  done
  echo "  ✓ Uploaded 20 objects to filter-bob-only"

  for i in {1..20}; do
    upload_object filter-shared "shared-file-${i}.txt" >/dev/null 2>&1
  done
  echo "  ✓ Uploaded 20 objects to filter-shared"

  # Create bucket with mixed permissions (some objects readable, some not)
  if mc ls "${S3_ALIAS}/filter-mixed-perms" >/dev/null 2>&1; then
    echo "  ⚠️  Bucket 'filter-mixed-perms' already exists, skipping"
  else
    mc mb "${S3_ALIAS}/filter-mixed-perms" --region="${S3_REGION}"
    echo "  ✓ Created bucket 'filter-mixed-perms'"
  fi

  # Create objects with different prefixes for partial permissions
  echo "📤 Creating objects with different permission prefixes..."
  upload_object filter-mixed-perms public/file1.txt
  upload_object filter-mixed-perms public/file2.txt
  upload_object filter-mixed-perms private/file1.txt
  upload_object filter-mixed-perms private/file2.txt
  upload_object filter-mixed-perms restricted/file1.txt
  upload_object filter-mixed-perms restricted/file2.txt
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
  if mc ls "${S3_ALIAS}/post-upload-test" >/dev/null 2>&1; then
    echo "  ⚠️  Bucket 'post-upload-test' already exists, skipping"
  else
    mc mb "${S3_ALIAS}/post-upload-test" --region="${S3_REGION}"
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
  <form action="http://localhost:9000/post-upload-test" method="post" enctype="multipart/form-data">
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
  echo "🧪 Testing Scenarios:"
  echo "  1. Conditional policies: Test IP, date, string, and numeric conditions"
  echo "  2. NotAction/NotResource: Test inverse matching (deny all except...)"
  echo "  3. Policy variables: Test \${aws:username}, \${aws:SourceIp} substitution"
  echo "  4. Result filtering: Test that ListBuckets/ListObjects only show allowed items"
  echo "  5. POST uploads: Test browser-based form uploads with signed policies"
  echo ""
  echo "📝 Next Steps:"
  echo "  - Apply policies to buckets using: mc anonymous set-json /tmp/policy-*.json ${S3_ALIAS}/bucket-name"
  echo "  - Test with different users/credentials to verify filtering"
  echo "  - Implement condition evaluation in DirIO policy engine"
  echo "  - Implement NotAction/NotResource support"
  echo "  - Implement policy variable substitution"
  echo "  - Implement ListBuckets/ListObjects result filtering"
  echo "  - Implement POST upload policy validation"
  echo ""
else
  echo ""
  echo "⏭️  Skipping advanced policy tests (set SETUP_POLICY_TESTS=true to enable)"
fi

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
echo "To setup advanced policy test scenarios (Phase 3.3):"
echo "  SETUP_POLICY_TESTS=true S3_ENDPOINT=${S3_ENDPOINT} \\"
echo "    S3_ACCESS_KEY=${S3_ACCESS_KEY} S3_SECRET_KEY=*** \\"
echo "    $0"
echo ""
echo "To remove the alias:"
echo "  mc alias rm ${S3_ALIAS}"
echo ""
