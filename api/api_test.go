package consoleapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminUserUUID(t *testing.T) {
	// AdminUserUUID must be a fixed, stable value — the console hardcodes it.
	assert.Equal(t, "badfc0de-fadd-fc0f-fee0-000dadbeef00", AdminUserUUID)
}

func TestCreateUserRequest_ZeroValue(t *testing.T) {
	var r CreateUserRequest
	assert.Empty(t, r.AccessKey)
	assert.Empty(t, r.SecretKey)
	assert.False(t, r.GenerateSecret)
}

func TestSimulateResult_Fields(t *testing.T) {
	result := SimulateResult{Allowed: true, Reason: "policy match", MatchedRule: "Allow s3:GetObject"}
	require.True(t, result.Allowed)
	assert.Equal(t, "policy match", result.Reason)
	assert.Equal(t, "Allow s3:GetObject", result.MatchedRule)
}
