package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/mallardduck/teapot-router/pkg/teapot"
	"github.com/mallardduck/teapot-router/pkg/urlbuilder"

	"github.com/mallardduck/dirio/internal/consts"
	minioHTTP "github.com/mallardduck/dirio/internal/minio/http"

	"github.com/mallardduck/dirio/internal/console"
	"github.com/mallardduck/dirio/internal/service"

	"github.com/mallardduck/dirio/internal/http/api/dirio"
	"github.com/mallardduck/dirio/internal/http/server/health"
	"github.com/mallardduck/dirio/internal/http/server/metrics"
	"github.com/mallardduck/dirio/internal/http/server/prof"

	"github.com/mallardduck/dirio/internal/http/api"
	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/http/middleware"
	miniomiddleware "github.com/mallardduck/dirio/internal/minio/middleware"
	"github.com/mallardduck/dirio/internal/policy"

	"github.com/mallardduck/dirio/internal/config/data"

	loggingHttp "github.com/mallardduck/dirio/internal/http/middleware/logging"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/mdns"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/persistence/storage"
	"github.com/mallardduck/dirio/internal/telemetry"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Config holds server configuration.
type Config struct {
	DataDir   string
	Port      int
	AccessKey string // CLI admin credentials
	SecretKey string // CLI admin credentials

	// mDNS settings
	MDNSEnabled  bool
	MDNSName     string
	MDNSHostname string
	MDNSMode     string

	// URL generation
	CanonicalDomain string

	// Debug mode
	Debug bool

	// DataConfig provides alternative admin credentials from .dirio/config.json.
	// Always populated when coming from the Starter (serve command).
	DataConfig *data.ConfigData

	// CLICredentialsExplicitlySet tracks whether AccessKey/SecretKey were
	// explicitly provided (via env, flag, or config) vs using defaults.
	CLICredentialsExplicitlySet bool

	// ShutdownTimeout is how long to wait for in-flight requests to finish
	// during graceful shutdown before connections are forcefully closed.
	ShutdownTimeout time.Duration

	// Pre-initialised components supplied by the Starter.
	// Both fields are required; server.New does not construct them itself.
	RootFS   billy.Filesystem
	Metadata *metadata.Manager

	// Telemetry is the OTel provider.  When non-nil the server registers the
	// Prometheus metrics endpoint and adds the otelhttp middleware.
	Telemetry *telemetry.Provider
}

// Server represents the S3-compatible HTTP server.
type Server struct {
	config       *Config
	router       *teapot.Router
	storage      *storage.Storage
	metadata     *metadata.Manager
	auth         *auth.Authenticator
	policyEngine *policy.Engine
	mdns         *mdns.Service
	telemetry    *telemetry.Provider
	log          *slog.Logger

	// consoleRouter is the optional web admin console router.
	// Set via SetConsole before calling Start.
	consoleRouter *teapot.Router
	consolePort   int // 0 = same port at /dirio/ui/

	// HTTP servers, set during Start.
	httpServer    *http.Server
	consoleServer *http.Server

	// Concurrency fields.
	// inShutdown is set to true at the start of gracefulShutdown so that
	// middleware can return 503 for any request that slips through after the
	// listener closes.
	inShutdown atomic.Bool
	// shutdownOnce ensures the gracefulShutdown body executes exactly once
	// even if multiple signals or context cancellations arrive simultaneously.
	shutdownOnce sync.Once
	// requestCount tracks the number of requests currently being handled.
	// Incremented/decremented by the trackRequest middleware.  Read during
	// shutdown to detect handler leaks.
	requestCount atomic.Int32
}

// Metadata returns the metadata manager (used by the console wire file).
func (s *Server) Metadata() *metadata.Manager { return s.metadata }

// Storage returns the storage backend (used by the console wire file).
func (s *Server) Storage() *storage.Storage { return s.storage }

// PolicyEngine returns the policy engine (used by the console wire file).
func (s *Server) PolicyEngine() *policy.Engine { return s.policyEngine }

// Router returns the S3 router (used by the console wire file for URL generation).
func (s *Server) Router() *teapot.Router { return s.router }

