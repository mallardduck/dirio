package clients_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	testAccessKey = "testaccess"
	testSecretKey = "testsecret"
	testRegion    = "us-east-1"
)

// Global cleanup tracking
var (
	activeContainers   = make([]testcontainers.Container, 0)
	activeServers      = make([]*TestServer, 0)
	cleanupMutex       sync.Mutex
	cleanupContext     context.Context
	cleanupCancelFunc  context.CancelFunc
	cleanupInitialized bool
)

// TestServer manages the DirIO server for testing
type TestServer struct {
	cmd     *exec.Cmd
	port    int
	dataDir string
}

func shouldRunSlowTests() bool {
	env := os.Getenv("RUN_SLOW_TESTS")
	return env == "1" || strings.ToLower(env) == "true"
}

func runSlowCheck(t *testing.T) {
	if !shouldRunSlowTests() {
		t.Skip("Skipping slow tests")
	}

	if testing.Short() {
		t.Skip("Skipping client tests in short mode")
	}
}

func TestMain(m *testing.M) {
	if !shouldRunSlowTests() {
		// Write directly to terminal (bypasses go test output capture)
		if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
			fmt.Fprintln(tty, "Slow client tests disabled. Set RUN_SLOW_TESTS=1 to enable.")
			tty.Close()
		}
		os.Exit(0)
	}
	if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		fmt.Fprintln(tty, "Running slow client tests...")
		tty.Close()
	}

	// Initialize cleanup context
	cleanupContext, cleanupCancelFunc = context.WithCancel(context.Background())
	cleanupInitialized = true

	// Set up signal handling for graceful cleanup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nReceived interrupt signal, cleaning up...")
		performCleanup()
		os.Exit(130) // Standard exit code for SIGINT
	}()

	exitCode := m.Run()

	// Clean up on normal exit too
	performCleanup()

	os.Exit(exitCode)
}

// registerContainer adds a container to the cleanup list
func registerContainer(container testcontainers.Container) {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()
	activeContainers = append(activeContainers, container)
}

// registerServer adds a server to the cleanup list
func registerServer(server *TestServer) {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()
	activeServers = append(activeServers, server)
}

// unregisterContainer removes a container from the cleanup list
func unregisterContainer(container testcontainers.Container) {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()
	for i, c := range activeContainers {
		if c == container {
			activeContainers = append(activeContainers[:i], activeContainers[i+1:]...)
			break
		}
	}
}

// unregisterServer removes a server from the cleanup list
func unregisterServer(server *TestServer) {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()
	for i, s := range activeServers {
		if s == server {
			activeServers = append(activeServers[:i], activeServers[i+1:]...)
			break
		}
	}
}

// performCleanup terminates all active containers and servers
func performCleanup() {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()

	if cleanupCancelFunc != nil {
		cleanupCancelFunc()
	}

	// Cleanup containers
	for _, container := range activeContainers {
		if container != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			container.Terminate(ctx)
			cancel()
		}
	}
	activeContainers = nil

	// Cleanup servers
	for _, server := range activeServers {
		if server != nil && server.cmd != nil && server.cmd.Process != nil {
			server.cmd.Process.Kill()
			server.cmd.Wait()
		}
		if server != nil && server.dataDir != "" {
			os.RemoveAll(server.dataDir)
		}
	}
	activeServers = nil
}

