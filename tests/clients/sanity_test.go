package clients_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mallardduck/dirio/tests/clients"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// mockServerType defines the behavior of the mock server
type mockServerType int

const (
	// mockServerFailing returns 500 for all requests
	mockServerFailing mockServerType = iota
	// mockServerDumbSuccess returns 200 OK with empty/minimal XML for all requests
	mockServerDumbSuccess
)

// createMockServer creates a test HTTP server with the specified behavior
func createMockServer(serverType mockServerType) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch serverType {
		case mockServerFailing:
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
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
	}))
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
	require.Greater(t, failCount, 0, "SANITY CHECK FAILED: No failures detected in %s!", testName)
	t.Logf("✅ Sanity check passed: %s correctly detected %d failures", testName, failCount)
}

// TestSanityCheck_FailingServer verifies that our test scripts actually detect failures
// by running them against a server that always returns errors.
// This proves our tests aren't just passing unconditionally.
func TestSanityCheck_FailingServer(t *testing.T) {
	t.Parallel()

	mockServer := createMockServer(mockServerFailing)
	defer mockServer.Close()

	t.Logf("Mock failing server started at %s", mockServer.URL)

	// Test AWS CLI against failing server - should fail
	t.Run("AWS_CLI_Should_Fail", func(t *testing.T) {
		t.Parallel()

		req := testcontainers.ContainerRequest{
			Image: "amazon/aws-cli:2.15.0",
			Env: map[string]string{
				"AWS_ACCESS_KEY_ID":     testAccessKey,
				"AWS_SECRET_ACCESS_KEY": testSecretKey,
				"AWS_DEFAULT_REGION":    testRegion,
				"DIRIO_ENDPOINT":        mockServer.URL,
			},
			Entrypoint: []string{"/bin/bash", "-c"},
			Cmd: []string{
				awsCLITestScript(),
			},
			WaitingFor: wait.ForExit().WithExitTimeout(1 * 60 * 1000000000), // 1 minute
		}

		runClientTest(t, "AWS CLI", req)
	})

	// Test boto3 against failing server - should fail
	t.Run("boto3_Should_Fail", func(t *testing.T) {
		t.Parallel()

		req := testcontainers.ContainerRequest{
			Image: "python:3.12-slim",
			Env: map[string]string{
				"DIRIO_ENDPOINT":   mockServer.URL,
				"DIRIO_ACCESS_KEY": testAccessKey,
				"DIRIO_SECRET_KEY": testSecretKey,
				"DIRIO_REGION":     testRegion,
			},
			Entrypoint: []string{"/bin/bash", "-c"},
			Cmd: []string{
				boto3TestScript(),
			},
			WaitingFor: wait.ForExit().WithExitTimeout(2 * 60 * 1000000000), // 2 minutes
		}

		runClientTest(t, "boto3", req)
	})

	// Test MinIO mc against failing server - should fail
	t.Run("MinIO_mc_Should_Fail", func(t *testing.T) {
		t.Parallel()

		envMap := map[string]string{
			"DIRIO_ENDPOINT":   mockServer.URL,
			"DIRIO_ACCESS_KEY": testAccessKey,
			"DIRIO_SECRET_KEY": testSecretKey,
		}
		req := clients.MinioClientContainer(envMap, minioMCTestScript())

		runClientTest(t, "MinIO mc", req)
	})
}

// TestSanityCheck_DumbSuccessServer verifies that our test scripts validate response content
// by running them against a server that returns 200 OK for everything but with empty/invalid responses.
// This catches false positives where tests pass just because status code is 200.
func TestSanityCheck_DumbSuccessServer(t *testing.T) {
	t.Parallel()

	mockServer := createMockServer(mockServerDumbSuccess)
	defer mockServer.Close()

	t.Logf("Mock dumb-success server started at %s", mockServer.URL)

	// Test AWS CLI against dumb success server - should fail
	t.Run("AWS_CLI_Should_Fail", func(t *testing.T) {
		t.Parallel()

		req := testcontainers.ContainerRequest{
			Image: "amazon/aws-cli:2.15.0",
			Env: map[string]string{
				"AWS_ACCESS_KEY_ID":     testAccessKey,
				"AWS_SECRET_ACCESS_KEY": testSecretKey,
				"AWS_DEFAULT_REGION":    testRegion,
				"DIRIO_ENDPOINT":        mockServer.URL,
			},
			Entrypoint: []string{"/bin/bash", "-c"},
			Cmd: []string{
				awsCLITestScript(),
			},
			WaitingFor: wait.ForExit().WithExitTimeout(1 * 60 * 1000000000), // 1 minute
		}

		runClientTest(t, "AWS CLI", req)
	})

	// Test boto3 against dumb success server - should fail
	t.Run("boto3_Should_Fail", func(t *testing.T) {
		t.Parallel()

		req := testcontainers.ContainerRequest{
			Image: "python:3.12-slim",
			Env: map[string]string{
				"DIRIO_ENDPOINT":   mockServer.URL,
				"DIRIO_ACCESS_KEY": testAccessKey,
				"DIRIO_SECRET_KEY": testSecretKey,
				"DIRIO_REGION":     testRegion,
			},
			Entrypoint: []string{"/bin/bash", "-c"},
			Cmd: []string{
				boto3TestScript(),
			},
			WaitingFor: wait.ForExit().WithExitTimeout(2 * 60 * 1000000000), // 2 minutes
		}

		runClientTest(t, "boto3", req)
	})

	// Test MinIO mc against dumb success server - should fail
	t.Run("MinIO_mc_Should_Fail", func(t *testing.T) {
		t.Parallel()

		envMap := map[string]string{
			"DIRIO_ENDPOINT":   mockServer.URL,
			"DIRIO_ACCESS_KEY": testAccessKey,
			"DIRIO_SECRET_KEY": testSecretKey,
		}
		req := clients.MinioClientContainer(envMap, minioMCTestScript())

		runClientTest(t, "MinIO mc", req)
	})
}
