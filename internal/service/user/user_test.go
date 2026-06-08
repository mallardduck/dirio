package user

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/user/mock"
	"github.com/mallardduck/dirio/sdk/iam"
)

func newMockService(t *testing.T) (*Service, *mock.MockRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mock.NewMockRepository(ctrl)
	return NewService(repo, nil), repo
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestCreate_Success(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), "alice").Return(nil, metadata.ErrUserNotFound)
	repo.EXPECT().CreateOrUpdateUser(gomock.Any(), gomock.AssignableToTypeOf(&iam.User{})).Return(nil)

	u, err := svc.Create(context.Background(), &CreateUserRequest{
		AccessKey: "alice",
		SecretKey: "supersecretpassword1",
		Status:    iam.UserStatusActive,
	})
	require.NoError(t, err)
	assert.Equal(t, "alice", u.AccessKey)
	assert.Equal(t, iam.UserStatusActive, u.Status)
}

func TestCreate_InvalidAccessKey(t *testing.T) {
	svc, _ := newMockService(t)
	_, err := svc.Create(context.Background(), &CreateUserRequest{AccessKey: ""})
	assert.True(t, svcerrors.IsValidation(err))
}

func TestCreate_InvalidSecretKey(t *testing.T) {
	svc, _ := newMockService(t)
	_, err := svc.Create(context.Background(), &CreateUserRequest{AccessKey: "alice", SecretKey: "short"})
	assert.True(t, svcerrors.IsValidation(err))
}

func TestCreate_AlreadyExists(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), "alice").Return(&iam.User{AccessKey: "alice"}, nil)

	_, err := svc.Create(context.Background(), &CreateUserRequest{
		AccessKey: "alice",
		SecretKey: "supersecretpassword1",
		Status:    iam.UserStatusActive,
	})
	assert.ErrorIs(t, err, svcerrors.ErrUserAlreadyExists)
}

