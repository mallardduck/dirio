#!/usr/bin/env bash
# validate-setup.sh
#
# Validates a running DirIO (or any S3-compatible) server against the test data
# created by minio-standalone-setup.sh or minio-2019-setup.sh.
#
# Tests "outside in": uses curl for raw HTTP checks and aws CLI for authenticated
# S3 API calls — the same tools a real client would use.
#
# Usage (from WSL):
#   S3_ENDPOINT=http://localhost:8080 \
#   S3_ACCESS_KEY=minioadmin \
#   S3_SECRET_KEY=minioadmin \
#   ./validate-setup.sh
#
# Environment variables:
#   S3_ENDPOINT    - Server URL, no trailing slash (default: http://localhost:8080)
#   S3_ACCESS_KEY  - Root/admin access key     (default: minioadmin)
#   S3_SECRET_KEY  - Root/admin secret key     (default: minioadmin)
#   DATASET        - "2019", "2022", or "standalone"    (default: 2019)
#   ALICE_PASS     - alice's secret key        (default: alicepass1234)
#   BOB_PASS       - bob's secret key          (default: bobpass1234)
#   CHARLIE_PASS   - charlie's secret key      (default: charliepass1234)
#   SKIP_IAM       - "true" to skip per-user tests (default: false)
#
# Exit code: number of failed tests (0 = all pass)

set -uo pipefail

S3_ENDPOINT="${S3_ENDPOINT:-http://localhost:9000}"
S3_ACCESS_KEY="${S3_ACCESS_KEY:-minioadmin}"
S3_SECRET_KEY="${S3_SECRET_KEY:-minioadmin}"
DATASET="${DATASET:-2019}"
ALICE_PASS="${ALICE_PASS:-alicepass1234}"
BOB_PASS="${BOB_PASS:-bobpass1234}"
CHARLIE_PASS="${CHARLIE_PASS:-charliepass1234}"
SKIP_IAM="${SKIP_IAM:-false}"
[[ $DATASET -gt 2021 ]] && MULTIPOLICY=true || MULTIPOLICY=false

S3_ENDPOINT="${S3_ENDPOINT%/}"   # strip trailing slash

# -----------------------
# Result tracking
# -----------------------
PASS=0
FAIL=0
SKIP=0

pass() { echo "  ✓ $1"; PASS=$((PASS + 1)); }
fail() { echo "  ✗ $1"; FAIL=$((FAIL + 1)); }
skip() { echo "  - $1 [skipped]"; SKIP=$((SKIP + 1)); }

section() {
  echo ""
  echo "━━━ $1 ━━━"
}

# -----------------------
# Prerequisites
# -----------------------
check_prereqs() {
  local missing=0
  if ! command -v curl &>/dev/null; then
    echo "❌ curl not found — install with: apt install curl"
    missing=1
  fi
  if ! command -v aws &>/dev/null; then
    echo "❌ aws CLI not found — install with: pip install awscli  OR  apt install awscli"
    missing=1
  fi
  if [ $missing -ne 0 ]; then exit 1; fi
}

# -----------------------
# Server Sniffing
# -----------------------
detect_server_type() {
  local headers
  # Get headers from a simple root request
  headers=$(curl -s -D - -o /dev/null "${S3_ENDPOINT}/" 2>/dev/null) || headers=""

  if echo "${headers}" | grep -qi "Server: MinIO"; then
    # Further refine MinIO version if possible
    local version_str
    version_str=$(echo "${headers}" | grep -i "Server: MinIO" | awk '{print $2}')
    echo "MINIO"
    [[ "$version_str" == *"2019"* ]] && echo "VERSION_2019"
  elif echo "${headers}" | grep -qi "Server: AmazonS3"; then
    echo "S3"
  elif echo "${headers}" | grep -qi "Server: DirIO"; then
    echo "DIRIO"
  else
    echo "UNKNOWN"
  fi
}

# -----------------------
# AWS CLI wrappers
# Credentials are passed via env vars so no ~/.aws/credentials setup is needed.
# -----------------------
aws_root() {
  AWS_ACCESS_KEY_ID="${S3_ACCESS_KEY}" \
  AWS_SECRET_ACCESS_KEY="${S3_SECRET_KEY}" \
  AWS_DEFAULT_REGION="us-east-1" \
  aws --endpoint-url "${S3_ENDPOINT}" --output json "$@"
}

