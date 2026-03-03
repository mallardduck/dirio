package cmd

import (
	"fmt"
	"os"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/cobra"

	intCliConfig "github.com/mallardduck/dirio/internal/cli/config"
	"github.com/mallardduck/dirio/internal/cli/output"
	"github.com/mallardduck/dirio/internal/config"
	"github.com/mallardduck/dirio/internal/config/data"
	"github.com/mallardduck/dirio/internal/crypto"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or update data directory configuration",
	Long: `View or update settings stored in .dirio/config.json.

Admin credentials are excluded — use "dirio credentials set" for those.
Changes take effect on the next server restart unless noted otherwise.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print all config values for the data directory",
	RunE:  func(cmd *cobra.Command, args []string) error { return runConfigGet(cmd, nil) },
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Print a config value, or all values if no key is given",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return runConfigGetAll(cmd)
	}
	key := args[0]
	f, ok := intCliConfig.ReadableFields[key]
	if !ok {
		return intCliConfig.UnknownReadableKeyError(key)
	}
	_, dc, _, err := openDataConfig(cmd)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(os.Stdout, f.Get(dc))
	return nil
}

func runConfigGetAll(cmd *cobra.Command) error {
	dataDir, dc, _, err := openDataConfig(cmd)
	if err != nil {
		return err
	}
	output.Header("Data config: " + dataDir)
	output.Blank()
	for _, k := range intCliConfig.SortedKeys(intCliConfig.ReadableFields) {
		output.Field(k, intCliConfig.ReadableFields[k].Get(dc))
	}
	output.Blank()
	output.Hint("Read-only fields: version, instance-id, credentials.access-key, created-at, updated-at")
	output.Hint(`credentials.secret-key is never displayed — use "dirio credentials set" to manage it.`)
	output.Hint("Most changes require a server restart to take effect.")
	output.Blank()
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]
	f, ok := intCliConfig.SettableFields[key]
	if !ok {
		return intCliConfig.UnknownSettableKeyError(key)
	}
	dataDir, dc, fs, err := openDataConfig(cmd)
	if err != nil {
		return err
	}
	if err := f.Set(dc, value); err != nil {
		return fmt.Errorf("invalid value for %s: %w", key, err)
	}
	if err := data.SaveDataConfig(fs, dc); err != nil {
		return fmt.Errorf("failed to save data config: %w", err)
	}
	output.Success(fmt.Sprintf("Updated %s", key))
	output.Field("Value", value)
	output.Field("Data dir", dataDir)
	output.Blank()
	return nil
}

// openDataConfig loads DataConfig from the data directory specified in the command flags.
// Returns the resolved dataDir, the loaded ConfigData, the filesystem, and any error.
func openDataConfig(cmd *cobra.Command) (string, *data.ConfigData, billy.Filesystem, error) {
	dataDir, _ := cmd.Flags().GetString(config.DataDir.GetFlagKey())

	if err := crypto.Init(dataDir); err != nil {
		return dataDir, nil, nil, fmt.Errorf("failed to initialise encryption: %w", err)
	}
	fs := osfs.New(dataDir)
	if !data.ConfigDataExists(fs) {
		return dataDir, nil, nil, fmt.Errorf(
			"no config.json found in %s — run \"dirio init\" first", dataDir)
	}
	dc, err := data.LoadDataConfig(fs)
	if err != nil {
		return dataDir, nil, nil, fmt.Errorf("failed to load data config: %w", err)
	}
	return dataDir, dc, fs, nil
}
