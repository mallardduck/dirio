#!/bin/bash
# Run all client compatibility tests for DirIO
#
# This script:
#   1. Builds the DirIO server
#   2. Starts a test instance
#   3. Runs all client test suites
#   4. Stops the server and cleans up
#
# Usage:
#   ./run_tests.sh [awscli|boto3|mc|all]
#
# Examples:
#   ./run_tests.sh          # Run all tests
#   ./run_tests.sh awscli   # Run only AWS CLI tests
#   ./run_tests.sh boto3    # Run only boto3 tests
#   ./run_tests.sh mc       # Run only MinIO client tests

# Don't use set -e as we want to continue even if some tests fail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

source "${SCRIPT_DIR}/config.sh"

# Which tests to run
TEST_SUITE="${1:-all}"

# Server process ID
SERVER_PID=""

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."

    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
        echo "Stopping DirIO server (PID: $SERVER_PID)..."
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi

    if [ -n "$DIRIO_DATA_DIR" ] && [ -d "$DIRIO_DATA_DIR" ]; then
        echo "Removing test data directory: $DIRIO_DATA_DIR"
        rm -rf "$DIRIO_DATA_DIR"
    fi

    echo "Cleanup complete."
}

# Set up trap for cleanup
trap cleanup EXIT INT TERM

print_header "DirIO Client Compatibility Test Suite"

# ============================================================================
# Build the server
# ============================================================================

echo "Building DirIO server..."
cd "${PROJECT_ROOT}"
go build -o "${PROJECT_ROOT}/bin/dirio" ./cmd/server

if [ ! -x "${PROJECT_ROOT}/bin/dirio" ]; then
    echo "Failed to build DirIO server"
    exit 1
fi

echo "Build successful: ${PROJECT_ROOT}/bin/dirio"

# ============================================================================
# Create test data directory
# ============================================================================

export DIRIO_DATA_DIR=$(mktemp -d -t dirio-client-test-XXXXXX)
echo "Test data directory: ${DIRIO_DATA_DIR}"

# ============================================================================
# Start the server
# ============================================================================

echo ""
echo "Starting DirIO server on port ${DIRIO_PORT}..."

"${PROJECT_ROOT}/bin/dirio" serve \
    --port "${DIRIO_PORT}" \
    --data-dir "${DIRIO_DATA_DIR}" \
    --access-key "${DIRIO_ACCESS_KEY}" \
    --secret-key "${DIRIO_SECRET_KEY}" \
    --log-level info \
    &

SERVER_PID=$!
echo "Server started with PID: $SERVER_PID"

# Wait for server to be ready
if ! wait_for_server; then
    echo "Server failed to start"
    exit 1
fi

# ============================================================================
# Run tests
# ============================================================================

RESULTS_DIR="${SCRIPT_DIR}/results"
mkdir -p "${RESULTS_DIR}"

run_awscli_tests() {
    print_header "Running AWS CLI Tests"
    if check_command aws; then
        bash "${SCRIPT_DIR}/test_awscli.sh" 2>&1 | tee "${RESULTS_DIR}/awscli.log"
        return ${PIPESTATUS[0]}
    else
        echo "AWS CLI not installed. Skipping."
        echo "Install with: brew install awscli"
        return 0
    fi
}

run_boto3_tests() {
    print_header "Running boto3 Tests"
    if check_command python3 && python3 -c "import boto3" 2>/dev/null; then
        python3 "${SCRIPT_DIR}/test_boto3.py" 2>&1 | tee "${RESULTS_DIR}/boto3.log"
        return ${PIPESTATUS[0]}
    else
        echo "boto3 not installed. Skipping."
        echo "Install with: pip install boto3"
        return 0
    fi
}

run_mc_tests() {
    print_header "Running MinIO Client Tests"
    if check_command mc; then
        bash "${SCRIPT_DIR}/test_minio_mc.sh" 2>&1 | tee "${RESULTS_DIR}/mc.log"
        return ${PIPESTATUS[0]}
    else
        echo "MinIO client (mc) not installed. Skipping."
        echo "Install with: brew install minio/stable/mc"
        return 0
    fi
}

EXIT_CODE=0

case "$TEST_SUITE" in
    awscli)
        run_awscli_tests || EXIT_CODE=$?
        ;;
    boto3)
        run_boto3_tests || EXIT_CODE=$?
        ;;
    mc)
        run_mc_tests || EXIT_CODE=$?
        ;;
    all)
        run_awscli_tests || EXIT_CODE=$?

        # Reset server state between test suites
        echo "Resetting test data..."
        rm -rf "${DIRIO_DATA_DIR:?}"/*

        run_boto3_tests || EXIT_CODE=$?

        # Reset server state
        echo "Resetting test data..."
        rm -rf "${DIRIO_DATA_DIR:?}"/*

        run_mc_tests || EXIT_CODE=$?
        ;;
    *)
        echo "Unknown test suite: $TEST_SUITE"
        echo "Usage: $0 [awscli|boto3|mc|all]"
        exit 1
        ;;
esac

# ============================================================================
# Summary
# ============================================================================

print_header "Test Results"
echo "Results saved in: ${RESULTS_DIR}"
echo ""

if [ -f "${RESULTS_DIR}/awscli.log" ]; then
    echo "AWS CLI: $(grep -E 'Passed:|Failed:' "${RESULTS_DIR}/awscli.log" | tail -2 | tr '\n' ' ')"
fi

if [ -f "${RESULTS_DIR}/boto3.log" ]; then
    echo "boto3:   $(grep -E 'Passed:|Failed:' "${RESULTS_DIR}/boto3.log" | tail -2 | tr '\n' ' ')"
fi

if [ -f "${RESULTS_DIR}/mc.log" ]; then
    echo "mc:      $(grep -E 'Passed:|Failed:' "${RESULTS_DIR}/mc.log" | tail -2 | tr '\n' ' ')"
fi

echo ""

exit $EXIT_CODE