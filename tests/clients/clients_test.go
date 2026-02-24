package clients_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

const (
	testAccessKey = "testaccess"
	testSecretKey = "testsecret"
	testRegion    = "us-east-1"
)

// JSON output structures for test results
type TestMeta struct {
	Client     string `json:"client"`
	Version    string `json:"version"`
	TestRunID  string `json:"test_run_id"`
	DurationMs int    `json:"duration_ms"`
}

type TestResultDetails struct {
	ValidationType string `json:"validation_type"`
}

type TestResult struct {
	Feature    string            `json:"feature"`
	Category   string            `json:"category"`
	Status     string            `json:"status"`
	DurationMs int               `json:"duration_ms"`
	Message    string            `json:"message"`
	Details    TestResultDetails `json:"details"`
}

type TestSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type TestOutput struct {
	Meta    TestMeta     `json:"meta"`
	Results []TestResult `json:"results"`
	Summary TestSummary  `json:"summary"`
}

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
	cmd          *exec.Cmd
	port         int
	externalPort int
	dataDir      string
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

	// Remove old binary to ensure fresh build
	binaryPath := filepath.Join(projectRoot, "bin", "dirio-test")
	os.Remove(binaryPath)          // Remove without .exe
	os.Remove(binaryPath + ".exe") // Remove .exe version on Windows

	// Build the server (force fresh build)
	// On Windows, add .exe extension to output path
	buildOutput := binaryPath
	if runtime.GOOS == "windows" {
		buildOutput += ".exe"
	}
	buildCmd := exec.Command("go", "build", "-a", "-o", buildOutput, "./cmd/server")
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
	externalPort := findAvailablePort(t)

	// Start server (on Windows, go build adds .exe automatically)
	serverPath := filepath.Join(projectRoot, "bin", "dirio-test")
	if runtime.GOOS == "windows" {
		serverPath += ".exe"
	}
	cmd := exec.Command(serverPath, "serve",
		"--port", fmt.Sprintf("%d", externalPort),
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
		cmd:          cmd,
		port:         externalPort,
		externalPort: externalPort,
		dataDir:      dataDir,
	}

	// Wait for server to be ready
	require.True(t, waitForServer(t, externalPort, 10*time.Second), "Server failed to start")

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
	return fmt.Sprintf("http://localhost:%d", ts.externalPort)
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

// parseTestOutput extracts JSON from log output and parses it
func parseTestOutput(logOutput string) (*TestOutput, error) {
	// Find JSON output (starts with { and ends with })
	// The logs will contain human-readable output followed by JSON
	startIdx := strings.LastIndex(logOutput, "\n{")
	if startIdx == -1 {
		startIdx = strings.Index(logOutput, "{")
	}
	if startIdx == -1 {
		return nil, fmt.Errorf("no JSON output found in logs")
	}

	jsonStr := logOutput[startIdx:]
	jsonStr = strings.TrimSpace(jsonStr)

	var output TestOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("failed to parse JSON output: %w", err)
	}

	return &output, nil
}

// TestAWSCLI runs AWS CLI compatibility tests
func TestAWSCLI(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Use the shared DirIO server (started once for all client tests)
	server := getSharedClientServer(t)

	envMap := map[string]string{
		"AWS_ACCESS_KEY_ID":     testAccessKey,
		"AWS_SECRET_ACCESS_KEY": testSecretKey,
		"AWS_DEFAULT_REGION":    testRegion,
		"DIRIO_ENDPOINT":        server.Endpoint(),
	}
	// Use alpine with AWS CLI installed (has proper shell)
	req := AwsClientContainer(envMap)

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

	// Parse JSON output
	testOutput, err := parseTestOutput(logOutput)
	if err != nil {
		t.Errorf("Failed to parse test output: %v", err)
		// Fallback to old method
		passCount := strings.Count(logOutput, "PASS:")
		failCount := strings.Count(logOutput, "FAIL:")
		t.Logf("AWS CLI: %d passed, %d failed (fallback parsing)", passCount, failCount)
	} else {
		t.Logf("AWS CLI Results: %d total, %d passed, %d failed, %d skipped",
			testOutput.Summary.Total,
			testOutput.Summary.Passed,
			testOutput.Summary.Failed,
			testOutput.Summary.Skipped)

		// Log failed tests
		for _, result := range testOutput.Results {
			if result.Status == "fail" {
				t.Errorf("  FAILED: %s - %s", result.Feature, result.Message)
			}
		}
	}

	if state.ExitCode != 0 {
		t.Errorf("AWS CLI tests failed with exit code %d", state.ExitCode)
	}
}

