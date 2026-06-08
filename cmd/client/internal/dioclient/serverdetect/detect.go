// Package serverdetect probes a remote endpoint to determine whether it is a
// DirIO server, a MinIO server, or a generic S3-compatible service.
package serverdetect

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ServerType identifies the kind of server backing a profile endpoint.
type ServerType string

const (
	// ServerTypeDirIO is a DirIO server (responds to /.dirio/health).
	ServerTypeDirIO ServerType = "dirio"
	// ServerTypeMinIO is a MinIO server (responds to /minio/health/live but not /.dirio/health).
	ServerTypeMinIO ServerType = "minio"
	// ServerTypeS3Generic is an S3-compatible service with no recognised admin health probe.
	ServerTypeS3Generic ServerType = "s3generic"
	// ServerTypeUnknown means detection has not been attempted or the endpoint was unreachable.
	ServerTypeUnknown ServerType = ""
)

// Detect probes the endpoint and returns the server type.
//
// Detection order:
//  1. GET /.dirio/health → 200 → DirIO   (DirIO also serves /minio/health/live, so this must come first)
//  2. GET /minio/health/live → 200 → MinIO
//  3. Otherwise → S3Generic
func Detect(ctx context.Context, endpoint string) (ServerType, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ServerTypeUnknown, err
	}

	base := u.Scheme + "://" + u.Host
	client := &http.Client{Timeout: 5 * time.Second}

	if probeOK(ctx, client, base+"/.dirio/health") {
		return ServerTypeDirIO, nil
	}
	if probeOK(ctx, client, base+"/minio/health/live") {
		return ServerTypeMinIO, nil
	}
	return ServerTypeS3Generic, nil
}

func probeOK(ctx context.Context, client *http.Client, rawURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