aws_as() {
  local key=$1 secret=$2; shift 2
  AWS_ACCESS_KEY_ID="${key}" \
  AWS_SECRET_ACCESS_KEY="${secret}" \
  AWS_DEFAULT_REGION="us-east-1" \
  aws --endpoint-url "${S3_ENDPOINT}" --output json "$@"
}

# Return HTTP status code for a URL, no auth
http_status() { curl -s -o /dev/null -w "%{http_code}" "$@"; }

# Return response headers for a URL, no auth
http_headers() { curl -s -I "$@"; }

# -----------------------
# 1. Connectivity
# -----------------------
test_connectivity() {
  section "1. Connectivity"

  local status
  # Try ListBuckets — authenticated, so the server will respond meaningfully
  if aws_root s3api list-buckets >/dev/null 2>&1; then
    pass "ListBuckets succeeded — server is up and credentials are valid"
  else
    # Server might be up but rejecting auth; check raw HTTP first
    status=$(http_status "${S3_ENDPOINT}/")
    if [ "${status}" = "000" ]; then
      fail "Cannot reach ${S3_ENDPOINT} (connection refused or timeout)"
      echo ""
      echo "  Aborting: server is not reachable."
      exit 1
    else
      fail "Server reachable (HTTP ${status}) but ListBuckets failed — check credentials"
    fi
  fi
}

# -----------------------
# 2. Bucket existence
# -----------------------
test_list_buckets() {
  section "2. ListBuckets"

  local bucket_list
  bucket_list=$(aws_root s3api list-buckets --query 'Buckets[].Name' --output text 2>/dev/null) || bucket_list=""

  for bucket in alpha beta gamma delta; do
    if echo "${bucket_list}" | grep -qw "${bucket}"; then
      pass "Bucket '${bucket}' present"
    else
      fail "Bucket '${bucket}' missing from ListBuckets response"
    fi
  done
}

# -----------------------
# 3. Core object existence (both datasets)
# -----------------------
test_core_objects() {
  section "3. Core Objects — HeadObject"

  local pairs=(
    "alpha alice-object.bin"
    "beta  bob-object.bin"
    "gamma public-object.bin"
    "delta charlie-object.bin"
  )

  for pair in "${pairs[@]}"; do
    local bucket key
    bucket=$(echo "${pair}" | awk '{print $1}')
    key=$(echo "${pair}" | awk '{print $2}')
    if aws_root s3api head-object --bucket "${bucket}" --key "${key}" >/dev/null 2>&1; then
      pass "${bucket}/${key} exists"
    else
      fail "${bucket}/${key} missing"
    fi
  done
}

# -----------------------
# 4. GetObject (downloads data, checks size > 0)
# -----------------------
test_get_object() {
  section "4. GetObject"

  local size
  size=$(aws_root s3api get-object \
    --bucket alpha --key alice-object.bin /dev/null \
    --query 'ContentLength' --output text 2>/dev/null) || size=""

  if [ -n "${size}" ] && [ "${size}" -gt 0 ] 2>/dev/null; then
    pass "alpha/alice-object.bin: GetObject returned ${size} bytes"
  else
    fail "alpha/alice-object.bin: GetObject failed or returned empty (ContentLength='${size}')"
  fi
}

# -----------------------
# 5. Anonymous (public) access via raw curl
#    gamma and beta should be public-read; alpha must require auth
# -----------------------
test_public_access() {
  section "5. Anonymous (Public) Access — curl"

  local status

  status=$(http_status "${S3_ENDPOINT}/gamma/public-object.bin")
  if [ "${status}" = "200" ]; then
    pass "gamma/public-object.bin: anonymous GET → HTTP 200"
  else
    fail "gamma/public-object.bin: should be public-read, got HTTP ${status}"
  fi

  status=$(http_status "${S3_ENDPOINT}/gamma/large-public.dat")
  if [ "${status}" = "200" ]; then
    pass "gamma/large-public.dat: anonymous GET → HTTP 200"
  else
    fail "gamma/large-public.dat: should be public-read, got HTTP ${status}"
  fi

  status=$(http_status "${S3_ENDPOINT}/gamma/index.html")
  if [ "${status}" = "200" ]; then
    pass "gamma/index.html: anonymous GET → HTTP 200 (browser smoke-test page)"
  else
    fail "gamma/index.html: should be public-read, got HTTP ${status}"
  fi

  status=$(http_status "${S3_ENDPOINT}/beta/bob-object.bin")
  if [ "${status}" = "200" ]; then
    pass "beta/bob-object.bin: anonymous GET → HTTP 200"
  else
    fail "beta/bob-object.bin: should be public-read, got HTTP ${status}"
  fi

  # alpha must deny anonymous access
  status=$(http_status "${S3_ENDPOINT}/alpha/alice-object.bin")
  if [ "${status}" = "403" ] || [ "${status}" = "401" ]; then
    pass "alpha/alice-object.bin: anonymous GET → HTTP ${status} (auth required ✓)"
  else
    fail "alpha/alice-object.bin: expected 403/401, got HTTP ${status}"
  fi
}

