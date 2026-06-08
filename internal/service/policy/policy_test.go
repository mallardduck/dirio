package policy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/policy/mock"
	"github.com/mallardduck/dirio/sdk/iam"
)

func newMockService(t *testing.T) (*Service, *mock.MockRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mock.NewMockRepository(ctrl)
	return NewService(repo), repo
}

var allowAllDoc = &iam.PolicyDocument{
	Version:   "2012-10-17",
	Statement: []iam.Statement{{Effect: "Allow", Action: []string{"s3:*"}, Resource: []string{"*"}}},
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestCreate_Success(t *testing.T) {
	svc, repo := newMockService(t)

	repo.EXPECT().GetPolicy(gomock.Any(), "ReadOnly").Return(nil, metadata.ErrPolicyNotFound)
	repo.EXPECT().SavePolicy(gomock.Any(), gomock.AssignableToTypeOf(&iam.Policy{})).Return(nil)

	p, err := svc.Create(context.Background(), &CreatePolicyRequest{Name: "ReadOnly", PolicyDocument: allowAllDoc})
	require.NoError(t, err)
	assert.Equal(t, "ReadOnly", p.Name)
	assert.False(t, p.CreateDate.IsZero())
}

func TestCreate_AlreadyExists(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetPolicy(gomock.Any(), "ReadOnly").Return(&iam.Policy{Name: "ReadOnly"}, nil)

	_, err := svc.Create(context.Background(), &CreatePolicyRequest{Name: "ReadOnly", PolicyDocument: allowAllDoc})
	assert.ErrorIs(t, err, svcerrors.ErrPolicyAlreadyExists)
}

func TestCreate_InvalidName(t *testing.T) {
	svc, _ := newMockService(t)
	_, err := svc.Create(context.Background(), &CreatePolicyRequest{Name: ""})
	assert.True(t, svcerrors.IsValidation(err))
}

func TestCreate_InvalidDocument(t *testing.T) {
	svc, _ := newMockService(t)
	_, err := svc.Create(context.Background(), &CreatePolicyRequest{
		Name:           "ValidName",
		PolicyDocument: &iam.PolicyDocument{},
	})
	assert.True(t, svcerrors.IsValidation(err))
}

func TestCreate_SaveError(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetPolicy(gomock.Any(), "MyPolicy").Return(nil, metadata.ErrPolicyNotFound)
	repo.EXPECT().SavePolicy(gomock.Any(), gomock.Any()).Return(errors.New("disk full"))

	_, err := svc.Create(context.Background(), &CreatePolicyRequest{Name: "MyPolicy", PolicyDocument: allowAllDoc})
	assert.EqualError(t, err, "disk full")
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestGet_Success(t *testing.T) {
	svc, repo := newMockService(t)
	want := &iam.Policy{Name: "ReadOnly"}
	repo.EXPECT().GetPolicy(gomock.Any(), "ReadOnly").Return(want, nil)

	got, err := svc.Get(context.Background(), "ReadOnly")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGet_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetPolicy(gomock.Any(), "Missing").Return(nil, metadata.ErrPolicyNotFound)

	_, err := svc.Get(context.Background(), "Missing")
	assert.ErrorIs(t, err, svcerrors.ErrPolicyNotFound)
}

func TestGet_InvalidName(t *testing.T) {
	svc, _ := newMockService(t)
	_, err := svc.Get(context.Background(), "")
	assert.True(t, svcerrors.IsValidation(err))
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestUpdate_Success(t *testing.T) {
	svc, repo := newMockService(t)
	existing := &iam.Policy{Name: "MyPolicy", CreateDate: time.Now()}
	newDoc := &iam.PolicyDocument{
		Version:   "2012-10-17",
		Statement: []iam.Statement{{Effect: "Deny", Action: []string{"s3:*"}, Resource: []string{"*"}}},
	}

	repo.EXPECT().GetPolicy(gomock.Any(), "MyPolicy").Return(existing, nil)
	repo.EXPECT().SavePolicy(gomock.Any(), gomock.Any()).Return(nil)

	got, err := svc.Update(context.Background(), "MyPolicy", &UpdatePolicyRequest{PolicyDocument: newDoc})
	require.NoError(t, err)
	assert.Equal(t, newDoc, got.PolicyDocument)
}

func TestUpdate_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetPolicy(gomock.Any(), "Missing").Return(nil, metadata.ErrPolicyNotFound)

	_, err := svc.Update(context.Background(), "Missing", &UpdatePolicyRequest{
		PolicyDocument: allowAllDoc,
	})
	assert.ErrorIs(t, err, svcerrors.ErrPolicyNotFound)
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestDelete_Success(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetPolicy(gomock.Any(), "MyPolicy").Return(&iam.Policy{Name: "MyPolicy"}, nil)
	repo.EXPECT().DeletePolicy(gomock.Any(), "MyPolicy").Return(nil)

	assert.NoError(t, svc.Delete(context.Background(), "MyPolicy"))
}

func TestDelete_BuiltinPolicy(t *testing.T) {
	svc, _ := newMockService(t)
	err := svc.Delete(context.Background(), "readwrite")
	assert.ErrorIs(t, err, svcerrors.ErrPolicyIsBuiltin)
}

func TestDelete_NotFound(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().GetPolicy(gomock.Any(), "CustomPolicy").Return(nil, metadata.ErrPolicyNotFound)

	err := svc.Delete(context.Background(), "CustomPolicy")
	assert.ErrorIs(t, err, svcerrors.ErrPolicyNotFound)
}

// ── List / ListNames ──────────────────────────────────────────────────────────

func TestListNames_Success(t *testing.T) {
	svc, repo := newMockService(t)
	repo.EXPECT().ListPolicyNames(gomock.Any()).Return([]string{"readonly", "readwrite"}, nil)

	names, err := svc.ListNames(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"readonly", "readwrite"}, names)
}

func TestList_Success(t *testing.T) {
	svc, repo := newMockService(t)
	policies := map[string]*metadata.Policy{"readonly": {Name: "readonly"}}
	repo.EXPECT().GetPolicies(gomock.Any()).Return(policies, nil)

	got, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, policies, got)
}