// Auth returns the authenticator (used by the console wire file for admin credential validation).
func (s *Server) Auth() *auth.Authenticator { return s.auth }

// SetConsole registers the console handler with the server.  When port is
// 0 the console is mounted at /dirio/ui/ on the main port.  When port is
// non-zero (e.g. 9001) a separate listener is started for the console.
// Must be called before Start.
func (s *Server) SetConsole(h *teapot.Router, port int) {
	s.consoleRouter = h
	s.consolePort = port
}

// New creates a new Server from a fully-prepared Config.
//
// The Config must carry RootFS and metadata from the Starter — server.New no
// longer constructs those itself, ensuring MinIO import and DataConfig
// finalisation have already completed before the HTTP layer is wired up.
func New(config *Config) (*Server, error) {
	log := logging.Component("server")

	rootFS := config.RootFS
	metaMgr := config.Metadata

	// Initialize storage backend.
	store, err := storage.New(rootFS, metaMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize authenticator with appropriate credentials.
	authenticator := Authenticator(metaMgr, config)

	// Initialize policy engine with IAM policy resolver.
	resolver := policy.NewMetadataResolver(metaMgr)
	policyEngine := policy.New(resolver)

	// Load all bucket policies from metadata at startup.
	// TODO: consider refactor of this metadata init code & get a real context into here
	bucketPolicies, err := metaMgr.GetAllBucketPolicies(context.Background())
	if err != nil {
		// TODO potentially this should be a hard failure
		log.Warn("failed to load bucket policies", "error", err)
	} else if len(bucketPolicies) > 0 {
		policyEngine.LoadBucketPolicies(context.Background(), bucketPolicies)
		log.Info("loaded bucket policies", "count", len(bucketPolicies))
	}

	srv := &Server{
		config:       config,
		storage:      store,
		metadata:     metaMgr,
		auth:         authenticator,
		policyEngine: policyEngine,
		telemetry:    config.Telemetry,
		log:          log,
	}

	// Register metadata observable instruments once both the provider and
	// the metadata manager are ready.
	if config.Telemetry != nil {
		if err := config.Telemetry.RegisterMetadataObservers(func() telemetry.MetadataStats {
			gets, misses, entries, dbBytes := metaMgr.MetricsSnapshot()
			return telemetry.MetadataStats{
				CacheGets:    gets,
				CacheMisses:  misses,
				CacheEntries: entries,
				DBSizeBytes:  dbBytes,
			}
		}); err != nil {
			log.Warn("failed to register metadata telemetry observers", "error", err)
		}
	}

	srv.setupRoutes()

	return srv, nil
}

// setupRoutes configures HTTP routing.
func (s *Server) setupRoutes() {
	s.router = teapot.New()

	s.router.Use(loggingHttp.RecoveryMiddleware)
	s.router.Use(chiMiddleware.StripSlashes)
	s.router.Use(middleware.Timing)
	s.router.Use(s.trackRequest) // count in-flight requests; check inShutdown
	s.router.Use(middleware.TraceID)
	s.router.Use(middleware.RequestID)
	s.router.Use(teapot.RouteContextMiddleware(s.router))
	s.router.Use(miniomiddleware.CompatHeaders)
	s.router.Use(loggingHttp.PrepareAccessLogMiddleware(s.log))

	// OTel HTTP instrumentation — records http.server.request.duration histogram
	// and http.server.active_requests gauge automatically.
	if s.telemetry != nil {
		s.router.Use(otelhttp.NewMiddleware("dirio.server",
			otelhttp.WithMeterProvider(s.telemetry.MeterProvider()),
		))
	}

	serviceFactory := service.NewServiceFactory(s.storage, s.metadata, s.policyEngine, s.auth)
	apiHandler := api.New(
		serviceFactory,
		s.auth,
		urlbuilder.New(s.config.CanonicalDomain),
	)

	consoleAdapter := console.NewAdapter(serviceFactory)

	deps := RouteDependencies{
		auth:         s.auth,
		policyEngine: s.policyEngine,
		metadata:     s.metadata,
		adminKeys:    s.auth,
		APIHandler:   apiHandler,

		// New things
		Health:   health.New(s.metadata, s.config.RootFS),
		Metrics:  metrics.New(s.telemetry),
		Minio:    minioHTTP.New(serviceFactory),
		Pprof:    prof.New(),
		DirioAPI: dirioapi.New(consoleAdapter, s.auth),
	}

	SetupRoutes(s.router, deps)
}

// trackRequest is a middleware that increments requestCount for each
// in-flight request and decrements it when the handler returns.  It also
// returns 503 early for any request that arrives after shutdown has begun,
// giving upstream load balancers a fast signal to stop routing here.
func (s *Server) trackRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.inShutdown.Load() {
			http.Error(w, "server shutting down", http.StatusServiceUnavailable)
			return
		}
		s.requestCount.Add(1)
		defer s.requestCount.Add(-1)
		next.ServeHTTP(w, r)
	})
}

