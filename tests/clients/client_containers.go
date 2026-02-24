package clients_test

import (
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const ContainerRepo = "dirio-test-local"

// AwsClientContainer returns a container request for the MinIO mc client
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
		Env:        envMap,
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
		Env:        envMap,
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
		Env:        envMap,
		Cmd:        []string{"-c", "python3 ./boto3cli.py"},
		WaitingFor: wait.ForExit().WithExitTimeout(2 * time.Minute),
	}
}
