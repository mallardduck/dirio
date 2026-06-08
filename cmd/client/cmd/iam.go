package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/render"
	"github.com/mallardduck/dirio/sdk/dioclient"
)

// --- flags ---

var (
	flagIAMPolicyUser  string
	flagIAMPolicyGroup string
	flagIAMPolicyFile  string
	flagIAMPolicyV1    bool
)

// --- command tree ---

var iamCmd = &cobra.Command{
	Use:   "iam",
	Short: "Manage IAM users and policies",
	Long: `Create and manage IAM users and policies.

These commands call the MinIO-compatible admin API and work on both DirIO and
MinIO servers. Use 'dio iam user' for user management and 'dio iam policy' for
policy management.`,
}

// iam user

var iamUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage IAM users",
}

var iamUserListCmd = &cobra.Command{
	Use:   "list",
	Short: "List IAM users",
	Args:  cobra.NoArgs,
	RunE:  runIAMUserList,
}

var iamUserCreateCmd = &cobra.Command{
	Use:   "create <access-key> <secret-key>",
	Short: "Create an IAM user",
	Args:  cobra.ExactArgs(2),
	RunE:  runIAMUserCreate,
}

var iamUserInfoCmd = &cobra.Command{
	Use:   "info <access-key>",
	Short: "Show details for an IAM user",
	Args:  cobra.ExactArgs(1),
	RunE:  runIAMUserInfo,
}

var iamUserDeleteCmd = &cobra.Command{
	Use:     "delete <access-key>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete an IAM user",
	Args:    cobra.ExactArgs(1),
	RunE:    runIAMUserDelete,
}

var iamUserEnableCmd = &cobra.Command{
	Use:   "enable <access-key>",
	Short: "Enable an IAM user",
	Args:  cobra.ExactArgs(1),
	RunE:  runIAMUserEnable,
}

var iamUserDisableCmd = &cobra.Command{
	Use:   "disable <access-key>",
	Short: "Disable an IAM user",
	Args:  cobra.ExactArgs(1),
	RunE:  runIAMUserDisable,
}

// iam policy

var iamPolicyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Manage IAM policies",
}

var iamPolicyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List named IAM policies",
	Args:  cobra.NoArgs,
	RunE:  runIAMPolicyList,
}

var iamPolicyCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create or replace a named IAM policy",
	Args:  cobra.ExactArgs(1),
	RunE:  runIAMPolicyCreate,
}

var iamPolicyInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show a named IAM policy",
	Args:  cobra.ExactArgs(1),
	RunE:  runIAMPolicyInfo,
}

var iamPolicyDeleteCmd = &cobra.Command{
	Use:     "delete <name>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete a named IAM policy",
	Args:    cobra.ExactArgs(1),
	RunE:    runIAMPolicyDelete,
}

var iamPolicyAttachCmd = &cobra.Command{
	Use:   "attach <policy-name>",
	Short: "Attach a policy to a user or group",
	Args:  cobra.ExactArgs(1),
	RunE:  runIAMPolicyAttach,
}

var iamPolicyDetachCmd = &cobra.Command{
	Use:   "detach <policy-name>",
	Short: "Detach a policy from a user or group",
	Args:  cobra.ExactArgs(1),
	RunE:  runIAMPolicyDetach,
}

func init() {
	rootCmd.AddCommand(iamCmd)
	iamCmd.AddCommand(iamUserCmd, iamPolicyCmd)

	iamUserCmd.AddCommand(
		iamUserListCmd,
		iamUserCreateCmd,
		iamUserInfoCmd,
		iamUserDeleteCmd,
		iamUserEnableCmd,
		iamUserDisableCmd,
	)

	iamPolicyCmd.AddCommand(
		iamPolicyListCmd,
		iamPolicyCreateCmd,
		iamPolicyInfoCmd,
		iamPolicyDeleteCmd,
		iamPolicyAttachCmd,
		iamPolicyDetachCmd,
	)

	iamPolicyCreateCmd.Flags().StringVar(&flagIAMPolicyFile, "file", "", "path to policy JSON file (required)")
	_ = iamPolicyCreateCmd.MarkFlagRequired("file")

	iamPolicyInfoCmd.Flags().BoolVar(&flagIAMPolicyV1, "v1", false, "use the legacy V1 admin API (for older MinIO or pre-fix DirIO builds)")

	iamPolicyAttachCmd.Flags().StringVar(&flagIAMPolicyUser, "user", "", "attach to this user (access key)")
	iamPolicyAttachCmd.Flags().StringVar(&flagIAMPolicyGroup, "group", "", "attach to this group")

	iamPolicyDetachCmd.Flags().StringVar(&flagIAMPolicyUser, "user", "", "detach from this user (access key)")
	iamPolicyDetachCmd.Flags().StringVar(&flagIAMPolicyGroup, "group", "", "detach from this group")
}