# -----------------------
# 6. Raw HTTP response headers for public objects
#    Validates that Content-Type etc. are surfaced at the HTTP layer (not just S3 API)
# -----------------------
test_curl_headers() {
  section "6. Raw HTTP Headers — curl HEAD on public objects"

  # gamma/public-object.bin — check server responds to HEAD correctly
  local status
  status=$(http_status -I "${S3_ENDPOINT}/gamma/public-object.bin")
  if [ "${status}" = "200" ]; then
    pass "gamma/public-object.bin: HEAD → HTTP 200"
  else
    fail "gamma/public-object.bin: HEAD returned HTTP ${status}"
  fi

  # gamma/index.html — was uploaded with Content-Type: text/html; check it shows up
  local headers
  headers=$(http_headers "${S3_ENDPOINT}/gamma/index.html" 2>/dev/null) || headers=""
  if echo "${headers}" | grep -qi "content-type.*text/html"; then
    pass "gamma/index.html: HTTP Content-Type header contains 'text/html'"
  else
    local ct_line
    ct_line=$(echo "${headers}" | grep -i "content-type" || echo "(no content-type header)")
    fail "gamma/index.html: expected Content-Type text/html — got: ${ct_line}"
  fi

  # Confirm alpha is auth-gated at the HTTP level
  status=$(http_status "${S3_ENDPOINT}/alpha/alice-object.bin")
  if [ "${status}" = "403" ] || [ "${status}" = "401" ]; then
    pass "alpha/alice-object.bin: curl returns ${status} (access denied ✓)"
  else
    fail "alpha/alice-object.bin: expected 403/401 from curl, got HTTP ${status}"
  fi
}

