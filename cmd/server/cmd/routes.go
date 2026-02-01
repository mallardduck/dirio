package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/mallardduck/dirio/internal/router"
	"github.com/mallardduck/dirio/internal/server"
	"github.com/spf13/cobra"
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
	r := router.New()
	server.SetupRoutes(r, nil)

	// Get routes with methods
	routes := r.RoutesWithMethods()

	if jsonOutput {
		return printRoutesJSON(routes)
	}

	return printRoutesTable(routes)
}

func printRoutesJSON(routes map[string]router.RouteInfo) error {
	type entry struct {
		Name    string `json:"name"`
		Method  string `json:"method"`
		Pattern string `json:"pattern"`
	}

	entries := make([]entry, 0, len(routes))
	for name, info := range routes {
		entries = append(entries, entry{
			Name:    name,
			Method:  info.Method,
			Pattern: info.Pattern,
		})
	}

	// Sort by pattern, then method
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Pattern != entries[j].Pattern {
			return entries[i].Pattern < entries[j].Pattern
		}
		return entries[i].Method < entries[j].Method
	})

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(map[string]interface{}{
		"routes": entries,
	})
}

func printRoutesTable(routes map[string]router.RouteInfo) error {
	type entry struct {
		name    string
		method  string
		pattern string
	}

	entries := make([]entry, 0, len(routes))
	for name, info := range routes {
		entries = append(entries, entry{
			name:    name,
			method:  info.Method,
			pattern: info.Pattern,
		})
	}

	// Sort by pattern, then method
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].pattern != entries[j].pattern {
			return entries[i].pattern < entries[j].pattern
		}
		return entries[i].method < entries[j].method
	})

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "METHOD\tPATTERN\tNAME")
	fmt.Fprintln(w, "------\t-------\t----")

	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\n", e.method, e.pattern, e.name)
	}

	return w.Flush()
}
