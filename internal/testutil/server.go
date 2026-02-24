// Package testutil provides a shared test server and helpers used across all
// dirio test suites (integration, admin, console).
//
// Every constructor goes through startup.Init + startup.Prepare so the server
// is always initialised the same way the real binary initialises it.  This
// prevents the class of failures where tests pass locally but fail after the
// Starter was introduced because RootFS / Metadata were nil.
package testutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"

	"github.com/mallardduck/dirio/internal/config/data"
	"github.com/mallardduck/dirio/internal/crypto"
	"github.com/mallardduck/dirio/internal/http/server"
	"github.com/mallardduck/dirio/internal/startup"
)

const (
	// DefaultAccessKey is the access key used by New and NewWithDataDir.
	DefaultAccessKey = "testaccess"
	// DefaultSecretKey is the secret key used by New and NewWithDataDir.
	DefaultSecretKey = "testsecretkey123"
)

// TestServer wraps a running dirio server for use in tests.
type TestServer struct {
	// Server is the underlying server instance.  Test packages that need to
	// access server internals (e.g. the console package wiring SetConsole) can
	// use this field directly.
	Server    *server.Server
	DataDir   string
	Port      int
	BaseURL   string
	AdminURL  string
	AccessKey string
	SecretKey string

	cancel context.CancelFunc
	done   chan struct{}
}

// New starts a server with the default test credentials (testaccess /
// testsecretkey123) explicitly set.  This is the right constructor for the
// vast majority of tests.
func New(t *testing.T) *TestServer {
	t.Helper()
	return NewWithCredentials(t, DefaultAccessKey, DefaultSecretKey, true)
}

