package cmd

import (
	"log"
	"os"
	"strings"

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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dirio/config.yaml)")
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
