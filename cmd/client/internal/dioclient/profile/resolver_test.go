package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolve_ProfileValues(t *testing.T) {
	cfg := Config{
		DefaultProfile: "local",
		Profiles: map[string]Profile{
			"local": {
				Endpoint:  "http://localhost:9000",
				AccessKey: "local-key",
				SecretKey: "local-secret",
				Region:    "eu-west-1",
			},
			"prod": {
				Endpoint:  "https://s3.prod.example.com",
				AccessKey: "prod-key",
				SecretKey: "prod-secret",
			},
		},
	}

	t.Run("default profile", func(t *testing.T) {
		r := Resolve(cfg, "")
		assert.Equal(t, "http://localhost:9000", r.Endpoint)
		assert.Equal(t, "local-key", r.AccessKey)
		assert.Equal(t, "eu-west-1", r.Region)
	})

	t.Run("named profile", func(t *testing.T) {
		r := Resolve(cfg, "prod")
		assert.Equal(t, "https://s3.prod.example.com", r.Endpoint)
		assert.Equal(t, "prod-key", r.AccessKey)
		// region not set → falls through to built-in default
		assert.Equal(t, "us-east-1", r.Region)
	})

	t.Run("unknown profile falls through to defaults", func(t *testing.T) {
		r := Resolve(cfg, "nonexistent")
		assert.Equal(t, "http://localhost:9000", r.Endpoint)
	})
}

func TestResolve_EnvVarOverrides(t *testing.T) {
	cfg := Config{
		DefaultProfile: "local",
		Profiles: map[string]Profile{
			"local": {
				Endpoint:  "http://localhost:9000",
				AccessKey: "local-key",
				SecretKey: "local-secret",
				Region:    "us-east-1",
			},
		},
	}

	t.Setenv("DIO_ENDPOINT", "http://dio-override:9000")
	t.Setenv("DIO_ACCESS_KEY", "dio-key")

	r := Resolve(cfg, "")
	assert.Equal(t, "http://dio-override:9000", r.Endpoint)
	assert.Equal(t, "dio-key", r.AccessKey)
	// SecretKey not overridden → profile value
	assert.Equal(t, "local-secret", r.SecretKey)
}

func TestResolve_AWSEnvTakesPrecedence(t *testing.T) {
	cfg := Config{
		DefaultProfile: "local",
		Profiles: map[string]Profile{
			"local": {
				Endpoint:  "http://localhost:9000",
				AccessKey: "local-key",
			},
		},
	}

	t.Setenv("DIO_ACCESS_KEY", "dio-key")
	t.Setenv("AWS_ACCESS_KEY_ID", "aws-key")

	r := Resolve(cfg, "")
	assert.Equal(t, "aws-key", r.AccessKey)
}
