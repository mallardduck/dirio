package cmd

import (
	"fmt"
	"os"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/internal/cli/output"
	"github.com/mallardduck/dirio/internal/config"
	"github.com/mallardduck/dirio/internal/config/data"
	"github.com/mallardduck/dirio/internal/crypto"
	"github.com/mallardduck/dirio/internal/logging"
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

	// Validate: credentials must be provided together or not at all.
	if (accessKey == "") != (secretKey == "") {
		return fmt.Errorf("--access-key and --secret-key must both be provided or both be omitted")
	}

	// Ensure the data directory exists.
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("cannot create data directory: %w", err)
	}

	// Initialise encryption keyring (creates one if needed) before any config writes.
	if err := crypto.Init(dataDir); err != nil {
		return fmt.Errorf("failed to initialise encryption: %w", err)
	}

	fs := osfs.New(dataDir)

	if data.ConfigDataExists(fs) {
		// Config already exists — only apply credentials if provided and not yet set.
		existing, err := data.LoadDataConfig(fs)
		if err != nil {
			return fmt.Errorf("config.json exists but could not be loaded: %w", err)
		}
		if accessKey != "" {
			if existing.Credentials.IsConfigured() {
				log.Warn("admin credentials already configured, skipping",
					"data_dir", dataDir,
					"access_key", existing.Credentials.AccessKey,
				)
				output.Warn("Admin credentials already configured — use \"dirio credentials set\" to update them.")
			} else {
				existing.Credentials.AccessKey = accessKey
				existing.Credentials.SecretKey = secretKey
				if err := data.SaveDataConfig(fs, existing); err != nil {
					return fmt.Errorf("failed to save credentials: %w", err)
				}
				log.Info("admin credentials configured", "data_dir", dataDir, "access_key", accessKey)
				printCredentialsSet(dataDir, accessKey)
			}
		} else {
			log.Info("data directory already initialised", "data_dir", dataDir)
			output.Success("Data directory already initialised: " + dataDir)
		}
	} else {
		// First init — create config with defaults and optional credentials.
		dc := data.DefaultDataConfig()
		if accessKey != "" {
			dc.Credentials.AccessKey = accessKey
			dc.Credentials.SecretKey = secretKey
		}
		if err := data.SaveDataConfig(fs, dc); err != nil {
			return fmt.Errorf("failed to save data config: %w", err)
		}
		log.Info("data directory initialised", "data_dir", dataDir, "credentials_set", accessKey != "")
		output.Blank()
		output.Success("Data directory initialised: " + dataDir)
		if accessKey != "" {
			printCredentialsSet(dataDir, accessKey)
		} else {
			output.Hint("No admin credentials set.")
			output.Hint("Run \"dirio credentials set\" or re-run \"dirio init --access-key ... --secret-key ...\" to configure them.")
			output.Blank()
		}
	}

	return nil
}

func printCredentialsSet(dataDir, accessKey string) {
	output.Success("Admin credentials configured")
	output.Field("Data dir", dataDir)
	output.Field("Access key", accessKey)
	output.Field("Secret key", "encrypted in .dirio/config.json")
	output.Blank()
}
