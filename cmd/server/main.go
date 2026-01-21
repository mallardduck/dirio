package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/yourusername/dirio/internal/server"
)

func main() {
	// Command line flags
	dataDir := flag.String("data-dir", "/data", "Path to data directory")
	port := flag.Int("port", 9000, "Server port")
	accessKey := flag.String("access-key", "minioadmin", "Root access key")
	secretKey := flag.String("secret-key", "minioadmin", "Root secret key")
	flag.Parse()

	// Validate data directory
	if err := validateDataDir(*dataDir); err != nil {
		log.Fatalf("Invalid data directory: %v", err)
	}

	// Create server configuration
	config := &server.Config{
		DataDir:   *dataDir,
		Port:      *port,
		AccessKey: *accessKey,
		SecretKey: *secretKey,
	}

	// Initialize and start server
	srv, err := server.New(config)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	log.Printf("Starting DirIO server on port %d", *port)
	log.Printf("Data directory: %s", *dataDir)
	
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func validateDataDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		// Try to create it
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("cannot create data directory: %w", err)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a directory")
	}
	return nil
}
