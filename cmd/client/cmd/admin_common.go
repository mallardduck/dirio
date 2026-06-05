package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mallardduck/dirio/internal/dioclient/profile"
	"github.com/mallardduck/dirio/internal/dioclient/serverdetect"
	"github.com/mallardduck/dirio/pkg/dioclient"
)

// resolveClientConfig loads the profile config and resolves clientCfg for the
// given profileName (empty = global --profile flag or default).
func resolveClientConfig(profileName string) (profile.Config, dioclient.Config, error) {
	cfg, err := profile.Load()
	if err != nil {
		return profile.Config{}, dioclient.Config{}, err
	}

	name := profileName
	if name == "" {
		name = flagProfile
	}

	clientCfg := profile.Resolve(cfg, name)
	applyFlagOverrides(&clientCfg)
	return cfg, clientCfg, nil
}

// warnIfGenericS3 prints a warning to stderr when the active profile's server
// type is generic S3. It also auto-detects and caches the server type on first
// use if not yet set. It never fails the command — warnings only.
func warnIfGenericS3(ctx context.Context) {
	cfg, err := profile.Load()
	if err != nil {
		return
	}

	name := flagProfile
	if name == "" {
		name = cfg.DefaultProfile
	}

	p, ok := cfg.Profiles[name]
	if !ok {
		return
	}

	st := serverdetect.ServerType(p.ServerType)

	if st == serverdetect.ServerTypeUnknown {
		detected, err := serverdetect.Detect(ctx, p.Endpoint)
		if err == nil && detected != serverdetect.ServerTypeUnknown {
			st = detected
			p.ServerType = string(detected)
			cfg.Profiles[name] = p
			_ = profile.Save(cfg) // best-effort cache
		}
	}

	if st == serverdetect.ServerTypeS3Generic {
		fmt.Fprintln(os.Stderr, "Warning: the active endpoint does not appear to be a DirIO or MinIO server. Admin commands may not be supported.")
	}
}

// requireDirIO returns an error if the active profile's server type is not DirIO.
// It auto-detects and caches the server type on first use.
func requireDirIO(ctx context.Context) error {
	cfg, err := profile.Load()
	if err != nil {
		return nil // non-fatal; let the actual command fail with a meaningful error
	}

	name := flagProfile
	if name == "" {
		name = cfg.DefaultProfile
	}

	p, ok := cfg.Profiles[name]
	if !ok {
		return nil
	}

	st := serverdetect.ServerType(p.ServerType)

	if st == serverdetect.ServerTypeUnknown {
		detected, err := serverdetect.Detect(ctx, p.Endpoint)
		if err == nil && detected != serverdetect.ServerTypeUnknown {
			st = detected
			p.ServerType = string(detected)
			cfg.Profiles[name] = p
			_ = profile.Save(cfg)
		}
	}

	if st != serverdetect.ServerTypeDirIO {
		return fmt.Errorf("this command requires a DirIO server (detected: %q); use a DirIO profile or set --endpoint to a DirIO instance", st)
	}
	return nil
}

// parseDuration parses durations like "24h", "7d", "30m".
// Standard Go duration strings are supported; "Nd" (days) is also accepted.
func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return d, nil
}
