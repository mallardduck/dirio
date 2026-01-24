#!/bin/bash
# Common configuration for all client tests

# Server configuration
export DIRIO_PORT="${DIRIO_PORT:-9876}"
export DIRIO_ENDPOINT="http://localhost:${DIRIO_PORT}"
export DIRIO_ACCESS_KEY="${DIRIO_ACCESS_KEY:-testaccess}"
export DIRIO_SECRET_KEY="${DIRIO_SECRET_KEY:-testsecret}"
export DIRIO_REGION="${DIRIO_REGION:-us-east-1}"

# Test data directory (created per test run)
export DIRIO_DATA_DIR="${DIRIO_DATA_DIR:-}"

# Test bucket and object names
export TEST_BUCKET="test-bucket-$(date +%s)"
export TEST_OBJECT_KEY="test-object.txt"
export TEST_OBJECT_CONTENT="Hello from DirIO client test!"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters for test results
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

# Print functions
print_header() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

print_test() {
    echo -e "\n${YELLOW}TEST: $1${NC}"
}

print_pass() {
    echo -e "${GREEN}PASS: $1${NC}"
    ((TESTS_PASSED++))
}

print_fail() {
    echo -e "${RED}FAIL: $1${NC}"
    if [ -n "$2" ]; then
        echo -e "${RED}      Error: $2${NC}"
    fi
    ((TESTS_FAILED++))
}

print_skip() {
    echo -e "${YELLOW}SKIP: $1${NC}"
    ((TESTS_SKIPPED++))
}

print_summary() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}TEST SUMMARY${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo -e "${GREEN}Passed:  ${TESTS_PASSED}${NC}"
    echo -e "${RED}Failed:  ${TESTS_FAILED}${NC}"
    echo -e "${YELLOW}Skipped: ${TESTS_SKIPPED}${NC}"
    echo -e "${BLUE}----------------------------------------${NC}"

    if [ ${TESTS_FAILED} -eq 0 ]; then
        echo -e "${GREEN}All tests passed!${NC}"
        return 0
    else
        echo -e "${RED}Some tests failed.${NC}"
        return 1
    fi
}

# Check if command exists
check_command() {
    if command -v "$1" &> /dev/null; then
        return 0
    else
        return 1
    fi
}

# Wait for server to be ready
wait_for_server() {
    local max_attempts=30
    local attempt=0

    echo "Waiting for server at ${DIRIO_ENDPOINT}..."
    while [ $attempt -lt $max_attempts ]; do
        if curl -s "${DIRIO_ENDPOINT}/" > /dev/null 2>&1; then
            echo "Server is ready!"
            return 0
        fi
        ((attempt++))
        sleep 0.5
    done

    echo "Server failed to start after ${max_attempts} attempts"
    return 1
}