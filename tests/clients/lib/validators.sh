#!/usr/bin/env bash
# Validation functions for S3 client integration tests
# Provides standardized validation across all bash-based test scripts
# NOTE: This file expects test_framework.sh to be sourced first

set -euo pipefail

# Validate content integrity by comparing MD5 hashes
# Usage: validate_content_integrity <original_file> <downloaded_file>
# Returns: 0 on success, 1 on failure (with error message to stderr)
validate_content_integrity() {
    local original="$1"
    local downloaded="$2"

    if [ ! -f "$original" ]; then
        fail_test "Original file not found: $original"
    fi

    if [ ! -f "$downloaded" ]; then
        fail_test "Downloaded file not found: $downloaded"
    fi

    local original_hash=$(compute_hash "$original")
    local downloaded_hash=$(compute_hash "$downloaded")

    if [ "$original_hash" != "$downloaded_hash" ]; then
        fail_test "Content integrity check failed: hash mismatch (expected: $original_hash, got: $downloaded_hash)"
    fi

    pass_test
}

# Validate partial content by checking file size
# Usage: validate_partial_content <file> <expected_size>
# Returns: 0 on success, 1 on failure (with error message to stderr)
validate_partial_content() {
    local file="$1"
    local expected_size="$2"

    if [ ! -f "$file" ]; then
        fail_test "File not found: $file"
    fi

    local actual_size=$(wc -c < "$file")

    if [ "$actual_size" -ne "$expected_size" ]; then
        fail_test "Partial content size mismatch: expected $expected_size bytes, got $actual_size bytes"
    fi

    pass_test
}

# Validate metadata presence and value (case-insensitive key matching)
# Usage: validate_metadata <metadata_output> <key> <expected_value>
# metadata_output can be a file path or string containing metadata
# Returns: 0 on success, 1 on failure (with error message to stderr)
validate_metadata() {
    local metadata="$1"
    local key="$2"
    local expected_value="$3"

    # If metadata is a file, read its contents
    if [ -f "$metadata" ]; then
        metadata=$(cat "$metadata")
    fi

    # Convert key to lowercase for case-insensitive matching
    local key_lower=$(echo "$key" | tr '[:upper:]' '[:lower:]')

    # Search for the key (case-insensitive) in the metadata
    # Support various formats: "key: value", "key=value", "key = value"
    local found=false
    local actual_value=""

    while IFS= read -r line; do
        # Extract key and value from line (support : and = separators)
        if [[ "$line" =~ ^([^:=]+)[:=](.+)$ ]]; then
            local line_key="${BASH_REMATCH[1]}"
            local line_value="${BASH_REMATCH[2]}"

            # Trim whitespace
            line_key=$(echo "$line_key" | xargs)
            line_value=$(echo "$line_value" | xargs)

            # Case-insensitive key comparison
            local line_key_lower=$(echo "$line_key" | tr '[:upper:]' '[:lower:]')

            if [ "$line_key_lower" = "$key_lower" ]; then
                found=true
                actual_value="$line_value"
                break
            fi
        fi
    done <<< "$metadata"

    if [ "$found" = false ]; then
        fail_test "Metadata key not found: $key"
    fi

    if [ "$actual_value" != "$expected_value" ]; then
        fail_test "Metadata value mismatch for key '$key': expected '$expected_value', got '$actual_value'"
    fi

    pass_test
}

# Validate JSON field presence and value
# Usage: validate_json_field <json_string> <field_path> <expected_value>
# field_path uses jq syntax (e.g., ".bucket.name", ".objects[0].key")
# expected_value can be empty to just check presence
# Returns: 0 on success, 1 on failure (with error message to stderr)
validate_json_field() {
    local json="$1"
    local field_path="$2"
    local expected_value="${3:-}"

    if ! command -v jq &> /dev/null; then
        fail_test "jq is required for JSON validation but not found"
    fi

    # If json is a file, read its contents
    if [ -f "$json" ]; then
        json=$(cat "$json")
    fi

    # Extract the field value
    local actual_value
    if ! actual_value=$(echo "$json" | jq -r "$field_path" 2>&1); then
        fail_test "Failed to extract field '$field_path' from JSON: $actual_value"
    fi

    # Check if field exists (not null)
    if [ "$actual_value" = "null" ]; then
        fail_test "JSON field not found or is null: $field_path"
    fi

    # If expected value provided, compare
    if [ -n "$expected_value" ]; then
        if [ "$actual_value" != "$expected_value" ]; then
            fail_test "JSON field value mismatch for '$field_path': expected '$expected_value', got '$actual_value'"
        fi
    fi

    pass_test
}

# Validate that a file exists
# Usage: validate_file_exists <file_path>
# Returns: 0 on success, 1 on failure
validate_file_exists() {
    local file="$1"

    if [ ! -f "$file" ]; then
        fail_test "File does not exist: $file"
    fi

    pass_test
}

# Validate that a file does not exist
# Usage: validate_file_not_exists <file_path>
# Returns: 0 on success, 1 on failure
validate_file_not_exists() {
    local file="$1"

    if [ -f "$file" ]; then
        fail_test "File exists but should not: $file"
    fi

    pass_test
}

# Validate command exit code
# Usage: validate_exit_code <expected_code> <command>
# Returns: 0 on success, 1 on failure
validate_exit_code() {
    local expected_code="$1"
    shift
    local command=("$@")

    set +e
    "${command[@]}" &> /dev/null
    local actual_code=$?
    set -e

    if [ "$actual_code" -ne "$expected_code" ]; then
        fail_test "Exit code mismatch: expected $expected_code, got $actual_code"
    fi

    pass_test
}
