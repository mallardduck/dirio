package cmd

import (
	"fmt"
	"os"

	"github.com/mallardduck/dirio/internal/config"
	"github.com/mallardduck/dirio/internal/logging"
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

	// Server flags - using the option definitions for flag keys
	serveCmd.Flags().StringP(config.DataDir.GetFlagKey(), "d", config.DataDir.GetDefaultAsString(), "Path to data directory")
	serveCmd.Flags().IntP(config.Port.GetFlagKey(), "p", 9000, "Server port")
	serveCmd.Flags().String(config.AccessKey.GetFlagKey(), config.AccessKey.GetDefaultAsString(), "Root access key")
	serveCmd.Flags().String(config.SecretKey.GetFlagKey(), config.SecretKey.GetDefaultAsString(), "Root secret key")

	// Logging flags
	serveCmd.Flags().String(config.LogLevel.GetFlagKey(), config.LogLevel.GetDefaultAsString(), "Log level (debug, info, warn, error)")
	serveCmd.Flags().String(config.LogFormat.GetFlagKey(), config.LogFormat.GetDefaultAsString(), "Log format (text, json)")
	serveCmd.Flags().String(config.Verbosity.GetFlagKey(), config.Verbosity.GetDefaultAsString(), "Verbosity level (quiet, normal, verbose)")
	serveCmd.Flags().Bool(config.Debug.GetFlagKey(), false, "Enable debug mode (sets log-level to debug)")

	// mDNS flags
	serveCmd.Flags().Bool(config.MDNSEnabled.GetFlagKey(), false, "Enable mDNS service discovery")
	serveCmd.Flags().String(config.MDNSName.GetFlagKey(), config.MDNSName.GetDefaultAsString(), "mDNS service name (e.g., dirio-s3)")
	serveCmd.Flags().String(config.MDNSHostname.GetFlagKey(), config.MDNSHostname.GetDefaultAsString(), "mDNS hostname component (defaults to system hostname, advertised as <name>.<hostname>.local)")
	serveCmd.Flags().String(config.MDNSMode.GetFlagKey(), config.MDNSMode.GetDefaultAsString(), "controls mDNS responder mode detection")
	serveCmd.Flags().String(config.CanonicalDomain.GetFlagKey(), config.CanonicalDomain.GetDefaultAsString(), "Canonical domain for URL generation (e.g., s3.example.com)")

	// Bind flags to viper for config file support
	_ = viper.BindPFlag(config.DataDir.GetViperKey(), serveCmd.Flags().Lookup(config.DataDir.GetFlagKey()))
	_ = viper.BindPFlag(config.Port.GetViperKey(), serveCmd.Flags().Lookup(config.Port.GetFlagKey()))
	_ = viper.BindPFlag(config.AccessKey.GetViperKey(), serveCmd.Flags().Lookup(config.AccessKey.GetFlagKey()))
	_ = viper.BindPFlag(config.SecretKey.GetViperKey(), serveCmd.Flags().Lookup(config.SecretKey.GetFlagKey()))
	_ = viper.BindPFlag(config.LogLevel.GetViperKey(), serveCmd.Flags().Lookup(config.LogLevel.GetFlagKey()))
	_ = viper.BindPFlag(config.LogFormat.GetViperKey(), serveCmd.Flags().Lookup(config.LogFormat.GetFlagKey()))
	_ = viper.BindPFlag(config.Verbosity.GetViperKey(), serveCmd.Flags().Lookup(config.Verbosity.GetFlagKey()))
	_ = viper.BindPFlag(config.Debug.GetViperKey(), serveCmd.Flags().Lookup(config.Debug.GetFlagKey()))
	_ = viper.BindPFlag(config.MDNSEnabled.GetViperKey(), serveCmd.Flags().Lookup(config.MDNSEnabled.GetFlagKey()))
	_ = viper.BindPFlag(config.MDNSName.GetViperKey(), serveCmd.Flags().Lookup(config.MDNSName.GetFlagKey()))
	_ = viper.BindPFlag(config.MDNSHostname.GetViperKey(), serveCmd.Flags().Lookup(config.MDNSHostname.GetFlagKey()))
	_ = viper.BindPFlag(config.MDNSMode.GetViperKey(), serveCmd.Flags().Lookup(config.MDNSMode.GetFlagKey()))
	_ = viper.BindPFlag(config.CanonicalDomain.GetViperKey(), serveCmd.Flags().Lookup(config.CanonicalDomain.GetFlagKey()))
}

func runServer(cmd *cobra.Command, args []string) error {
	// Load configuration using the new config system
	settings, err := config.LoadConfig(cmd.Flags(), nil)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate configuration
	if err := settings.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Initialize logging
	logging.Setup(logging.Config{
		Level:     settings.LogLevel,
		Format:    settings.LogFormat,
		Verbosity: settings.Verbosity,
	})

	log := logging.Component("server")
	log.Info("logging service setup", "level", settings.LogLevel, "format", settings.LogFormat, "verbosity", settings.Verbosity)

	// Validate data directory exists or can be created
	if err := validateDataDir(settings.DataDir); err != nil {
		return fmt.Errorf("invalid data directory: %w", err)
	}

	// Create server configuration from settings
	serverConfig := &server.Config{
		DataDir:         settings.DataDir,
		Port:            settings.Port,
		AccessKey:       settings.AccessKey,
		SecretKey:       settings.SecretKey,
		MDNSEnabled:     settings.MDNSEnabled,
		MDNSName:        settings.MDNSName,
		MDNSHostname:    settings.MDNSHostname,
		MDNSMode:        settings.MDNSMode,
		CanonicalDomain: settings.CanonicalDomain,
	}

	// Initialize and start server
	srv, err := server.New(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	log.Info("starting server", "port", settings.Port, "data_dir", settings.DataDir)

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