// consoleSamePort reports whether the console should be mounted on the main port.
func (s *Server) consoleSamePort() bool {
	return s.consolePort == 0 || s.consolePort == s.config.Port
}

// buildHandler constructs the top-level http.Handler, mounting the console
// when it is configured for same-port operation.
func (s *Server) buildHandler() http.Handler {
	if s.consoleRouter == nil || !s.consoleSamePort() {
		return s.router
	}

	s.router.MountNamed("/dirio/ui", "dirio", s.consoleRouter)
	s.log.Info("console mounted on main port", "path", "/dirio/ui/")
	return s.router
}

// Start begins serving HTTP requests with graceful shutdown support.
// It listens for SIGINT, SIGTERM, and ctx cancellation to trigger a graceful
// shutdown, draining both the main and console HTTP servers before stopping.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.config.Port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      middleware.SetDefaultHeadersMiddleware(s.buildHandler()),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start separate console listener if configured on a different port.
	if s.consoleRouter != nil && !s.consoleSamePort() {
		consoleAddr := fmt.Sprintf(":%d", s.consolePort)
		// TODO when on dedicated port we should also add loggingHttp.RecoveryMiddleware like main router too
		s.consoleServer = &http.Server{
			Addr:         consoleAddr,
			Handler:      middleware.SetDefaultHeadersMiddleware(s.consoleRouter),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		go func() {
			s.log.Info("console listening on separate port", "addr", consoleAddr)
			if err := s.consoleServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.log.Error("console server error", "error", err)
			}
		}()
	}

	// Start mDNS service if enabled.
	if s.config.MDNSEnabled {
		// In dual-port mode advertise the admin/console port separately so that
		// mDNS clients can discover both the S3 data plane and the control plane.
		adminPort := s.consolePort
		if s.consoleSamePort() {
			adminPort = 0 // single-port mode: no separate admin advertisement
		}
		mdnsSvc, err := mdns.New(&mdns.Config{
			ServiceName: s.config.MDNSName,
			Port:        s.config.Port,
			AdminPort:   adminPort,
		})
		if err != nil {
			return fmt.Errorf("failed to create mDNS service: %w", err)
		}
		if err := mdnsSvc.Start(ctx); err != nil {
			return fmt.Errorf("failed to start mDNS service: %w", err)
		}
		s.mdns = mdnsSvc
		s.log.InfoContext(ctx, "mdns service started", "host", mdnsSvc.GetAdvertisedHost())
	}

	// Watch .dirio/config.json for credential changes made by external commands
	// (e.g. "dirio credentials set"). The watcher stops when ctx is cancelled.
	go s.watchDataConfig(ctx)

	serverErr := make(chan error, 1)
	go func() {
		s.log.Info("server listening", "addr", addr)
		if err := s.httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	select {
	case err := <-serverErr:
		s.gracefulShutdown()
		return err
	case <-ctx.Done():
		s.log.Info("context cancelled, initiating graceful shutdown")
	case sig := <-sigChan:
		s.log.Info("received signal, initiating graceful shutdown", "signal", sig)
	}

	s.gracefulShutdown()
	s.log.Info("server stopped gracefully")
	return nil
}

