package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestProfile_ServerTypeRoundTrip(t *testing.T) {
	cfg := Config{
		DefaultProfile: "local",
		Profiles: map[string]Profile{
			"local": {
				Endpoint:   "http://localhost:9000",
				AccessKey:  "testkey",
				SecretKey:  "testsecret",
				ServerType: "dirio",
			},
			"minio-prod": {
				Endpoint:   "http://minio.example.com:9000",
				AccessKey:  "miniokey",
				SecretKey:  "miniosecret",
				ServerType: "minio",
			},
			"s3": {
				Endpoint:  "https://s3.amazonaws.com",
				AccessKey: "awskey",
				SecretKey: "awssecret",
				// ServerType intentionally empty
			},
		},
	}

	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	var loaded Config
	require.NoError(t, yaml.Unmarshal(data, &loaded))

	assert.Equal(t, "dirio", loaded.Profiles["local"].ServerType)
	assert.Equal(t, "minio", loaded.Profiles["minio-prod"].ServerType)
	assert.Equal(t, "", loaded.Profiles["s3"].ServerType, "absent server_type should remain empty")
}

func TestProfile_ServerTypeAbsentFromOldYAML(t *testing.T) {
	// Simulate loading a config file that predates the server_type field.
	oldYAML := `default_profile: local
profiles:
  local:
    endpoint: http://localhost:9000
    access_key: testkey
    secret_key: testsecret
`
	var loaded Config
	require.NoError(t, yaml.Unmarshal([]byte(oldYAML), &loaded))

	assert.Equal(t, "", loaded.Profiles["local"].ServerType,
		"pre-existing config with no server_type field should load cleanly with empty ServerType")
	assert.Equal(t, "http://localhost:9000", loaded.Profiles["local"].Endpoint)
}

func TestProfile_ServerTypeOmittedWhenEmpty(t *testing.T) {
	// server_type with omitempty must not appear in YAML when empty.
	cfg := Config{
		DefaultProfile: "local",
		Profiles: map[string]Profile{
			"local": {
				Endpoint:  "http://localhost:9000",
				AccessKey: "k",
				SecretKey: "s",
			},
		},
	}

	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "server_type",
		"server_type should not appear in YAML when empty")
}
