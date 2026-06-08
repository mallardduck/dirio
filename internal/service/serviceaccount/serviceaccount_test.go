package serviceaccount

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/serviceaccount/mock"
	"github.com/mallardduck/dirio/sdk/iam"
)

func newMockService(t *testing.T) (*Service, *mock.MockRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mock.NewMockRepository(ctrl)
	return NewService(repo), repo
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestCreate_AutoGeneratesKeys(t *testing.T) {
	svc, repo := newMockService(t)

	// Auto-generation path: no duplicate check against users (access key unknown until generated).
	// The generated key won't collide — so GetUserByAccessKey and GetServiceAccount both return not-found.
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), gomock.Any()).Return(nil, metadata.ErrUserNotFound)
	repo.EXPECT().GetServiceAccount(gomock.Any(), gomock.Any()).Return(nil, metadata.ErrServiceAccountNotFound)
	repo.EXPECT().CreateServiceAccount(gomock.Any(), gomock.AssignableToTypeOf(&iam.ServiceAccount{})).Return(nil)

	sa, err := svc.Create(context.Background(), &CreateServiceAccountRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, sa.AccessKey)
	assert.Equal(t, iam.ServiceAcctStatusActive, sa.Status)
}

func TestCreate_ExplicitKeys(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), "svcacc1").Return(nil, metadata.ErrUserNotFound)
	repo.EXPECT().GetServiceAccount(gomock.Any(), "svcacc1").Return(nil, metadata.ErrServiceAccountNotFound)
	repo.EXPECT().CreateServiceAccount(gomock.Any(), gomock.Any()).Return(nil)

	sa, err := svc.Create(context.Background(), &CreateServiceAccountRequest{
		AccessKey: "svcacc1",
		SecretKey: "topsecretpassword",
	})
	require.NoError(t, err)
	assert.Equal(t, "svcacc1", sa.AccessKey)
}

func TestCreate_AlreadyExists_AccessKeyTaken(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), "svcacc1").Return(&iam.User{AccessKey: "svcacc1"}, nil)

	_, err := svc.Create(context.Background(), &CreateServiceAccountRequest{
		AccessKey: "svcacc1",
		SecretKey: "topsecretpassword",
	})
	assert.ErrorIs(t, err, svcerrors.ErrServiceAccountAlreadyExists)
}

func TestCreate_WithParentUserByAccessKey(t *testing.T) {
	svc, repo := newMockService(t)
	parentUID := uuid.New()
	parentAK := "parentuser"

	// First call: uniqueness check for svcacc1 — not a user
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), "svcacc1").Return(nil, metadata.ErrUserNotFound)
	repo.EXPECT().GetServiceAccount(gomock.Any(), "svcacc1").Return(nil, metadata.ErrServiceAccountNotFound)
	// Second call: resolve parent
	repo.EXPECT().GetUserByAccessKey(gomock.Any(), parentAK).Return(&iam.User{UUID: parentUID, AccessKey: parentAK}, nil)
	repo.EXPECT().CreateServiceAccount(gomock.Any(), gomock.Any()).Return(nil)

	sa, err := svc.Create(context.Background(), &CreateServiceAccountRequest{
		AccessKey:  "svcacc1",
		SecretKey:  "topsecretpassword",
		ParentUser: &parentAK,
	})
	require.NoError(t, err)
	require.NotNil(t, sa.ParentUserUUID)
	assert.Equal(t, parentUID, *sa.ParentUserUUID)
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestGet_Success(t *testing.T) {
	svc, repo := newMockService(t)
	want := &iam.ServiceAccount{AccessKey: "svcacc1"}
	repo.EXPECT().GetServiceAccount(gomock.Any(), "svcacc1").Return(want, nil)

	got, err := svc.Get(context.Background(), "svcacc1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGet_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetServiceAccount(gomock.Any(), "missing").Return(nil, metadata.ErrServiceAccountNotFound)

	_, err := svc.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, svcerrors.ErrServiceAccountNotFound)
}

// ── GetByUUID ─────────────────────────────────────────────────────────────────

func TestGetByUUID_Success(t *testing.T) {
	svc, repo := newMockService(t)
	uid := uuid.New()
	want := &iam.ServiceAccount{UUID: uid}
	repo.EXPECT().GetServiceAccountByUUID(gomock.Any(), uid).Return(want, nil)

	got, err := svc.GetByUUID(context.Background(), uid)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestDelete_Success(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetServiceAccount(gomock.Any(), "svcacc1").Return(&iam.ServiceAccount{AccessKey: "svcacc1"}, nil)
	repo.EXPECT().DeleteServiceAccount(gomock.Any(), "svcacc1").Return(nil)

	assert.NoError(t, svc.Delete(context.Background(), "svcacc1"))
}

func TestDelete_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetServiceAccount(gomock.Any(), "missing").Return(nil, metadata.ErrServiceAccountNotFound)

	err := svc.Delete(context.Background(), "missing")
	assert.ErrorIs(t, err, svcerrors.ErrServiceAccountNotFound)
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestList_Success(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().ListServiceAccountKeys(gomock.Any()).Return([]string{"svc1", "svc2"}, nil)

	keys, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"svc1", "svc2"}, keys)
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestUpdate_SecretKey(t *testing.T) {
	svc, repo := newMockService(t)
	newSecret := "mynewlongsecretkey"
	repo.EXPECT().GetServiceAccount(gomock.Any(), "svcacc1").Return(
		&iam.ServiceAccount{AccessKey: "svcacc1", Status: iam.ServiceAcctStatusActive}, nil)
	repo.EXPECT().SaveServiceAccount(gomock.Any(), gomock.Any()).Return(nil)

	got, err := svc.Update(context.Background(), "svcacc1", &UpdateServiceAccountRequest{SecretKey: &newSecret})
	require.NoError(t, err)
	assert.Equal(t, newSecret, got.SecretKey)
}

func TestUpdate_InvalidStatus(t *testing.T) {
	svc, repo := newMockService(t)
	bad := iam.ServiceAcctStatus("bad")
	repo.EXPECT().GetServiceAccount(gomock.Any(), "svcacc1").Return(
		&iam.ServiceAccount{AccessKey: "svcacc1", Status: iam.ServiceAcctStatusActive}, nil)

	_, err := svc.Update(context.Background(), "svcacc1", &UpdateServiceAccountRequest{Status: &bad})
	assert.True(t, svcerrors.IsValidation(err))
}

// ── SetStatus ─────────────────────────────────────────────────────────────────

func TestSetStatus_Disable(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetServiceAccount(gomock.Any(), "svcacc1").Return(
		&iam.ServiceAccount{AccessKey: "svcacc1", Status: iam.ServiceAcctStatusActive}, nil)
	repo.EXPECT().SaveServiceAccount(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, sa *iam.ServiceAccount) error {
			assert.Equal(t, iam.ServiceAcctStatusDisabled, sa.Status)
			return nil
		})

	assert.NoError(t, svc.SetStatus(context.Background(), "svcacc1", iam.ServiceAcctStatusDisabled))
}
