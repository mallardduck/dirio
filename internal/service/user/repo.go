package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
)

//go:generate mockgen -source=repo.go -destination=mock/mock_repo.go -package=mock

// Repository is the persistence interface required by the user Service.
type Repository interface {
	GetUserByAccessKey(ctx context.Context, accessKey string) (*metadata.User, error)
	CreateOrUpdateUser(ctx context.Context, user *metadata.User) error
	GetUser(ctx context.Context, userUID uuid.UUID) (*metadata.User, error)
	GetGroupNamesForUser(ctx context.Context, userUID uuid.UUID) ([]string, error)
	RemoveUserFromGroup(ctx context.Context, groupName string, userUID uuid.UUID) error
	DeleteUser(ctx context.Context, userUID uuid.UUID) error
	ListUsers(ctx context.Context) ([]uuid.UUID, error)
	GetPolicy(ctx context.Context, name string) (*metadata.Policy, error)
}
