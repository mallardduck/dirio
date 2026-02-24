package clients_test

import (
	"runtime"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const ContainerRepo = "dirio-test-local"

// hostGatewayHosts returns the extra host mapping needed so containers can reach
// the test server running on the host.
//
// On Linux (including GitHub Actions), host.docker.internal is not defined in
// containers by default — it requires an explicit --add-host entry pointing at
// the Docker bridge gateway. On macOS/Windows Docker Desktop the name is already
// injected by the daemon, so no override is needed.
func hostGatewayHosts() []string {
	if runtime.GOOS == "linux" {
		return []string{"host.docker.internal:host-gateway"}
	}
	return nil
}

// AwsClientContainer returns a container request for the AWS CLI client
// with the pre-built image. The Docker image is built once and cached for reuse.
func AwsClientContainer(envMap map[string]string) testcontainers.ContainerRequest {
	return testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "docker/Dockerfile-aws",
			Repo:       ContainerRepo,
			Tag:        "aws",
			KeepImage:  true, // Keep the image for reuse across tests
		},
		Env: envMap,
		HostConfigModifier: func(config *container.HostConfig) {
			if extraGateways := hostGatewayHosts(); extraGateways != nil {
				config.ExtraHosts = append(config.ExtraHosts, extraGateways...)
			}
		},
		Entrypoint: []string{"./awscli.sh"},
		WaitingFor: wait.ForExit().WithExitTimeout(3 * time.Minute),
	}
}

// MinioClientContainer returns a container request for the MinIO mc client
// with the pre-built image. The Docker image is built once and cached for reuse.
func MinioClientContainer(envMap map[string]string) testcontainers.ContainerRequest {
	return testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "docker/Dockerfile-minio",
			Repo:       ContainerRepo,
			Tag:        "mc",
			KeepImage:  true, // Keep the image for reuse across tests
		},
		Env: envMap,
		HostConfigModifier: func(config *container.HostConfig) {
			if extraGateways := hostGatewayHosts(); extraGateways != nil {
				config.ExtraHosts = append(config.ExtraHosts, extraGateways...)
			}
		},
		Cmd:        []string{"./mc.sh"},
		WaitingFor: wait.ForExit().WithExitTimeout(3 * time.Minute),
	}
}

// Boto3ClientContainer returns a container request for the boto3 client
// with the pre-built image. The Docker image is built once and cached for reuse.
func Boto3ClientContainer(envMap map[string]string) testcontainers.ContainerRequest {
	return testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "docker/Dockerfile-boto3",
			Repo:       ContainerRepo,
			Tag:        "boto3",
			KeepImage:  true, // Keep the image for reuse across tests
		},
		Env: envMap,
		HostConfigModifier: func(config *container.HostConfig) {
			if extraGateways := hostGatewayHosts(); extraGateways != nil {
				config.ExtraHosts = append(config.ExtraHosts, extraGateways...)
			}
		},
		Cmd:        []string{"-c", "python3 ./boto3cli.py"},
		WaitingFor: wait.ForExit().WithExitTimeout(2 * time.Minute),
	}
}
