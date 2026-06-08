package serviceaccount

import (
	"context"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
)

//go:generate mockgen -source=repo.go -destination=mock/mock_repo.go -package=mock

// Repository is the persistence interface required by the serviceaccount Service.
type Repository interface {
	GetUserByAccessKey(ctx context.Context, accessKey string) (*metadata.User, error)
	GetServiceAccount(ctx context.Context, accessKey string) (*metadata.ServiceAccount, error)
	CreateServiceAccount(ctx context.Context, sa *metadata.ServiceAccount) error
	SaveServiceAccount(ctx context.Context, sa *metadata.ServiceAccount) error
	DeleteServiceAccount(ctx context.Context, accessKey string) error
	ListServiceAccountKeys(ctx context.Context) ([]string, error)
	GetServiceAccountByUUID(ctx context.Context, saUUID uuid.UUID) (*metadata.ServiceAccount, error)
}
