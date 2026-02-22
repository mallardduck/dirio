package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/internal/version"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of DirIO",
	Long:  `All software has versions. This is DirIO's.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("DirIO version:    %s\n", version.Version)
		fmt.Printf("Git commit:       %s\n", version.Commit)
		fmt.Printf("Git tree state:   %s\n", version.GitTreeState)
		fmt.Printf("Built at:         %s\n", version.BuildTime)
		fmt.Printf("Built by:         %s\n", version.BuiltBy)
		fmt.Printf("Go version:       %s\n", version.GoVersion)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