# -----------------------
# 7. Per-user authentication (IAM users)
# -----------------------
test_user_auth() {
  section "7. Per-User Auth — alice, bob, and charlie"

  if [ "${SKIP_IAM}" = "true" ]; then
    skip "alice can access alpha (SKIP_IAM=true)"
    skip "bob can access beta (SKIP_IAM=true)"
    skip "alice denied access to beta (SKIP_IAM=true)"
    skip "bob denied access to alpha (SKIP_IAM=true)"
    skip "charlie (multi-policy): can access alpha via alpha-rw (SKIP_IAM=true)"
    skip "charlie (multi-policy): can access delta via delta-rw (SKIP_IAM=true)"
    skip "charlie (multi-policy): beta access check (SKIP_IAM=true)"
    return
  fi

  # alice → alpha (should work — alice has alpha-rw)
  if aws_as "alice" "${ALICE_PASS}" s3api head-object \
      --bucket alpha --key alice-object.bin >/dev/null 2>&1; then
    pass "alice: HeadObject alpha/alice-object.bin → allowed"
  else
    fail "alice: HeadObject alpha/alice-object.bin → denied (should be allowed)"
  fi

  # bob → beta (should work — bob has beta-rw)
  if aws_as "bob" "${BOB_PASS}" s3api head-object \
      --bucket beta --key bob-object.bin >/dev/null 2>&1; then
    pass "bob: HeadObject beta/bob-object.bin → allowed"
  else
    fail "bob: HeadObject beta/bob-object.bin → denied (should be allowed)"
  fi

  # alice → beta (should fail — alice only has alpha-rw)
  if [[ "$SERVER_TYPE" == "MINIO" ]]; then
    if aws_as "alice" "${ALICE_PASS}" s3api head-object \
          --bucket beta --key bob-object.bin >/dev/null 2>&1; then
        fail "alice: HeadObject beta/bob-object.bin → allowed (should be denied)"
      else
        pass "alice: HeadObject beta/bob-object.bin → denied (correct ✓)"
      fi
  else
      if aws_as "alice" "${ALICE_PASS}" s3api head-object \
          --bucket beta --key bob-object.bin >/dev/null 2>&1; then
        pass "alice: HeadObject beta/bob-object.bin → allowed (correct ✓; even authed users get access via bucket policy)"
      else
        fail "alice: HeadObject beta/bob-object.bin → denied (should be allowed via Bucket policy)"
      fi
  fi

  # bob → alpha (should fail — bob only has beta-rw)
  if aws_as "bob" "${BOB_PASS}" s3api head-object \
      --bucket alpha --key alice-object.bin >/dev/null 2>&1; then
    fail "bob: HeadObject alpha/alice-object.bin → allowed (should be denied)"
  else
    pass "bob: HeadObject alpha/alice-object.bin → denied (correct ✓)"
  fi

  # --- charlie: multi-policy tests (alpha-rw + delta-rw) ---
  #
  # MinIO 2019 does not support multi-policy natively. So it will set
  # MULTIPOLICY=false when validating the live MinIO 2019 instance to
  # skip the delta test; leave it true (default) for DirIO or MinIO 2022.

  # charlie → delta (should always work — charlie has delta-rw in all setups)
  if aws_as "charlie" "${CHARLIE_PASS}" s3api head-object \
      --bucket delta --key charlie-object.bin >/dev/null 2>&1; then
    pass "charlie: HeadObject delta/charlie-object.bin → allowed via delta-rw"
  else
    fail "charlie: HeadObject delta/charlie-object.bin → denied (should be allowed via delta-rw)"
  fi

  # charlie → alpha (requires multi-policy support)
  if [ "${MULTIPOLICY}" = "false" ]; then
    skip "charlie (multi-policy): HeadObject alpha/alice-object.bin — MULTIPOLICY=false (required on 2019)"
  elif aws_as "charlie" "${CHARLIE_PASS}" s3api head-object \
      --bucket alpha --key alice-object.bin >/dev/null 2>&1; then
    pass "charlie (multi-policy): HeadObject alpha/alice-object.bin → allowed via alpha-rw"
  else
    fail "charlie (multi-policy): HeadObject alpha/alice-object.bin → denied (should be allowed via alpha-rw)"
  fi

  if [[ "$SERVER_TYPE" == "MINIO" ]]; then
    # charlie → beta (no beta-rw IAM policy)
    if aws_as "charlie" "${CHARLIE_PASS}" s3api head-object \
        --bucket beta --key bob-object.bin >/dev/null 2>&1; then
      fail "charlie (multi-policy): HeadObject beta/bob-object.bin → allowed (should be denied)"
    else
      pass "charlie (multi-policy): HeadObject beta/bob-object.bin → denied (correct ✓)"
    fi
  else
    if aws_as "charlie" "${CHARLIE_PASS}" s3api head-object \
        --bucket beta --key bob-object.bin >/dev/null 2>&1; then
      pass "charlie (multi-policy): HeadObject beta/bob-object.bin → allowed (correct ✓; even authed users get access via bucket policy)"
    else
      fail "charlie (multi-policy): HeadObject beta/bob-object.bin → denied (should be allowed via Bucket policy)"
    fi
  fi


}

# -----------------------
# 8. ListObjects with prefix + delimiter (folder simulation)
# -----------------------
test_folder_structure() {
  section "8. Folder Structure — ListObjects prefix/delimiter"

  # Top-level listing with delimiter="/": folder1/ and folder2/ should be common prefixes
  local prefixes
  prefixes=$(aws_root s3api list-objects \
    --bucket alpha --delimiter "/" \
    --query 'CommonPrefixes[].Prefix' --output text 2>/dev/null) || prefixes=""

  for prefix in "folder1/" "folder2/"; do
    if echo "${prefixes}" | grep -qF "${prefix}"; then
      pass "alpha: delimiter='/' listing returns common prefix '${prefix}'"
    else
      fail "alpha: expected common prefix '${prefix}' (got: ${prefixes:-none})"
    fi
  done

  # root-file.txt should appear as a top-level key (not inside a prefix)
  local top_keys
  top_keys=$(aws_root s3api list-objects \
    --bucket alpha --delimiter "/" \
    --query 'Contents[].Key' --output text 2>/dev/null) || top_keys=""

  if echo "${top_keys}" | grep -qw "root-file.txt"; then
    pass "alpha: root-file.txt visible in top-level (non-prefixed) listing"
  else
    fail "alpha: root-file.txt missing from top-level listing"
  fi

  # Prefix filter: folder1/ should contain file1.txt, file2.txt, and the nested subfolder
  local folder1_keys
  folder1_keys=$(aws_root s3api list-objects \
    --bucket alpha --prefix "folder1/" \
    --query 'Contents[].Key' --output text 2>/dev/null) || folder1_keys=""

  for key in "folder1/file1.txt" "folder1/file2.txt" "folder1/subfolder/deep.txt"; do
    if echo "${folder1_keys}" | grep -qw "${key}"; then
      pass "alpha: '${key}' present under prefix 'folder1/'"
    else
      fail "alpha: '${key}' missing under prefix 'folder1/'"
    fi
  done

  # Beta prefix structure
  local beta_keys
  beta_keys=$(aws_root s3api list-objects \
    --bucket beta --prefix "prefix/" \
    --query 'Contents[].Key' --output text 2>/dev/null) || beta_keys=""

  for key in "prefix/test.txt" "prefix/data/nested.txt"; do
    if echo "${beta_keys}" | grep -qw "${key}"; then
      pass "beta: '${key}' present under prefix 'prefix/'"
    else
      fail "beta: '${key}' missing under prefix 'prefix/'"
    fi
  done
}

