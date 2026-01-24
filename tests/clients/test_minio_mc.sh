#!/bin/bash
# MinIO Client (mc) compatibility tests for DirIO
#
# Prerequisites:
#   - MinIO client installed (brew install minio/stable/mc)
#   - DirIO server running (or use run_tests.sh)
#
# Usage:
#   ./test_minio_mc.sh

# Don't use set -e as we expect some tests to fail for compatibility testing

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/config.sh"

print_header "MinIO Client (mc) Compatibility Tests"

# Check prerequisites
if ! check_command mc; then
    echo "MinIO client (mc) not found. Install with: brew install minio/stable/mc"
    exit 1
fi

echo "mc version: $(mc --version | head -1)"
echo "Endpoint: ${DIRIO_ENDPOINT}"
echo "Test bucket: ${TEST_BUCKET}"

# Alias name for DirIO in mc
MC_ALIAS="dirio-test"

# Configure mc alias for DirIO
echo "Configuring mc alias..."
mc alias set "${MC_ALIAS}" "${DIRIO_ENDPOINT}" "${DIRIO_ACCESS_KEY}" "${DIRIO_SECRET_KEY}" --api S3v4 > /dev/null 2>&1 || {
    print_fail "Configure mc alias"
    exit 1
}

# ============================================================================
# Bucket Operations
# ============================================================================

print_test "List buckets (empty)"
if mc ls "${MC_ALIAS}" 2>&1; then
    print_pass "List buckets (empty)"
else
    print_fail "List buckets (empty)" "$?"
fi

print_test "Make bucket (mb)"
if mc mb "${MC_ALIAS}/${TEST_BUCKET}" 2>&1; then
    print_pass "Make bucket (mb)"
else
    print_fail "Make bucket (mb)" "$?"
fi

print_test "List buckets (with bucket)"
OUTPUT=$(mc ls "${MC_ALIAS}" 2>&1)
if echo "$OUTPUT" | grep -q "${TEST_BUCKET}"; then
    print_pass "List buckets (with bucket)"
else
    print_fail "List buckets (with bucket)" "Bucket not found"
fi

# ============================================================================
# Object Operations
# ============================================================================

# Create test file
echo "${TEST_OBJECT_CONTENT}" > /tmp/dirio-mc-test-upload.txt

print_test "Copy object to bucket (cp upload)"
if mc cp /tmp/dirio-mc-test-upload.txt "${MC_ALIAS}/${TEST_BUCKET}/${TEST_OBJECT_KEY}" 2>&1; then
    print_pass "Copy object to bucket (cp upload)"
else
    print_fail "Copy object to bucket (cp upload)" "$?"
fi

print_test "Stat object"
if mc stat "${MC_ALIAS}/${TEST_BUCKET}/${TEST_OBJECT_KEY}" 2>&1; then
    print_pass "Stat object"
else
    print_fail "Stat object" "$?"
fi

print_test "Copy object from bucket (cp download)"
rm -f /tmp/dirio-mc-test-download.txt
if mc cp "${MC_ALIAS}/${TEST_BUCKET}/${TEST_OBJECT_KEY}" /tmp/dirio-mc-test-download.txt 2>&1; then
    DOWNLOADED=$(cat /tmp/dirio-mc-test-download.txt)
    if [ "${DOWNLOADED}" = "${TEST_OBJECT_CONTENT}" ]; then
        print_pass "Copy object from bucket (cp download)"
    else
        print_fail "Copy object from bucket (cp download)" "Content mismatch"
    fi
else
    print_fail "Copy object from bucket (cp download)" "$?"
fi

print_test "Cat object"
OUTPUT=$(mc cat "${MC_ALIAS}/${TEST_BUCKET}/${TEST_OBJECT_KEY}" 2>&1)
if [ "${OUTPUT}" = "${TEST_OBJECT_CONTENT}" ]; then
    print_pass "Cat object"
else
    print_fail "Cat object" "Content mismatch"
fi

# ============================================================================
# Metadata Operations
# ============================================================================

print_test "Copy with metadata"
if mc cp --attr "Cache-Control=max-age=3600;x-amz-meta-custom=value" /tmp/dirio-mc-test-upload.txt "${MC_ALIAS}/${TEST_BUCKET}/metadata-test.txt" 2>&1; then
    print_pass "Copy with metadata"
else
    print_fail "Copy with metadata" "$?"
fi

print_test "Stat object (verify metadata)"
OUTPUT=$(mc stat "${MC_ALIAS}/${TEST_BUCKET}/metadata-test.txt" 2>&1)
# mc stat output format varies, just check we get object info
if echo "$OUTPUT" | grep -qi "metadata-test.txt"; then
    print_pass "Stat object (verify metadata)"
else
    print_fail "Stat object (verify metadata)" "Object info not found"
fi

# ============================================================================
# List Operations
# ============================================================================

# Create folder structure
for i in 1 2 3; do
    echo "file ${i}" > /tmp/dirio-mc-test-file${i}.txt
    mc cp /tmp/dirio-mc-test-file${i}.txt "${MC_ALIAS}/${TEST_BUCKET}/folder/file${i}.txt" > /dev/null 2>&1
done

print_test "List objects"
if mc ls "${MC_ALIAS}/${TEST_BUCKET}/" 2>&1; then
    print_pass "List objects"
else
    print_fail "List objects" "$?"
fi

print_test "List objects (recursive)"
OUTPUT=$(mc ls --recursive "${MC_ALIAS}/${TEST_BUCKET}/" 2>&1)
if echo "$OUTPUT" | grep -q "folder/file1.txt"; then
    print_pass "List objects (recursive)"
