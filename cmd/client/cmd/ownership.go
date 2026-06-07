package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/profile"
	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/render"
	"github.com/mallardduck/dirio/sdk/dioclient"
)

// --- command tree ---

var ownershipCmd = &cobra.Command{
	Use:   "ownership",
	Short: "View and transfer ownership of buckets and objects",
	Long: `View and transfer ownership of buckets and objects.

These commands use the DirIO-specific REST API and require a DirIO server.
They are not available on generic S3 or MinIO endpoints.`,
}

var ownershipGetCmd = &cobra.Command{
	Use:   "get <[profile/]bucket[/object]>",
	Short: "Show the owner of a bucket or object",
	Args:  cobra.ExactArgs(1),
	RunE:  runOwnershipGet,
}

var ownershipTransferCmd = &cobra.Command{
	Use:   "transfer <[profile/]bucket> <new-owner>",
	Short: "Transfer bucket ownership to another user (admin only)",
	Args:  cobra.ExactArgs(2),
	RunE:  runOwnershipTransfer,
}

func init() {
	rootCmd.AddCommand(ownershipCmd)
	ownershipCmd.AddCommand(ownershipGetCmd, ownershipTransferCmd)
}

// --- handlers ---

func runOwnershipGet(cmd *cobra.Command, args []string) error {
	if err := requireDirIO(cmd.Context()); err != nil {
		return err
	}

	clientCfg, parsed, err := resolveWithPath(args[0])
	if err != nil {
		return err
	}
	if parsed.Bucket == "" {
		return fmt.Errorf("ownership get: bucket name is required")
	}

	dc := dioclient.NewDirioClient(clientCfg)
	mode := outputMode()

	if parsed.Prefix != "" {
		info, err := dc.GetObjectOwner(cmd.Context(), parsed.Bucket, parsed.Prefix)
		if err != nil {
			return fmt.Errorf("ownership get: %w", err)
		}
		renderOwner(mode, parsed.Bucket, parsed.Prefix, info)
	} else {
		info, err := dc.GetBucketOwner(cmd.Context(), parsed.Bucket)
		if err != nil {
			return fmt.Errorf("ownership get: %w", err)
		}
		renderOwner(mode, parsed.Bucket, "", info)
	}
	return nil
}

func runOwnershipTransfer(cmd *cobra.Command, args []string) error {
	if err := requireDirIO(cmd.Context()); err != nil {
		return err
	}

	clientCfg, parsed, err := resolveWithPath(args[0])
	if err != nil {
		return err
	}
	if parsed.Bucket == "" {
		return fmt.Errorf("ownership transfer: bucket name is required")
	}

	newOwner := args[1]
	dc := dioclient.NewDirioClient(clientCfg)
	mode := outputMode()

	info, err := dc.TransferBucketOwner(cmd.Context(), parsed.Bucket, newOwner)
	if err != nil {
		return fmt.Errorf("ownership transfer: %w", err)
	}
	renderOwner(mode, parsed.Bucket, "", info)
	return nil
}

// resolveWithPath loads the profile config, parses the path arg, and resolves
// the effective dioclient.Config (honouring any profile prefix in the path).
func resolveWithPath(pathArg string) (dioclient.Config, profile.ParsedPath, error) {
	cfg, clientCfg, err := resolveClientConfig(flagProfile)
	if err != nil {
		return dioclient.Config{}, profile.ParsedPath{}, err
	}
	parsed := profile.ParsePath(pathArg, cfg)
	if parsed.Profile != "" {
		_, clientCfg, err = resolveClientConfig(parsed.Profile)
		if err != nil {
			return dioclient.Config{}, profile.ParsedPath{}, err
		}
	}
	return clientCfg, parsed, nil
}

// renderOwner writes owner info to stdout in the current output mode.
func renderOwner(mode render.OutputMode, bucket, key string, info *dioclient.OwnerInfo) {
	if mode == render.ModeJSON {
		m := map[string]any{
			"bucket":     bucket,
			"owner_uuid": info.UUID,
			"access_key": info.AccessKey,
			"username":   info.Username,
		}
		if key != "" {
			m["key"] = key
		}
		render.JSON(os.Stdout, os.Stderr, m)
		return
	}

	target := bucket
	if key != "" {
		target = bucket + "/" + key
	}
	owner := info.AccessKey
	if owner == "" {
		owner = "(admin)"
	}
	headers := []string{"TARGET", "OWNER", "UUID"}
	rows := [][]string{{target, owner, info.UUID}}
	render.Table(os.Stdout, headers, rows, mode)
}
