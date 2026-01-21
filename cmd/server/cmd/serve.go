package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/mallardduck/dirio/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the DirIO S3-compatible server",
	Long: `Start the DirIO server which provides an S3-compatible API.

The server stores objects directly on the filesystem, making it easy to inspect,
backup, and migrate data.

Examples:
  dirio serve
  dirio serve --port 9000 --data-dir /var/lib/dirio
  dirio serve -p 8080 -d ./data`,
	RunE: runServer,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Server flags
	serveCmd.Flags().StringP("data-dir", "d", "/data", "Path to data directory")
	serveCmd.Flags().IntP("port", "p", 9000, "Server port")
	serveCmd.Flags().String("access-key", "dirio-admin", "Root access key")
	serveCmd.Flags().String("secret-key", "dirio-admin-secret", "Root secret key")

	// Bind flags to viper
	viper.BindPFlag("data_dir", serveCmd.Flags().Lookup("data-dir"))
	viper.BindPFlag("port", serveCmd.Flags().Lookup("port"))
	viper.BindPFlag("access_key", serveCmd.Flags().Lookup("access-key"))
	viper.BindPFlag("secret_key", serveCmd.Flags().Lookup("secret-key"))
}

func runServer(cmd *cobra.Command, args []string) error {
	dataDir := viper.GetString("data_dir")
	port := viper.GetInt("port")
	accessKey := viper.GetString("access_key")
	secretKey := viper.GetString("secret_key")

	// Validate data directory
	if err := validateDataDir(dataDir); err != nil {
		return fmt.Errorf("invalid data directory: %w", err)
	}

	// Create server configuration
	config := &server.Config{
		DataDir:   dataDir,
		Port:      port,
		AccessKey: accessKey,
		SecretKey: secretKey,
	}

	// Initialize and start server
	srv, err := server.New(config)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	log.Printf("Starting DirIO server on port %d", port)
	log.Printf("Data directory: %s", dataDir)

	if err := srv.Start(); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
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