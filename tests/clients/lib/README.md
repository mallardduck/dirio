# Test Framework Library Documentation

This directory contains shared test framework and validation libraries for S3 client integration tests.

## Overview

The test framework provides:
- **Dual output:** Human-readable progress (stderr) + structured JSON results (stdout)
- **Standardized validation:** Consistent content integrity and metadata checks
- **Test lifecycle management:** Initialization, execution, result collection, and reporting
- **Cross-language support:** Bash and Python implementations with identical APIs

## Files

- `test_framework.sh` - Bash test runner with JSON output
- `test_framework.py` - Python test runner with JSON output
- `validators.sh` - Bash validation functions
- `validators.py` - Python validation functions

---

## Bash Test Framework (`test_framework.sh`)

### API Functions

#### `init_test_runner <client_name> <version>`

Initialize the test runner with client metadata.

**Parameters:**
- `client_name` - Name of the S3 client (e.g., "awscli", "boto3", "mc")
- `version` - Client version string

**Example:**
```bash
AWS_VERSION=$(aws --version 2>&1 | head -n1)
init_test_runner "awscli" "$AWS_VERSION"
```

#### `run_test <feature_name> <category> <validation_type> <test_function>`

Execute a test function and capture the result.

**Parameters:**
- `feature_name` - Feature name matching features.yaml (e.g., "PutObject")
- `category` - Category name (e.g., "object_operations")
- `validation_type` - Type of validation (e.g., "content_integrity")
- `test_function` - Name of the bash function to execute

**Example:**
```bash
test_put_object() {
    aws s3api put-object --bucket $BUCKET --key test.txt --body /tmp/test.txt
}

run_test "PutObject" "object_operations" "content_integrity" test_put_object
```

**Test function behavior:**
- Exit code 0 = PASS
- Exit code 77 = SKIP
- Any other exit code = FAIL

#### `finalize_test_runner`

Output complete JSON results to stdout. Call this at the end of the test script.

**Example:**
```bash
finalize_test_runner
```

#### `compute_hash <file_path>`

Compute MD5 hash of a file.

**Parameters:**
- `file_path` - Path to the file

**Returns:** MD5 hash string (stdout)

**Example:**
```bash
HASH=$(compute_hash /tmp/test.txt)
```

#### Helper Functions

**`skip_test <reason>`**
- Skip the current test with a reason
- Exits with code 77

**`fail_test <message>`**
- Fail the current test with an error message
- Exits with code 1

**`pass_test`**
- Pass the current test
- Exits with code 0

**Example:**
```bash
test_optional_feature() {
    if ! command -v special_tool &> /dev/null; then
        skip_test "special_tool not available"
    fi

    if ! special_tool --do-something; then
        fail_test "special_tool failed"
    fi

    pass_test
}
```

### Complete Example

```bash
#!/bin/bash
set -euo pipefail

# Source framework
source "$(dirname "$0")/../lib/test_framework.sh"

# Initialize
init_test_runner "myclient" "1.0.0"

# Define test function
test_list_buckets() {
    myclient list-buckets > /dev/null
}

# Run test
run_test "ListBuckets" "bucket_operations" "exit_code" test_list_buckets

# Output results
finalize_test_runner
```

---

## Bash Validators (`validators.sh`)

### API Functions

#### `validate_content_integrity <original_file> <downloaded_file>`

Validate that two files have identical content by comparing MD5 hashes.

**Parameters:**
- `original_file` - Path to original file
- `downloaded_file` - Path to downloaded file

**Returns:** Exits with 0 (pass) or 1 (fail)

**Example:**
```bash
test_get_object() {
    echo "test content" > /tmp/original.txt
    aws s3api put-object --bucket $BUCKET --key test.txt --body /tmp/original.txt
    aws s3api get-object --bucket $BUCKET --key test.txt /tmp/downloaded.txt

    # Validate
    validate_content_integrity /tmp/original.txt /tmp/downloaded.txt
}
```

#### `validate_partial_content <file> <expected_size>`

Validate that a file has the expected size (for range requests).

**Parameters:**
- `file` - Path to the file
- `expected_size` - Expected size in bytes

