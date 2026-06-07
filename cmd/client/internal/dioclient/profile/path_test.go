package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePath(t *testing.T) {
	cfg := Config{
		DefaultProfile: "local",
		Profiles: map[string]Profile{
			"local": {Endpoint: "http://localhost:9000"},
			"prod":  {Endpoint: "https://s3.example.com"},
		},
	}

	tests := []struct {
		name    string
		input   string
		wantP   string
		wantB   string
		wantPfx string
	}{
		{"empty", "", "", "", ""},
		{"bucket only", "mybucket", "", "mybucket", ""},
		{"bucket+prefix", "mybucket/prefix/", "", "mybucket", "prefix/"},
		{"bucket+deep prefix", "mybucket/a/b/c", "", "mybucket", "a/b/c"},
		{"profile+bucket", "local/mybucket", "local", "mybucket", ""},
		{"profile+bucket+prefix", "prod/mybucket/prefix/", "prod", "mybucket", "prefix/"},
		{"profile+bucket+deep prefix", "prod/mybucket/a/b/c", "prod", "mybucket", "a/b/c"},
		// A name not in profiles is treated as a bucket name.
		{"unknown first segment", "notaprofile/mybucket", "", "notaprofile", "mybucket"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParsePath(tc.input, cfg)
			assert.Equal(t, tc.wantP, got.Profile, "Profile")
			assert.Equal(t, tc.wantB, got.Bucket, "Bucket")
			assert.Equal(t, tc.wantPfx, got.Prefix, "Prefix")
		})
	}
}