func TestCreate_InvalidStatus(t *testing.T) {
	svc, _ := newMockService(t)
	_, err := svc.Create(context.Background(), &CreateUserRequest{
		AccessKey: "alice",
		SecretKey: "supersecretpassword1",
		Status:    iam.UserStatus("bogus"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestGet_Success(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	want := &iam.User{UUID: uid, AccessKey: "alice"}
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(want, nil)

	got, err := svc.Get(context.Background(), uid)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGet_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(nil, metadata.ErrUserNotFound)

	_, err := svc.Get(context.Background(), uid)
	assert.ErrorIs(t, err, svcerrors.ErrUserNotFound)
}

// ── GetByAccessKey ────────────────────────────────────────────────────────────

func TestGetByAccessKey_Success(t *testing.T) {
	svc, repo := newMockService(t)
	want := &iam.User{AccessKey: "alice"}
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), "alice").Return(want, nil)

	got, err := svc.GetByAccessKey(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGetByAccessKey_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), "ghost").Return(nil, metadata.ErrUserNotFound)

	_, err := svc.GetByAccessKey(context.Background(), "ghost")
	assert.ErrorIs(t, err, svcerrors.ErrUserNotFound)
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestUpdate_SecretKey(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	existing := &iam.User{UUID: uid, AccessKey: "alice", SecretKey: "old", Status: iam.UserStatusActive}
	newSecret := "newlongenoughsecretkey"

	repo.EXPECT().GetUser(gomock.Any(), uid).Return(existing, nil)
	repo.EXPECT().CreateOrUpdateUser(gomock.Any(), gomock.Any()).Return(nil)

	got, err := svc.Update(context.Background(), uid, &UpdateUserRequest{SecretKey: &newSecret})
	require.NoError(t, err)
	assert.Equal(t, newSecret, got.SecretKey)
}

func TestUpdate_InvalidSecretKey(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(&iam.User{UUID: uid, Status: iam.UserStatusActive}, nil)
	bad := "x"

	_, err := svc.Update(context.Background(), uid, &UpdateUserRequest{SecretKey: &bad})
	assert.True(t, svcerrors.IsValidation(err))
}

func TestUpdate_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(nil, metadata.ErrUserNotFound)

	_, err := svc.Update(context.Background(), uid, &UpdateUserRequest{})
	assert.ErrorIs(t, err, svcerrors.ErrUserNotFound)
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestDelete_Success(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(&iam.User{UUID: uid}, nil)
	repo.EXPECT().GetGroupNamesForUser(gomock.Any(), uid).Return([]string{}, nil)
	repo.EXPECT().DeleteUser(gomock.Any(), uid).Return(nil)

	assert.NoError(t, svc.Delete(context.Background(), uid))
}

func TestDelete_RemovesFromGroups(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(&iam.User{UUID: uid}, nil)
	repo.EXPECT().GetGroupNamesForUser(gomock.Any(), uid).Return([]string{"devs", "admins"}, nil)
	repo.EXPECT().RemoveUserFromGroup(gomock.Any(), "devs", uid).Return(nil)
	repo.EXPECT().RemoveUserFromGroup(gomock.Any(), "admins", uid).Return(nil)
	repo.EXPECT().DeleteUser(gomock.Any(), uid).Return(nil)

	assert.NoError(t, svc.Delete(context.Background(), uid))
}

func TestDelete_SystemAdmin(t *testing.T) {
	svc, _ := newMockService(t)
	err := svc.Delete(context.Background(), iam.AdminUserUUID)
	assert.ErrorIs(t, err, svcerrors.ErrUserIsSystemAdmin)
}

func TestDelete_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(nil, metadata.ErrUserNotFound)

	err := svc.Delete(context.Background(), uid)
	assert.ErrorIs(t, err, svcerrors.ErrUserNotFound)
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestList_Success(t *testing.T) {
	svc, repo := newMockService(t)
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	repo.EXPECT().ListUsers(gomock.Any()).Return(ids, nil)

	got, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, ids, got)
}

// ── AttachPolicy ──────────────────────────────────────────────────────────────

func TestAttachPolicy_Success(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetPolicy(gomock.Any(), "ReadOnly").Return(&iam.Policy{Name: "ReadOnly"}, nil)
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(&iam.User{UUID: uid, AttachedPolicies: []string{}}, nil)
	repo.EXPECT().CreateOrUpdateUser(gomock.Any(), gomock.Any()).Return(nil)

	assert.NoError(t, svc.AttachPolicy(context.Background(), uid, "ReadOnly"))
}

func TestAttachPolicy_Idempotent(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetPolicy(gomock.Any(), "ReadOnly").Return(&iam.Policy{Name: "ReadOnly"}, nil)
	// Policy already attached; CreateOrUpdateUser should NOT be called.
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(
		&iam.User{UUID: uid, AttachedPolicies: []string{"ReadOnly"}}, nil)

	assert.NoError(t, svc.AttachPolicy(context.Background(), uid, "ReadOnly"))
}

func TestAttachPolicy_PolicyNotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetPolicy(gomock.Any(), "Missing").Return(nil, metadata.ErrPolicyNotFound)

	err := svc.AttachPolicy(context.Background(), uuid.New(), "Missing")
	assert.ErrorIs(t, err, svcerrors.ErrPolicyNotFound)
}

// ── DetachPolicy ──────────────────────────────────────────────────────────────

func TestDetachPolicy_Success(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(
		&iam.User{UUID: uid, AttachedPolicies: []string{"ReadOnly", "Other"}}, nil)
	repo.EXPECT().CreateOrUpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, u *iam.User) error {
			assert.NotContains(t, u.AttachedPolicies, "ReadOnly")
			assert.Contains(t, u.AttachedPolicies, "Other")
			return nil
		})

	assert.NoError(t, svc.DetachPolicy(context.Background(), uid, "ReadOnly"))
}

// ── GetGroups ─────────────────────────────────────────────────────────────────

func TestGetGroups_Success(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetGroupNamesForUser(gomock.Any(), uid).Return([]string{"devs"}, nil)

	groups, err := svc.GetGroups(context.Background(), uid)
	require.NoError(t, err)
	assert.Equal(t, []string{"devs"}, groups)
}

func TestGetGroups_Error(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetGroupNamesForUser(gomock.Any(), uid).Return(nil, errors.New("db error"))

	_, err := svc.GetGroups(context.Background(), uid)
	assert.EqualError(t, err, "db error")
}