# -----------------------
# 9. Custom object metadata (x-amz-meta-*)
# -----------------------
test_metadata() {
  section "9. Custom Object Metadata (x-amz-meta-*)"

  # alpha/metadata-test.bin: uploaded with x-amz-meta-author, x-amz-meta-project, x-amz-meta-version
  local meta
  meta=$(aws_root s3api head-object \
    --bucket alpha --key metadata-test.bin \
    --query 'Metadata' --output json 2>/dev/null) || meta="{}"

  for field in "author" "project" "version"; do
    if echo "${meta}" | grep -qi "\"${field}\""; then
      pass "alpha/metadata-test.bin: x-amz-meta-${field} present"
    else
      fail "alpha/metadata-test.bin: x-amz-meta-${field} missing (metadata: ${meta})"
    fi
  done

  # alpha/userdata.txt: uploaded with x-amz-meta-user-id, x-amz-meta-department, x-amz-meta-uploaded-by
  local user_meta
  user_meta=$(aws_root s3api head-object \
    --bucket alpha --key userdata.txt \
    --query 'Metadata' --output json 2>/dev/null) || user_meta="{}"

  for field in "user-id" "department" "uploaded-by"; do
    if echo "${user_meta}" | grep -qi "\"${field}\""; then
      pass "alpha/userdata.txt: x-amz-meta-${field} present"
    else
      fail "alpha/userdata.txt: x-amz-meta-${field} missing (metadata: ${user_meta})"
    fi
  done

  # alpha/simulated-tagged.txt: x-amz-meta-environment, x-amz-meta-project
  local tag_meta
  tag_meta=$(aws_root s3api head-object \
    --bucket alpha --key simulated-tagged.txt \
    --query 'Metadata' --output json 2>/dev/null) || tag_meta="{}"

  for field in "environment" "project"; do
    if echo "${tag_meta}" | grep -qi "\"${field}\""; then
      pass "alpha/simulated-tagged.txt: x-amz-meta-${field} present"
    else
      fail "alpha/simulated-tagged.txt: x-amz-meta-${field} missing (metadata: ${tag_meta})"
    fi
  done
}

# -----------------------
# 10. Standard content headers (Content-Type, Content-Encoding, Content-Language)
# -----------------------
test_content_headers() {
  section "10. Standard Content Headers"

  local actual

  # alpha/data.json → Content-Type: application/json
  actual=$(aws_root s3api head-object \
    --bucket alpha --key data.json \
    --query 'ContentType' --output text 2>/dev/null) || actual=""
  if echo "${actual}" | grep -qi "application/json"; then
    pass "alpha/data.json: Content-Type='${actual}'"
  else
    fail "alpha/data.json: expected application/json, got '${actual}'"
  fi

  # gamma/index.html → Content-Type: text/html
  actual=$(aws_root s3api head-object \
    --bucket gamma --key index.html \
    --query 'ContentType' --output text 2>/dev/null) || actual=""
  if echo "${actual}" | grep -qi "text/html"; then
    pass "gamma/index.html: Content-Type='${actual}'"
  else
    fail "gamma/index.html: expected text/html, got '${actual}'"
  fi

  # beta/data.gz → Content-Encoding: gzip
  actual=$(aws_root s3api head-object \
    --bucket beta --key data.gz \
    --query 'ContentEncoding' --output text 2>/dev/null) || actual=""
  if echo "${actual}" | grep -qi "gzip"; then
    pass "beta/data.gz: Content-Encoding='${actual}'"
  else
    fail "beta/data.gz: expected gzip Content-Encoding, got '${actual}'"
  fi

  # alpha/french.txt → Content-Language: fr
  actual=$(aws_root s3api head-object \
    --bucket alpha --key french.txt \
    --query 'ContentLanguage' --output text 2>/dev/null) || actual=""
  if echo "${actual}" | grep -qi "fr"; then
    pass "alpha/french.txt: Content-Language='${actual}'"
  else
    fail "alpha/french.txt: expected Content-Language fr, got '${actual}'"
  fi
}