**Example:**
```bash
test_range_request() {
    # Request first 10 bytes
    aws s3api get-object --bucket $BUCKET --key test.txt --range bytes=0-9 /tmp/partial.txt

    # Validate size
    validate_partial_content /tmp/partial.txt 10
}
```

#### `validate_metadata <metadata_output> <key> <expected_value>`

Validate metadata key-value pair (case-insensitive key matching).

**Parameters:**
- `metadata_output` - File path or string containing metadata
- `key` - Metadata key to check (case-insensitive)
- `expected_value` - Expected value (case-sensitive)

**Example:**
```bash
test_custom_metadata_get() {
    aws s3api head-object --bucket $BUCKET --key test.txt > /tmp/metadata.txt

    # Validate metadata
    validate_metadata /tmp/metadata.txt "custom-key" "custom-value"
}
```

#### `validate_json_field <json> <field_path> <expected_value>`

Validate JSON field presence and value using jq syntax.

**Parameters:**
- `json` - JSON string or file path
- `field_path` - jq field path (e.g., ".bucket.name")
- `expected_value` - Expected value (optional, checks presence if empty)

**Example:**
```bash
test_get_bucket_location() {
    aws s3api get-bucket-location --bucket $BUCKET > /tmp/location.json

    # Validate JSON structure
    validate_json_field /tmp/location.json ".LocationConstraint" ""
}
```

---

## Python Test Framework (`test_framework.py`)

### API Classes and Functions

#### `TestRunner(client: str, version: str)`

Main test runner class.

**Parameters:**
- `client` - Name of the S3 client
- `version` - Client version string

**Methods:**
- `register_test(feature, category, validation_type, test_func)` - Register a test
- `run_all_tests()` - Execute all registered tests
- `output_json()` - Output JSON results to stdout
- `get_summary()` - Get TestSummary object

**Example:**
```python
import boto3
from test_framework import TestRunner

runner = TestRunner("boto3", boto3.__version__)

def test_list_buckets():
    s3 = boto3.client('s3', endpoint_url=ENDPOINT)
    s3.list_buckets()

runner.register_test("ListBuckets", "bucket_operations", "exit_code", test_list_buckets)
runner.run_all_tests()
runner.output_json()
```

#### `compute_hash(file_path: str) -> str`

Compute MD5 hash of a file.

**Example:**
```python
from test_framework import compute_hash

original_hash = compute_hash("/tmp/original.txt")
downloaded_hash = compute_hash("/tmp/downloaded.txt")
```

#### `skip_test(reason: str = "")`

Skip the current test.

**Example:**
```python
from test_framework import skip_test

def test_optional_feature():
    if not feature_available():
        skip_test("Feature not available")
    # Test code...
```

### Complete Example

```python
#!/usr/bin/env python3
import sys
sys.path.insert(0, '/tmp/lib')

from test_framework import TestRunner, skip_test
from validators import validate_content_integrity

# Initialize runner
runner = TestRunner("myclient", "1.0.0")

# Define test
def test_put_object():
    with open("/tmp/test.txt", "wb") as f:
        f.write(b"test content")

    client.put_object(Bucket=BUCKET, Key="test.txt", Body=b"test content")

# Register and run
runner.register_test("PutObject", "object_operations", "content_integrity", test_put_object)
runner.run_all_tests()
runner.output_json()

# Exit with appropriate code
summary = runner.get_summary()
sys.exit(1 if summary.failed > 0 else 0)
```

---

## Python Validators (`validators.py`)

All validators return `(success: bool, error_message: str)` tuples.

### API Functions

#### `validate_content_integrity(original_file: str, downloaded_file: str) -> Tuple[bool, str]`

Validate content integrity by comparing MD5 hashes.

**Example:**
```python
from validators import validate_content_integrity

success, error = validate_content_integrity("/tmp/original.txt", "/tmp/downloaded.txt")
if not success:
    raise Exception(error)
```

#### `validate_partial_content(file_path: str, expected_size: int) -> Tuple[bool, str]`

Validate file size for range requests.

**Example:**
```python
from validators import validate_partial_content

# After downloading with Range: bytes=0-9
success, error = validate_partial_content("/tmp/partial.txt", 10)
if not success:
    raise Exception(error)
```

#### `validate_metadata(metadata: Dict[str, str], key: str, expected_value: str) -> Tuple[bool, str]`

