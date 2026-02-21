package cmd

import (
	"fmt"
	"os"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mallardduck/dirio/internal/crypto"
	"github.com/mallardduck/dirio/internal/http/server"

	"github.com/mallardduck/dirio/internal/config"
	"github.com/mallardduck/dirio/internal/config/data"
	"github.com/mallardduck/dirio/internal/logging"
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
	serveCmd.Flags().String(config.Region.GetFlagKey(), config.Region.GetDefaultAsString(), "AWS-style region (ignored if data config exists)")
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

	// Console flags
	serveCmd.Flags().Bool(config.ConsoleEnabled.GetFlagKey(), true, "Enable the embedded web admin console (default: true; use --console=false to disable)")
	serveCmd.Flags().Int(config.ConsolePort.GetFlagKey(), 0, "Optional separate port for the console (e.g. 9001); defaults to main port at /dirio/ui/")

	// Lifecycle flags
	serveCmd.Flags().Int(config.ShutdownTimeout.GetFlagKey(), 30, "Graceful shutdown timeout in seconds")

	// Bind flags to viper for config file support
	_ = viper.BindPFlag(config.DataDir.GetViperKey(), serveCmd.Flags().Lookup(config.DataDir.GetFlagKey()))
	_ = viper.BindPFlag(config.Port.GetViperKey(), serveCmd.Flags().Lookup(config.Port.GetFlagKey()))
	_ = viper.BindPFlag(config.Region.GetViperKey(), serveCmd.Flags().Lookup(config.Region.GetFlagKey()))
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
	_ = viper.BindPFlag(config.ConsoleEnabled.GetViperKey(), serveCmd.Flags().Lookup(config.ConsoleEnabled.GetFlagKey()))
	_ = viper.BindPFlag(config.ConsolePort.GetViperKey(), serveCmd.Flags().Lookup(config.ConsolePort.GetFlagKey()))
	_ = viper.BindPFlag(config.ShutdownTimeout.GetViperKey(), serveCmd.Flags().Lookup(config.ShutdownTimeout.GetFlagKey()))
}

func runServer(cmd *cobra.Command, args []string) error {
	// Initialize credential encryption before any data config or metadata
	// operations. Read the data dir flag early — cobra has already parsed flags
	// before RunE fires, so this is safe even before full config loading.
	dataDir, _ := cmd.Flags().GetString(config.DataDir.GetFlagKey())
	if dataDir == "" {
		dataDir = config.DataDir.GetDefaultAsString()
	}
	if err := crypto.Init(dataDir); err != nil {
		return fmt.Errorf("failed to initialise encryption: %w", err)
	}

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

	// Initialize or migrate: create data config if missing
	if err := initOrMigrateDataConfig(settings); err != nil {
		return fmt.Errorf("failed to initialize data config: %w", err)
	}

	// Create server configuration from settings
	serverConfig := &server.Config{
		DataDir:                     settings.DataDir,
		Port:                        settings.Port,
		AccessKey:                   settings.AccessKey, // CLI admin
		SecretKey:                   settings.SecretKey, // CLI admin
		MDNSEnabled:                 settings.MDNSEnabled,
		MDNSName:                    settings.MDNSName,
		MDNSHostname:                settings.MDNSHostname,
		MDNSMode:                    settings.MDNSMode,
		CanonicalDomain:             settings.CanonicalDomain,
		Debug:                       settings.Debug,
		DataConfig:                  settings.DataConfig, // Data admin (if exists)
		CLICredentialsExplicitlySet: settings.CLICredentialsExplicitlySet,
		ShutdownTimeout:             settings.ShutdownTimeout,
	}

	// Initialize and start server
	srv, err := server.New(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Wire the web admin console (no-op when built with -tags noconsole)
	setupConsole(srv, settings.ConsoleEnabled, settings.ConsolePort)

	log.Info("starting server", "port", settings.Port, "data_dir", settings.DataDir)

	ctx := cmd.Context()
	if err := srv.Start(ctx); err != nil {
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

// initOrMigrateDataConfig creates data config for new or existing installations
// that don't have .dirio/config.json yet
func initOrMigrateDataConfig(settings *config.Settings) error {
	log := logging.Component("init")

	// If data config already exists (either loaded or from MinIO import), skip
	if settings.DataConfig != nil {
		return nil
	}

	// Create filesystem for data directory
	fs := osfs.New(settings.DataDir)

	// Check if .dirio/config.json already exists
	if data.ConfigDataExists(fs) {
		// Should have been loaded, but wasn't - this might indicate a problem
		log.Warn("Data config file exists but wasn't loaded - skipping initialization")
		return nil
	}

	// Check if this looks like an existing installation (has .metadata directory)
	metadataExists := false
	if _, err := fs.Stat(".metadata"); err == nil {
		metadataExists = true
	}

	if metadataExists {
		log.Info("Migrating existing installation - creating data config from CLI settings")
	} else {
		log.Info("Initializing new DirIO data directory")
	}

	// Create data config with region and storage defaults.
	dc := data.DefaultDataConfig()
	dc.Region = settings.Region

	// Only persist credentials if they were explicitly provided — never bake
	// defaults into config.json. Use "dirio init" to configure admin credentials.
	if settings.CLICredentialsExplicitlySet {
		dc.Credentials.AccessKey = settings.AccessKey
		dc.Credentials.SecretKey = settings.SecretKey
		log.Info("persisting explicit CLI credentials to data config", "access_key", settings.AccessKey)
	} else {
		log.Info("no explicit credentials provided — data config created without credentials",
			"hint", "run \"dirio init --access-key ... --secret-key ...\" or \"dirio credentials set\" to configure admin credentials",
		)
	}

	if err := data.SaveDataConfig(fs, dc); err != nil {
		return fmt.Errorf("failed to save data config: %w", err)
	}

	// Update settings to use the new data config
	settings.DataConfig = dc

	if metadataExists {
		log.Info("Migration complete - data config created",
			"admin", dc.Credentials.AccessKey,
			"region", dc.Region)
	} else {
		log.Info("Initialization complete - core config files created",
			"admin", dc.Credentials.AccessKey,
			"region", dc.Region,
			"compression", dc.Compression.Enabled,
			"worm", dc.WORMEnabled)
	}

	return nil
}