// NewWithCredentials starts a server with the given CLI credentials.
// Set explicit=true to simulate credentials provided via --access-key flag;
// set explicit=false to simulate the "no explicit credentials" startup path.
func NewWithCredentials(t *testing.T, accessKey, secretKey string, explicit bool) *TestServer {
	t.Helper()

	dataDir, err := os.MkdirTemp("", "dirio-test-*")
	if err != nil {
		t.Fatalf("testutil: create temp dir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	s, err := startup.Init(dataDir)
	if err != nil {
		os.RemoveAll(dataDir)
		cancel()
		t.Fatalf("testutil: startup.Init: %v", err)
	}
	if err := s.Prepare(ctx, "", accessKey, secretKey, explicit); err != nil {
		os.RemoveAll(dataDir)
		cancel()
		t.Fatalf("testutil: startup.Prepare: %v", err)
	}

	cfg := &server.Config{
		DataDir:                     dataDir,
		Port:                        FindFreePort(t),
		AccessKey:                   accessKey,
		SecretKey:                   secretKey,
		CLICredentialsExplicitlySet: explicit,
		RootFS:                      s.RootFS(),
		Metadata:                    s.MetadataManager(),
	}
	return startServer(t, dataDir, cfg, cancel)
}

// NewWithDataDir starts a server against an existing, pre-populated data
// directory.  Useful for MinIO import tests where the data dir is seeded
// before the server starts.  The server uses the default test credentials.
// Call Stop() to shut down without removing the data directory.
func NewWithDataDir(t *testing.T, dataDir string) *TestServer {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	s, err := startup.Init(dataDir)
	if err != nil {
		cancel()
		t.Fatalf("testutil: startup.Init: %v", err)
	}
	if err := s.Prepare(ctx, "us-east-1", DefaultAccessKey, DefaultSecretKey, true); err != nil {
		cancel()
		t.Fatalf("testutil: startup.Prepare: %v", err)
	}

	cfg := &server.Config{
		DataDir:                     dataDir,
		Port:                        FindFreePort(t),
		AccessKey:                   DefaultAccessKey,
		SecretKey:                   DefaultSecretKey,
		DataConfig:                  s.DataConfig,
		CLICredentialsExplicitlySet: true,
		RootFS:                      s.RootFS(),
		Metadata:                    s.MetadataManager(),
	}
	return startServer(t, dataDir, cfg, cancel)
}

// NewDualAdmin starts a server with both CLI credentials (explicit) and a
// pre-existing data config holding separate admin credentials.  Both sets of
// credentials will be accepted.
func NewDualAdmin(t *testing.T, cliAccessKey, cliSecretKey, dataAccessKey, dataSecretKey string) *TestServer {
	t.Helper()

	dataDir, err := os.MkdirTemp("", "dirio-test-*")
	if err != nil {
		t.Fatalf("testutil: create temp dir: %v", err)
	}

	// Write the data config to disk before Init so it is loaded during Init.
	dc := writeDataConfig(t, dataDir, dataAccessKey, dataSecretKey)

	ctx, cancel := context.WithCancel(context.Background())

	s, err := startup.Init(dataDir)
	if err != nil {
		os.RemoveAll(dataDir)
		cancel()
		t.Fatalf("testutil: startup.Init: %v", err)
	}
	// isNew is false (existing config loaded), so Prepare only wires the
	// metadata manager and sets the global instance ID.
	if err := s.Prepare(ctx, "", "", "", false); err != nil {
		os.RemoveAll(dataDir)
		cancel()
		t.Fatalf("testutil: startup.Prepare: %v", err)
	}

	cfg := &server.Config{
		DataDir:                     dataDir,
		Port:                        FindFreePort(t),
		AccessKey:                   cliAccessKey,
		SecretKey:                   cliSecretKey,
		DataConfig:                  dc,
		CLICredentialsExplicitlySet: true,
		RootFS:                      s.RootFS(),
		Metadata:                    s.MetadataManager(),
	}
	return startServer(t, dataDir, cfg, cancel)
}

// NewDataConfigOnly starts a server where admin credentials come exclusively
// from a data config.  The CLI credentials are treated as defaults (not
// explicitly set) and will therefore be ignored when the data config is
// present.
func NewDataConfigOnly(t *testing.T, dataAccessKey, dataSecretKey string) *TestServer {
	t.Helper()

	dataDir, err := os.MkdirTemp("", "dirio-test-*")
	if err != nil {
		t.Fatalf("testutil: create temp dir: %v", err)
	}

	dc := writeDataConfig(t, dataDir, dataAccessKey, dataSecretKey)

	ctx, cancel := context.WithCancel(context.Background())

	s, err := startup.Init(dataDir)
	if err != nil {
		os.RemoveAll(dataDir)
		cancel()
		t.Fatalf("testutil: startup.Init: %v", err)
	}
	if err := s.Prepare(ctx, "", "", "", false); err != nil {
		os.RemoveAll(dataDir)
		cancel()
		t.Fatalf("testutil: startup.Prepare: %v", err)
	}

	cfg := &server.Config{
		DataDir:                     dataDir,
		Port:                        FindFreePort(t),
		AccessKey:                   DefaultAccessKey, // default, will be ignored
		SecretKey:                   DefaultSecretKey,
		DataConfig:                  dc,
		CLICredentialsExplicitlySet: false,
		RootFS:                      s.RootFS(),
		Metadata:                    s.MetadataManager(),
	}
	return startServer(t, dataDir, cfg, cancel)
}

// NewWithPreStartHook starts a server with the default test credentials and
// calls hook on the *server.Server after New but before Start.  This is used
// by the console test package so it can call SetConsole before the HTTP
// listener is opened.
func NewWithPreStartHook(t *testing.T, hook func(*server.Server)) *TestServer {
	t.Helper()

	dataDir, err := os.MkdirTemp("", "dirio-test-*")
	if err != nil {
		t.Fatalf("testutil: create temp dir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	s, err := startup.Init(dataDir)
	if err != nil {
		os.RemoveAll(dataDir)
		cancel()
		t.Fatalf("testutil: startup.Init: %v", err)
	}
	if err := s.Prepare(ctx, "", DefaultAccessKey, DefaultSecretKey, true); err != nil {
		os.RemoveAll(dataDir)
		cancel()
		t.Fatalf("testutil: startup.Prepare: %v", err)
	}

	cfg := &server.Config{
		DataDir:                     dataDir,
		Port:                        FindFreePort(t),
		AccessKey:                   DefaultAccessKey,
		SecretKey:                   DefaultSecretKey,
		CLICredentialsExplicitlySet: true,
		RootFS:                      s.RootFS(),
		Metadata:                    s.MetadataManager(),
	}
	return startServer(t, dataDir, cfg, cancel, hook)
}

// startServer is the shared wiring: server.New → optional pre-start hook →
// start goroutine → waitForReady → register cleanup.
// It never removes dataDir itself — that is handled by Cleanup (called from t.Cleanup).
func startServer(t *testing.T, dataDir string, cfg *server.Config, cancel context.CancelFunc, preStart ...func(*server.Server)) *TestServer {
	t.Helper()

	srv, err := server.New(cfg)
	if err != nil {
		os.RemoveAll(dataDir)
		cancel()
		t.Fatalf("testutil: server.New: %v", err)
	}

	for _, hook := range preStart {
		if hook != nil {
			hook(srv)
		}
	}

	done := make(chan struct{})
	ctx, cancelCtx := context.WithCancel(context.Background())

	// Replace the caller-supplied cancel with one that also cancels the
	// server context.  We keep the original cancel to release resources from
	// startup.Prepare (it is idempotent).
	combined := func() {
		cancel()
		cancelCtx()
	}

	ts := &TestServer{
		Server:    srv,
		DataDir:   dataDir,
		Port:      cfg.Port,
		BaseURL:   fmt.Sprintf("http://localhost:%d", cfg.Port),
		AdminURL:  fmt.Sprintf("http://localhost:%d/minio/admin/v3", cfg.Port),
		AccessKey: cfg.AccessKey,
		SecretKey: cfg.SecretKey,
		cancel:    combined,
		done:      done,
	}

	go func() {
		defer close(done)
		_ = srv.Start(ctx)
	}()

	if !ts.waitForReady(5 * time.Second) {
		ts.Cleanup()
		t.Fatalf("testutil: server failed to start within timeout")
	}

	t.Cleanup(ts.Cleanup)
	return ts
}

// Stop shuts down the server gracefully and waits for it to fully stop.
// The data directory is NOT removed — use Cleanup for that.
func (ts *TestServer) Stop() {
	if ts.cancel != nil {
		ts.cancel()
		ts.cancel = nil
	}
	if ts.done != nil {
		<-ts.done
		ts.done = nil
	}
}

// Cleanup stops the server and removes the temporary data directory.
// Registered automatically with t.Cleanup by all constructors.
func (ts *TestServer) Cleanup() {
	ts.Stop()
	if ts.DataDir != "" {
		os.RemoveAll(ts.DataDir)
	}
}

func (ts *TestServer) waitForReady(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 100 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(ts.BaseURL + "/")
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// URL returns the full URL for a server-relative path (e.g. "/mybucket").
func (ts *TestServer) URL(path string) string { return ts.BaseURL + path }

// BucketURL returns the URL for a bucket.
func (ts *TestServer) BucketURL(bucket string) string {
	return fmt.Sprintf("%s/%s", ts.BaseURL, bucket)
}

// ObjectURL returns the URL for an object within a bucket.
func (ts *TestServer) ObjectURL(bucket, key string) string {
	return fmt.Sprintf("%s/%s/%s", ts.BaseURL, bucket, key)
}

// DataPath returns an absolute path rooted at the server's data directory.
func (ts *TestServer) DataPath(parts ...string) string {
	joined := ts.DataDir
	for _, p := range parts {
		joined = filepath.Join(joined, p)
	}
	return joined
}

// writeDataConfig creates a .dirio/config.json in dataDir with the given
// admin credentials and returns the resulting ConfigData.
//
// crypto.Init is called first so the keyring is generated for this specific
// dataDir before SaveDataConfig encrypts the secret key.  startup.Init
// (called next in the constructor) will find the same keyring and use the
// same key, so LoadDataConfig can decrypt the credentials successfully.
func writeDataConfig(t *testing.T, dataDir, accessKey, secretKey string) *data.ConfigData {
	t.Helper()
	if err := crypto.Init(dataDir); err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("testutil: crypto.Init before writeDataConfig: %v", err)
	}
	fs := osfs.New(dataDir)
	dc := data.DefaultDataConfig()
	dc.Credentials.AccessKey = accessKey
	dc.Credentials.SecretKey = secretKey
	if err := data.SaveDataConfig(fs, dc); err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("testutil: write data config: %v", err)
	}
	return dc
}

// CreateDataConfigWithCredentials writes (or overwrites) a .dirio/config.json
// in the server's data directory with the given credentials.  Useful for
// testing live credential-reload without restarting the server.
func CreateDataConfigWithCredentials(ts *TestServer, accessKey, secretKey string) {
	writeDataConfigRaw(ts.DataDir, accessKey, secretKey)
}

// writeDataConfigRaw is the panic-on-error variant used in non-test contexts.
func writeDataConfigRaw(dataDir, accessKey, secretKey string) {
	fs := osfs.New(dataDir)
	dc := data.DefaultDataConfig()
	dc.Credentials.AccessKey = accessKey
	dc.Credentials.SecretKey = secretKey
	if err := data.SaveDataConfig(fs, dc); err != nil {
		panic(fmt.Sprintf("testutil: write data config: %v", err))
	}
}

// FindFreePort finds an available TCP port on 127.0.0.1 and returns it.
func FindFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testutil: find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// DrainAndClose discards the response body and closes it.
func DrainAndClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
