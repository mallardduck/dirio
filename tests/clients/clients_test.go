package clients_test

import (
	"context"
	_ "embed"
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

	"github.com/mallardduck/dirio/tests/clients"
)

//go:embed scripts/awscli.sh
var awsCLIScript string

//go:embed scripts/boto3.py
var boto3Script string

//go:embed scripts/mc.sh
var mcScript string

const (
	testAccessKey = "testaccess"
	testSecretKey = "testsecret"
	testRegion    = "us-east-1"
)

// preserveTestData returns true if test data should be preserved (not cleaned up)
func preserveTestData() bool {
	return os.Getenv("DIRIO_PRESERVE_TEST_DATA") != ""
}

// Global cleanup tracking
var (
	activeContainers   = make([]testcontainers.Container, 0)
	activeServers      = make([]*TestServer, 0)
	cleanupMutex       sync.Mutex
	cleanupContext     context.Context
	cleanupCancelFunc  context.CancelFunc
	cleanupInitialized bool
)

// Shared server for all client tests
var (
	sharedClientServer     *TestServer
	sharedClientServerOnce sync.Once
	sharedClientServerErr  error
)

// TestServer manages the DirIO server for testing
type TestServer struct {
	cmd     *exec.Cmd
	port    int
	dataDir string
}

func TestMain(m *testing.M) {
	if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		fmt.Fprintln(tty, "Running slow client tests...")
		if preserveTestData() {
			fmt.Fprintln(tty, "⚠️  DIRIO_PRESERVE_TEST_DATA is set - test data will be preserved")
		}
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
	preserve := preserveTestData()
	for _, server := range activeServers {
		if server != nil && server.cmd != nil && server.cmd.Process != nil {
			server.cmd.Process.Kill()
			server.cmd.Wait()
		}
		if server != nil && server.dataDir != "" {
			if preserve {
				fmt.Fprintf(os.Stderr, "PRESERVED TEST DATA: %s\n", server.dataDir)
			} else {
				os.RemoveAll(server.dataDir)
			}
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

	t.Logf("Test server data directory: %s", dataDir)
	if preserveTestData() {
		t.Logf("⚠️  DIRIO_PRESERVE_TEST_DATA is set - data will NOT be cleaned up")
	}

	// Find available port
	port := findAvailablePort(t)

	// Start server
	serverPath := filepath.Join(projectRoot, "bin", "dirio-test")
	cmd := exec.Command(serverPath, "serve",
		"--port", fmt.Sprintf("%d", port),
		"--data-dir", dataDir,
		"--access-key", testAccessKey,
		"--secret-key", testSecretKey,
		"--log-level", "info",
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
		if preserveTestData() {
			t.Logf("PRESERVED TEST DATA: %s", ts.dataDir)
		} else {
			os.RemoveAll(ts.dataDir)
		}
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

// getSharedClientServer returns a shared DirIO server for all client tests.
// The server is started only once, regardless of how many tests call this function.
func getSharedClientServer(t *testing.T) *TestServer {
	t.Helper()

	sharedClientServerOnce.Do(func() {
		sharedClientServer = startTestServer(t)
		registerServer(sharedClientServer)
		t.Logf("Shared client test server started on port %d", sharedClientServer.port)
	})

	if sharedClientServerErr != nil {
		t.Fatalf("Failed to start shared client server: %v", sharedClientServerErr)
	}

	return sharedClientServer
}

// registerServer adds a server to the cleanup list
func registerServer(server *TestServer) {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()
	activeServers = append(activeServers, server)
}

// TestAWSCLI runs AWS CLI compatibility tests
func TestAWSCLI(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Use the shared DirIO server (started once for all client tests)
	server := getSharedClientServer(t)

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
	t.Parallel()

	ctx := context.Background()

	// Use the shared DirIO server (started once for all client tests)
	server := getSharedClientServer(t)

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
	t.Parallel()

	ctx := context.Background()

	// Use the shared DirIO server (started once for all client tests)
	server := getSharedClientServer(t)

	// Use pre-built mc container with mc installed
	envMap := map[string]string{
		"DIRIO_ENDPOINT":   server.Endpoint(),
		"DIRIO_ACCESS_KEY": testAccessKey,
		"DIRIO_SECRET_KEY": testSecretKey,
	}

	req := clients.MinioClientContainer(envMap, minioMCTestScript())

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
	return awsCLIScript
}

// boto3TestScript returns the Python test script for boto3
func boto3TestScript() string {
	return `pip install --quiet boto3 requests
python3 << 'PYTHON_SCRIPT'
` + boto3Script + `
PYTHON_SCRIPT
`
}

// minioMCTestScript returns the test script for MinIO mc
func minioMCTestScript() string {
	return mcScript
}
