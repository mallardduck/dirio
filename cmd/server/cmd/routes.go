package cmd

import (
	"os"

	"github.com/mallardduck/teapot-router/pkg/teapot"
	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/internal/http/server"
)

var routesCmd = &cobra.Command{
	Use:   "routes",
	Short: "List all registered HTTP routes",
	Long: `Display all registered HTTP routes in the DirIO server.

This command shows the route name, HTTP method, and URL pattern for
each endpoint without starting the server.`,
	RunE: runRoutes,
}

var jsonOutput bool

func init() {
	rootCmd.AddCommand(routesCmd)
	routesCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output routes as JSON")
}

func runRoutes(cmd *cobra.Command, args []string) error {
	// Create router and setup routes WITHOUT starting server
	r := teapot.New()
	server.SetupRoutes(r, nil)

	// Get all routes
	routes := r.Routes()

	// Format output based on flags
	var err error
	if jsonOutput {
		err = teapot.FormatRoutesJSON(os.Stdout, routes)
	} else {
		err = teapot.FormatRoutesTable(os.Stdout, routes)
	}

	return err
}
