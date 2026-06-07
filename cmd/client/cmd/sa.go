package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/internal/dioclient/render"
	"github.com/mallardduck/dirio/sdk/dioclient"
)

// --- flags ---

var (
	flagSAUser   string
	flagSAName   string
	flagSAPolicy string
	flagSAExpiry string
)

// --- command tree ---

var saCmd = &cobra.Command{
	Use:   "sa",
	Short: "Manage service accounts",
	Long: `Create and manage service accounts (long-lived credentials scoped to an IAM user).

Service accounts are supported on DirIO and MinIO servers.`,
}

var saListCmd = &cobra.Command{
	Use:   "list",
	Short: "List service accounts",
	Args:  cobra.NoArgs,
	RunE:  runSAList,
}

var saCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new service account",
	Args:  cobra.NoArgs,
	RunE:  runSACreate,
}

var saInfoCmd = &cobra.Command{
	Use:   "info <access-key>",
	Short: "Show details for a service account",
	Args:  cobra.ExactArgs(1),
	RunE:  runSAInfo,
}

var saUpdateCmd = &cobra.Command{
	Use:   "update <access-key>",
	Short: "Update a service account",
	Args:  cobra.ExactArgs(1),
	RunE:  runSAUpdate,
}

var saRmCmd = &cobra.Command{
	Use:     "rm <access-key>",
	Aliases: []string{"delete", "remove"},
	Short:   "Delete a service account",
	Args:    cobra.ExactArgs(1),
	RunE:    runSARm,
}

func init() {
	rootCmd.AddCommand(saCmd)
	saCmd.AddCommand(saListCmd, saCreateCmd, saInfoCmd, saUpdateCmd, saRmCmd)

	saListCmd.Flags().StringVar(&flagSAUser, "user", "", "list service accounts for this user (default: authenticated user)")

	saCreateCmd.Flags().StringVar(&flagSAUser, "user", "", "parent user (default: authenticated user)")
	saCreateCmd.Flags().StringVar(&flagSAName, "name", "", "human-readable name for the service account")
	saCreateCmd.Flags().StringVar(&flagSAPolicy, "policy", "", "path to policy JSON file (default: inherit parent policy)")
	saCreateCmd.Flags().StringVar(&flagSAExpiry, "expiry", "", "expiry duration, e.g. 24h, 7d (default: no expiry)")

	saUpdateCmd.Flags().StringVar(&flagSAName, "name", "", "update the service account name")
	saUpdateCmd.Flags().StringVar(&flagSAPolicy, "policy", "", "path to new policy JSON file")
	saUpdateCmd.Flags().StringVar(&flagSAExpiry, "expiry", "", "new expiry duration, e.g. 24h, 7d")
}

// --- handlers ---

func runSAList(cmd *cobra.Command, _ []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	resp, err := ac.ListServiceAccounts(cmd.Context(), flagSAUser)
	if err != nil {
		return fmt.Errorf("sa list: %w", err)
	}

	mode := outputMode()

	if mode == render.ModeJSON {
		for _, a := range resp.Accounts {
			render.JSON(os.Stdout, os.Stderr, serviceAccountToMap(a))
		}
		return nil
	}

	headers := []string{"ACCESS KEY", "PARENT USER", "STATUS", "NAME", "EXPIRY"}
	rows := make([][]string, len(resp.Accounts))
	for i, a := range resp.Accounts {
		rows[i] = []string{
			a.AccessKey,
			a.ParentUser,
			a.AccountStatus,
			a.Name,
			formatExpiry(a.Expiration),
		}
	}
	render.Table(os.Stdout, headers, rows, mode)
	return nil
}

