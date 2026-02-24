#!/bin/bash
# MinIO Client (mc) Admin API Integration Tests
# Tests DirIO server compatibility with mc admin commands

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source test framework - handle both container and local execution
if [ -f "$SCRIPT_DIR/../lib/test_framework.sh" ]; then
    source "$SCRIPT_DIR/../lib/test_framework.sh"
    source "$SCRIPT_DIR/../lib/validators.sh"
elif [ -f "$SCRIPT_DIR/lib/test_framework.sh" ]; then
      source "$SCRIPT_DIR/lib/test_framework.sh"
      source "$SCRIPT_DIR/lib/validators.sh"
elif [ -f "/tmp/test_framework.sh" ]; then
    source /tmp/test_framework.sh
    source /tmp/validators.sh
else
    echo "ERROR: Cannot find test_framework.sh" >&2
    exit 1
fi

# Initialize test runner
MC_VERSION=$(mc --version 2>&1 | head -n1)
init_test_runner "mc-admin" "$MC_VERSION"

ENDPOINT="${DIRIO_ENDPOINT}"
MC_ALIAS="${MC_ALIAS:-dirio}"

# Capture the stdout (JSON) into a variable
# We keep stderr (2) directed to /dev/null to keep the console clean
ALIAS_JSON=$(mc alias ls "${MC_ALIAS}" --json 2>/dev/null)
EXIT_CODE=$?
[[ $EXIT_CODE -eq 0 ]] && ALIAS_EXISTS=1 || ALIAS_EXISTS=0

if [ "$ALIAS_EXISTS" -eq 1 ]; then
    # Map to Environment Variables
    export ENDPOINT=$(echo "$ALIAS_JSON" | jq -r .URL)
    export DIRIO_ACCESS_KEY=$(echo "$ALIAS_JSON" | jq -r .accessKey)
    export DIRIO_SECRET_KEY=$(echo "$ALIAS_JSON" | jq -r .secretKey)

    echo "Config loaded for: ${MC_ALIAS}"
fi


echo "=== MinIO mc Admin Tests ===" >&2
echo "Alias: ${MC_ALIAS}" >&2
echo "Endpoint: ${ENDPOINT}" >&2
echo "mc version: ${MC_VERSION}" >&2

# Network probe
PROBE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -m 5 "${ENDPOINT}/" || echo "000")
if [ "${PROBE_CODE}" = "000" ]; then
    echo "FATAL: Cannot reach server at ${ENDPOINT}" >&2
    exit 1
fi
echo "GET / -> HTTP ${PROBE_CODE}" >&2

# -----------------------
# Server Sniffing
# -----------------------
detect_server_type() {
    local headers
    headers=$(curl -s -D - -o /dev/null "${ENDPOINT}/" 2>/dev/null) || headers=""

    if echo "${headers}" | grep -qi "Server: MinIO"; then
        echo "MINIO"
    elif echo "${headers}" | grep -qi "Server: AmazonS3"; then
        echo "S3"
    elif echo "${headers}" | grep -qi "Server: DirIO"; then
        echo "DIRIO"
    else
        echo "UNKNOWN"
    fi
}

SERVER_TYPE=$(detect_server_type)
echo "Server type: ${SERVER_TYPE}" >&2

# Configure mc alias
if [ "$ALIAS_EXISTS" -eq 0 ]; then
  mc alias set ${MC_ALIAS} ${ENDPOINT} ${DIRIO_ACCESS_KEY} ${DIRIO_SECRET_KEY} --api S3v4 2>/dev/null
  if [ $? -ne 0 ]; then
      echo "FATAL: Failed to configure mc alias" >&2
      exit 1
  fi
fi

# Unique suffix to avoid key collisions across test runs
TS="$(date +%s)"
TEST_USER="mcadmuser${TS}"
TEST_USER_SECRET="mcadmusersecret1"
TEST_POLICY="mcadmpol${TS}"
TEST_GROUP="mcadmgroup${TS}"
TEST_BUCKET="mcadmbucket${TS}"

#------------------------------------------------------------------------------
# Test Functions
#------------------------------------------------------------------------------

test_admin_user_add() {
    mc admin user add ${MC_ALIAS} ${TEST_USER} ${TEST_USER_SECRET} > /dev/null 2>&1
}

test_admin_user_list() {
    mc admin user list ${MC_ALIAS} 2>/dev/null | grep -q "${TEST_USER}"
}