// --- IAM user handlers ---

func runIAMUserList(cmd *cobra.Command, _ []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	users, err := ac.ListUsers(cmd.Context())
	if err != nil {
		return fmt.Errorf("iam user list: %w", err)
	}

	// Sort by access key for deterministic output.
	keys := make([]string, 0, len(users))
	for k := range users {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	mode := outputMode()

	if mode == render.ModeJSON {
		for _, k := range keys {
			u := users[k]
			render.JSON(os.Stdout, os.Stderr, map[string]any{
				"access_key": k,
				"status":     string(u.Status),
				"policy":     u.PolicyName,
				"member_of":  u.MemberOf,
				"updated_at": u.UpdatedAt.Format(time.RFC3339),
			})
		}
		return nil
	}

	headers := []string{"ACCESS KEY", "STATUS", "POLICY", "GROUPS"}
	rows := make([][]string, len(keys))
	for i, k := range keys {
		u := users[k]
		groups := "-"
		if len(u.MemberOf) > 0 {
			groups = joinStrings(u.MemberOf, ", ")
		}
		rows[i] = []string{k, string(u.Status), u.PolicyName, groups}
	}
	render.Table(os.Stdout, headers, rows, mode)
	return nil
}

func runIAMUserCreate(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	if err := ac.AddUser(cmd.Context(), args[0], args[1]); err != nil {
		return fmt.Errorf("iam user create: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Created user %s\n", args[0])
	return nil
}

func runIAMUserInfo(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	info, err := ac.GetUserInfo(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("iam user info: %w", err)
	}

	mode := outputMode()
	if mode == render.ModeJSON {
		render.JSON(os.Stdout, os.Stderr, map[string]any{
			"access_key": args[0],
			"status":     string(info.Status),
			"policy":     info.PolicyName,
			"member_of":  info.MemberOf,
			"updated_at": info.UpdatedAt.Format(time.RFC3339),
		})
		return nil
	}

	groups := "-"
	if len(info.MemberOf) > 0 {
		groups = joinStrings(info.MemberOf, ", ")
	}
	rows := [][]string{
		{"Status", string(info.Status)},
		{"Policy", info.PolicyName},
		{"Groups", groups},
		{"Updated", info.UpdatedAt.Format("2006-01-02 15:04:05")},
	}
	render.Table(os.Stdout, []string{"FIELD", "VALUE"}, rows, mode)
	return nil
}

func runIAMUserDelete(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	if err := ac.RemoveUser(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf("iam user delete: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Deleted user %s\n", args[0])
	return nil
}

func runIAMUserEnable(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	if err := ac.SetUserStatus(cmd.Context(), args[0], dioclient.AccountEnabled); err != nil {
		return fmt.Errorf("iam user enable: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Enabled user %s\n", args[0])
	return nil
}

func runIAMUserDisable(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	if err := ac.SetUserStatus(cmd.Context(), args[0], dioclient.AccountDisabled); err != nil {
		return fmt.Errorf("iam user disable: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Disabled user %s\n", args[0])
	return nil
}

// --- IAM policy handlers ---

func runIAMPolicyList(cmd *cobra.Command, _ []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	policies, err := ac.ListCannedPolicies(cmd.Context())
	if err != nil {
		return fmt.Errorf("iam policy list: %w", err)
	}

	// Sort policy names.
	names := make([]string, 0, len(policies))
	for n := range policies {
		names = append(names, n)
	}
	sort.Strings(names)

	mode := outputMode()

	if mode == render.ModeJSON {
		for _, n := range names {
			render.JSON(os.Stdout, os.Stderr, map[string]any{
				"name":   n,
				"policy": policies[n],
			})
		}
		return nil
	}

	headers := []string{"NAME"}
	rows := make([][]string, len(names))
	for i, n := range names {
		rows[i] = []string{n}
	}
	render.Table(os.Stdout, headers, rows, mode)
	return nil
}

func runIAMPolicyCreate(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	policyJSON, err := os.ReadFile(flagIAMPolicyFile)
	if err != nil {
		return fmt.Errorf("iam policy create: read policy file: %w", err)
	}

	if err := ac.AddCannedPolicy(cmd.Context(), args[0], policyJSON); err != nil {
		return fmt.Errorf("iam policy create: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Created policy %s\n", args[0])
	return nil
}

func runIAMPolicyInfo(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	ctx := cmd.Context()
	if flagIAMPolicyV1 {
		ctx = dioclient.WithV1API(ctx)
	}

	info, err := ac.InfoCannedPolicy(ctx, args[0])
	if err != nil {
		return fmt.Errorf("iam policy info: %w", err)
	}

	mode := outputMode()
	if mode == render.ModeJSON {
		render.JSON(os.Stdout, os.Stderr, map[string]any{
			"name":       info.PolicyName,
			"policy":     info.Policy,
			"created_at": info.CreateDate.Format(time.RFC3339),
			"updated_at": info.UpdateDate.Format(time.RFC3339),
		})
		return nil
	}

	// Pretty-print the policy JSON with indentation.
	var buf []byte
	if pretty, err := json.MarshalIndent(info.Policy, "", "  "); err == nil {
		buf = pretty
	} else {
		buf = info.Policy
	}

	rows := [][]string{
		{"Name", info.PolicyName},
		{"Created", info.CreateDate.Format("2006-01-02 15:04:05")},
		{"Updated", info.UpdateDate.Format("2006-01-02 15:04:05")},
		{"Policy", string(buf)},
	}
	render.Table(os.Stdout, []string{"FIELD", "VALUE"}, rows, mode)
	return nil
}

func runIAMPolicyDelete(cmd *cobra.Command, args []string) error {
	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	if err := ac.DeleteCannedPolicy(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf("iam policy delete: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Deleted policy %s\n", args[0])
	return nil
}

func runIAMPolicyAttach(cmd *cobra.Command, args []string) error {
	if flagIAMPolicyUser == "" && flagIAMPolicyGroup == "" {
		return fmt.Errorf("iam policy attach: specify --user or --group")
	}

	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	req := dioclient.PolicyAssociationReq{
		Policies: []string{args[0]},
		User:     flagIAMPolicyUser,
		Group:    flagIAMPolicyGroup,
	}

	if _, err := ac.AttachPolicy(cmd.Context(), req); err != nil {
		return fmt.Errorf("iam policy attach: %w", err)
	}

	target := flagIAMPolicyUser
	if target == "" {
		target = "group:" + flagIAMPolicyGroup
	}
	fmt.Fprintf(os.Stdout, "Attached policy %s to %s\n", args[0], target)
	return nil
}

func runIAMPolicyDetach(cmd *cobra.Command, args []string) error {
	if flagIAMPolicyUser == "" && flagIAMPolicyGroup == "" {
		return fmt.Errorf("iam policy detach: specify --user or --group")
	}

	ac, err := buildAdminClient()
	if err != nil {
		return err
	}

	warnIfGenericS3(cmd.Context())

	req := dioclient.PolicyAssociationReq{
		Policies: []string{args[0]},
		User:     flagIAMPolicyUser,
		Group:    flagIAMPolicyGroup,
	}

	if _, err := ac.DetachPolicy(cmd.Context(), req); err != nil {
		return fmt.Errorf("iam policy detach: %w", err)
	}

	target := flagIAMPolicyUser
	if target == "" {
		target = "group:" + flagIAMPolicyGroup
	}
	fmt.Fprintf(os.Stdout, "Detached policy %s from %s\n", args[0], target)
	return nil
}

// joinStrings concatenates strings with a separator.
func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
