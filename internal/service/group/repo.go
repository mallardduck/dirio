package group

import (
	"context"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/sdk/iam"
)

//go:generate mockgen -source=repo.go -destination=mock/mock_repo.go -package=mock

// Repository is the persistence interface required by the group Service.
type Repository interface {
	CreateGroup(ctx context.Context, groupName string) error
	GetGroup(ctx context.Context, groupName string) (*metadata.Group, error)
	DeleteGroup(ctx context.Context, groupName string) error
	ListGroupNames(ctx context.Context) ([]string, error)
	GetUser(ctx context.Context, userUID uuid.UUID) (*metadata.User, error)
	GetUserByAccessKey(ctx context.Context, accessKey string) (*metadata.User, error)
	AddUserToGroup(ctx context.Context, groupName string, userUID uuid.UUID) error
	RemoveUserFromGroup(ctx context.Context, groupName string, userUID uuid.UUID) error
	AttachPolicyToGroup(ctx context.Context, groupName, policyName string) error
	DetachPolicyFromGroup(ctx context.Context, groupName, policyName string) error
	SetGroupStatus(ctx context.Context, groupName string, status iam.GroupStatus) error
	GetPolicy(ctx context.Context, name string) (*metadata.Policy, error)
}
