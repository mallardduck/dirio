# Debugging Client Tests

## Preserving Test Data

When debugging client test failures, you often need to inspect the server's data directory to understand what happened. By default, tests clean up temporary directories after completion.

### Preserve Test Data on Windows

To preserve test data when running Go tests on Windows:

```powershell
# PowerShell
$env:DIRIO_PRESERVE_TEST_DATA="1"
go test -v -run TestAWSCLI ./tests/clients

# Or in a single command
$env:DIRIO_PRESERVE_TEST_DATA="1"; go test -v -run TestAWSCLI ./tests/clients
```

```cmd
# Command Prompt
set DIRIO_PRESERVE_TEST_DATA=1
go test -v -run TestAWSCLI .\tests\clients
```

### Preserve Test Data on Linux/Mac

```bash
DIRIO_PRESERVE_TEST_DATA=1 go test -v -run TestAWSCLI ./tests/clients
```

### What Gets Preserved

When `DIRIO_PRESERVE_TEST_DATA` is set:
1. The temporary server data directory is NOT deleted after tests
2. The test output will show the data directory path:
   ```
   Test server data directory: C:\Users\Dan\AppData\Local\Temp\dirio-client-test-123456
   ⚠️  DIRIO_PRESERVE_TEST_DATA is set - data will NOT be cleaned up
   ```
3. On test completion, you'll see:
   ```
   PRESERVED TEST DATA: C:\Users\Dan\AppData\Local\Temp\dirio-client-test-123456
   ```

### Inspecting Preserved Data

Navigate to the preserved directory to inspect:
- Bucket structures
- Object contents
- Metadata files
- Any server-created artifacts

```powershell
# Navigate to the preserved directory
cd "C:\Users\Dan\AppData\Local\Temp\dirio-client-test-123456"

# List contents
dir /s

# View specific bucket
dir testbucket
```

### Running Specific Tests

Run individual test functions to isolate issues:

```powershell
# Run only AWS CLI tests
$env:DIRIO_PRESERVE_TEST_DATA="1"; go test -v -run TestAWSCLI ./tests/clients

# Run only boto3 tests
$env:DIRIO_PRESERVE_TEST_DATA="1"; go test -v -run TestBoto3 ./tests/clients

# Run only MinIO mc tests
$env:DIRIO_PRESERVE_TEST_DATA="1"; go test -v -run TestMinIOMC ./tests/clients

# Run sanity checks
$env:DIRIO_PRESERVE_TEST_DATA="1"; go test -v -run TestSanityCheck ./tests/clients
```

### Cleanup

Remember to manually delete preserved directories when done:

```powershell
# Find all preserved test directories
Get-ChildItem $env:TEMP -Filter "dirio-client-test-*" -Directory

# Delete them
Get-ChildItem $env:TEMP -Filter "dirio-client-test-*" -Directory | Remove-Item -Recurse -Force
```

## Running Tests with Verbose Output

For maximum debugging information:

```powershell
# Run with verbose output and preserve data
$env:DIRIO_PRESERVE_TEST_DATA="1"; go test -v -run TestAWSCLI ./tests/clients 2>&1 | Tee-Object -FilePath test-output.log
```

This will:
- Show verbose test output (`-v`)
- Preserve the server data directory
- Save all output to `test-output.log`

## Debugging Container Tests

The tests run client commands inside Docker containers. To see what the containers are doing:

1. **Review container logs**: Test output includes container logs automatically
2. **Check server logs**: The DirIO server output goes to stdout/stderr (visible in test output with `-v`)
3. **Inspect preserved data**: Use `DIRIO_PRESERVE_TEST_DATA=1` to see what the server created

## Common Issues

### Tests Pass But Shouldn't?

Run the sanity checks to ensure tests are actually validating:

```powershell
go test -v -run TestSanityCheck ./tests/clients
```

These tests verify that the test scripts correctly detect failures.

### Need to Debug Server Behavior?

1. Set `DIRIO_PRESERVE_TEST_DATA=1`
2. Run the specific failing test
3. Check the test output for the data directory path
4. Inspect the directory structure and file contents
5. Compare against expected results