Validate metadata key-value pair (case-insensitive key matching).

**Example:**
```python
from validators import validate_metadata

response = s3.head_object(Bucket=BUCKET, Key="test.txt")
metadata = response.get("Metadata", {})

success, error = validate_metadata(metadata, "custom-key", "custom-value")
if not success:
    raise Exception(error)
```

#### Other Validators

**`validate_file_exists(file_path: str) -> Tuple[bool, str]`**
- Check if file exists

**`validate_file_not_exists(file_path: str) -> Tuple[bool, str]`**
- Check if file does NOT exist

**`validate_response_code(actual: int, expected: int) -> Tuple[bool, str]`**
- Validate HTTP response code

**`validate_not_empty(value: str, field_name: str = "value") -> Tuple[bool, str]`**
- Check if string is not empty

**`validate_contains(haystack: str, needle: str, case_sensitive: bool = True) -> Tuple[bool, str]`**
- Check if string contains substring

---

## Best Practices

### 1. Always Validate Content Integrity

For any operation that reads or writes data:
```bash
# Bash
echo "test data" > /tmp/original.txt
aws s3api put-object --bucket $BUCKET --key test.txt --body /tmp/original.txt
aws s3api get-object --bucket $BUCKET --key test.txt /tmp/downloaded.txt
validate_content_integrity /tmp/original.txt /tmp/downloaded.txt
```

```python
# Python
with open("/tmp/original.txt", "wb") as f:
    f.write(b"test data")

s3.put_object(Bucket=BUCKET, Key="test.txt", Body=b"test data")
s3.get_object(Bucket=BUCKET, Key="test.txt")

success, error = validate_content_integrity("/tmp/original.txt", "/tmp/downloaded.txt")
if not success:
    raise Exception(error)
```

### 2. Use Descriptive Error Messages

```bash
if ! some_command; then
    fail_test "Command failed with unexpected error"
fi
```

```python
if not condition:
    raise Exception("Detailed error message explaining what went wrong")
```

### 3. Handle Optional Features

```bash
test_optional_feature() {
    if ! command -v special_tool &> /dev/null; then
        skip_test "special_tool not installed"
    fi
    # Test code...
}
```

```python
def test_optional_feature():
    if not has_feature():
        skip_test("Feature not available")
    # Test code...
```

### 4. Clean Up Test Data

```bash
# Create temporary files in /tmp
# They'll be cleaned up automatically on container exit
```

### 5. Match Feature Names Exactly

Feature names in test code must match `features.yaml` exactly:
```yaml
# features.yaml
- name: CustomMetadata_Set
```

```bash
# awscli.sh
run_test "CustomMetadata_Set" "metadata_operations" "content_integrity" test_custom_metadata_set
```

```python
# boto3.py
runner.register_test("CustomMetadata_Set", "metadata_operations", "content_integrity", test_custom_metadata_set)
```

---

## Troubleshooting

### JSON Output Not Generated

**Problem:** Test script doesn't output JSON

**Solution:** Ensure you call `finalize_test_runner` (bash) or `runner.output_json()` (Python)

### Content Integrity Failures

**Problem:** Hash mismatch errors

**Solution:**
1. Check file paths are correct
2. Ensure files exist before validation
3. Verify no extra data is appended during transfer

### Import Errors (Python)

**Problem:** `ImportError: No module named 'test_framework'`

**Solution:** Ensure lib directory is in Python path:
```python
import sys
sys.path.insert(0, '/tmp/lib')  # In containers
# or
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'lib'))  # Local
```

### Test Functions Not Found (Bash)

**Problem:** `bash: test_function: command not found`

**Solution:** Ensure function is defined before calling `run_test`:
```bash
test_my_feature() {
    # Test code
}

# THEN call run_test
run_test "MyFeature" "category" "validation" test_my_feature
```

---

## Contributing

When adding new validation functions:

1. **Add to both bash and Python** versions
2. **Use consistent naming** across languages
3. **Return appropriate types**:
   - Bash: Exit with 0 (success) or 1 (failure)
   - Python: Return `(bool, str)` tuple
4. **Include error messages** that help debug failures
5. **Update this README** with documentation and examples
