package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/internal/cli/output"
	"github.com/mallardduck/dirio/internal/config"
	"github.com/mallardduck/dirio/internal/config/data"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/startup"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise a DirIO data directory",
	Long: `Initialise a DirIO data directory: creates the encryption keyring and
.dirio/config.json with default storage settings.

Optionally set admin credentials at the same time with --access-key and
--secret-key. If omitted, the data directory is initialised without credentials
and the server falls back to CLI/env credentials until you run
"dirio credentials set".

Safe to run against an existing data directory — only missing pieces are created.
Existing credentials are never overwritten; use "dirio credentials set" for that.

Examples:
  dirio init --data-dir /var/lib/dirio
  dirio init --data-dir /var/lib/dirio --access-key myadmin --secret-key mysecret`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().StringP(config.DataDir.GetFlagKey(), "d", config.DataDir.GetDefaultAsString(), "Path to data directory")
	initCmd.Flags().String(config.AccessKey.GetFlagKey(), "", "Admin access key (optional)")
	initCmd.Flags().String(config.SecretKey.GetFlagKey(), "", "Admin secret key (optional)")
}

func runInit(cmd *cobra.Command, _ []string) error {
	log := logging.Component("init")

	dataDir, _ := cmd.Flags().GetString(config.DataDir.GetFlagKey())
	accessKey, _ := cmd.Flags().GetString(config.AccessKey.GetFlagKey())
	secretKey, _ := cmd.Flags().GetString(config.SecretKey.GetFlagKey())

	// Credentials must be provided together or not at all.
	if (accessKey == "") != (secretKey == "") {
		return fmt.Errorf("--access-key and --secret-key must both be provided or both be omitted")
	}

	// Init: MkdirAll, crypto keyring, rootFS, DataConfig load-or-default.
	s, err := startup.Init(dataDir)
	if err != nil {
		return err
	}

	// MinIO migration: detect and import .minio.sys if present.  Must run
	// before credential handling so that DataConfig reflects the imported
	// InstanceID and settings rather than the fresh default.  The metadata
	// manager (if created) is closed when init exits.
	defer func() { _ = s.Close() }()
	if err := s.MigrateMinIO(cmd.Context()); err != nil {
		log.Warn("minio check/import failed", "error", err)
	}

	if !s.IsNew() {
		return handleExistingDir(s, dataDir, accessKey, secretKey)
	}
	return handleFirstInit(s, dataDir, accessKey, secretKey)
}

// handleExistingDir handles "dirio init" against a data directory that already exists.
// Credentials are applied only if provided and not already configured.
func handleExistingDir(s *startup.Starter, dataDir, accessKey, secretKey string) error {
	log := logging.Component("init")

	if accessKey == "" {
		log.Info("data directory already initialised", "data_dir", dataDir)
		output.Success("Data directory already initialised: " + dataDir)
		return nil
	}

	if s.DataConfig.Credentials.IsConfigured() {
		log.Warn("admin credentials already configured, skipping",
			"data_dir", dataDir,
			"access_key", s.DataConfig.Credentials.AccessKey,
		)
		output.Warn("Admin credentials already configured — use \"dirio credentials set\" to update them.")
		return nil
	}

	s.DataConfig.Credentials.AccessKey = accessKey
	s.DataConfig.Credentials.SecretKey = secretKey
	if err := data.SaveDataConfig(s.RootFS(), s.DataConfig); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}
	log.Info("admin credentials configured", "data_dir", dataDir, "access_key", accessKey)
	printCredentialsSet(dataDir, accessKey)
	return nil
}

// handleFirstInit handles the first-time "dirio init" for a fresh data directory.
func handleFirstInit(s *startup.Starter, dataDir, accessKey, secretKey string) error {
	log := logging.Component("init")

	if accessKey != "" {
		s.DataConfig.Credentials.AccessKey = accessKey
		s.DataConfig.Credentials.SecretKey = secretKey
	}
	if err := data.SaveDataConfig(s.RootFS(), s.DataConfig); err != nil {
		return fmt.Errorf("failed to save data config: %w", err)
	}
	log.Info("data directory initialised", "data_dir", dataDir, "credentials_set", accessKey != "")
	output.Blank()
	output.Success("Data directory initialised: " + dataDir)
	if accessKey != "" {
		printCredentialsSet(dataDir, accessKey)
		return nil
	}
	output.Hint("No admin credentials set.")
	output.Hint("Run \"dirio credentials set\" or re-run \"dirio init --access-key ... --secret-key ...\" to configure them.")
	output.Blank()
	return nil
}

func printCredentialsSet(dataDir, accessKey string) {
	output.Success("Admin credentials configured")
	output.Field("Data dir", dataDir)
	output.Field("Access key", accessKey)
	output.Field("Secret key", "encrypted in .dirio/config.json")
	output.Blank()
}