// gracefulShutdown drains all HTTP servers and stops ancillary services.
// It is safe to call multiple times — the shutdown body executes exactly once.
func (s *Server) gracefulShutdown() {
	s.shutdownOnce.Do(func() {
		// Signal middleware to return 503 for any straggling requests.
		s.inShutdown.Store(true)

		timeout := s.config.ShutdownTimeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// Shut down both HTTP servers concurrently.
		var wg sync.WaitGroup

		if s.consoleServer != nil {
			wg.Go(func() {
				if err := s.consoleServer.Shutdown(ctx); err != nil {
					s.log.Error("console server shutdown error", "error", err)
				}
			})
		}

		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.log.Error("http server shutdown error", "error", err)
		}

		wg.Wait()

		// Sanity-check: http.Server.Shutdown waits for handlers to return, so
		// the counter should be zero here.  A non-zero value indicates a handler
		// that spawned a goroutine and returned early without finishing its work.
		if n := s.requestCount.Load(); n != 0 {
			s.log.Warn("shutdown complete but request counter non-zero — possible handler leak",
				"count", n)
		}

		// Stop mDNS service.
		if s.mdns != nil {
			if err := s.mdns.Stop(); err != nil {
				s.log.Error("mdns shutdown error", "error", err)
			}
			s.mdns = nil
		}

		// Close the metadata bolt index.
		if err := s.metadata.Close(); err != nil {
			s.log.Error("metadata close error", "error", err)
		}
	})
}

// watchDataConfig watches .dirio/config.json for changes and reloads admin
// credentials into the running authenticator when the file is written.
// This allows "dirio credentials set" to take effect without a server restart.
// The goroutine exits when ctx is cancelled (i.e. on graceful shutdown).
func (s *Server) watchDataConfig(ctx context.Context) {
	log := logging.Component("config-watcher")

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error("failed to create config file watcher", "error", err)
		return
	}
	defer watcher.Close()

	// Watch the directory so we catch rename/replace patterns used by some
	// editors and atomic-write helpers, not just in-place writes.
	watchDir := filepath.Join(s.config.DataDir, consts.DirIOMetadataDir)
	if err := watcher.Add(watchDir); err != nil {
		log.Warn("config watcher: failed to watch directory — live credential reload unavailable",
			"dir", watchDir, "error", err)
		return
	}

	log.Info("watching for credential changes", "dir", watchDir)

	configFile := filepath.Join(watchDir, "config.json")

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Only act on the config file itself.
			if filepath.Clean(event.Name) != filepath.Clean(configFile) {
				continue
			}

			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			// Debounce: drain any follow-up events for 100 ms then reload once.
			debounce := time.After(100 * time.Millisecond)
		drain:
			for {
				select {
				case <-debounce:
					break drain
				case _, ok := <-watcher.Events:
					if !ok {
						return
					}
				case <-ctx.Done():
					return
				}
			}

			s.reloadDataCredentials(log)

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Warn("config watcher error", "error", err)
		}
	}
}

// reloadDataCredentials reads the current data config and updates the
// authenticator's credentials in the appropriate slot (primary or alt).
func (s *Server) reloadDataCredentials(log *slog.Logger) {
	fs := osfs.New(s.config.DataDir)
	dc, err := data.LoadDataConfig(fs)
	if err != nil {
		log.Warn("credential reload: failed to read data config", "error", err)
		return
	}

	if !dc.Credentials.IsConfigured() {
		log.Debug("credential reload: data config has no credentials configured — skipping")
		return
	}

	// In dual-admin mode (CLI creds + data config creds) the data config
	// credentials live in the authenticator's alt slot; otherwise primary.
	if s.config.CLICredentialsExplicitlySet {
		s.auth.UpdateAltCredentials(dc.Credentials.AccessKey, dc.Credentials.SecretKey)
	} else {
		s.auth.UpdatePrimaryCredentials(dc.Credentials.AccessKey, dc.Credentials.SecretKey)
	}

	log.Info("admin credentials reloaded from data config",
		"access_key", dc.Credentials.AccessKey)
}

// ActiveRequests returns the current number of in-flight requests.
// Exposed for debug endpoints and tests.
func (s *Server) ActiveRequests() int32 {
	return s.requestCount.Load()
}

// InShutdown reports whether the server has begun graceful shutdown.
func (s *Server) InShutdown() bool {
	return s.inShutdown.Load()
}
