package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/mallardduck/dirio/internal/api"
	"github.com/mallardduck/dirio/internal/auth"
	"github.com/mallardduck/dirio/internal/dataconfig"
	"github.com/mallardduck/dirio/internal/logging"
	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
	"github.com/mallardduck/dirio/internal/mdns"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/middleware"
	"github.com/mallardduck/dirio/internal/path"
	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/teapot-router/pkg/teapot"
	"github.com/mallardduck/teapot-router/pkg/urlbuilder"
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
	DataConfig *dataconfig.DataConfig

	// CLICredentialsExplicitlySet tracks whether AccessKey/SecretKey were
	// explicitly provided (via env, flag, or config) vs using defaults
	CLICredentialsExplicitlySet bool
}

// Server represents the S3-compatible HTTP server
type Server struct {
	config   *Config
	router   *teapot.Router
	storage  *storage.Storage
	metadata *metadata.Manager
	auth     *auth.Authenticator
	mdns     *mdns.Service
	log      *slog.Logger
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

	if config.DataConfig != nil {
		// Data config exists - use smart credential selection
		if config.CLICredentialsExplicitlySet {
			// User explicitly configured CLI credentials - use dual admin mode
			log.Info("Configured dual admin access",
				"cli_admin", config.AccessKey,
				"data_admin", config.DataConfig.Credentials.AccessKey)
			authenticator = auth.New(metaMgr, config.AccessKey, config.SecretKey)
			authenticator = authenticator.WithAlternativeRoot(
				config.DataConfig.Credentials.AccessKey,
				config.DataConfig.Credentials.SecretKey,
			)
		} else {
			// CLI credentials not explicitly set - only use data config admin
			log.Info("Using data config admin credentials only (CLI credentials not explicitly set)",
				"data_admin", config.DataConfig.Credentials.AccessKey)
			authenticator = auth.New(metaMgr,
				config.DataConfig.Credentials.AccessKey,
				config.DataConfig.Credentials.SecretKey,
			)
		}
	} else {
		// No data config exists - use CLI credentials (needed for initial setup)
		if !config.CLICredentialsExplicitlySet {
			log.Warn("Using default credentials - change these in production!",
				"admin", config.AccessKey)
		}
		authenticator = auth.New(metaMgr, config.AccessKey, config.SecretKey)
	}

	// Create server
	srv := &Server{
		config:   config,
		storage:  store,
		metadata: metaMgr,
		auth:     authenticator,
		log:      log,
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
	s.router.Use(loggingHttp.PrepareAccessLogMiddleware(s.log))

	// Create URL builder
	urlBuilder := urlbuilder.New(s.config.CanonicalDomain)

	// Create API handler
	apiHandler := api.New(s.storage, s.metadata, s.auth, urlBuilder)

	// Setup routes using shared function
	deps := &RouteDependencies{
		Auth:       s.auth,
		APIHandler: apiHandler,
		Debug:      s.config.Debug,
	}

	SetupRoutes(s.router, deps)
}

// notImplemented is a placeholder handler for IAM routes that returns 501 Not Implemented
func (s *Server) notImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"This IAM operation is not yet implemented","code":"NotImplemented"}`))
}

// Start begins serving HTTP requests with graceful shutdown support.
// It listens for SIGINT and SIGTERM to trigger a graceful shutdown,
// properly stopping mDNS service and draining HTTP connections.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Port)

	// Create HTTP server with timeouts
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start mDNS service if enabled
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

	// Channel to receive server errors
	serverErr := make(chan error, 1)

	// Start HTTP server in a goroutine
	go func() {
		s.log.Info("server listening", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for either server error or shutdown signal
	select {
	case err := <-serverErr:
		s.shutdown()
		return err
	case sig := <-sigChan:
		s.log.Info("received signal, initiating graceful shutdown", "signal", sig)
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop accepting new connections and drain existing ones
	if err := httpServer.Shutdown(ctx); err != nil {
		s.log.Error("http server shutdown error", "error", err)
	}

	// Stop mDNS service
	s.shutdown()

	s.log.Info("server stopped gracefully")
	return nil
}

// shutdown performs cleanup operations during server shutdown
func (s *Server) shutdown() {
	if s.mdns != nil {
		if err := s.mdns.Stop(); err != nil {
			s.log.Error("mdns shutdown error", "error", err)
		}
		s.mdns = nil
	}
}
