package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mallardduck/dirio/internal/config"
	"github.com/mallardduck/dirio/internal/http/server"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/startup"
	"github.com/mallardduck/dirio/internal/telemetry"
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
	serveCmd.Flags().Bool(config.ConsoleDedicatedPort.GetFlagKey(), false, "Serve the admin console on its own port (dual-port mode); when false the console shares the main S3 port")
	serveCmd.Flags().Int(config.ConsolePort.GetFlagKey(), 9010, "Port for the admin console and control plane when --console-dedicated-port is enabled (default: 9010)")

	// Lifecycle flags
	serveCmd.Flags().Int(config.ShutdownTimeout.GetFlagKey(), 30, "Graceful shutdown timeout in seconds")

	// Telemetry / OTLP flags
	serveCmd.Flags().Bool(config.OTLPMetricsEnabled.GetFlagKey(), false, "Push metrics to an OTLP endpoint")
	serveCmd.Flags().String(config.OTLPMetricsEndpoint.GetFlagKey(), config.OTLPMetricsEndpoint.GetDefaultAsString(), "OTLP HTTP endpoint base URL (e.g. http://localhost:4318)")
	serveCmd.Flags().Int(config.OTLPMetricsInterval.GetFlagKey(), 30, "OTLP metrics push interval in seconds")

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
	_ = viper.BindPFlag(config.ConsoleDedicatedPort.GetViperKey(), serveCmd.Flags().Lookup(config.ConsoleDedicatedPort.GetFlagKey()))
	_ = viper.BindPFlag(config.ConsolePort.GetViperKey(), serveCmd.Flags().Lookup(config.ConsolePort.GetFlagKey()))
	_ = viper.BindPFlag(config.ShutdownTimeout.GetViperKey(), serveCmd.Flags().Lookup(config.ShutdownTimeout.GetFlagKey()))
	_ = viper.BindPFlag(config.OTLPMetricsEnabled.GetViperKey(), serveCmd.Flags().Lookup(config.OTLPMetricsEnabled.GetFlagKey()))
	_ = viper.BindPFlag(config.OTLPMetricsEndpoint.GetViperKey(), serveCmd.Flags().Lookup(config.OTLPMetricsEndpoint.GetFlagKey()))
	_ = viper.BindPFlag(config.OTLPMetricsInterval.GetViperKey(), serveCmd.Flags().Lookup(config.OTLPMetricsInterval.GetFlagKey()))
}

func runServer(cmd *cobra.Command, _ []string) error {
	// Read the data directory flag early — the Starter needs it before
	// config.LoadConfig runs so that the encryption keyring is ready for
	// credential decryption inside LoadConfig.
	dataDir, _ := cmd.Flags().GetString(config.DataDir.GetFlagKey())
	if dataDir == "" {
		dataDir = config.DataDir.GetDefaultAsString()
	}

	// Phase 1 — Starter init: MkdirAll, crypto, rootFS, DataConfig load/default.
	starter, err := startup.Init(dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialise data directory: %w", err)
	}

	// Load full application config (viper/cobra).  Crypto is now initialised
	// so any encrypted credential values in config.json can be decrypted.
	settings, err := config.LoadConfig(cmd.Flags(), viper.GetViper())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := settings.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	logging.Setup(logging.Config{
		Level:     settings.LogLevel,
		Format:    settings.LogFormat,
		Verbosity: settings.Verbosity,
	})

	log := logging.Component("server")
	log.Info("logging service setup",
		"level", settings.LogLevel,
		"format", settings.LogFormat,
		"verbosity", settings.Verbosity,
	)

	// Phase 2 — Starter prepare: metadata manager, MinIO import, DataConfig
	// finalisation, global InstanceID.  After this returns DataConfig is
	// authoritative and MetadataManager is ready to hand to server.New.
	if err := starter.Prepare(
		cmd.Context(),
		settings.Region,
		settings.AccessKey,
		settings.SecretKey,
		settings.CLICredentialsExplicitlySet,
	); err != nil {
		return fmt.Errorf("failed to prepare data directory: %w", err)
	}

	// Phase 3 — initialise telemetry (Prometheus pull + optional OTLP push).
	telCfg := telemetry.Config{
		OTLPEnabled:  settings.OTLPMetricsEnabled,
		OTLPEndpoint: settings.OTLPMetricsEndpoint,
		OTLPInterval: settings.OTLPMetricsInterval,
	}
	telProvider, err := telemetry.Setup(cmd.Context(), telCfg)
	if err != nil {
		return fmt.Errorf("failed to initialise telemetry: %w", err)
	}

	// Phase 4 — wire up the HTTP server using the Starter's finalised components.
	serverConfig := &server.Config{
		DataDir:                     settings.DataDir,
		Port:                        settings.Port,
		AccessKey:                   settings.AccessKey,
		SecretKey:                   settings.SecretKey,
		MDNSEnabled:                 settings.MDNSEnabled,
		MDNSName:                    settings.MDNSName,
		MDNSHostname:                settings.MDNSHostname,
		MDNSMode:                    settings.MDNSMode,
		CanonicalDomain:             settings.CanonicalDomain,
		Debug:                       settings.Debug,
		DataConfig:                  starter.DataConfig,
		CLICredentialsExplicitlySet: settings.CLICredentialsExplicitlySet,
		ShutdownTimeout:             settings.ShutdownTimeout,
		RootFS:                      starter.RootFS(),
		Metadata:                    starter.MetadataManager(),
		Telemetry:                   telProvider,
	}

	srv, err := server.New(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	setupConsole(srv, settings.ConsoleEnabled, settings.ConsoleDedicatedPort, settings.ConsolePort)

	log.Info("starting server", "port", settings.Port, "data_dir", settings.DataDir)

	if err := srv.Start(cmd.Context()); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	// Flush and close telemetry exporters after the server stops.
	if err := telProvider.Shutdown(cmd.Context()); err != nil {
		log.Warn("telemetry shutdown error", "error", err)
	}

	return nil
}
