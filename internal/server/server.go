package server

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mallardduck/dirio/internal/api"
	"github.com/mallardduck/dirio/internal/auth"
	"github.com/mallardduck/dirio/internal/logging"
	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
	"github.com/mallardduck/dirio/internal/mdns"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/middleware"
	"github.com/mallardduck/dirio/internal/path"
	"github.com/mallardduck/dirio/internal/router"
	"github.com/mallardduck/dirio/internal/sigv4"
	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/dirio/internal/urlbuilder"
	"github.com/mallardduck/dirio/pkg/s3types"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
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
	router   *router.Router
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
	if err := metaMgr.CheckAndImportMinIO(); err != nil {
		log.Warn("minio data check & import failed", "error", err)
	}

	// Initialize storage backend
	store, err := storage.New(rootFS, metaMgr)
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
	s.router = router.New()

	// Add middleware (timing first for accurate timestamps, then trace ID, request ID, logging, auth)
	s.router.Use(chiMiddleware.StripSlashes)
	s.router.Use(middleware.Timing)
	s.router.Use(middleware.TraceID)
	s.router.Use(middleware.RequestID)
	s.router.Use(loggingHttp.PrepareAccessLogMiddleware(s.log))
	s.router.Use(s.authMiddleware)

	// Create URL builder
	urlBuilder := urlbuilder.New(s.config.CanonicalDomain)

	// Create API handler
	apiHandler := api.New(s.storage, s.metadata, s.auth, urlBuilder)

	// Root - ListBuckets
	s.router.Get("/", apiHandler.ListBuckets, "index")

	// Bucket operations
	bucketHandler := apiHandler.BucketResourceHandler()
	s.router.Head("/{bucket}", bucketHandler.HeadHandler, "buckets.head")
	s.router.Put("/{bucket}", bucketHandler.StoreHandler, "buckets.store")
	s.router.Get("/{bucket}", bucketHandler.ShowHandler, "buckets.show")
	s.router.Delete("/{bucket}", bucketHandler.DestroyHandler, "buckets.destroy")

	// Object operations (use /* for catch-all to match keys with slashes)
	objectHandler := apiHandler.ObjectResourceHandler()
	s.router.Head("/{bucket}/*", objectHandler.HeadHandler, "objects.head")
	s.router.Put("/{bucket}/*", objectHandler.StoreHandler, "objects.create")
	s.router.Get("/{bucket}/*", objectHandler.ShowHandler, "objects.show")
	s.router.Delete("/{bucket}/*", objectHandler.DestroyHandler, "objects.destroy")
}

// authMiddleware validates authentication for all requests
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication for health check endpoints (if we add them in the future)
		// For now, require auth for all requests

		requestID := middleware.GetRequestID(r.Context())

		// Extract access key from Authorization header
		accessKey, err := sigv4.GetAccessKey(r)
		if err != nil {
			// Missing or invalid Authorization header
			s.writeAuthError(w, requestID, s3types.ErrAccessDenied)
			return
		}

		// Look up user and get secret key
		user, err := s.auth.GetUserForAccessKey(accessKey)
		if err != nil || user == nil {
			s.writeAuthError(w, requestID, s3types.ErrInvalidAccessKeyID)
			return
		}

		// Check if user account is active
		if user.Status != "on" {
			s.writeAuthError(w, requestID, s3types.ErrAccessDenied)
			return
		}

		// Verify AWS Signature V4
		if err := sigv4.VerifySignature(r, user.SecretKey); err != nil {
			s.writeAuthError(w, requestID, s3types.ErrSignatureDoesNotMatch)
			return
		}

		// Authentication successful - proceed to next handler
		next.ServeHTTP(w, r)
	})
}

// writeAuthError writes an S3 error response
func (s *Server) writeAuthError(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(errCode.HTTPStatus())

	response := s3types.ErrorResponse{
		Code:      errCode.String(),
		Message:   errCode.Description(),
		RequestID: requestID,
	}

	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(response)
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
