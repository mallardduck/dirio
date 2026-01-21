package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mallardduck/dirio/internal/api"
	"github.com/mallardduck/dirio/internal/auth"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/storage"
)

// Config holds server configuration
type Config struct {
	DataDir   string
	Port      int
	AccessKey string
	SecretKey string
}

// Server represents the S3-compatible HTTP server
type Server struct {
	config   *Config
	router   *mux.Router
	storage  *storage.Storage
	metadata *metadata.Manager
	auth     *auth.Authenticator
}

// New creates a new server instance
func New(config *Config) (*Server, error) {
	// Initialize metadata manager
	metaMgr, err := metadata.New(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metadata: %w", err)
	}

	// Check for MinIO migration
	if err := metaMgr.CheckAndImportMinIO(); err != nil {
		log.Printf("Warning: MinIO import failed: %v", err)
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
	}

	// Setup routes
	srv.setupRoutes()

	return srv, nil
}

// setupRoutes configures HTTP routing
func (s *Server) setupRoutes() {
	s.router = mux.NewRouter()

	// Create API handler
	apiHandler := api.New(s.storage, s.metadata, s.auth)

	// Root - ListBuckets
	s.router.HandleFunc("/", apiHandler.ListBuckets).Methods("GET")

	// Bucket operations
	s.router.HandleFunc("/{bucket}", apiHandler.BucketHandler).Methods("GET", "PUT", "HEAD", "DELETE")

	// Object operations
	s.router.HandleFunc("/{bucket}/{key:.*}", apiHandler.ObjectHandler).Methods("GET", "PUT", "HEAD", "DELETE")

	// Add authentication middleware
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

// Start begins serving HTTP requests
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Port)
	log.Printf("Server listening on %s", addr)
	return http.ListenAndServe(addr, s.router)
}
