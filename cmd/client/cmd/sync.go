package cmd

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/profile"
	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/render"
	"github.com/mallardduck/dirio/sdk/dioclient"
)

var (
	flagSyncDelete bool
	flagSyncDryRun bool
)

var syncCmd = &cobra.Command{
	Use:   "sync <src> <dst>",
	Short: "Sync files between local filesystem and DirIO",
	Long: `Sync files between a local directory and a DirIO bucket prefix.

Path formats:
  Local  — absolute path, or starts with ./ or ../
  Remote — [profile/]bucket[/prefix/]

Direction is determined by which side is local:
  dio sync ./data/         mybucket/backup/   upload local dir to bucket
  dio sync mybucket/backup/ ./data/            download bucket prefix to local dir

By default sync only adds and updates. Use --delete to also remove
destination items that are absent from the source.`,
	Args: cobra.ExactArgs(2),
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().BoolVar(&flagSyncDelete, "delete", false, "remove destination items absent from source")
	syncCmd.Flags().BoolVar(&flagSyncDryRun, "dry-run", false, "show what would be synced without making changes")
}

type syncOp struct {
	action    string // "upload" | "download" | "delete-remote" | "delete-local"
	localPath string // local filesystem path (abs)
	remoteKey string // S3 key (no bucket prefix)
	size      int64
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := profile.Load()
	if err != nil {
		return err
	}

	mode, ok := render.ParseMode(flagOutput)
	if !ok {
		return fmt.Errorf("unknown output format %q (valid: tui, plain, json)", flagOutput)
	}
	if flagOutput == "" {
		mode = render.DetectMode()
	}

	src := parseEndpoint(args[0], cfg)
	dst := parseEndpoint(args[1], cfg)

	ctx := context.Background()

	switch {
	case src.local && !dst.local:
		return syncUpload(ctx, cfg, src.path, dst, mode)
	case !src.local && dst.local:
		return syncDownload(ctx, cfg, src, dst.path, mode)
	default:
		return fmt.Errorf("sync: one side must be local and the other remote")
	}
}

// syncUpload syncs a local directory to a remote bucket prefix.
func syncUpload(ctx context.Context, cfg profile.Config, localDir string, dst endpoint, mode render.OutputMode) error {
	clientCfg := profile.Resolve(cfg, resolveProfileName(dst))
	applyFlagOverrides(&clientCfg)

	client, err := dioclient.New(clientCfg)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	absLocalDir, err := filepath.Abs(localDir)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	dstPrefix := dst.parsed.Prefix

	// Walk local directory.
	localSizes := make(map[string]int64)  // s3Key → size
	localPaths := make(map[string]string) // s3Key → abs local path
	err = filepath.WalkDir(absLocalDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(absLocalDir, path)
		if err != nil {
			return err
		}
		s3Rel := filepath.ToSlash(rel)
		key := s3Rel
		if dstPrefix != "" {
			key = strings.TrimSuffix(dstPrefix, "/") + "/" + s3Rel
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		localSizes[key] = fi.Size()
		localPaths[key] = path
		return nil
	})
	if err != nil {
		return fmt.Errorf("sync: walk %s: %w", localDir, err)
	}

	// List remote objects.
	remoteSizes := make(map[string]int64)
	for obj := range client.ListObjects(ctx, dst.parsed.Bucket, dstPrefix, true) {
		if obj.Size == -1 {
			return fmt.Errorf("sync: list %s: server error", dst.parsed.Bucket)
		}
		remoteSizes[obj.Key] = obj.Size
	}

	// Build plan.
	var plan []syncOp
	for key, localSize := range localSizes {
		if remoteSize, exists := remoteSizes[key]; !exists || remoteSize != localSize {
			plan = append(plan, syncOp{
				action:    "upload",
				localPath: localPaths[key],
				remoteKey: key,
				size:      localSize,
			})
		}
	}
	if flagSyncDelete {
		for key := range remoteSizes {
			if _, exists := localSizes[key]; !exists {
				plan = append(plan, syncOp{action: "delete-remote", remoteKey: key})
			}
		}
	}

	return executePlan(ctx, client, dst.parsed.Bucket, plan, mode)
}

