package cmd

import (
	"path/filepath"
	"strings"

	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/profile"
)

// endpoint is a resolved source or destination for cp/sync.
type endpoint struct {
	local  bool               // true = local filesystem path
	path   string             // valid when local == true
	parsed profile.ParsedPath // valid when local == false
}

// parseEndpoint classifies a user-supplied argument as local or remote.
// Local: absolute path, starts with ./ ../ .\ ..\ or is exactly "." or "..".
// Remote: everything else — treated as [profile/]bucket[/key].
func parseEndpoint(s string, cfg profile.Config) endpoint {
	if isLocalPath(s) {
		return endpoint{local: true, path: s}
	}
	return endpoint{parsed: profile.ParsePath(s, cfg)}
}

func isLocalPath(s string) bool {
	if filepath.IsAbs(s) {
		return true
	}
	return s == "." || s == ".." ||
		strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, ".\\") || strings.HasPrefix(s, "..\\")
}

// resolveProfileName returns the profile name embedded in the endpoint's
// remote path, or falls back to the global --profile flag.
func resolveProfileName(ep endpoint) string {
	if !ep.local && ep.parsed.Profile != "" {
		return ep.parsed.Profile
	}
	return flagProfile
}
