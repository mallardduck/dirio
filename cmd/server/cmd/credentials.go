package cmd

import (
	"fmt"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/internal/cli/output"
	"github.com/mallardduck/dirio/internal/config"
	"github.com/mallardduck/dirio/internal/config/data"
	"github.com/mallardduck/dirio/internal/crypto"
	"github.com/mallardduck/dirio/internal/logging"
)

var credentialsCmd = &cobra.Command{
	Use:   "credentials",
	Short: "Manage data directory admin credentials",
}

var credentialsSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set or update the admin credentials in .dirio/config.json",
	Long: `Set or update the admin access key and/or secret key stored in the
data directory's .dirio/config.json. Works whether credentials have been
configured before or not — use this after the server has auto-initialised the
data directory and you want to lock down the admin account.

The new secret key is encrypted using the current keyring. At least one of
--access-key or --secret-key must be provided; omitted fields are left
unchanged if credentials already exist, or produce an error if setting for the
first time (both are needed to form a valid pair).

Example:
  dirio credentials set --data-dir /var/lib/dirio --access-key admin --secret-key mysecret
  dirio credentials set --data-dir /var/lib/dirio --secret-key mynewsecret`,
	RunE: runCredentialsSet,
}

func init() {
	rootCmd.AddCommand(credentialsCmd)
	credentialsCmd.AddCommand(credentialsSetCmd)

	credentialsSetCmd.Flags().String(config.AccessKey.GetFlagKey(), "", "New admin access key (optional)")
	credentialsSetCmd.Flags().String(config.SecretKey.GetFlagKey(), "", "New admin secret key (optional)")
}

func runCredentialsSet(cmd *cobra.Command, _ []string) error {
	dataDir, _ := cmd.Flags().GetString(config.DataDir.GetFlagKey())
	accessKey, _ := cmd.Flags().GetString(config.AccessKey.GetFlagKey())
	secretKey, _ := cmd.Flags().GetString(config.SecretKey.GetFlagKey())

	if accessKey == "" && secretKey == "" {
		return fmt.Errorf("at least one of --access-key or --secret-key must be provided")
	}

	// Initialize encryption before loading/saving config.
	if err := crypto.Init(dataDir); err != nil {
		return fmt.Errorf("failed to initialise encryption: %w", err)
	}

	fs := osfs.New(dataDir)

	if !data.ConfigDataExists(fs) {
		return fmt.Errorf(
			"no config.json found in %s — run \"dirio init\" first", dataDir)
	}

	dc, err := data.LoadDataConfig(fs)
	if err != nil {
		return fmt.Errorf("failed to load data config: %w", err)
	}

	// Apply the provided fields, merging with any existing values.
	if accessKey != "" {
		dc.Credentials.AccessKey = accessKey
	}
	if secretKey != "" {
		dc.Credentials.SecretKey = secretKey
	}

	// After merging, the pair must be complete — catch the case where the user
	// only provides one field and no prior value exists for the other.
	if !dc.Credentials.IsConfigured() {
		return fmt.Errorf("both --access-key and --secret-key are required when setting credentials for the first time")
	}

	if err := data.SaveDataConfig(fs, dc); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	log := logging.Component("credentials")
	log.Info("admin credentials updated", "data_dir", dataDir, "access_key", dc.Credentials.AccessKey)

	output.Success("Admin credentials updated")
	output.Field("Data dir", dataDir)
	output.Field("Access key", dc.Credentials.AccessKey)
	output.Field("Secret key", "encrypted in .dirio/config.json")
	output.Blank()

	return nil
}
