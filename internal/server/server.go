package server

import (
	"context"
	"fmt"
	"io"
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
	"github.com/mallardduck/dirio/internal/router"
	"github.com/mallardduck/dirio/internal/server/favicon"
	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/dirio/internal/urlbuilder"
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

	// Data directory configuration (optional)
	// If present, provides alternative admin credentials from data config
	DataConfig *dataconfig.DataConfig
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
	if err := metaMgr.CheckAndImportMinIO(context.Background()); err != nil {
		log.Warn("minio data check & import failed", "error", err)
	}

	// Initialize storage backend
	store, err := storage.New(rootFS, metaMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize authenticator with CLI admin credentials
	authenticator := auth.New(metaMgr, config.AccessKey, config.SecretKey)

	// Add data config admin credentials if they exist
	if config.DataConfig != nil {
		authenticator = authenticator.WithAlternativeRoot(
			config.DataConfig.Credentials.AccessKey,
			config.DataConfig.Credentials.SecretKey,
		)
		log.Info("Configured dual admin access",
			"cli_admin", config.AccessKey,
			"data_admin", config.DataConfig.Credentials.AccessKey)
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
	s.router = router.New()

	// Add middleware (timing first for accurate timestamps, then trace ID, request ID, logging, auth)
	s.router.Use(chiMiddleware.StripSlashes)
	s.router.Use(middleware.Timing)
	s.router.Use(middleware.TraceID)
	s.router.Use(middleware.RequestID)
	s.router.Use(loggingHttp.PrepareAccessLogMiddleware(s.log))

	// Public Routes (no auth required)
	s.router.MiddlewareGroup(func(r *router.Router) {
		// Add any public routes here without auth middleware
		// For example: health checks, public assets, etc.
		r.Get("/favicon.ico", favicon.HandleFavicon, "favicon")
	})

	// Create URL builder
	urlBuilder := urlbuilder.New(s.config.CanonicalDomain)

	// Create API handler
	apiHandler := api.New(s.storage, s.metadata, s.auth, urlBuilder)

	// Base Routes
	s.router.MiddlewareGroup(func(r *router.Router) {
		// Authentication middleware - verifies AWS SigV4 signatures
		r.Use(s.auth.AuthMiddleware)

		// Chunked encoding middleware - decodes AWS SigV4 chunked transfer encoding
		// Must run AFTER auth middleware, BEFORE handlers
		r.Use(middleware.ChunkedEncoding(func(r io.Reader) io.Reader {
			return auth.NewChunkedReader(r)
		}))

		// Root - ListBuckets
		r.Get("/", apiHandler.S3Handler.ListBuckets, "index")

		// Bucket operations
		bucketHandler := apiHandler.BucketResourceHandler()
		r.Head("/{bucket}", bucketHandler.HeadHandler, "buckets.head")
		r.Put("/{bucket}", bucketHandler.StoreHandler, "buckets.store")
		r.Get("/{bucket}", bucketHandler.ShowHandler, "buckets.show")
		r.Delete("/{bucket}", bucketHandler.DestroyHandler, "buckets.destroy")

		// Object operations (use /* for catch-all to match keys with slashes)
		objectHandler := apiHandler.ObjectResourceHandler()
		r.Head("/{bucket}/*", objectHandler.HeadHandler, "objects.head")
		r.Put("/{bucket}/*", objectHandler.StoreHandler, "objects.create")
		r.Get("/{bucket}/*", objectHandler.ShowHandler, "objects.show")
		r.Delete("/{bucket}/*", objectHandler.DestroyHandler, "objects.destroy")
	})

	// IAM API Routes (RESTful style - Phase 5)
	s.router.NameGroup("iam.", "/api/iam", func(r *router.Router) {
		// Authentication required for IAM operations
		r.Use(s.auth.AuthMiddleware)
		// TODO maybe even an Admin level auth middleware? needed here

		// User Management
		r.Get("/users", s.notImplemented, "users.list")                 // ListUsers
		r.Post("/users", s.notImplemented, "users.create")              // CreateUser(s3)/StoreUser
		r.Get("/users/{username}", s.notImplemented, "users.get")       // GetUser
		r.Put("/users/{username}", s.notImplemented, "users.update")    // UpdateUser
		r.Delete("/users/{username}", s.notImplemented, "users.delete") // DeleteUser
		// TODO: replace above with s.Resource based configs

		// Group Management
		r.Post("/groups", s.notImplemented, "groups.create")                                    // CreateGroup
		r.Get("/groups", s.notImplemented, "groups.list")                                       // ListGroups
		r.Get("/groups/{groupname}", s.notImplemented, "groups.get")                            // GetGroup
		r.Delete("/groups/{groupname}", s.notImplemented, "groups.delete")                      // DeleteGroup
		r.Post("/groups/{groupname}/users/{username}", s.notImplemented, "groups.adduser")      // AddUserToGroup
		r.Delete("/groups/{groupname}/users/{username}", s.notImplemented, "groups.removeuser") // RemoveUserFromGroup

		// Role Management
		r.Post("/roles", s.notImplemented, "roles.create")              // CreateRole
		r.Get("/roles", s.notImplemented, "roles.list")                 // ListRoles
		r.Get("/roles/{rolename}", s.notImplemented, "roles.get")       // GetRole
		r.Delete("/roles/{rolename}", s.notImplemented, "roles.delete") // DeleteRole

		// Policy Management
		r.Post("/policies", s.notImplemented, "policies.create")               // CreatePolicy
		r.Get("/policies", s.notImplemented, "policies.list")                  // ListPolicies
		r.Get("/policies/{policyarn}", s.notImplemented, "policies.get")       // GetPolicy
		r.Delete("/policies/{policyarn}", s.notImplemented, "policies.delete") // DeletePolicy

		// Policy Attachments - Users
		r.Post("/users/{username}/policies/{policyarn}", s.notImplemented, "users.attachpolicy")           // AttachUserPolicy
		r.Delete("/users/{username}/policies/{policyarn}", s.notImplemented, "users.detachpolicy")         // DetachUserPolicy
		r.Put("/users/{username}/policies/inline/{policyname}", s.notImplemented, "users.putpolicy")       // PutUserPolicy
		r.Delete("/users/{username}/policies/inline/{policyname}", s.notImplemented, "users.deletepolicy") // DeleteUserPolicy

		// Policy Attachments - Groups
		r.Post("/groups/{groupname}/policies/{policyarn}", s.notImplemented, "groups.attachpolicy")           // AttachGroupPolicy
		r.Delete("/groups/{groupname}/policies/{policyarn}", s.notImplemented, "groups.detachpolicy")         // DetachGroupPolicy
		r.Put("/groups/{groupname}/policies/inline/{policyname}", s.notImplemented, "groups.putpolicy")       // PutGroupPolicy
		r.Delete("/groups/{groupname}/policies/inline/{policyname}", s.notImplemented, "groups.deletepolicy") // DeleteGroupPolicy

		// Policy Attachments - Roles
		r.Post("/roles/{rolename}/policies/{policyarn}", s.notImplemented, "roles.attachpolicy")           // AttachRolePolicy
		r.Delete("/roles/{rolename}/policies/{policyarn}", s.notImplemented, "roles.detachpolicy")         // DetachRolePolicy
		r.Put("/roles/{rolename}/policies/inline/{policyname}", s.notImplemented, "roles.putpolicy")       // PutRolePolicy
		r.Delete("/roles/{rolename}/policies/inline/{policyname}", s.notImplemented, "roles.deletepolicy") // DeleteRolePolicy

		// Access Key Management
		r.Post("/users/{username}/access-keys", s.notImplemented, "accesskeys.create")                 // CreateAccessKey
		r.Get("/users/{username}/access-keys", s.notImplemented, "accesskeys.list")                    // ListAccessKeys
		r.Put("/users/{username}/access-keys/{accesskeyid}", s.notImplemented, "accesskeys.update")    // UpdateAccessKey
		r.Delete("/users/{username}/access-keys/{accesskeyid}", s.notImplemented, "accesskeys.delete") // DeleteAccessKey

		// Account & Authorization
		r.Get("/account/authorization-details", s.notImplemented, "account.authdetails") // GetAccountAuthorizationDetails
		r.Post("/simulate-policy", s.notImplemented, "simulate.policy")                  // SimulatePrincipalPolicy
	})
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
