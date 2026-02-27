package clients_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

// mockServerType defines the behavior of the mock server
type mockServerType int

const (
	// mockServerFailing returns 500 for all requests
	mockServerFailing mockServerType = iota
	// mockServerDumbSuccess returns 200 OK with empty/minimal XML for all requests
	mockServerDumbSuccess
)

// mockServerWrapper wraps an HTTP server for cleanup
type mockServerWrapper struct {
	server *http.Server
	port   string
}

func (m *mockServerWrapper) Close() {
	if m.server != nil {
		m.server.Close()
	}
}

// createMockServer creates a test HTTP server with the specified behavior.
// It binds to 0.0.0.0 on a random port so Docker containers can reach it via host.docker.internal.
func createMockServer(port int, serverType mockServerType) (wrapper *mockServerWrapper, addr string) {
	// Create handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch serverType {
		case mockServerFailing:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>InternalError</Code>
  <Message>Mock server - all requests fail</Message>
</Error>`))
		case mockServerDumbSuccess:
			// Return 200 OK with minimal S3-like XML response
			// This should cause tests to fail because responses lack expected data
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Response></Response>`))
		}
	})

	// Create listener on 0.0.0.0 with random port (tcp4 forces IPv4)
	listener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		panic(fmt.Sprintf("failed to create listener: %v", err))
	}

	// Create HTTP server
	server := &http.Server{
		Handler: handler,
	}

	// Start serving in background
	go server.Serve(listener)

	// Build URL using host.docker.internal for container access to host
	addr = fmt.Sprintf("http://host.docker.internal:%d", port)

	wrapper = &mockServerWrapper{
		server: server,
		port:   strconv.Itoa(port),
	}

	return wrapper, addr
}

// runClientTest is a helper that runs a client test against a mock server
// and verifies that it fails (exits with non-zero code)
func runClientTest(t *testing.T, testName string, req testcontainers.ContainerRequest) {
	t.Helper()
	ctx := context.Background()

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

	t.Logf("%s output:\n%s", testName, logOutput)

	state, err := container.State(ctx)
	require.NoError(t, err)

	// The tests SHOULD fail against a mock server
	require.NotEqual(t, 0, state.ExitCode, "SANITY CHECK FAILED: %s tests passed against mock server!", testName)

	// Verify we got some failures
	failCount := strings.Count(logOutput, "FAIL:")
	require.Positive(t, failCount, "SANITY CHECK FAILED: No failures detected in %s!", testName)
	t.Logf("✅ Sanity check passed: %s correctly detected %d failures", testName, failCount)
}

// TestSanityCheck_FailingServer verifies that our test scripts actually detect failures
// by running them against a server that always returns errors.
// This proves our tests aren't just passing unconditionally.
func TestSanityCheck_FailingServer(t *testing.T) {
	t.Parallel()
	externalPort := findAvailablePort(t)
	mockServer, containerURL := createMockServer(externalPort, mockServerFailing)
	defer mockServer.Close()

	t.Logf("Mock failing server started on port %s (container URL: %s)", mockServer.port, containerURL)

	// Test AWS CLI against failing server - should fail
	t.Run("AWS_CLI_Should_Fail", func(t *testing.T) {
		t.Parallel()

		envMap := map[string]string{
			"AWS_ACCESS_KEY_ID":     testAccessKey,
			"AWS_SECRET_ACCESS_KEY": testSecretKey,
			"AWS_DEFAULT_REGION":    testRegion,
			"DIRIO_ENDPOINT":        containerURL,
		}
		// Use alpine with AWS CLI installed (has proper shell)
		req := AwsClientContainer(envMap)

		runClientTest(t, "AWS CLI", req)
	})

	// Test boto3 against failing server - should fail
	t.Run("boto3_Should_Fail", func(t *testing.T) {
		t.Parallel()

		envMap := map[string]string{
			"AWS_ACCESS_KEY_ID":     testAccessKey,
			"AWS_SECRET_ACCESS_KEY": testSecretKey,
			"AWS_DEFAULT_REGION":    testRegion,
			"DIRIO_ENDPOINT":        containerURL,
		}
		req := Boto3ClientContainer(envMap)

		runClientTest(t, "boto3", req)
	})

	// Test MinIO mc against failing server - should fail
	t.Run("MinIO_mc_Should_Fail", func(t *testing.T) {
		t.Parallel()

		envMap := map[string]string{
			"DIRIO_ENDPOINT":   containerURL,
			"DIRIO_ACCESS_KEY": testAccessKey,
			"DIRIO_SECRET_KEY": testSecretKey,
		}
		req := MinioClientContainer(envMap)

		runClientTest(t, "MinIO mc", req)
	})
}

// TestSanityCheck_DumbSuccessServer verifies that our test scripts validate response content
// by running them against a server that returns 200 OK for everything but with empty/invalid responses.
// This catches false positives where tests pass just because status code is 200.
func TestSanityCheck_DumbSuccessServer(t *testing.T) {
	t.Parallel()
	externalPort := findAvailablePort(t)
	mockServer, containerURL := createMockServer(externalPort, mockServerDumbSuccess)
	defer mockServer.Close()

	t.Logf("Mock dumb-success server started on port %s (container URL: %s)", mockServer.port, containerURL)

	// Test AWS CLI against dumb success server - should fail
	t.Run("AWS_CLI_Should_Fail", func(t *testing.T) {
		t.Parallel()

		envMap := map[string]string{
			"AWS_ACCESS_KEY_ID":     testAccessKey,
			"AWS_SECRET_ACCESS_KEY": testSecretKey,
			"AWS_DEFAULT_REGION":    testRegion,
			"DIRIO_ENDPOINT":        containerURL,
		}
		// Use alpine with AWS CLI installed (has proper shell)
		req := AwsClientContainer(envMap)

		runClientTest(t, "AWS CLI", req)
	})

	// Test boto3 against dumb success server - should fail
	t.Run("boto3_Should_Fail", func(t *testing.T) {
		t.Parallel()

		envMap := map[string]string{
			"AWS_ACCESS_KEY_ID":     testAccessKey,
			"AWS_SECRET_ACCESS_KEY": testSecretKey,
			"AWS_DEFAULT_REGION":    testRegion,
			"DIRIO_ENDPOINT":        containerURL,
		}
		req := Boto3ClientContainer(envMap)

		runClientTest(t, "boto3", req)
	})

	// Test MinIO mc against dumb success server - should fail
	t.Run("MinIO_mc_Should_Fail", func(t *testing.T) {
		t.Parallel()

		envMap := map[string]string{
			"DIRIO_ENDPOINT":   containerURL,
			"DIRIO_ACCESS_KEY": testAccessKey,
			"DIRIO_SECRET_KEY": testSecretKey,
		}
		req := MinioClientContainer(envMap)

		runClientTest(t, "MinIO mc", req)
	})
}
