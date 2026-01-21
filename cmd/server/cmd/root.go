package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mallardduck/dirio/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dirio",
	Short: "DirIO - A lightweight S3-compatible object storage server",
	Long: `DirIO is a filesystem-first S3-compatible object storage server.

It stores objects directly on the filesystem, making it easy to inspect,
backup, and migrate data. DirIO is designed for self-hosted environments
where simplicity and transparency are valued over distributed features.`,
	RunE: runServer,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Persistent flags available to all commands
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dirio/config.yaml)")

	// Server flags
	rootCmd.Flags().StringP("data-dir", "d", "/data", "Path to data directory")
	rootCmd.Flags().IntP("port", "p", 9000, "Server port")
	rootCmd.Flags().String("access-key", "dirio-admin", "Root access key")
	rootCmd.Flags().String("secret-key", "dirio-admin-secret", "Root secret key")

	// Bind flags to viper
	viper.BindPFlag("data_dir", rootCmd.Flags().Lookup("data-dir"))
	viper.BindPFlag("port", rootCmd.Flags().Lookup("port"))
	viper.BindPFlag("access_key", rootCmd.Flags().Lookup("access-key"))
	viper.BindPFlag("secret_key", rootCmd.Flags().Lookup("secret-key"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("Warning: could not find home directory: %v", err)
		} else {
			// Search for config in home directory
			viper.AddConfigPath(home + "/.dirio")
		}

		// Also search in /etc/dirio
		viper.AddConfigPath("/etc/dirio")

		// Search in current directory
		viper.AddConfigPath(".")

		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// Environment variables
	viper.SetEnvPrefix("DIRIO")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	// Read config file if it exists
	if err := viper.ReadInConfig(); err == nil {
		log.Printf("Using config file: %s", viper.ConfigFileUsed())
	}
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
