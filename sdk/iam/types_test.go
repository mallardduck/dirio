package iam

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── UserStatus ────────────────────────────────────────────────────────────────

func TestUserStatus_Validate(t *testing.T) {
	assert.NoError(t, UserStatusActive.Validate())
	assert.NoError(t, UserStatusDisabled.Validate())
	assert.Error(t, UserStatus("suspended").Validate())
	assert.Error(t, UserStatus("").Validate())
}

func TestUserStatus_IsActive(t *testing.T) {
	assert.True(t, UserStatusActive.IsActive())
	assert.False(t, UserStatusDisabled.IsActive())
}

func TestUserStatus_String(t *testing.T) {
	assert.Equal(t, "on", UserStatusActive.String())
	assert.Equal(t, "off", UserStatusDisabled.String())
}

func TestUserStatus_MinioString(t *testing.T) {
	assert.Equal(t, "enabled", UserStatusActive.MinioString())
	assert.Equal(t, "disabled", UserStatusDisabled.MinioString())
}

// ── GroupStatus ───────────────────────────────────────────────────────────────

func TestGroupStatus_Validate(t *testing.T) {
	assert.NoError(t, GroupStatusActive.Validate())
	assert.NoError(t, GroupStatusDisabled.Validate())
	assert.Error(t, GroupStatus("unknown").Validate())
}

func TestGroupStatus_IsActive(t *testing.T) {
	assert.True(t, GroupStatusActive.IsActive())
	assert.False(t, GroupStatusDisabled.IsActive())
}

func TestGroupStatus_String(t *testing.T) {
	assert.Equal(t, "on", GroupStatusActive.String())
	assert.Equal(t, "off", GroupStatusDisabled.String())
}

// ── ServiceAcctStatus ─────────────────────────────────────────────────────────

func TestServiceAcctStatus_Validate(t *testing.T) {
	assert.NoError(t, ServiceAcctStatusActive.Validate())
	assert.NoError(t, ServiceAcctStatusDisabled.Validate())
	assert.Error(t, ServiceAcctStatus("locked").Validate())
}

func TestServiceAcctStatus_IsActive(t *testing.T) {
	assert.True(t, ServiceAcctStatusActive.IsActive())
	assert.False(t, ServiceAcctStatusDisabled.IsActive())
}

func TestServiceAcctStatus_String(t *testing.T) {
	assert.Equal(t, "on", ServiceAcctStatusActive.String())
	assert.Equal(t, "off", ServiceAcctStatusDisabled.String())
}

// ── NewServiceAccount ─────────────────────────────────────────────────────────

func TestNewServiceAccount(t *testing.T) {
	id := uuid.New()
	parent := uuid.New()
	exp := time.Now().Add(24 * time.Hour)

	sa := NewServiceAccount(
		id, "mykey", "mysecret", "alice",
		"my-sa", "a description",
		&parent,
		PolicyModeOverride,
		ServiceAcctStatusActive,
		`{"Version":"2012-10-17"}`,
		&exp,
	)

	require.NotNil(t, sa)
	assert.Equal(t, id, sa.UUID)
	assert.Equal(t, "mykey", sa.AccessKey)
	assert.Equal(t, "mysecret", sa.SecretKey)
	assert.Equal(t, "alice", sa.Username)
	assert.Equal(t, "my-sa", sa.Name)
	assert.Equal(t, "a description", sa.Description)
	assert.Equal(t, &parent, sa.ParentUserUUID)
	assert.Equal(t, PolicyModeOverride, sa.PolicyMode)
	assert.Equal(t, ServiceAcctStatusActive, sa.Status)
	assert.NotNil(t, sa.ExpiresAt)
	assert.Equal(t, ServiceAccountMetadataVersion, sa.Version)
	assert.False(t, sa.CreatedAt.IsZero())
}

func TestNewServiceAccount_NilExpiry(t *testing.T) {
	sa := NewServiceAccount(uuid.New(), "k", "s", "u", "", "", nil, PolicyModeInherit, ServiceAcctStatusActive, "", nil)
	assert.Nil(t, sa.ExpiresAt)
	assert.Nil(t, sa.ParentUserUUID)
}

// ── Builtins ──────────────────────────────────────────────────────────────────

func TestIsBuiltinPolicy(t *testing.T) {
	for _, name := range BuiltinPolicyNames {
		assert.True(t, IsBuiltinPolicy(name), "expected %q to be a builtin", name)
	}
	assert.False(t, IsBuiltinPolicy("custom-policy"))
	assert.False(t, IsBuiltinPolicy(""))
}

func TestBuiltinPolicyDocument(t *testing.T) {
	for _, name := range BuiltinPolicyNames {
		doc := BuiltinPolicyDocument(name)
		require.NotNil(t, doc, "expected non-nil document for builtin %q", name)
		assert.NoError(t, doc.Validate(), "builtin policy %q should be valid", name)
	}
	assert.Nil(t, BuiltinPolicyDocument("nonexistent"))
}