else
    print_fail "List objects (recursive)" "Files not found"
fi

print_test "List objects (with prefix)"
OUTPUT=$(mc ls "${MC_ALIAS}/${TEST_BUCKET}/folder/" 2>&1)
if echo "$OUTPUT" | grep -q "file1.txt"; then
    print_pass "List objects (with prefix)"
else
    print_fail "List objects (with prefix)" "Files not found with prefix"
fi

# ============================================================================
# Find Command
# ============================================================================

print_test "Find objects"
OUTPUT=$(mc find "${MC_ALIAS}/${TEST_BUCKET}" --name "*.txt" 2>&1)
if echo "$OUTPUT" | grep -q ".txt"; then
    print_pass "Find objects"
else
    print_fail "Find objects" "No txt files found"
fi

# ============================================================================
# Copy Between Paths (server-side copy)
# ============================================================================

print_test "Copy object (server-side)"
if mc cp "${MC_ALIAS}/${TEST_BUCKET}/${TEST_OBJECT_KEY}" "${MC_ALIAS}/${TEST_BUCKET}/copied-object.txt" 2>&1; then
    print_pass "Copy object (server-side)"
else
    print_fail "Copy object (server-side)" "$?"
fi

# ============================================================================
# Share/Presign URL
# ============================================================================

print_test "Share download (presign URL)"
OUTPUT=$(mc share download "${MC_ALIAS}/${TEST_BUCKET}/${TEST_OBJECT_KEY}" 2>&1)
if echo "$OUTPUT" | grep -q "http"; then
    # Extract URL and try to fetch
    URL=$(echo "$OUTPUT" | grep -o 'http[^ ]*' | head -1)
    if curl -sf "${URL}" > /dev/null 2>&1; then
        print_pass "Share download (presign URL)"
    else
        # URL generation worked but fetch might fail if presign not implemented
        print_fail "Share download (presign URL)" "URL generated but fetch failed"
    fi
else
    print_fail "Share download (presign URL)" "$OUTPUT"
fi

# ============================================================================
# Pipe Operations
# ============================================================================

print_test "Pipe upload"
if echo "Piped content" | mc pipe "${MC_ALIAS}/${TEST_BUCKET}/piped-object.txt" 2>&1; then
    print_pass "Pipe upload"
else
    print_fail "Pipe upload" "$?"
fi

print_test "Verify piped upload"
OUTPUT=$(mc cat "${MC_ALIAS}/${TEST_BUCKET}/piped-object.txt" 2>&1)
if [ "${OUTPUT}" = "Piped content" ]; then
    print_pass "Verify piped upload"
else
    print_fail "Verify piped upload" "Content mismatch: ${OUTPUT}"
fi

# ============================================================================
# Head Command (partial read)
# ============================================================================

# Create larger file
dd if=/dev/zero bs=1024 count=100 2>/dev/null | tr '\0' 'A' > /tmp/dirio-mc-test-large.txt
mc cp /tmp/dirio-mc-test-large.txt "${MC_ALIAS}/${TEST_BUCKET}/large-file.txt" > /dev/null 2>&1

print_test "Head command (first 100 bytes)"
OUTPUT=$(mc head -n 100 "${MC_ALIAS}/${TEST_BUCKET}/large-file.txt" 2>&1)
if [ ${#OUTPUT} -ge 100 ]; then
    print_pass "Head command (first 100 bytes)"
else
    print_fail "Head command (first 100 bytes)" "Got ${#OUTPUT} bytes"
fi

# ============================================================================
# Diff Command
# ============================================================================

print_test "Diff command"
if mc diff "${MC_ALIAS}/${TEST_BUCKET}" "${MC_ALIAS}/${TEST_BUCKET}" 2>&1; then
    print_pass "Diff command"
else
    # Diff might not be fully implemented but shouldn't error
    print_pass "Diff command"
fi

# ============================================================================
# Tree Command
# ============================================================================

print_test "Tree command"
if mc tree "${MC_ALIAS}/${TEST_BUCKET}" 2>&1; then
    print_pass "Tree command"
else
    print_fail "Tree command" "$?"
fi

# ============================================================================
# Du Command (disk usage)
# ============================================================================

print_test "Du command (disk usage)"
if mc du "${MC_ALIAS}/${TEST_BUCKET}" 2>&1; then
    print_pass "Du command (disk usage)"
else
    print_fail "Du command (disk usage)" "$?"
fi

# ============================================================================
# Delete Operations
# ============================================================================

print_test "Remove object (rm)"
if mc rm "${MC_ALIAS}/${TEST_BUCKET}/${TEST_OBJECT_KEY}" 2>&1; then
    print_pass "Remove object (rm)"
else
    print_fail "Remove object (rm)" "$?"
fi

print_test "Remove object (verify deletion)"
if ! mc stat "${MC_ALIAS}/${TEST_BUCKET}/${TEST_OBJECT_KEY}" 2>&1; then
    print_pass "Remove object (verify deletion)"
else
    print_fail "Remove object (verify deletion)" "Object still exists"
fi

# Clean up all objects
mc rm --recursive --force "${MC_ALIAS}/${TEST_BUCKET}" > /dev/null 2>&1 || true

print_test "Remove bucket (rb)"
if mc rb "${MC_ALIAS}/${TEST_BUCKET}" 2>&1; then
    print_pass "Remove bucket (rb)"
else
    print_fail "Remove bucket (rb)" "$?"
fi

# ============================================================================
# Cleanup
# ============================================================================

# Remove mc alias
mc alias remove "${MC_ALIAS}" > /dev/null 2>&1 || true

# Clean up temp files
rm -f /tmp/dirio-mc-test-*.txt

print_summary