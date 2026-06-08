package profile

import (
	"os"

	"github.com/mallardduck/dirio/sdk/dioclient"
)

// Resolve returns a dioclient.Config for the given profile name (or the
// default profile when name is ""), applying environment variable overrides.
//
// Resolution order (highest priority first):
//  1. AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_ENDPOINT_URL env vars
//  2. DIO_ACCESS_KEY / DIO_SECRET_KEY / DIO_ENDPOINT env vars
//  3. Named profile from cfg (or cfg.DefaultProfile when name == "")
//  4. Built-in defaults (http://localhost:9000, us-east-1)
func Resolve(cfg Config, name string) dioclient.Config {
	// Start with built-in defaults.
	out := dioclient.Config{
		Endpoint:  "http://localhost:9000",
		AccessKey: "",
		SecretKey: "",
		Region:    "us-east-1",
	}

	// Layer profile values.
	profileName := name
	if profileName == "" {
		profileName = cfg.DefaultProfile
	}
	if p, ok := cfg.Profiles[profileName]; ok {
		if p.Endpoint != "" {
			out.Endpoint = p.Endpoint
		}
		if p.AccessKey != "" {
			out.AccessKey = p.AccessKey
		}
		if p.SecretKey != "" {
			out.SecretKey = p.SecretKey
		}
		if p.Region != "" {
			out.Region = p.Region
		}
	}

	// DIO_* env vars.
	if v := os.Getenv("DIO_ENDPOINT"); v != "" {
		out.Endpoint = v
	}
	if v := os.Getenv("DIO_ACCESS_KEY"); v != "" {
		out.AccessKey = v
	}
	if v := os.Getenv("DIO_SECRET_KEY"); v != "" {
		out.SecretKey = v
	}
	if v := os.Getenv("DIO_REGION"); v != "" {
		out.Region = v
	}

	// AWS_* env vars (highest priority).
	if v := os.Getenv("AWS_ENDPOINT_URL"); v != "" {
		out.Endpoint = v
	}
	if v := os.Getenv("AWS_ACCESS_KEY_ID"); v != "" {
		out.AccessKey = v
	}
	if v := os.Getenv("AWS_SECRET_ACCESS_KEY"); v != "" {
		out.SecretKey = v
	}
	if v := os.Getenv("AWS_DEFAULT_REGION"); v != "" {
		out.Region = v
	}

	return out
}
