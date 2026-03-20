package hostname

import (
	"os"
	"strings"
)

const (
	// envOverride allows overriding the hostname via environment variable.
	// Future enhancement: integrate with cobra+viper for unified environment
	// variable handling and configuration management.
	envOverride = "DIRIO_HOSTNAME"
	defaultBase = "dirio-s3"
)

// Base returns the short, sanitized base hostname without suffixes or domains.
// It checks (in order): environment variable, OS hostname, or a default value.
func Base() string {
	if v := os.Getenv(envOverride); v != "" {
		if s := sanitize(v); s != "" {
			return s
		}
	}

	if h, err := os.Hostname(); err == nil && h != "" {
		h = strings.Split(h, ".")[0]
		if s := sanitize(h); s != "" {
			return s
		}
	}

	return defaultBase
}

// baseIsExplicit reports whether the base hostname was explicitly provided
// via env var or OS hostname, as opposed to falling back to the hardcoded default.
func baseIsExplicit() bool {
	if v := os.Getenv(envOverride); v != "" {
		if s := sanitize(v); s != "" {
			return true
		}
	}

	if h, err := os.Hostname(); err == nil && h != "" {
		h = strings.Split(h, ".")[0]
		if s := sanitize(h); s != "" {
			return true
		}
	}

	return false
}
