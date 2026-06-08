package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/profile"
	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/render"
	"github.com/mallardduck/dirio/sdk/dioclient"
)

var flagRecursive bool

var lsCmd = &cobra.Command{
	Use:   "ls [[profile/]bucket[/prefix]]",
	Short: "List buckets or objects",
	Long: `List buckets (no argument) or objects within a bucket.

Path forms:
  dio ls                          list all buckets (default profile)
  dio ls mybucket                 list objects in mybucket (default profile)
  dio ls mybucket/prefix/         list objects under prefix
  dio ls prod/mybucket            list objects in mybucket (prod profile)
  dio ls prod/mybucket/prefix/    list with profile, bucket, and prefix`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLs,
}

func init() {
	rootCmd.AddCommand(lsCmd)
	lsCmd.Flags().BoolVarP(&flagRecursive, "recursive", "r", false, "list all objects recursively (no virtual directory grouping)")
}

func runLs(cmd *cobra.Command, args []string) error {
	cfg, err := profile.Load()
	if err != nil {
		return err
	}

	var pathArg string
	if len(args) > 0 {
		pathArg = args[0]
	}

	parsed := profile.ParsePath(pathArg, cfg)

	// Determine the active profile name: path prefix > --profile flag > default.
	profileName := parsed.Profile
	if profileName == "" {
		profileName = flagProfile
	}

	// Apply inline flag overrides on top of the resolved profile.
	clientCfg := profile.Resolve(cfg, profileName)
	applyFlagOverrides(&clientCfg)

	client, err := dioclient.New(clientCfg)
	if err != nil {
		return fmt.Errorf("ls: %w", err)
	}

	// Determine output mode.
	mode, ok := render.ParseMode(flagOutput)
	if !ok {
		return fmt.Errorf("unknown output format %q (valid: tui, plain, json)", flagOutput)
	}
	if flagOutput == "" {
		mode = render.DetectMode()
	}

	ctx := context.Background()

	if parsed.Bucket == "" {
		return listBuckets(ctx, client, mode)
	}
	return listObjects(ctx, client, parsed.Bucket, parsed.Prefix, flagRecursive, mode)
}

func listBuckets(ctx context.Context, client *dioclient.Client, mode render.OutputMode) error {
	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("list buckets: %w", err)
	}

	if mode == render.ModeJSON {
		for _, b := range buckets {
			render.JSON(os.Stdout, os.Stderr, map[string]string{
				"name":       b.Name,
				"created_at": b.CreatedAt.Format(time.RFC3339),
			})
		}
		return nil
	}

	headers := []string{"NAME", "CREATED"}
	rows := make([][]string, len(buckets))
	for i, b := range buckets {
		rows[i] = []string{b.Name, b.CreatedAt.Format("2006-01-02 15:04:05")}
	}
	render.Table(os.Stdout, headers, rows, mode)
	return nil
}

func listObjects(ctx context.Context, client *dioclient.Client, bucket, prefix string, recursive bool, mode render.OutputMode) error {
	ch := client.ListObjects(ctx, bucket, prefix, recursive)

	headers := []string{"KEY", "SIZE", "MODIFIED", "ETAG"}
	var rows [][]string

	for obj := range ch {
		if obj.Size == -1 {
			return fmt.Errorf("list objects in %s: server error", bucket)
		}

		if mode == render.ModeJSON {
			render.JSON(os.Stdout, os.Stderr, map[string]any{
				"key":           obj.Key,
				"size":          obj.Size,
				"last_modified": obj.LastModified.Format(time.RFC3339),
				"etag":          obj.ETag,
				"content_type":  obj.ContentType,
			})
			continue
		}

		rows = append(rows, []string{
			obj.Key,
			formatSize(obj.Size),
			obj.LastModified.Format("2006-01-02 15:04"),
			trimETag(obj.ETag),
		})
	}

	if mode != render.ModeJSON {
		render.Table(os.Stdout, headers, rows, mode)
	}
	return nil
}

// applyFlagOverrides layers explicit --endpoint / --access-key / --secret-key /
// --region flags on top of the profile-resolved config.
func applyFlagOverrides(cfg *dioclient.Config) {
	if flagEndpoint != "" {
		cfg.Endpoint = flagEndpoint
	}
	if flagAccessKey != "" {
		cfg.AccessKey = flagAccessKey
	}
	if flagSecretKey != "" {
		cfg.SecretKey = flagSecretKey
	}
	if flagRegion != "" {
		cfg.Region = flagRegion
	}
}

func formatSize(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GiB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MiB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KiB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// trimETag strips surrounding quotes from S3 ETags.
func trimETag(etag string) string {
	if len(etag) >= 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
		return etag[1 : len(etag)-1]
	}
	return etag
}
