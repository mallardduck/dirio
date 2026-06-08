package version

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionDefaults(t *testing.T) {
	assert.Equal(t, "dev", Version)
	assert.Equal(t, "none", Commit)
	assert.Equal(t, "unknown", BuildTime)
	assert.Equal(t, "local", BuiltBy)
	assert.Equal(t, "unknown", GitTreeState)
	assert.Equal(t, runtime.Version(), GoVersion)
	assert.True(t, strings.HasPrefix(GoVersion, "go1."), "GoVersion should have go1. prefix, got %q", GoVersion)
}
