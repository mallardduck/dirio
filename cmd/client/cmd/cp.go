package cmd

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/profile"
	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/render"
	"github.com/mallardduck/dirio/sdk/dioclient"
)

var cpCmd = &cobra.Command{
	Use:   "cp <src> <dst>",
	Short: "Copy files or objects between local filesystem and DirIO",
	Long: `Copy a single file or object.

Path formats:
  Local  — absolute path, or starts with ./ or ../
  Remote — [profile/]bucket/key

Operations (determined by which side is local):
  dio cp ./report.pdf    mybucket/reports/report.pdf   upload
  dio cp mybucket/img.jpg  ./img.jpg                   download
  dio cp mybucket/a.txt  mybucket/b.txt                server-side copy`,
	Args: cobra.ExactArgs(2),
	RunE: runCp,
}

func init() {
	rootCmd.AddCommand(cpCmd)
}

func runCp(cmd *cobra.Command, args []string) error {
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
		return cpUpload(ctx, cfg, src.path, dst, mode)
	case !src.local && dst.local:
		return cpDownload(ctx, cfg, src, dst.path, mode)
	case !src.local && !dst.local:
		return cpServerCopy(ctx, cfg, src, dst, mode)
	default:
		return fmt.Errorf("cp: both paths are local — use the system cp command for local-to-local copies")
	}
}

func cpUpload(ctx context.Context, cfg profile.Config, localPath string, dst endpoint, mode render.OutputMode) error {
	clientCfg := profile.Resolve(cfg, resolveProfileName(dst))
	applyFlagOverrides(&clientCfg)

	client, err := dioclient.New(clientCfg)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}

	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}
	size := fi.Size()

	key := dst.parsed.Prefix
	if key == "" {
		key = filepath.Base(localPath)
	}
	contentType := mime.TypeByExtension(filepath.Ext(localPath))

	showProgress := mode == render.ModeTUI
	pr := newProgressReader(f, size, filepath.Base(localPath), showProgress)
	err = client.PutObject(ctx, dst.parsed.Bucket, key, pr, size, contentType)
	pr.finish()
	if err != nil {
		return fmt.Errorf("cp: upload %s → %s/%s: %w", localPath, dst.parsed.Bucket, key, err)
	}

	reportCpTransfer(mode, "upload", localPath, dst.parsed.Bucket+"/"+key, size)
	return nil
}

func cpDownload(ctx context.Context, cfg profile.Config, src endpoint, localPath string, mode render.OutputMode) error {
	clientCfg := profile.Resolve(cfg, resolveProfileName(src))
	applyFlagOverrides(&clientCfg)

	client, err := dioclient.New(clientCfg)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}

	key := src.parsed.Prefix
	body, info, err := client.GetObject(ctx, src.parsed.Bucket, key)
	if err != nil {
		return fmt.Errorf("cp: get %s/%s: %w", src.parsed.Bucket, key, err)
	}
	defer body.Close()

	destPath := localPath
	if fi, err2 := os.Stat(localPath); err2 == nil && fi.IsDir() {
		destPath = filepath.Join(localPath, filepath.Base(key))
	}

	if err = os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("cp: %w", err)
	}

	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}
	defer outFile.Close()

	showProgress := mode == render.ModeTUI
	pw := newProgressWriter(outFile, info.Size, filepath.Base(key), showProgress)
	if _, err = io.Copy(pw, body); err != nil {
		return fmt.Errorf("cp: download %s/%s: %w", src.parsed.Bucket, key, err)
	}
	pw.finish()

	reportCpTransfer(mode, "download", src.parsed.Bucket+"/"+key, destPath, info.Size)
	return nil
}

func cpServerCopy(ctx context.Context, cfg profile.Config, src, dst endpoint, mode render.OutputMode) error {
	// Prefer src profile, then dst profile, then --profile flag.
	profileName := resolveProfileName(src)
	if profileName == "" {
		profileName = resolveProfileName(dst)
	}
	clientCfg := profile.Resolve(cfg, profileName)
	applyFlagOverrides(&clientCfg)

	client, err := dioclient.New(clientCfg)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}

	srcKey := src.parsed.Prefix
	dstKey := dst.parsed.Prefix
	if dstKey == "" {
		dstKey = filepath.Base(srcKey)
	}

	if err = client.CopyObject(ctx, src.parsed.Bucket, srcKey, dst.parsed.Bucket, dstKey); err != nil {
		return fmt.Errorf("cp: copy %s/%s → %s/%s: %w",
			src.parsed.Bucket, srcKey, dst.parsed.Bucket, dstKey, err)
	}

	srcRef := src.parsed.Bucket + "/" + srcKey
	dstRef := dst.parsed.Bucket + "/" + dstKey
	if mode == render.ModeJSON {
		render.JSON(os.Stdout, os.Stderr, map[string]any{
			"action": "copy",
			"src":    srcRef,
			"dst":    dstRef,
		})
	} else {
		fmt.Fprintf(os.Stderr, "  copy: %s → %s\n", srcRef, dstRef)
	}
	return nil
}

func reportCpTransfer(mode render.OutputMode, action, src, dst string, size int64) {
	if mode == render.ModeJSON {
		render.JSON(os.Stdout, os.Stderr, map[string]any{
			"action": action,
			"src":    src,
			"dst":    dst,
			"size":   size,
		})
		return
	}
	// For both TUI and plain: progress bar already flushed a newline;
	// print a concise completion line.
	fmt.Fprintf(os.Stderr, "  %s: %s → %s (%s)\n", action, src, dst, formatSize(size))
}
