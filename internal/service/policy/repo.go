package policy

import (
	"context"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
)

//go:generate mockgen -source=repo.go -destination=mock/mock_repo.go -package=mock

// Repository is the persistence interface required by the policy Service.
type Repository interface {
	GetPolicy(ctx context.Context, name string) (*metadata.Policy, error)
	SavePolicy(ctx context.Context, policy *metadata.Policy) error
	DeletePolicy(ctx context.Context, name string) error
	GetPolicies(ctx context.Context) (map[string]*metadata.Policy, error)
	ListPolicyNames(ctx context.Context) ([]string, error)
}