// syncDownload syncs a remote bucket prefix to a local directory.
func syncDownload(ctx context.Context, cfg profile.Config, src endpoint, localDir string, mode render.OutputMode) error {
	clientCfg := profile.Resolve(cfg, resolveProfileName(src))
	applyFlagOverrides(&clientCfg)

	client, err := dioclient.New(clientCfg)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	absLocalDir, err := filepath.Abs(localDir)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	srcPrefix := src.parsed.Prefix

	// List remote objects.
	type remoteEntry struct {
		key  string
		size int64
	}
	remoteByRel := make(map[string]remoteEntry) // relPath (slash-sep) → entry
	for obj := range client.ListObjects(ctx, src.parsed.Bucket, srcPrefix, true) {
		if obj.Size == -1 {
			return fmt.Errorf("sync: list %s: server error", src.parsed.Bucket)
		}
		rel := strings.TrimPrefix(obj.Key, srcPrefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}
		remoteByRel[rel] = remoteEntry{key: obj.Key, size: obj.Size}
	}

	// Walk local directory (may not exist yet).
	localSizes := make(map[string]int64) // relPath (slash-sep) → size
	if _, statErr := os.Stat(absLocalDir); statErr == nil {
		err = filepath.WalkDir(absLocalDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return walkErr
			}
			rel, err := filepath.Rel(absLocalDir, path)
			if err != nil {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			localSizes[filepath.ToSlash(rel)] = fi.Size()
			return nil
		})
		if err != nil {
			return fmt.Errorf("sync: walk %s: %w", localDir, err)
		}
	}

	// Build plan.
	var plan []syncOp
	for relPath, entry := range remoteByRel {
		destPath := filepath.Join(absLocalDir, filepath.FromSlash(relPath))
		if localSize, exists := localSizes[relPath]; !exists || localSize != entry.size {
			plan = append(plan, syncOp{
				action:    "download",
				localPath: destPath,
				remoteKey: entry.key,
				size:      entry.size,
			})
		}
	}
	if flagSyncDelete {
		for relPath := range localSizes {
			if _, exists := remoteByRel[relPath]; !exists {
				plan = append(plan, syncOp{
					action:    "delete-local",
					localPath: filepath.Join(absLocalDir, filepath.FromSlash(relPath)),
				})
			}
		}
	}

	return executePlan(ctx, client, src.parsed.Bucket, plan, mode)
}

