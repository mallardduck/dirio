package group

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
	"github.com/mallardduck/dirio/internal/service/group/mock"
	"github.com/mallardduck/dirio/sdk/iam"
)

func newMockService(t *testing.T) (*Service, *mock.MockRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mock.NewMockRepository(ctrl)
	return NewService(repo), repo
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestCreate_Success(t *testing.T) {
	svc, repo := newMockService(t)
	want := &iam.Group{Name: "admins"}
	repo.EXPECT().CreateGroup(gomock.Any(), "admins").Return(nil)
	repo.EXPECT().GetGroup(gomock.Any(), "admins").Return(want, nil)

	got, err := svc.Create(context.Background(), &CreateGroupRequest{Name: "admins"})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestCreate_EmptyName(t *testing.T) {
	svc, _ := newMockService(t)
	_, err := svc.Create(context.Background(), &CreateGroupRequest{Name: ""})
	assert.True(t, svcerrors.IsValidation(err))
}

func TestCreate_AlreadyExists(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().CreateGroup(gomock.Any(), "admins").Return(metadata.ErrGroupAlreadyExists)

	_, err := svc.Create(context.Background(), &CreateGroupRequest{Name: "admins"})
	assert.ErrorIs(t, err, svcerrors.ErrGroupAlreadyExists)
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestGet_Success(t *testing.T) {
	svc, repo := newMockService(t)
	want := &iam.Group{Name: "devs"}
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(want, nil)

	got, err := svc.Get(context.Background(), "devs")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGet_EmptyName(t *testing.T) {
	svc, _ := newMockService(t)
	_, err := svc.Get(context.Background(), "")
	assert.True(t, svcerrors.IsValidation(err))
}

func TestGet_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetGroup(gomock.Any(), "missing").Return(nil, metadata.ErrGroupNotFound)

	_, err := svc.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, svcerrors.ErrGroupNotFound)
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestDelete_Success(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(&iam.Group{Name: "devs"}, nil)
	repo.EXPECT().DeleteGroup(gomock.Any(), "devs").Return(nil)

	assert.NoError(t, svc.Delete(context.Background(), "devs"))
}

func TestDelete_EmptyName(t *testing.T) {
	svc, _ := newMockService(t)
	assert.True(t, svcerrors.IsValidation(svc.Delete(context.Background(), "")))
}

func TestDelete_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetGroup(gomock.Any(), "missing").Return(nil, metadata.ErrGroupNotFound)

	err := svc.Delete(context.Background(), "missing")
	assert.ErrorIs(t, err, svcerrors.ErrGroupNotFound)
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestList_Success(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().ListGroupNames(gomock.Any()).Return([]string{"admins", "devs"}, nil)

	names, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"admins", "devs"}, names)
}

// ── AddMember ─────────────────────────────────────────────────────────────────

func TestAddMember_Success(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(&iam.Group{Name: "devs"}, nil)
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(&iam.User{UUID: uid}, nil)
	repo.EXPECT().AddUserToGroup(gomock.Any(), "devs", uid).Return(nil)

	assert.NoError(t, svc.AddMember(context.Background(), "devs", uid))
}

func TestAddMember_UserNotFound(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(&iam.Group{Name: "devs"}, nil)
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(nil, metadata.ErrUserNotFound)

	err := svc.AddMember(context.Background(), "devs", uid)
	assert.ErrorIs(t, err, svcerrors.ErrUserNotFound)
}

func TestAddMember_GroupNotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetGroup(gomock.Any(), "missing").Return(nil, metadata.ErrGroupNotFound)

	err := svc.AddMember(context.Background(), "missing", uuid.New())
	assert.ErrorIs(t, err, svcerrors.ErrGroupNotFound)
}

// ── AddMemberByAccessKey ──────────────────────────────────────────────────────

func TestAddMemberByAccessKey_Success(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), "alice").Return(&iam.User{UUID: uid, AccessKey: "alice"}, nil)
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(&iam.Group{Name: "devs"}, nil)
	repo.EXPECT().GetUser(gomock.Any(), uid).Return(&iam.User{UUID: uid}, nil)
	repo.EXPECT().AddUserToGroup(gomock.Any(), "devs", uid).Return(nil)

	assert.NoError(t, svc.AddMemberByAccessKey(context.Background(), "devs", "alice"))
}