// TestBoto3 runs boto3 (Python) compatibility tests
func TestBoto3(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Use the shared DirIO server (started once for all client tests)
	server := getSharedClientServer(t)

	// Create Python container with boto3
	envMap := map[string]string{
		"AWS_ACCESS_KEY_ID":     testAccessKey,
		"AWS_SECRET_ACCESS_KEY": testSecretKey,
		"AWS_DEFAULT_REGION":    testRegion,
		"DIRIO_ENDPOINT":        server.Endpoint(),
	}
	req := Boto3ClientContainer(envMap)

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

	// Parse JSON output
	testOutput, err := parseTestOutput(logOutput)
	if err != nil {
		t.Errorf("Failed to parse test output: %v", err)
		// Fallback to old method
		passCount := strings.Count(logOutput, "PASS:")
		failCount := strings.Count(logOutput, "FAIL:")
		t.Logf("boto3: %d passed, %d failed (fallback parsing)", passCount, failCount)
	} else {
		t.Logf("boto3 Results: %d total, %d passed, %d failed, %d skipped",
			testOutput.Summary.Total,
			testOutput.Summary.Passed,
			testOutput.Summary.Failed,
			testOutput.Summary.Skipped)

		// Log failed tests
		for _, result := range testOutput.Results {
			if result.Status == "fail" {
				t.Errorf("  FAILED: %s - %s", result.Feature, result.Message)
			}
		}
	}

	if state.ExitCode != 0 {
		t.Errorf("boto3 tests failed with exit code %d", state.ExitCode)
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

	req := MinioClientContainer(envMap)

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

	// Parse JSON output
	testOutput, err := parseTestOutput(logOutput)
	if err != nil {
		t.Errorf("Failed to parse test output: %v", err)
		// Fallback to old method
		passCount := strings.Count(logOutput, "PASS:")
		failCount := strings.Count(logOutput, "FAIL:")
		t.Logf("MinIO mc: %d passed, %d failed (fallback parsing)", passCount, failCount)
	} else {
		t.Logf("MinIO mc Results: %d total, %d passed, %d failed, %d skipped",
			testOutput.Summary.Total,
			testOutput.Summary.Passed,
			testOutput.Summary.Failed,
			testOutput.Summary.Skipped)

		// Log failed tests
		for _, result := range testOutput.Results {
			if result.Status == "fail" {
				t.Errorf("  FAILED: %s - %s", result.Feature, result.Message)
			}
		}
	}

	if state.ExitCode != 0 {
		t.Errorf("MinIO mc tests failed with exit code %d", state.ExitCode)
	}
}

// TestMCAdmin runs MinIO mc admin command compatibility tests.
// It exercises the DirIO admin API via the real mc binary: user add/list/info,
// policy create/list/info/attach, group add/list/info, and user disable/enable/remove.
func TestMCAdmin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	server := getSharedClientServer(t)

	envMap := map[string]string{
		"DIRIO_ENDPOINT":   server.Endpoint(),
		"DIRIO_ACCESS_KEY": testAccessKey,
		"DIRIO_SECRET_KEY": testSecretKey,
	}

	req := MinioClientContainer(envMap)
	req.Cmd = []string{"mc_admin.sh"}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	defer container.Terminate(ctx)

	logs, err := container.Logs(ctx)
	require.NoError(t, err)
	defer logs.Close()

	logBytes, err := io.ReadAll(logs)
	require.NoError(t, err)
	logOutput := string(logBytes)

	t.Logf("mc admin test output:\n%s", logOutput)

	state, err := container.State(ctx)
	require.NoError(t, err)

	testOutput, err := parseTestOutput(logOutput)
	if err != nil {
		t.Errorf("Failed to parse test output: %v", err)
		passCount := strings.Count(logOutput, "PASS:")
		failCount := strings.Count(logOutput, "FAIL:")
		t.Logf("mc admin: %d passed, %d failed (fallback parsing)", passCount, failCount)
	} else {
		t.Logf("mc admin Results: %d total, %d passed, %d failed, %d skipped",
			testOutput.Summary.Total,
			testOutput.Summary.Passed,
			testOutput.Summary.Failed,
			testOutput.Summary.Skipped)

		for _, result := range testOutput.Results {
			if result.Status == "fail" {
				t.Errorf("  FAILED: %s - %s", result.Feature, result.Message)
			}
		}
	}

	if state.ExitCode != 0 {
		t.Errorf("mc admin tests failed with exit code %d", state.ExitCode)
	}
}