// startTestServer builds and starts the DirIO server
func startTestServer(t *testing.T) *TestServer {
	t.Helper()

	// Find project root
	projectRoot := findProjectRoot(t)

	// Build the server
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(projectRoot, "bin", "dirio-test"), "./cmd/server")
	buildCmd.Dir = projectRoot
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "Failed to build server: %s", string(output))

	// Create temp data directory
	dataDir, err := os.MkdirTemp("", "dirio-client-test-*")
	require.NoError(t, err)

	// Find available port
	port := findAvailablePort(t)

	// Start server
	serverPath := filepath.Join(projectRoot, "bin", "dirio-test")
	cmd := exec.Command(serverPath, "serve",
		"--port", fmt.Sprintf("%d", port),
		"--data-dir", dataDir,
		"--access-key", testAccessKey,
		"--secret-key", testSecretKey,
		"--log-level", "warn",
	)
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	require.NoError(t, err, "Failed to start server")

	ts := &TestServer{
		cmd:     cmd,
		port:    port,
		dataDir: dataDir,
	}

	// Wait for server to be ready
	require.True(t, waitForServer(t, port, 10*time.Second), "Server failed to start")

	return ts
}

func (ts *TestServer) Stop(t *testing.T) {
	t.Helper()

	if ts.cmd != nil && ts.cmd.Process != nil {
		ts.cmd.Process.Kill()
		ts.cmd.Wait()
	}

	if ts.dataDir != "" {
		os.RemoveAll(ts.dataDir)
	}
}

func (ts *TestServer) Endpoint() string {
	return fmt.Sprintf("http://host.docker.internal:%d", ts.port)
}

func (ts *TestServer) LocalEndpoint() string {
	return fmt.Sprintf("http://localhost:%d", ts.port)
}

func findProjectRoot(t *testing.T) string {
	t.Helper()

	// Start from current directory and walk up
	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find project root (go.mod)")
		}
		dir = parent
	}
}

func findAvailablePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func waitForServer(t *testing.T, port int, timeout time.Duration) bool {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// TestAWSCLI runs AWS CLI compatibility tests
func TestAWSCLI(t *testing.T) {
	runSlowCheck(t)

	ctx := context.Background()

	// Start the DirIO server
	server := startTestServer(t)
	defer server.Stop(t)

	t.Logf("DirIO server started on port %d", server.port)

	// Use alpine with AWS CLI installed (has proper shell)
	req := testcontainers.ContainerRequest{
		Image: "amazon/aws-cli:2.15.0",
		Env: map[string]string{
			"AWS_ACCESS_KEY_ID":     testAccessKey,
			"AWS_SECRET_ACCESS_KEY": testSecretKey,
			"AWS_DEFAULT_REGION":    testRegion,
			"DIRIO_ENDPOINT":        server.Endpoint(),
		},
		Entrypoint: []string{"/bin/bash", "-c"},
		Cmd: []string{
			awsCLITestScript(),
		},
		WaitingFor: wait.ForExit().WithExitTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	defer container.Terminate(ctx)

	// Get container logs
	logs, err := container.Logs(ctx)
	require.NoError(t, err)
	defer logs.Close()

	logBytes, err := io.ReadAll(logs)
	require.NoError(t, err)
	logOutput := string(logBytes)

	t.Logf("AWS CLI test output:\n%s", logOutput)

	// Check for test results
	state, err := container.State(ctx)
	require.NoError(t, err)

	if state.ExitCode != 0 {
		t.Errorf("AWS CLI tests failed with exit code %d", state.ExitCode)
	}

	// Parse and report results
	if strings.Contains(logOutput, "All tests passed") {
		t.Log("AWS CLI: All tests passed")
	} else {
		// Count passed/failed from output
		passCount := strings.Count(logOutput, "PASS:")
		failCount := strings.Count(logOutput, "FAIL:")
		t.Logf("AWS CLI: %d passed, %d failed", passCount, failCount)
	}
}

// TestBoto3 runs boto3 (Python) compatibility tests
func TestBoto3(t *testing.T) {
	runSlowCheck(t)

	ctx := context.Background()

	// Start the DirIO server
	server := startTestServer(t)
	defer server.Stop(t)

	t.Logf("DirIO server started on port %d", server.port)

	// Create Python container with boto3
	req := testcontainers.ContainerRequest{
		Image: "python:3.12-slim",
		Env: map[string]string{
			"DIRIO_ENDPOINT":   server.Endpoint(),
			"DIRIO_ACCESS_KEY": testAccessKey,
			"DIRIO_SECRET_KEY": testSecretKey,
			"DIRIO_REGION":     testRegion,
		},
		Entrypoint: []string{"/bin/bash", "-c"},
		Cmd: []string{
			boto3TestScript(),
		},
		WaitingFor: wait.ForExit().WithExitTimeout(3 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	defer container.Terminate(ctx)

	// Get container logs
	logs, err := container.Logs(ctx)
	require.NoError(t, err)
	defer logs.Close()

	logBytes, err := io.ReadAll(logs)
	require.NoError(t, err)
	logOutput := string(logBytes)

	t.Logf("boto3 test output:\n%s", logOutput)

	// Check for test results
	state, err := container.State(ctx)
	require.NoError(t, err)

	if state.ExitCode != 0 {
		t.Errorf("boto3 tests failed with exit code %d", state.ExitCode)
	}

	// Parse and report results
	if strings.Contains(logOutput, "All tests passed") {
		t.Log("boto3: All tests passed")
	} else {
		passCount := strings.Count(logOutput, "PASS:")
		failCount := strings.Count(logOutput, "FAIL:")
		t.Logf("boto3: %d passed, %d failed", passCount, failCount)
	}
}

// TestMinIOMC runs MinIO client compatibility tests
func TestMinIOMC(t *testing.T) {
	runSlowCheck(t)

	ctx := context.Background()

	// Start the DirIO server
	server := startTestServer(t)
	defer server.Stop(t)

	t.Logf("DirIO server started on port %d", server.port)

	// Use alpine with mc installed (has proper shell)
	// The official mc image doesn't have a shell
	req := testcontainers.ContainerRequest{
		Image: "alpine:3.19",
		Env: map[string]string{
			"DIRIO_ENDPOINT":   server.Endpoint(),
			"DIRIO_ACCESS_KEY": testAccessKey,
			"DIRIO_SECRET_KEY": testSecretKey,
		},
		Entrypoint: []string{"/bin/sh", "-c"},
		Cmd: []string{
			// Install mc first, then run tests
			`apk add --no-cache curl && curl -sL https://dl.min.io/client/mc/release/linux-amd64/mc -o /usr/local/bin/mc && chmod +x /usr/local/bin/mc && ` + minioMCTestScript(),
		},
		WaitingFor: wait.ForExit().WithExitTimeout(3 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	defer container.Terminate(ctx)

	// Get container logs
	logs, err := container.Logs(ctx)
	require.NoError(t, err)
	defer logs.Close()

	logBytes, err := io.ReadAll(logs)
	require.NoError(t, err)
	logOutput := string(logBytes)

	t.Logf("MinIO mc test output:\n%s", logOutput)

	// Check for test results
	state, err := container.State(ctx)
	require.NoError(t, err)

	if state.ExitCode != 0 {
		t.Errorf("MinIO mc tests failed with exit code %d", state.ExitCode)
	}

	// Parse and report results
	if strings.Contains(logOutput, "All tests passed") {
		t.Log("MinIO mc: All tests passed")
	} else {
		passCount := strings.Count(logOutput, "PASS:")
		failCount := strings.Count(logOutput, "FAIL:")
		t.Logf("MinIO mc: %d passed, %d failed", passCount, failCount)
	}
}

// awsCLITestScript returns the test script for AWS CLI
func awsCLITestScript() string {
	return `
set +e

# Cleanup handler for signals
cleanup() {
  echo "Received signal, cleaning up..."
  exit 130
}
trap cleanup SIGINT SIGTERM

PASSED=0
FAILED=0

pass() { echo "PASS: $1"; PASSED=$((PASSED+1)); }
fail() { echo "FAIL: $1 - $2"; FAILED=$((FAILED+1)); }

BUCKET="test-bucket-$(date +%s)"
ENDPOINT="${DIRIO_ENDPOINT}"
AWS="aws --endpoint-url ${ENDPOINT}"

echo "=== AWS CLI Tests ==="
echo "Endpoint: ${ENDPOINT}"

# ListBuckets
$AWS s3api list-buckets && pass "ListBuckets" || fail "ListBuckets"

# CreateBucket
$AWS s3api create-bucket --bucket ${BUCKET} && pass "CreateBucket" || fail "CreateBucket"

# HeadBucket
$AWS s3api head-bucket --bucket ${BUCKET} && pass "HeadBucket" || fail "HeadBucket"

# PutObject
echo "test content" > /tmp/test.txt
$AWS s3api put-object --bucket ${BUCKET} --key test.txt --body /tmp/test.txt && pass "PutObject" || fail "PutObject"

# HeadObject
$AWS s3api head-object --bucket ${BUCKET} --key test.txt && pass "HeadObject" || fail "HeadObject"

# GetObject
$AWS s3api get-object --bucket ${BUCKET} --key test.txt /tmp/download.txt && pass "GetObject" || fail "GetObject"

# ListObjectsV2
$AWS s3api list-objects-v2 --bucket ${BUCKET} && pass "ListObjectsV2" || fail "ListObjectsV2"

# s3 cp upload
$AWS s3 cp /tmp/test.txt s3://${BUCKET}/hl-test.txt && pass "s3 cp upload" || fail "s3 cp upload"

# s3 cp download
$AWS s3 cp s3://${BUCKET}/hl-test.txt /tmp/hl-download.txt && pass "s3 cp download" || fail "s3 cp download"

# DeleteObject
$AWS s3api delete-object --bucket ${BUCKET} --key test.txt && pass "DeleteObject" || fail "DeleteObject"

# Cleanup and DeleteBucket
$AWS s3 rm s3://${BUCKET} --recursive 2>/dev/null || true
$AWS s3api delete-bucket --bucket ${BUCKET} && pass "DeleteBucket" || fail "DeleteBucket"

echo ""
echo "=== Summary ==="
echo "Passed: ${PASSED}"
echo "Failed: ${FAILED}"
if [ ${FAILED} -eq 0 ]; then
  echo "All tests passed"
  exit 0
else
  exit 1
fi
`
}

// boto3TestScript returns the Python test script for boto3
func boto3TestScript() string {
	return `
pip install --quiet boto3 requests
python3 << 'PYTHON_SCRIPT'
import os
import time
import boto3
import requests
from botocore.config import Config
from botocore.exceptions import ClientError

passed = 0
failed = 0

def log_pass(name):
    global passed
    print(f"PASS: {name}")
    passed += 1

def log_fail(name, error=""):
    global failed
    print(f"FAIL: {name} - {error}")
    failed += 1

endpoint = os.environ.get("DIRIO_ENDPOINT")
access_key = os.environ.get("DIRIO_ACCESS_KEY")
secret_key = os.environ.get("DIRIO_SECRET_KEY")
region = os.environ.get("DIRIO_REGION", "us-east-1")

config = Config(signature_version="s3v4", s3={"addressing_style": "path"})
s3 = boto3.client(
    "s3",
    endpoint_url=endpoint,
    aws_access_key_id=access_key,
    aws_secret_access_key=secret_key,
    region_name=region,
    config=config,
)

bucket = f"test-bucket-{int(time.time())}"

print("=== boto3 Tests ===")
print(f"Endpoint: {endpoint}")

# ListBuckets
try:
    s3.list_buckets()
    log_pass("ListBuckets")
except Exception as e:
    log_fail("ListBuckets", str(e))

# CreateBucket
try:
    s3.create_bucket(Bucket=bucket)
    log_pass("CreateBucket")
except Exception as e:
    log_fail("CreateBucket", str(e))

# GetBucketLocation
try:
    response = s3.get_bucket_location(Bucket=bucket)
    log_pass("GetBucketLocation")
except Exception as e:
    log_fail("GetBucketLocation", str(e))

# HeadBucket
try:
    s3.head_bucket(Bucket=bucket)
    log_pass("HeadBucket")
except Exception as e:
    log_fail("HeadBucket", str(e))

# PutObject
try:
    s3.put_object(Bucket=bucket, Key="test.txt", Body=b"test content")
    log_pass("PutObject")
except Exception as e:
    log_fail("PutObject", str(e))

# HeadObject
try:
    s3.head_object(Bucket=bucket, Key="test.txt")
    log_pass("HeadObject")
except Exception as e:
    log_fail("HeadObject", str(e))

# GetObject
try:
    response = s3.get_object(Bucket=bucket, Key="test.txt")
    body = response["Body"].read()
    if body == b"test content":
        log_pass("GetObject")
    else:
        log_fail("GetObject", "content mismatch")
except Exception as e:
    log_fail("GetObject", str(e))

# ListObjectsV2 (basic)
try:
    s3.list_objects_v2(Bucket=bucket)
    log_pass("ListObjectsV2 (basic)")
except Exception as e:
    log_fail("ListObjectsV2 (basic)", str(e))

# Create some objects for advanced list tests
try:
    s3.put_object(Bucket=bucket, Key="folder1/file1.txt", Body=b"f1")
    s3.put_object(Bucket=bucket, Key="folder1/file2.txt", Body=b"f2")
    s3.put_object(Bucket=bucket, Key="folder2/file3.txt", Body=b"f3")
    s3.put_object(Bucket=bucket, Key="root.txt", Body=b"root")
except Exception as e:
    print(f"Warning: failed to create test objects: {e}")

# ListObjectsV2 with prefix
try:
    response = s3.list_objects_v2(Bucket=bucket, Prefix="folder1/")
    contents = response.get("Contents", [])
    if len(contents) == 2:
        log_pass("ListObjectsV2 (prefix)")
    else:
        log_fail("ListObjectsV2 (prefix)", f"expected 2 objects, got {len(contents)}")
except Exception as e:
    log_fail("ListObjectsV2 (prefix)", str(e))

# ListObjectsV2 with delimiter (CommonPrefixes)
try:
    response = s3.list_objects_v2(Bucket=bucket, Delimiter="/")
    prefixes = response.get("CommonPrefixes", [])
    contents = response.get("Contents", [])
    # Should have folder1/ and folder2/ as common prefixes, and root.txt + test.txt as contents
    if len(prefixes) >= 2:
        log_pass("ListObjectsV2 (delimiter)")
    else:
        log_fail("ListObjectsV2 (delimiter)", f"expected 2+ CommonPrefixes, got {len(prefixes)}: {prefixes}")
except Exception as e:
    log_fail("ListObjectsV2 (delimiter)", str(e))

# ListObjectsV2 with max-keys
try:
    response = s3.list_objects_v2(Bucket=bucket, MaxKeys=2)
    contents = response.get("Contents", [])
    is_truncated = response.get("IsTruncated", False)
    if len(contents) == 2 and is_truncated:
        log_pass("ListObjectsV2 (max-keys)")
    elif len(contents) == 2:
        log_fail("ListObjectsV2 (max-keys)", "IsTruncated should be True")
    else:
        log_fail("ListObjectsV2 (max-keys)", f"expected 2 objects, got {len(contents)}")
except Exception as e:
    log_fail("ListObjectsV2 (max-keys)", str(e))

# ListObjectsV1
try:
    response = s3.list_objects(Bucket=bucket)
    contents = response.get("Contents", [])
    if len(contents) > 0:
        log_pass("ListObjectsV1")
    else:
        log_fail("ListObjectsV1", "no objects returned")
except Exception as e:
    log_fail("ListObjectsV1", str(e))

# PutObject with metadata
try:
    s3.put_object(
        Bucket=bucket,
        Key="metadata.txt",
        Body=b"test with metadata",
        Metadata={"custom-key": "custom-value"},
    )
    log_pass("PutObject with metadata")
except Exception as e:
    log_fail("PutObject with metadata", str(e))

# GetObject metadata (verify custom metadata is returned)
try:
    response = s3.head_object(Bucket=bucket, Key="metadata.txt")
    metadata = response.get("Metadata", {})
    if metadata.get("custom-key") == "custom-value":
        log_pass("GetObject metadata")
    else:
        log_fail("GetObject metadata", f"metadata not returned correctly: {metadata}")
except Exception as e:
    log_fail("GetObject metadata", str(e))

# Range request
try:
    # Put a larger object
    large_content = b"0123456789" * 10  # 100 bytes
    s3.put_object(Bucket=bucket, Key="range-test.txt", Body=large_content)
    response = s3.get_object(Bucket=bucket, Key="range-test.txt", Range="bytes=0-9")
    body = response["Body"].read()
    if body == b"0123456789":
        log_pass("Range request")
    else:
        log_fail("Range request", f"expected first 10 bytes, got {len(body)} bytes: {body[:20]}")
except Exception as e:
    log_fail("Range request", str(e))

# CopyObject
try:
    s3.copy_object(
        Bucket=bucket,
        Key="copied.txt",
        CopySource={"Bucket": bucket, "Key": "test.txt"},
    )
    # Verify copy exists AND has correct content
    response = s3.get_object(Bucket=bucket, Key="copied.txt")
    copied_body = response["Body"].read()
    if copied_body == b"test content":
        log_pass("CopyObject")
    else:
        log_fail("CopyObject", f"copied content mismatch: expected 'test content', got '{copied_body[:50]}'")
except Exception as e:
    log_fail("CopyObject", str(e))

# Pre-signed URL (generate and fetch)
try:
    url = s3.generate_presigned_url(
        "get_object",
        Params={"Bucket": bucket, "Key": "test.txt"},
        ExpiresIn=300,
    )
    response = requests.get(url)
    if response.status_code == 200 and response.content == b"test content":
        log_pass("Pre-signed URL")
    else:
        log_fail("Pre-signed URL", f"status={response.status_code}, body={response.content[:50]}")
except Exception as e:
    log_fail("Pre-signed URL", str(e))

# Multipart upload
try:
    # Create multipart upload
    mpu = s3.create_multipart_upload(Bucket=bucket, Key="multipart.txt")
    upload_id = mpu["UploadId"]

    # Upload parts (minimum 5MB for real S3, but we'll test with smaller)
    part1 = s3.upload_part(
        Bucket=bucket,
        Key="multipart.txt",
        UploadId=upload_id,
        PartNumber=1,
        Body=b"part1 content",
    )
    part2 = s3.upload_part(
        Bucket=bucket,
        Key="multipart.txt",
        UploadId=upload_id,
        PartNumber=2,
        Body=b"part2 content",
    )

    # Complete multipart upload
    s3.complete_multipart_upload(
        Bucket=bucket,
        Key="multipart.txt",
        UploadId=upload_id,
        MultipartUpload={
            "Parts": [
                {"PartNumber": 1, "ETag": part1["ETag"]},
                {"PartNumber": 2, "ETag": part2["ETag"]},
            ]
        },
    )

    # Verify object exists
    s3.head_object(Bucket=bucket, Key="multipart.txt")
    log_pass("Multipart upload")
except Exception as e:
    log_fail("Multipart upload", str(e))

# Object tagging
try:
    s3.put_object_tagging(
        Bucket=bucket,
        Key="test.txt",
        Tagging={"TagSet": [{"Key": "env", "Value": "test"}]},
    )
    response = s3.get_object_tagging(Bucket=bucket, Key="test.txt")
    tags = response.get("TagSet", [])
    if any(t["Key"] == "env" and t["Value"] == "test" for t in tags):
        log_pass("Object tagging")
    else:
        log_fail("Object tagging", f"tags not returned correctly: {tags}")
except Exception as e:
    log_fail("Object tagging", str(e))

# DeleteObject
try:
    s3.delete_object(Bucket=bucket, Key="test.txt")
    log_pass("DeleteObject")
except Exception as e:
    log_fail("DeleteObject", str(e))

# Cleanup and DeleteBucket
try:
    response = s3.list_objects_v2(Bucket=bucket)
    for obj in response.get("Contents", []):
        s3.delete_object(Bucket=bucket, Key=obj["Key"])
    s3.delete_bucket(Bucket=bucket)
    log_pass("DeleteBucket")
except Exception as e:
    log_fail("DeleteBucket", str(e))

print()
print("=== Summary ===")
print(f"Passed: {passed}")
print(f"Failed: {failed}")
if failed == 0:
    print("All tests passed")
exit(1 if failed > 0 else 0)
PYTHON_SCRIPT
`
}

// minioMCTestScript returns the test script for MinIO mc
func minioMCTestScript() string {
	return `
set +e

# Cleanup handler for signals
cleanup() {
  echo "Received signal, cleaning up..."
  exit 130
}
trap cleanup SIGINT SIGTERM

PASSED=0
FAILED=0

pass() { echo "PASS: $1"; PASSED=$((PASSED+1)); }
fail() { echo "FAIL: $1 - $2"; FAILED=$((FAILED+1)); }

BUCKET="test-bucket-$(date +%s)"
ENDPOINT="${DIRIO_ENDPOINT}"
MC_ALIAS="dirio"

echo "=== MinIO mc Tests ==="
echo "Endpoint: ${ENDPOINT}"

# Configure alias
mc alias set ${MC_ALIAS} ${ENDPOINT} ${DIRIO_ACCESS_KEY} ${DIRIO_SECRET_KEY} --api S3v4 2>/dev/null
if [ $? -eq 0 ]; then
  pass "Configure alias"
else
  fail "Configure alias"
  exit 1
fi

# List buckets
mc ls ${MC_ALIAS} && pass "List buckets" || fail "List buckets"

# Make bucket
mc mb ${MC_ALIAS}/${BUCKET} 2>&1
if [ $? -eq 0 ]; then
  pass "Make bucket"
else
  fail "Make bucket"
fi

# Upload object
echo "test content" > /tmp/test.txt
mc cp /tmp/test.txt ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "Upload object"
else
  fail "Upload object"
fi

# Stat object
mc stat ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "Stat object"
else
  fail "Stat object"
fi

# Download object
mc cp ${MC_ALIAS}/${BUCKET}/test.txt /tmp/download.txt 2>&1
if [ $? -eq 0 ]; then
  pass "Download object"
else
  fail "Download object"
fi

# Cat object
mc cat ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "Cat object"
else
  fail "Cat object"
fi

# List objects
mc ls ${MC_ALIAS}/${BUCKET}/ 2>&1
if [ $? -eq 0 ]; then
  pass "List objects"
else
  fail "List objects"
fi

# Remove object
mc rm ${MC_ALIAS}/${BUCKET}/test.txt 2>&1
if [ $? -eq 0 ]; then
  pass "Remove object"
else
  fail "Remove object"
fi

# Remove bucket
mc rb ${MC_ALIAS}/${BUCKET} 2>&1
if [ $? -eq 0 ]; then
  pass "Remove bucket"
else
  fail "Remove bucket"
fi

echo ""
echo "=== Summary ==="
echo "Passed: ${PASSED}"
echo "Failed: ${FAILED}"
if [ ${FAILED} -eq 0 ]; then
  echo "All tests passed"
  exit 0
else
  exit 1
fi
`
}
