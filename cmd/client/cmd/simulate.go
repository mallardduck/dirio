package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/render"
	"github.com/mallardduck/dirio/sdk/dioclient"
)

// --- flags ---

var (
	flagSimulateUser       string
	flagSimulateAllActions bool
)

// --- command ---

var simulateCmd = &cobra.Command{
	Use:   "simulate",
	Short: "Simulate IAM policy evaluation for S3 actions",
	Long: `Simulate IAM policy evaluation for S3 actions on a DirIO server.

Two modes:

  Single action (default):
    dio simulate <action> <bucket>[/key] [--user USER]
    Evaluates whether the given action is allowed and prints the reason.

  All actions (--all-actions):
    dio simulate <bucket> --all-actions [--user USER]
    Lists every known S3 action as allowed or denied.

The --user flag lets admins simulate as another user. Without it, the
simulation uses the authenticated user's own access key.

These commands require a DirIO server (not available on generic S3 or MinIO).`,
	RunE: runSimulate,
}

func init() {
	rootCmd.AddCommand(simulateCmd)
	simulateCmd.Flags().StringVar(&flagSimulateUser, "user", "", "access key of the user to simulate (admin only; default: authenticated user)")
	simulateCmd.Flags().BoolVar(&flagSimulateAllActions, "all-actions", false, "list all allowed and denied actions instead of simulating a single action")
}

// --- handlers ---

func runSimulate(cmd *cobra.Command, args []string) error {
	if err := requireDirIO(cmd.Context()); err != nil {
		return err
	}

	if flagSimulateAllActions {
		return runSimulateAllActions(cmd, args)
	}
	return runSimulateAction(cmd, args)
}

// runSimulateAction handles: dio simulate <action> <bucket>[/key]
func runSimulateAction(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("simulate: requires <action> and <bucket>[/key] (or use --all-actions for all actions)")
	}

	action := args[0]

	clientCfg, parsed, err := resolveWithPath(args[1])
	if err != nil {
		return err
	}
	if parsed.Bucket == "" {
		return fmt.Errorf("simulate: bucket name is required")
	}

	// Determine the user to simulate as.
	userKey := flagSimulateUser
	if userKey == "" {
		userKey = clientCfg.AccessKey
	}

	dc := dioclient.NewDirioClient(clientCfg)
	result, err := dc.Simulate(cmd.Context(), dioclient.SimulateRequest{
		AccessKey: userKey,
		Bucket:    parsed.Bucket,
		Action:    action,
		Key:       parsed.Prefix,
	})
	if err != nil {
		return fmt.Errorf("simulate: %w", err)
	}

	mode := outputMode()
	if mode == render.ModeJSON {
		render.JSON(os.Stdout, os.Stderr, map[string]any{
			"user":    userKey,
			"bucket":  parsed.Bucket,
			"key":     parsed.Prefix,
			"action":  action,
			"allowed": result.Allowed,
			"reason":  result.Reason,
		})
		return nil
	}

	allowed := "denied"
	if result.Allowed {
		allowed = "allowed"
	}
	headers := []string{"USER", "ACTION", "RESULT", "REASON"}
	rows := [][]string{{userKey, action, allowed, result.Reason}}
	render.Table(os.Stdout, headers, rows, mode)
	return nil
}

// runSimulateAllActions handles: dio simulate <bucket> --all-actions
func runSimulateAllActions(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("simulate --all-actions: requires <bucket>")
	}

	clientCfg, parsed, err := resolveWithPath(args[0])
	if err != nil {
		return err
	}
	if parsed.Bucket == "" {
		return fmt.Errorf("simulate --all-actions: bucket name is required")
	}

	userKey := flagSimulateUser
	if userKey == "" {
		userKey = clientCfg.AccessKey
	}

	dc := dioclient.NewDirioClient(clientCfg)
	perms, err := dc.GetEffectivePermissions(cmd.Context(), parsed.Bucket, userKey)
	if err != nil {
		return fmt.Errorf("simulate --all-actions: %w", err)
	}

	mode := outputMode()
	if mode == render.ModeJSON {
		render.JSON(os.Stdout, os.Stderr, map[string]any{
			"user":            perms.AccessKey,
			"bucket":          perms.Bucket,
			"allowed_actions": perms.AllowedActions,
			"denied_actions":  perms.DeniedActions,
		})
		return nil
	}

	headers := []string{"ACTION", "RESULT"}
	var rows [][]string
	for _, a := range sorted(perms.AllowedActions) {
		rows = append(rows, []string{a, "allowed"})
	}
	for _, a := range sorted(perms.DeniedActions) {
		rows = append(rows, []string{a, "denied"})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
	render.Table(os.Stdout, headers, rows, mode)
	return nil
}

func sorted(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}