// executePlan runs or previews the sync plan.
func executePlan(ctx context.Context, client *dioclient.Client, bucket string, plan []syncOp, mode render.OutputMode) error {
	if len(plan) == 0 {
		if mode != render.ModeJSON {
			fmt.Fprintln(os.Stderr, "  already in sync, nothing to do")
		}
		return nil
	}

	if flagSyncDryRun {
		printSyncPlan(plan, mode)
		return nil
	}

	showProgress := mode == render.ModeTUI
	var completed []syncOp

	for _, op := range plan {
		var opErr error

		switch op.action {
		case "upload":
			f, err := os.Open(op.localPath)
			if err != nil {
				return fmt.Errorf("sync: open %s: %w", op.localPath, err)
			}
			contentType := mime.TypeByExtension(filepath.Ext(op.localPath))
			pr := newProgressReader(f, op.size, filepath.Base(op.localPath), showProgress)
			opErr = client.PutObject(ctx, bucket, op.remoteKey, pr, op.size, contentType)
			pr.finish()
			f.Close()
			if opErr == nil && mode == render.ModePlain {
				fmt.Fprintf(os.Stderr, "  upload: %s (%s)\n", op.remoteKey, formatSize(op.size))
			} else if opErr == nil && mode == render.ModeJSON {
				render.JSON(os.Stdout, os.Stderr, map[string]any{
					"action": "upload", "src": op.localPath, "dst": bucket + "/" + op.remoteKey, "size": op.size,
				})
			}

		case "download":
			body, info, err := client.GetObject(ctx, bucket, op.remoteKey)
			if err != nil {
				return fmt.Errorf("sync: get %s/%s: %w", bucket, op.remoteKey, err)
			}
			if err = os.MkdirAll(filepath.Dir(op.localPath), 0o755); err != nil {
				body.Close()
				return fmt.Errorf("sync: mkdir: %w", err)
			}
			outFile, err := os.Create(op.localPath)
			if err != nil {
				body.Close()
				return fmt.Errorf("sync: create %s: %w", op.localPath, err)
			}
			pw := newProgressWriter(outFile, info.Size, filepath.Base(op.localPath), showProgress)
			_, opErr = io.Copy(pw, body)
			pw.finish()
			outFile.Close()
			body.Close()
			if opErr == nil && mode == render.ModePlain {
				fmt.Fprintf(os.Stderr, "  download: %s (%s)\n", op.localPath, formatSize(info.Size))
			} else if opErr == nil && mode == render.ModeJSON {
				render.JSON(os.Stdout, os.Stderr, map[string]any{
					"action": "download", "src": bucket + "/" + op.remoteKey, "dst": op.localPath, "size": info.Size,
				})
			}

		case "delete-remote":
			opErr = client.RemoveObject(ctx, bucket, op.remoteKey)
			if opErr == nil && mode == render.ModePlain {
				fmt.Fprintf(os.Stderr, "  delete: %s/%s\n", bucket, op.remoteKey)
			} else if opErr == nil && mode == render.ModeJSON {
				render.JSON(os.Stdout, os.Stderr, map[string]any{
					"action": "delete", "dst": bucket + "/" + op.remoteKey,
				})
			}

		case "delete-local":
			opErr = os.Remove(op.localPath)
			if opErr == nil && mode == render.ModePlain {
				fmt.Fprintf(os.Stderr, "  delete: %s\n", op.localPath)
			} else if opErr == nil && mode == render.ModeJSON {
				render.JSON(os.Stdout, os.Stderr, map[string]any{
					"action": "delete", "dst": op.localPath,
				})
			}
		}

		if opErr != nil {
			return fmt.Errorf("sync: %s %s: %w", op.action, op.remoteKey+op.localPath, opErr)
		}
		completed = append(completed, op)
	}

	if mode == render.ModeTUI {
		printSyncSummary(completed)
	}
	return nil
}

// printSyncPlan prints the planned actions without executing them (--dry-run).
func printSyncPlan(plan []syncOp, mode render.OutputMode) {
	if mode == render.ModeJSON {
		for _, op := range plan {
			m := map[string]any{"action": op.action, "dry_run": true}
			if op.localPath != "" {
				m["local"] = op.localPath
			}
			if op.remoteKey != "" {
				m["remote_key"] = op.remoteKey
			}
			if op.size > 0 {
				m["size"] = op.size
			}
			render.JSON(os.Stdout, os.Stderr, m)
		}
		return
	}

	headers := []string{"ACTION", "KEY / PATH", "SIZE"}
	rows := make([][]string, len(plan))
	for i, op := range plan {
		ref := op.remoteKey
		if ref == "" {
			ref = op.localPath
		}
		sizeStr := ""
		if op.size > 0 {
			sizeStr = formatSize(op.size)
		}
		rows[i] = []string{op.action, ref, sizeStr}
	}
	render.Table(os.Stdout, headers, rows, mode)
}

// printSyncSummary prints a one-line completion summary to stderr (TUI mode only).
func printSyncSummary(completed []syncOp) {
	var uploaded, downloaded, deleted int
	var totalBytes int64
	for _, op := range completed {
		switch op.action {
		case "upload":
			uploaded++
			totalBytes += op.size
		case "download":
			downloaded++
			totalBytes += op.size
		case "delete-remote", "delete-local":
			deleted++
		}
	}
	msg := fmt.Sprintf("  sync complete: %d uploaded, %d downloaded", uploaded, downloaded)
	if deleted > 0 {
		msg += fmt.Sprintf(", %d deleted", deleted)
	}
	msg += fmt.Sprintf(" (%s transferred)", formatSize(totalBytes))
	fmt.Fprintln(os.Stderr, msg)
}
