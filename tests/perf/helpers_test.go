//go:build perf

package perf_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/http/server"
)

const (
	perfAccessKey = "perfaccess"
	perfSecretKey = "perfsecret1234"
	perfRegion    = "us-east-1"
)

// perfServer wraps an in-process DirIO server configured for profiling.
// Debug is always true so pprof endpoints are available.
type perfServer struct {
	srv     *server.Server
	port    int
	dataDir string
	baseURL string
	cancel  context.CancelFunc
}

// newPerfServer starts an in-process server with Debug: true and registers
// cleanup via t.Cleanup.
func newPerfServer(t *testing.T) *perfServer {
	t.Helper()

	dataDir, err := os.MkdirTemp("", "dirio-perf-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}

	port := freePort(t)

	cfg := &server.Config{
		DataDir:   dataDir,
		Port:      port,
		AccessKey: perfAccessKey,
		SecretKey: perfSecretKey,
		Debug:     true,
	}

	srv, err := server.New(cfg)
	if err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	ps := &perfServer{
		srv:     srv,
		port:    port,
		dataDir: dataDir,
		baseURL: fmt.Sprintf("http://localhost:%d", port),
		cancel:  cancel,
	}

	go func() { _ = srv.Start(ctx) }()

	if !ps.waitReady(15 * time.Second) {
		cancel()
		os.RemoveAll(dataDir)
		t.Fatalf("server failed to start within timeout")
	}

	t.Cleanup(func() {
		cancel()
		os.RemoveAll(dataDir)
	})

	return ps
}

// dockerEndpoint returns the address reachable from within Docker containers.
func (ps *perfServer) dockerEndpoint() string {
	return fmt.Sprintf("http://host.docker.internal:%d", ps.port)
}

func (ps *perfServer) waitReady(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(ps.baseURL + "/healthz")
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// listObjectsV2 issues one ListObjectsV2 call and returns the HTTP status.
// Signs the request using the perf credentials.
func (ps *perfServer) listObjectsV2(t *testing.T, bucket, prefix string, maxKeys int) int {
	t.Helper()
	url := fmt.Sprintf("%s/%s?list-type=2&max-keys=%d", ps.baseURL, bucket, maxKeys)
	if prefix != "" {
		url += "&prefix=" + prefix
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build ListObjectsV2 request: %v", err)
	}
	signRequest(req, nil, perfAccessKey, perfSecretKey, perfRegion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("ListObjectsV2: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	return resp.StatusCode
}

// signRequest signs req using AWS Signature V4.
func signRequest(req *http.Request, body []byte, accessKey, secretKey, region string) {
	timestamp := time.Now().UTC()

	var payloadHash string
	if len(body) > 0 {
		h := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(h[:])
	} else {
		h := sha256.Sum256([]byte{})
		payloadHash = hex.EncodeToString(h[:])
	}

	req.Header.Set("X-Amz-Date", timestamp.Format("20060102T150405Z"))
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("Host", req.Host)

	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	sort.Strings(signedHeaders)

	canonicalReq := auth.BuildCanonicalRequest(req, signedHeaders, payloadHash)
	stringToSign := auth.BuildStringToSign(timestamp, region, canonicalReq)
	signature := auth.ComputeSignature(secretKey, timestamp, region, stringToSign)

	dateStamp := timestamp.Format("20060102")
	credScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credScope, strings.Join(signedHeaders, ";"), signature,
	))
}

// seedBucket runs an mc container to upload objectCount 1 KB objects across
// several prefix patterns (flat/, prefix-a/, prefix-b/, deep/) into bucket.
// Blocks until the container exits.
func seedBucket(t *testing.T, ps *perfServer, bucket string, objectCount int) {
	t.Helper()

	ctx := context.Background()

	flat := objectCount * 40 / 100
	alpha := objectCount * 20 / 100
	beta := objectCount * 20 / 100
	deep := objectCount - flat - alpha - beta

	// seq -w zero-pads to the same width as the largest number,
	// giving consistent key names without needing printf format strings.
	script := fmt.Sprintf(`
set -e
mc alias set bench %s %s %s --quiet
mc mb bench/%s --quiet 2>/dev/null || true

dd if=/dev/urandom of=/tmp/seed bs=1024 count=1 2>/dev/null

seed_prefix() {
  local prefix="$1"
  local n="$2"
  mkdir -p /tmp/seeds/"$prefix"
  for key in $(seq -w 1 "$n"); do
    cp /tmp/seed /tmp/seeds/"$prefix"/obj-"$key"
  done
  mc cp --recursive --quiet /tmp/seeds/"$prefix"/ bench/%s/"$prefix"/
  echo "seeded $n objects under $prefix/"
}

seed_prefix flat       %d
seed_prefix prefix-a   %d
seed_prefix prefix-b   %d
seed_prefix deep       %d

TOTAL=$(mc ls --recursive bench/%s | wc -l)
echo "total objects in bucket: $TOTAL"
`,
		ps.dockerEndpoint(), perfAccessKey, perfSecretKey,
		bucket,
		bucket,
		flat, alpha, beta, deep,
		bucket,
	)

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../clients/minio",
			Dockerfile: "Dockerfile",
			Repo:       "dirio-mc-test",
			Tag:        "local",
			KeepImage:  true,
		},
		Entrypoint: []string{"/bin/bash", "-c"},
		Cmd:        []string{script},
		WaitingFor: wait.ForExit().WithExitTimeout(10 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start mc seeding container: %v", err)
	}
	defer container.Terminate(ctx) //nolint:errcheck

	logs, err := container.Logs(ctx)
	if err != nil {
		t.Fatalf("get seeding logs: %v", err)
	}
	defer logs.Close()
	logBytes, _ := io.ReadAll(logs)
	t.Logf("seeding output:\n%s", string(logBytes))

	state, err := container.State(ctx)
	if err != nil {
		t.Fatalf("get container state: %v", err)
	}
	if state.ExitCode != 0 {
		t.Fatalf("seeding container exited with code %d", state.ExitCode)
	}
}

// captureProfile downloads a pprof snapshot from the server's /debug/pprof/
// endpoint and writes it to tests/perf/profiles/<type>-<timestamp>.pprof.
// For the "profile" type (CPU), seconds controls the sample duration.
// Returns the path to the written file.
func captureProfile(t *testing.T, ps *perfServer, profileType string, seconds int) string {
	t.Helper()

	outDir := filepath.Join("profiles")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create profiles dir: %v", err)
	}

	url := fmt.Sprintf("%s/debug/pprof/%s", ps.baseURL, profileType)
	if profileType == "profile" && seconds > 0 {
		url = fmt.Sprintf("%s?seconds=%d", url, seconds)
	}

	timeout := 30 * time.Second
	if seconds > 0 {
		timeout = time.Duration(seconds+20) * time.Second
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("fetch %s profile: %v", profileType, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("fetch %s profile: status %d body: %s", profileType, resp.StatusCode, body)
	}

	name := fmt.Sprintf("%s-%s.pprof", profileType, time.Now().Format("20060102-150405"))
	outPath := filepath.Join(outDir, name)
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create profile file: %v", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	t.Logf("profile written: %s", outPath)
	return outPath
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}
