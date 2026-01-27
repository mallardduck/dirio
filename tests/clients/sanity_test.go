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

// TestSanityCheck_FailingServer verifies that our test scripts actually detect failures
// by running them against a server that always returns errors.
// This proves our tests aren't just passing unconditionally.
func TestSanityCheck_FailingServer(t *testing.T) {
	t.Parallel()

	// Create a mock server that returns 500 for everything
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>InternalError</Code>
  <Message>Mock server - all requests fail</Message>
</Error>`))
	}))
	defer mockServer.Close()

	t.Logf("Mock failing server started at %s", mockServer.URL)

	ctx := context.Background()

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

		t.Logf("AWS CLI output against failing server:\n%s", logOutput)

		state, err := container.State(ctx)
		require.NoError(t, err)

		// The tests SHOULD fail against a failing server
		require.NotEqual(t, 0, state.ExitCode, "SANITY CHECK FAILED: AWS CLI tests passed against a failing server!")

		// Verify we got some failures
		failCount := strings.Count(logOutput, "FAIL:")
		require.Greater(t, failCount, 0, "SANITY CHECK FAILED: No failures detected against failing server!")
		t.Logf("✅ Sanity check passed: AWS CLI correctly detected %d failures", failCount)
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

		t.Logf("boto3 output against failing server:\n%s", logOutput)

		state, err := container.State(ctx)
		require.NoError(t, err)

		// The tests SHOULD fail against a failing server
		require.NotEqual(t, 0, state.ExitCode, "SANITY CHECK FAILED: boto3 tests passed against a failing server!")

		// Verify we got some failures
		failCount := strings.Count(logOutput, "FAIL:")
		require.Greater(t, failCount, 0, "SANITY CHECK FAILED: No failures detected against failing server!")
		t.Logf("✅ Sanity check passed: boto3 correctly detected %d failures", failCount)
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

		t.Logf("MinIO mc output against failing server:\n%s", logOutput)

		state, err := container.State(ctx)
		require.NoError(t, err)

		// The tests SHOULD fail against a failing server
		require.NotEqual(t, 0, state.ExitCode, "SANITY CHECK FAILED: MinIO mc tests passed against a failing server!")

		// Verify we got some failures
		failCount := strings.Count(logOutput, "FAIL:")
		require.Greater(t, failCount, 0, "SANITY CHECK FAILED: No failures detected against failing server!")
		t.Logf("✅ Sanity check passed: MinIO mc correctly detected %d failures", failCount)
	})
}