func TestAddMemberByAccessKey_UserNotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), "ghost").Return(nil, metadata.ErrUserNotFound)

	err := svc.AddMemberByAccessKey(context.Background(), "devs", "ghost")
	assert.ErrorIs(t, err, svcerrors.ErrUserNotFound)
}

// ── RemoveMember ──────────────────────────────────────────────────────────────

func TestRemoveMember_Success(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(&iam.Group{Name: "devs"}, nil)
	repo.EXPECT().RemoveUserFromGroup(gomock.Any(), "devs", uid).Return(nil)

	assert.NoError(t, svc.RemoveMember(context.Background(), "devs", uid))
}

// ── AttachPolicy / DetachPolicy ───────────────────────────────────────────────

func TestAttachPolicy_Success(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(&iam.Group{Name: "devs"}, nil)
	repo.EXPECT().GetPolicy(gomock.Any(), "ReadOnly").Return(&iam.Policy{Name: "ReadOnly"}, nil)
	repo.EXPECT().AttachPolicyToGroup(gomock.Any(), "devs", "ReadOnly").Return(nil)

	assert.NoError(t, svc.AttachPolicy(context.Background(), "devs", "ReadOnly"))
}

func TestAttachPolicy_PolicyNotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(&iam.Group{Name: "devs"}, nil)
	repo.EXPECT().GetPolicy(gomock.Any(), "Missing").Return(nil, metadata.ErrPolicyNotFound)

	err := svc.AttachPolicy(context.Background(), "devs", "Missing")
	assert.ErrorIs(t, err, svcerrors.ErrPolicyNotFound)
}

func TestDetachPolicy_Success(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(&iam.Group{Name: "devs"}, nil)
	repo.EXPECT().DetachPolicyFromGroup(gomock.Any(), "devs", "ReadOnly").Return(nil)

	assert.NoError(t, svc.DetachPolicy(context.Background(), "devs", "ReadOnly"))
}

// ── SetStatus ─────────────────────────────────────────────────────────────────

func TestSetStatus_Success(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(&iam.Group{Name: "devs"}, nil)
	repo.EXPECT().SetGroupStatus(gomock.Any(), "devs", iam.GroupStatusActive).Return(nil)

	assert.NoError(t, svc.SetStatus(context.Background(), "devs", iam.GroupStatusActive))
}

func TestSetStatus_InvalidStatus(t *testing.T) {
	svc, _ := newMockService(t)
	err := svc.SetStatus(context.Background(), "devs", iam.GroupStatus("bogus"))
	assert.True(t, svcerrors.IsValidation(err))
}

func TestSetStatus_StorageError(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(&iam.Group{Name: "devs"}, nil)
	repo.EXPECT().SetGroupStatus(gomock.Any(), "devs", iam.GroupStatusDisabled).Return(errors.New("write error"))

	err := svc.SetStatus(context.Background(), "devs", iam.GroupStatusDisabled)
	assert.EqualError(t, err, "write error")
}

// ── GetMemberAccessKeys ───────────────────────────────────────────────────────

func TestGetMemberAccessKeys_Success(t *testing.T) {
	svc, repo := newMockService(t)
	uid1, uid2 := uuid.New(), uuid.New()
	grp := &iam.Group{Name: "devs", Members: []uuid.UUID{uid1, uid2}}

	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(grp, nil)
	repo.EXPECT().GetUser(gomock.Any(), uid1).Return(&iam.User{UUID: uid1, AccessKey: "alice"}, nil)
	repo.EXPECT().GetUser(gomock.Any(), uid2).Return(&iam.User{UUID: uid2, AccessKey: "bob"}, nil)

	keys, err := svc.GetMemberAccessKeys(context.Background(), "devs")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alice", "bob"}, keys)
}

func TestGetMemberAccessKeys_SkipsDeletedUsers(t *testing.T) {
	svc, repo := newMockService(t)
	uid1, uid2 := uuid.New(), uuid.New()
	grp := &iam.Group{Name: "devs", Members: []uuid.UUID{uid1, uid2}}

	repo.EXPECT().GetGroup(gomock.Any(), "devs").Return(grp, nil)
	repo.EXPECT().GetUser(gomock.Any(), uid1).Return(&iam.User{UUID: uid1, AccessKey: "alice"}, nil)
	repo.EXPECT().GetUser(gomock.Any(), uid2).Return(nil, errors.New("not found"))

	keys, err := svc.GetMemberAccessKeys(context.Background(), "devs")
	require.NoError(t, err)
	assert.Equal(t, []string{"alice"}, keys)
}
