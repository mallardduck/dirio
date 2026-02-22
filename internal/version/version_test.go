package version

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionInfo(t *testing.T) {
	// Default values check
	assert.Equal(t, "dev", Version)
	assert.Equal(t, "none", Commit)
	assert.Equal(t, "unknown", BuildTime)
	assert.Equal(t, "local", BuiltBy)
	assert.True(t, strings.HasPrefix(runtime.Version(), "go1."))
	assert.Equal(t, "unknown", GitTreeState)
}