func runSACreate(cmd *cobra.Command, _ []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	req := dioclient.AddServiceAccountReq{
		TargetUser: flagSAUser,
		Name:       flagSAName,
	}

	if flagSAPolicy != "" {
		policyJSON, err := os.ReadFile(flagSAPolicy)
		if err != nil {
			return fmt.Errorf("sa create: read policy file: %w", err)
		}
		req.Policy = json.RawMessage(policyJSON)
	}

	if flagSAExpiry != "" {
		exp, err := parseDuration(flagSAExpiry)
		if err != nil {
			return fmt.Errorf("sa create: %w", err)
		}
		t := time.Now().Add(exp)
		req.Expiration = &t
	}

	creds, err := ac.AddServiceAccount(cmd.Context(), req)
	if err != nil {
		return fmt.Errorf("sa create: %w", err)
	}

	mode := outputMode()
	if mode == render.ModeJSON {
		render.JSON(os.Stdout, os.Stderr, map[string]any{
			"access_key": creds.AccessKey,
			"secret_key": creds.SecretKey,
		})
		return nil
	}

	fmt.Fprintf(os.Stdout, "Access Key: %s\nSecret Key: %s\n", creds.AccessKey, creds.SecretKey)
	return nil
}

func runSAInfo(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	info, err := ac.InfoServiceAccount(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("sa info: %w", err)
	}

	mode := outputMode()
	if mode == render.ModeJSON {
		render.JSON(os.Stdout, os.Stderr, map[string]any{
			"access_key":     args[0],
			"parent_user":    info.ParentUser,
			"status":         info.AccountStatus,
			"name":           info.Name,
			"description":    info.Description,
			"implied_policy": info.ImpliedPolicy,
			"expiry":         formatExpiry(info.Expiration),
		})
		return nil
	}

	rows := [][]string{
		{"Parent User", info.ParentUser},
		{"Status", info.AccountStatus},
		{"Name", info.Name},
		{"Description", info.Description},
		{"Implied Policy", fmt.Sprintf("%v", info.ImpliedPolicy)},
		{"Expiry", formatExpiry(info.Expiration)},
	}
	render.Table(os.Stdout, []string{"FIELD", "VALUE"}, rows, mode)
	return nil
}

func runSAUpdate(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	req := dioclient.UpdateServiceAccountReq{
		NewName: flagSAName,
	}

	if flagSAPolicy != "" {
		policyJSON, err := os.ReadFile(flagSAPolicy)
		if err != nil {
			return fmt.Errorf("sa update: read policy file: %w", err)
		}
		req.NewPolicy = json.RawMessage(policyJSON)
	}

	if flagSAExpiry != "" {
		exp, err := parseDuration(flagSAExpiry)
		if err != nil {
			return fmt.Errorf("sa update: %w", err)
		}
		t := time.Now().Add(exp)
		req.NewExpiration = &t
	}

	if err := ac.UpdateServiceAccount(cmd.Context(), args[0], req); err != nil {
		return fmt.Errorf("sa update: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Updated service account %s\n", args[0])
	return nil
}

func runSARm(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	if err := ac.DeleteServiceAccount(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf("sa rm: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Deleted service account %s\n", args[0])
	return nil
}

// --- helpers ---

func serviceAccountToMap(a dioclient.ServiceAccountInfo) map[string]any {
	m := map[string]any{
		"access_key":  a.AccessKey,
		"parent_user": a.ParentUser,
		"status":      a.AccountStatus,
		"name":        a.Name,
	}
	if a.Expiration != nil {
		m["expiry"] = a.Expiration.Format(time.RFC3339)
	}
	return m
}

func formatExpiry(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "never"
	}
	return t.Format("2006-01-02 15:04")
}

// buildAdminClient resolves the active profile and constructs an AdminClient.
func buildAdminClient() (*dioclient.AdminClient, error) {
	cfg, clientCfg, err := resolveClientConfig(flagProfile)
	if err != nil {
		return nil, err
	}
	_ = cfg
	return dioclient.NewAdminClient(clientCfg)
}

// outputMode returns the OutputMode from the global --output flag.
func outputMode() render.OutputMode {
	mode, ok := render.ParseMode(flagOutput)
	if !ok {
		return render.DetectMode()
	}
	if flagOutput == "" {
		return render.DetectMode()
	}
	return mode
}
