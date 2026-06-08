package context

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/sdk/iam"
)

// ── authz decision ────────────────────────────────────────────────────────────

func TestWithAuthzDecision(t *testing.T) {
	ctx := WithAuthzDecision(context.Background(), "allow")
	assert.Equal(t, "allow", GetAuthzDecision(ctx))
}

func TestGetAuthzDecision_Missing(t *testing.T) {
	assert.Nil(t, GetAuthzDecision(context.Background()))
}

// ── anonymous request ─────────────────────────────────────────────────────────

func TestWithAnonymousRequest(t *testing.T) {
	ctx := WithAnonymousRequest(context.Background())
	assert.True(t, IsAnonymousRequest(ctx))
}

func TestIsAnonymousRequest_False(t *testing.T) {
	assert.False(t, IsAnonymousRequest(context.Background()))
}

// ── user ──────────────────────────────────────────────────────────────────────

func TestWithUser(t *testing.T) {
	user := &iam.User{AccessKey: "alice"}
	ctx := WithUser(context.Background(), user)

	got, err := GetUser(ctx)
	require.NoError(t, err)
	assert.Equal(t, "alice", got.AccessKey)
}

func TestGetUser_Missing(t *testing.T) {
	_, err := GetUser(context.Background())
	assert.Error(t, err)
}

// ── pre-signed request ────────────────────────────────────────────────────────

func TestWithPreSignedUser(t *testing.T) {
	user := &iam.User{AccessKey: "signer"}
	expiry := "2099-01-01T00:00:00Z"
	ctx := WithPreSignedUser(context.Background(), user, expiry)

	assert.True(t, IsPreSignedRequest(ctx))

	got, err := GetUser(ctx)
	require.NoError(t, err)
	assert.Equal(t, "signer", got.AccessKey)

	exp, ok := GetPreSignedExpiresAt(ctx)
	assert.True(t, ok)
	assert.Equal(t, expiry, exp)
}

func TestIsPreSignedRequest_False(t *testing.T) {
	assert.False(t, IsPreSignedRequest(context.Background()))
}

func TestGetPreSignedExpiresAt_Missing(t *testing.T) {
	_, ok := GetPreSignedExpiresAt(context.Background())
	assert.False(t, ok)
}

// ── POST policy request ───────────────────────────────────────────────────────

func TestWithPostPolicyRequest(t *testing.T) {
	user := &iam.User{AccessKey: "uploader"}
	policy := "eyJleHBpcmF0aW9uIjoiMjA5OS0wMS0wMVQwMDowMDowMFoifQ=="
	ctx := WithPostPolicyRequest(context.Background(), user, policy)

	assert.True(t, IsPostPolicyRequest(ctx))
	assert.Equal(t, policy, GetPostPolicyPolicyB64(ctx))

	got, err := GetUser(ctx)
	require.NoError(t, err)
	assert.Equal(t, "uploader", got.AccessKey)
}

func TestIsPostPolicyRequest_False(t *testing.T) {
	assert.False(t, IsPostPolicyRequest(context.Background()))
}

func TestGetPostPolicyPolicyB64_Missing(t *testing.T) {
	assert.Empty(t, GetPostPolicyPolicyB64(context.Background()))
}

// ── service account info ──────────────────────────────────────────────────────

func TestWithServiceAccountInfo(t *testing.T) {
	id := uuid.New()
	info := &ServiceAccountInfo{
		ParentUserUUID:     &id,
		PolicyMode:         iam.PolicyModeOverride,
		EmbeddedPolicyJSON: `{"Version":"2012-10-17"}`,
	}
	ctx := WithServiceAccountInfo(context.Background(), info)

	got := GetServiceAccountInfo(ctx)
	require.NotNil(t, got)
	assert.Equal(t, &id, got.ParentUserUUID)
	assert.Equal(t, iam.PolicyModeOverride, got.PolicyMode)
	assert.Equal(t, `{"Version":"2012-10-17"}`, got.EmbeddedPolicyJSON)
}

func TestGetServiceAccountInfo_Missing(t *testing.T) {
	assert.Nil(t, GetServiceAccountInfo(context.Background()))
}
