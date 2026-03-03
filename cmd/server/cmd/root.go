package cmd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mallardduck/dirio/internal/config"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dirio",
	Short: "DirIO - A lightweight S3-compatible object storage server",
	Long: `DirIO is a filesystem-first S3-compatible object storage server.

It stores objects directly on the filesystem, making it easy to inspect,
backup, and migrate data. DirIO is designed for self-hosted environments
where simplicity and transparency are valued over distributed features.

Use "dirio serve" to start the server.`,
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
	rootCmd.PersistentFlags().StringP(config.DataDir.GetFlagKey(), "d", config.DataDir.GetDefaultAsString(), "Path to data directory")
	_ = viper.BindPFlag(config.DataDir.GetViperKey(), rootCmd.PersistentFlags().Lookup(config.DataDir.GetFlagKey()))
}

// initConfig wires up ENV variable support for all flags.
// Server configuration comes from CLI flags and DIRIO_* env vars only —
// there is no YAML config file for the server binary.
func initConfig() {
	viper.SetEnvPrefix("DIRIO")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()
}
