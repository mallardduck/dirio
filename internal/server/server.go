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

	"github.com/gorilla/mux"
	"github.com/mallardduck/dirio/internal/api"
	"github.com/mallardduck/dirio/internal/auth"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/mdns"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/dirio/internal/urlbuilder"
)

// Config holds server configuration
type Config struct {
	DataDir   string
	Port      int
	AccessKey string
	SecretKey string

	// mDNS settings
	MDNSEnabled  bool
	MDNSName     string
	MDNSHostname string
	MDNSMode     string

	// URL generation
	CanonicalDomain string
}

// Server represents the S3-compatible HTTP server
type Server struct {
	config   *Config
	router   *mux.Router
	storage  *storage.Storage
	metadata *metadata.Manager
	auth     *auth.Authenticator
	mdns     *mdns.Service
	log      *slog.Logger
}

// New creates a new server instance
func New(config *Config) (*Server, error) {
	log := logging.Component("server")

	// Initialize metadata manager
	metaMgr, err := metadata.New(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metadata: %w", err)
	}

	// Check for MinIO migration
	if err := metaMgr.CheckAndImportMinIO(); err != nil {
		log.Warn("minio import failed", "error", err)
	}

	// Initialize storage backend
	store, err := storage.New(config.DataDir, metaMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize authenticator
	authenticator := auth.New(metaMgr, config.AccessKey, config.SecretKey)

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
	s.router = mux.NewRouter()

	// Create URL builder
	urlBuilder := urlbuilder.New(s.config.CanonicalDomain)

	// Create API handler
	apiHandler := api.New(s.storage, s.metadata, s.auth, urlBuilder)

	// Root - ListBuckets
	s.router.HandleFunc("/", apiHandler.ListBuckets).Methods("GET")

	// Bucket operations
	s.router.HandleFunc("/{bucket}", apiHandler.BucketHandler).Methods("GET", "PUT", "HEAD", "DELETE")

	// Object operations
	s.router.HandleFunc("/{bucket}/{key:.*}", apiHandler.ObjectHandler).Methods("GET", "PUT", "HEAD", "DELETE")

	// Add middleware
	s.router.Use(s.loggingMiddleware)
	s.router.Use(s.authMiddleware)
}

// authMiddleware validates authentication for all requests
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement AWS Signature V4 authentication
		// For now, just pass through - we'll add this in phase 2
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs incoming HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		s.log.Info("request handled",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"remote", r.RemoteAddr,
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
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