test_admin_user_info() {
    mc admin user info ${MC_ALIAS} ${TEST_USER} > /dev/null 2>&1
}

test_admin_policy_create() {
    # Write a minimal policy document to disk then create it
    cat > /tmp/mcadm-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:GetObject", "s3:PutObject"],
      "Resource": ["arn:aws:s3:::*", "arn:aws:s3:::*/*"]
    }
  ]
}
EOF
    mc admin policy create ${MC_ALIAS} ${TEST_POLICY} /tmp/mcadm-policy.json > /dev/null 2>&1
}

test_admin_policy_list() {
    mc admin policy list ${MC_ALIAS} 2>/dev/null | grep -q "${TEST_POLICY}"
}

test_admin_policy_info() {
    mc admin policy info ${MC_ALIAS} ${TEST_POLICY} > /dev/null 2>&1
}

test_admin_policy_attach() {
    if [ "${SERVER_TYPE}" = "MINIO" ]; then
        skip_test "Policy attach not supported in MinIO FS mode"
    fi
    mc admin policy attach ${MC_ALIAS} ${TEST_POLICY} --user ${TEST_USER} > /dev/null 2>&1
}

test_admin_policy_attached_to_user() {
    if [ "${SERVER_TYPE}" = "MINIO" ]; then
        skip_test "Policy attach not supported in MinIO FS mode"
    fi
    # Verify that the user-info shows the attached policy
    mc admin user info ${MC_ALIAS} ${TEST_USER} 2>/dev/null | grep -q "${TEST_POLICY}"
}

test_admin_group_add() {
    mc admin group add ${MC_ALIAS} ${TEST_GROUP} ${TEST_USER} > /dev/null 2>&1
}

test_admin_group_list() {
    mc admin group list ${MC_ALIAS} 2>/dev/null | grep -q "${TEST_GROUP}"
}

test_admin_group_info() {
    mc admin group info ${MC_ALIAS} ${TEST_GROUP} 2>/dev/null | grep -q "${TEST_USER}"
}

test_admin_user_disable() {
    mc admin user disable ${MC_ALIAS} ${TEST_USER} > /dev/null 2>&1
}

test_admin_user_enable() {
    mc admin user enable ${MC_ALIAS} ${TEST_USER} > /dev/null 2>&1
}

test_admin_user_remove() {
    mc admin user remove ${MC_ALIAS} ${TEST_USER} > /dev/null 2>&1
    # Verify removed
    if mc admin user info ${MC_ALIAS} ${TEST_USER} > /dev/null 2>&1; then
        echo "User still exists after removal"
        return 1
    fi
    return 0
}

test_admin_policy_remove() {
    mc admin policy remove ${MC_ALIAS} ${TEST_POLICY} > /dev/null 2>&1
}

#------------------------------------------------------------------------------
# Run All Tests
#------------------------------------------------------------------------------

run_test "AdminUserAdd"              "user_management"   "exit_code" test_admin_user_add
run_test "AdminUserList"             "user_management"   "exit_code" test_admin_user_list
run_test "AdminUserInfo"             "user_management"   "exit_code" test_admin_user_info
run_test "AdminPolicyCreate"         "policy_management" "exit_code" test_admin_policy_create
run_test "AdminPolicyList"           "policy_management" "exit_code" test_admin_policy_list
run_test "AdminPolicyInfo"           "policy_management" "exit_code" test_admin_policy_info
run_test "AdminPolicyAttach"         "policy_management" "exit_code" test_admin_policy_attach
run_test "AdminPolicyAttachedToUser" "policy_management" "exit_code" test_admin_policy_attached_to_user
run_test "AdminGroupAdd"             "group_management"  "exit_code" test_admin_group_add
run_test "AdminGroupList"            "group_management"  "exit_code" test_admin_group_list
run_test "AdminGroupInfo"            "group_management"  "exit_code" test_admin_group_info
run_test "AdminUserDisable"          "user_management"   "exit_code" test_admin_user_disable
run_test "AdminUserEnable"           "user_management"   "exit_code" test_admin_user_enable
run_test "AdminUserRemove"           "user_management"   "exit_code" test_admin_user_remove
run_test "AdminPolicyRemove"         "policy_management" "exit_code" test_admin_policy_remove

# Output JSON results
finalize_test_runner

if [ $TEST_FAILED -gt 0 ]; then
    exit 1
else
    exit 0
fi