# -----------------------
# 11. Large file (multipart upload artifact)
# -----------------------
test_large_file() {
  section "11. Large File (10MB Multipart Upload)"

  local size

  # alpha/large-file.dat
  size=$(aws_root s3api head-object \
    --bucket alpha --key large-file.dat \
    --query 'ContentLength' --output text 2>/dev/null) || size=""
  if [ -n "${size}" ] && [ "${size}" -ge 10485760 ] 2>/dev/null; then
    pass "alpha/large-file.dat: ${size} bytes (≥10MB ✓)"
  elif [ -n "${size}" ]; then
    fail "alpha/large-file.dat: ${size} bytes (expected ≥10485760)"
  else
    fail "alpha/large-file.dat: missing or HeadObject failed"
  fi

  # gamma/large-public.dat — verify size via authenticated HeadObject
  size=$(aws_root s3api head-object \
    --bucket gamma --key large-public.dat \
    --query 'ContentLength' --output text 2>/dev/null) || size=""
  if [ -n "${size}" ] && [ "${size}" -ge 10485760 ] 2>/dev/null; then
    pass "gamma/large-public.dat: ${size} bytes (≥10MB ✓)"
  else
    fail "gamma/large-public.dat: missing or wrong size (got '${size}')"
  fi
}

# -----------------------
# 12. Server-side copies (CopyObject)
# -----------------------
test_server_side_copies() {
  section "12. Server-Side Copies (CopyObject)"

  if aws_root s3api head-object --bucket alpha --key alice-copy.bin >/dev/null 2>&1; then
    pass "alpha/alice-copy.bin exists (same-bucket copy of alice-object.bin)"
  else
    fail "alpha/alice-copy.bin missing (same-bucket CopyObject)"
  fi

  if aws_root s3api head-object --bucket beta --key copied-from-alpha.txt >/dev/null 2>&1; then
    pass "beta/copied-from-alpha.txt exists (cross-bucket copy from alpha)"
  else
    fail "beta/copied-from-alpha.txt missing (cross-bucket CopyObject)"
  fi
}

check_prereqs

# Sniff server
DETECTED_INFO=$(detect_server_type)
SERVER_TYPE=$(echo "$DETECTED_INFO" | head -n 1)

# -----------------------
# Main
# -----------------------
echo "┌──────────────────────────────────────────────┐"
echo "│  DirIO S3 Validation                         │"
echo "└──────────────────────────────────────────────┘"
echo ""
echo "  Endpoint   : ${S3_ENDPOINT}"
echo "  Auth       : ${S3_ACCESS_KEY} / (secret)"
echo "  Server     : ${SERVER_TYPE}"
echo "  Dataset    : ${DATASET}"
echo "  Skip IAM   : ${SKIP_IAM}"
echo "  Multipolicy: ${MULTIPOLICY} (set MULTIPOLICY=false for live MinIO 2019)"
echo "  Charlie    : charliepass=${CHARLIE_PASS} (multi-policy: alpha-rw + delta-rw)"
echo ""

# Tests common to both datasets
test_connectivity
test_list_buckets
test_core_objects
test_get_object
test_public_access
test_curl_headers
test_user_auth
test_metadata
test_content_headers
test_large_file
test_server_side_copies

# -----------------------
# Summary
# -----------------------
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
TOTAL=$((PASS + FAIL + SKIP))
if [ "${FAIL}" -eq 0 ]; then
  echo "✅  All tests passed  (${PASS}/${TOTAL} passed, ${SKIP} skipped)"
else
  echo "❌  ${FAIL} test(s) FAILED  (${PASS} passed, ${FAIL} failed, ${SKIP} skipped)"
fi
echo ""

exit "${FAIL}"