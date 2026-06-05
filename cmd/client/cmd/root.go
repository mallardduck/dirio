package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "dio",
	Short: "dio — DirIO CLI client",
	Long: `dio is the first-party CLI client for DirIO servers.

It provides profile-based access to one or more DirIO instances, with
intuitive output for interactive use and machine-readable JSON for scripts.

Examples:
  dio config init               # create or update ~/.dirio/client.yaml
  dio ls                        # list buckets on the default profile
  dio ls mybucket               # list objects in a bucket
  dio ls prod/mybucket/prefix/  # list with a named profile and prefix
  dio sa list                   # list service accounts
  dio iam user list             # list IAM users
  dio iam policy list           # list named IAM policies`,
}

// Execute runs the root command. Called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// flagProfile is the global --profile flag value.
var flagProfile string

// flagOutput is the global --output flag value (tui|plain|json).
var flagOutput string

// flagEndpoint, flagAccessKey, flagSecretKey, flagRegion are inline overrides.
var (
	flagEndpoint  string
	flagAccessKey string
	flagSecretKey string
	flagRegion    string
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&flagProfile, "profile", "p", "", "profile to use (default: default_profile in ~/.dirio/client.yaml)")
	rootCmd.PersistentFlags().StringVarP(&flagOutput, "output", "o", "", "output format: tui (default), plain, json")
	rootCmd.PersistentFlags().StringVar(&flagEndpoint, "endpoint", "", "override endpoint URL")
	rootCmd.PersistentFlags().StringVar(&flagAccessKey, "access-key", "", "override access key")
	rootCmd.PersistentFlags().StringVar(&flagSecretKey, "secret-key", "", "override secret key")
	rootCmd.PersistentFlags().StringVar(&flagRegion, "region", "", "override region")

	_ = viper.BindPFlag("profile", rootCmd.PersistentFlags().Lookup("profile"))
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("endpoint", rootCmd.PersistentFlags().Lookup("endpoint"))
	_ = viper.BindPFlag("access_key", rootCmd.PersistentFlags().Lookup("access-key"))
	_ = viper.BindPFlag("secret_key", rootCmd.PersistentFlags().Lookup("secret-key"))
	_ = viper.BindPFlag("region", rootCmd.PersistentFlags().Lookup("region"))
}

func initConfig() {
	viper.SetEnvPrefix("DIO")
	viper.AutomaticEnv()
}
