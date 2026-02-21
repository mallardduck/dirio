package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/mallardduck/teapot-router/pkg/teapot"
	"github.com/mallardduck/teapot-router/pkg/urlbuilder"

	"github.com/mallardduck/dirio/internal/http/api"
	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/policy"

	"github.com/mallardduck/dirio/internal/config/data"

	"github.com/mallardduck/dirio/internal/logging"
	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
	"github.com/mallardduck/dirio/internal/mdns"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/persistence/path"
	"github.com/mallardduck/dirio/internal/persistence/storage"
)

// Config holds server configuration
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

	// Data directory configuration (optional)
	// If present, provides alternative admin credentials from data config
	DataConfig *data.ConfigData

	// CLICredentialsExplicitlySet tracks whether AccessKey/SecretKey were
	// explicitly provided (via env, flag, or config) vs using defaults
	CLICredentialsExplicitlySet bool

	// ShutdownTimeout is how long to wait for in-flight requests to finish
	// during graceful shutdown before connections are forcefully closed.
	ShutdownTimeout time.Duration
}

// Server represents the S3-compatible HTTP server
type Server struct {
	config       *Config
	router       *teapot.Router
	storage      *storage.Storage
	metadata     *metadata.Manager
	auth         *auth.Authenticator
	policyEngine *policy.Engine
	mdns         *mdns.Service
	log          *slog.Logger

	// console is the optional web admin console handler.
	// Set via SetConsole before calling Start.
	consoleHandler http.Handler
	consolePort    int // 0 = same port at /dirio/ui/

	// HTTP servers, set during Start.
	httpServer    *http.Server
	consoleServer *http.Server
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

// SetConsole registers the console handler with the server. When port is
// 0 the console is mounted at /dirio/ui/ on the main port. When port is
// non-zero (e.g. 9001) a separate listener is started for the console.
// Must be called before Start.
func (s *Server) SetConsole(h http.Handler, port int) {
	s.consoleHandler = h
	s.consolePort = port
}

// New creates a new server instance
func New(config *Config) (*Server, error) {
	log := logging.Component("server")

	// Create root filesystem with chroot protection
	rootFS, err := path.NewRootFS(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create root filesystem: %w", err)
	}

	// Initialize metadata manager
	metaMgr, err := metadata.New(rootFS)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metadata: %w", err)
	}

	// Check for MinIO migration
	if err := metaMgr.CheckAndImportMinIO(context.Background()); err != nil {
		log.Warn("minio data check & import failed", "error", err)
	}

	// Initialize storage backend
	store, err := storage.New(rootFS, metaMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize authenticator with appropriate credentials
	var authenticator *auth.Authenticator

	dataCredsConfigured := config.DataConfig != nil && config.DataConfig.Credentials.IsConfigured()

	if dataCredsConfigured && config.CLICredentialsExplicitlySet {
		// Both data config credentials and explicit CLI credentials present — dual admin mode.
		log.Info("Configured dual admin access",
			"cli_admin", config.AccessKey,
			"data_admin", config.DataConfig.Credentials.AccessKey)
		authenticator = auth.New(metaMgr, config.AccessKey, config.SecretKey)
		authenticator = authenticator.WithAlternativeRoot(
			config.DataConfig.Credentials.AccessKey,
			config.DataConfig.Credentials.SecretKey,
		)
	} else if dataCredsConfigured {
		// Data config credentials configured, no explicit CLI override — data config admin only.
		log.Info("Using data config admin credentials",
			"data_admin", config.DataConfig.Credentials.AccessKey)
		authenticator = auth.New(metaMgr,
			config.DataConfig.Credentials.AccessKey,
			config.DataConfig.Credentials.SecretKey,
		)
	} else {
		// No configured data credentials — fall back to CLI/env credentials.
		// This covers: new data dirs, or existing dirs where credentials haven't
		// been set yet via "dirio init".
		if !config.CLICredentialsExplicitlySet {
			log.Warn("No admin credentials configured — using defaults. Run \"dirio init\" to set up admin credentials.",
				"admin", config.AccessKey)
		}
		authenticator = auth.New(metaMgr, config.AccessKey, config.SecretKey)
	}

	// Initialize policy engine
	policyEngine := policy.New()

	// Load all bucket policies from metadata at startup
	bucketPolicies, err := metaMgr.GetAllBucketPolicies(context.Background())
	if err != nil {
		log.Warn("failed to load bucket policies", "error", err)
	} else if len(bucketPolicies) > 0 {
		policyEngine.LoadBucketPolicies(context.Background(), bucketPolicies)
		log.Info("loaded bucket policies", "count", len(bucketPolicies))
	}

	// Create server
	srv := &Server{
		config:       config,
		storage:      store,
		metadata:     metaMgr,
		auth:         authenticator,
		policyEngine: policyEngine,
		log:          log,
	}

	// Setup routes
	srv.setupRoutes()

	return srv, nil
}

// setupRoutes configures HTTP routing
func (s *Server) setupRoutes() {
	s.router = teapot.New()

	// Add middleware (timing first for accurate timestamps, then trace ID, request ID, logging, auth)
	s.router.Use(chiMiddleware.StripSlashes)
	s.router.Use(middleware.Timing)
	s.router.Use(middleware.TraceID)
	s.router.Use(middleware.RequestID)
	s.router.Use(teapot.RouteContextMiddleware(s.router))
	s.router.Use(loggingHttp.PrepareAccessLogMiddleware(s.log))

	// Get root access keys for authorization middleware and filtering.
	// altRootAccessKey is only set when data config credentials are explicitly configured.
	rootAccessKey := s.config.AccessKey
	altRootAccessKey := ""
	if s.config.DataConfig != nil && s.config.DataConfig.Credentials.IsConfigured() {
		altRootAccessKey = s.config.DataConfig.Credentials.AccessKey
	}

	// Create API handler
	apiHandler := api.New(
		s.storage,
		s.metadata,
		s.auth,
		urlbuilder.New(s.config.CanonicalDomain),
		s.policyEngine,
		rootAccessKey,
		altRootAccessKey,
	)

	// Setup routes using shared function
	deps := &RouteDependencies{
		Auth:             s.auth,
		PolicyEngine:     s.policyEngine,
		Metadata:         s.metadata,
		RootAccessKey:    rootAccessKey,
		AltRootAccessKey: altRootAccessKey,
		APIHandler:       apiHandler,
		Debug:            s.config.Debug,
	}

	SetupRoutes(s.router, deps)
}

// consoleSamePort reports whether the console should be mounted on the main
// port. This is true when no separate port is configured, or when the
// configured port matches the main server port.
func (s *Server) consoleSamePort() bool {
	return s.consolePort == 0 || s.consolePort == s.config.Port
}

// buildHandler constructs the top-level http.Handler, mounting the console when
// it is configured for same-port operation.
func (s *Server) buildHandler() http.Handler {
	if s.consoleHandler == nil || !s.consoleSamePort() {
		return s.router
	}

	// Same-port console: mount at /dirio/ui/, everything else goes to the S3 router.
	mux := http.NewServeMux()
	mux.Handle("/dirio/ui/", http.StripPrefix("/dirio/ui", s.consoleHandler))
	mux.Handle("/", s.router)
	s.log.Info("console mounted on main port", "path", "/dirio/ui/")
	return mux
}

// Start begins serving HTTP requests with graceful shutdown support.
// It listens for SIGINT, SIGTERM, and ctx cancellation to trigger a graceful
// shutdown, draining both the main and console HTTP servers before stopping.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.config.Port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.buildHandler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start separate console listener if configured on a different port.
	if s.consoleHandler != nil && !s.consoleSamePort() {
		consoleAddr := fmt.Sprintf(":%d", s.consolePort)
		s.consoleServer = &http.Server{
			Addr:         consoleAddr,
			Handler:      s.consoleHandler,
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
		mdnsSvc, err := mdns.New(&mdns.Config{
			ServiceName: s.config.MDNSName,
			Port:        s.config.Port,
		})
		if err != nil {
			return fmt.Errorf("failed to create mDNS service: %w", err)
		}
		if err := mdnsSvc.Start(); err != nil {
			return fmt.Errorf("failed to start mDNS service: %w", err)
		}
		s.mdns = mdnsSvc
		s.log.Info("mdns service started", "host", mdnsSvc.GetAdvertisedHost())
	}

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
// It uses the configured ShutdownTimeout for draining in-flight requests.
func (s *Server) gracefulShutdown() {
	timeout := s.config.ShutdownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Shut down both HTTP servers concurrently.
	var wg sync.WaitGroup

	if s.consoleServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.consoleServer.Shutdown(ctx); err != nil {
				s.log.Error("console server shutdown error", "error", err)
			}
		}()
	}

	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.log.Error("http server shutdown error", "error", err)
	}

	wg.Wait()

	// Stop mDNS service.
	if s.mdns != nil {
		if err := s.mdns.Stop(); err != nil {
			s.log.Error("mdns shutdown error", "error", err)
		}
		s.mdns = nil
	}
}
