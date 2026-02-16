#!/usr/bin/env bash
# Test framework for bash-based S3 client integration tests
# Provides structured test execution with JSON output

set -euo pipefail

# Global variables for test state
declare -g TEST_CLIENT=""
declare -g TEST_VERSION=""
declare -g TEST_RUN_ID=""
declare -g TEST_START_TIME=""
declare -a TEST_RESULTS=()
declare -g TEST_TOTAL=0
declare -g TEST_PASSED=0
declare -g TEST_FAILED=0
declare -g TEST_SKIPPED=0

# Initialize the test runner with client metadata
# Usage: init_test_runner <client_name> <version>
init_test_runner() {
    local client="$1"
    local version="$2"

    TEST_CLIENT="$client"
    TEST_VERSION="$version"
    TEST_RUN_ID="$(date +%s)"
    TEST_START_TIME="$(date +%s%3N)"
    TEST_RESULTS=()
    TEST_TOTAL=0
    TEST_PASSED=0
    TEST_FAILED=0
    TEST_SKIPPED=0

    echo "Initializing test runner for $client version $version" >&2
}

# Compute MD5 hash of a file
# Usage: compute_hash <file_path>
# Returns: MD5 hash string
compute_hash() {
    local file="$1"

    if command -v md5sum &> /dev/null; then
        md5sum "$file" | awk '{print $1}'
    elif command -v md5 &> /dev/null; then
        md5 -q "$file"
    else
        echo "ERROR: No MD5 utility available" >&2
        return 1
    fi
}

# Escape JSON string values
json_escape() {
    local str="$1"
    # Escape backslashes, quotes, and newlines
    str="${str//\\/\\\\}"
    str="${str//\"/\\\"}"
    str="${str//$'\n'/\\n}"
    str="${str//$'\r'/\\r}"
    str="${str//$'\t'/\\t}"
    echo "$str"
}

# Run a test and capture the result
# Usage: run_test <feature_name> <category> <validation_type> <test_function>
# Example: run_test "PutObject" "object_operations" "content_integrity" test_put_object
run_test() {
    local feature="$1"
    local category="$2"
    local validation_type="$3"
    local test_function="$4"

    local start_time="$(date +%s%3N)"
    local status="pass"
    local message=""

    ((TEST_TOTAL++)) || true

    # Execute the test function and capture output/errors
    if output=$("$test_function" 2>&1); then
        status="pass"
        ((TEST_PASSED++)) || true
        echo "PASS: $feature" >&2
    else
        local exit_code=$?
        if [ $exit_code -eq 77 ]; then
            # Exit code 77 indicates skip
            status="skip"
            message="Test skipped"
            ((TEST_SKIPPED++)) || true
            echo "SKIP: $feature" >&2
        else
            status="fail"
            message="$output"
            ((TEST_FAILED++)) || true
            echo "FAIL: $feature" >&2
            echo "  Error: $output" >&2
        fi
    fi

    local end_time="$(date +%s%3N)"
    local duration=$((end_time - start_time))

    # Escape message for JSON
    message="$(json_escape "$message")"

    # Store result as JSON string
    local result=$(cat <<EOF
{
  "feature": "$feature",
  "category": "$category",
  "status": "$status",
  "duration_ms": $duration,
  "message": "$message",
  "details": {
    "validation_type": "$validation_type"
  }
}
EOF
)

    TEST_RESULTS+=("$result")
}

# Finalize test runner and output JSON results
# Usage: finalize_test_runner
# Outputs JSON to stdout
finalize_test_runner() {
    local end_time="$(date +%s%3N)"
    local total_duration=$((end_time - TEST_START_TIME))

    # Build results array
    local results_json="["
    local first=true
    for result in "${TEST_RESULTS[@]}"; do
        if [ "$first" = true ]; then
            first=false
        else
            results_json+=","
        fi
        results_json+=$'\n'"  $result"
    done
    results_json+=$'\n'"]"

    # Output complete JSON document
    cat <<EOF
{
  "meta": {
    "client": "$TEST_CLIENT",
    "version": "$TEST_VERSION",
    "test_run_id": "$TEST_RUN_ID",
    "duration_ms": $total_duration
  },
  "results": $results_json,
  "summary": {
    "total": $TEST_TOTAL,
    "passed": $TEST_PASSED,
    "failed": $TEST_FAILED,
    "skipped": $TEST_SKIPPED
  }
}
EOF
}

# Helper function to skip a test
# Usage: skip_test "Reason for skipping"
skip_test() {
    local reason="$1"
    echo "SKIP: $reason" >&2
    exit 77  # Special exit code for skip
}

# Helper function to fail a test with message
# Usage: fail_test "Error message"
fail_test() {
    local message="$1"
    echo "$message" >&2
    exit 1
}

# Helper function to pass a test
# Usage: pass_test
pass_test() {
    exit 0
}
