package clients

import (
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// MinioClientContainer returns a container request for the MinIO mc client
// with the pre-built image. The Docker image is built once and cached for reuse.
func MinioClientContainer(envMap map[string]string, script string) testcontainers.ContainerRequest {
	return testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "./minio",
			Dockerfile: "Dockerfile",
			Repo:       "dirio-mc-test",
			Tag:        "local",
			KeepImage:  true, // Keep the image for reuse across tests
		},
		Env:        envMap,
		Entrypoint: []string{"/bin/sh", "-c"},
		Cmd:        []string{script},
		WaitingFor: wait.ForExit().WithExitTimeout(3 * time.Minute),
	}
}
