package policy

import (
	"context"
	"errors"
	"time"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/validation"
	"github.com/mallardduck/dirio/pkg/iam"
)

// Service provides policy management operations
type Service struct {
	metadataManager *metadata.Manager
}

// NewService creates a new policy service
func NewService(metadataManager *metadata.Manager) *Service {
	return &Service{
		metadataManager: metadataManager,
	}
}

// Create creates a new policy with validation
func (s *Service) Create(ctx context.Context, req *CreatePolicyRequest) (*iam.Policy, error) {
	// Validate inputs
	if err := validation.ValidatePolicyName(req.Name); err != nil {
		return nil, err
	}
	if err := validation.ValidatePolicyDocument(req.PolicyDocument); err != nil {
		return nil, err
	}

	// Check if policy already exists
	existing, err := s.metadataManager.GetPolicy(ctx, req.Name)
	if err == nil && existing != nil {
		return nil, svcerrors.ErrPolicyAlreadyExists
	}

	// Create policy with automatic field management
	now := time.Now()
	policy := &iam.Policy{
		Version:        iam.PolicyMetadataVersion,
		Name:           req.Name,
		PolicyDocument: req.PolicyDocument,
		CreateDate:     now,
		UpdateDate:     now,
	}

	// Persist policy
	if err := s.metadataManager.SavePolicy(ctx, policy); err != nil {
		return nil, err
	}

	return policy, nil
}

// Get retrieves a policy by name
func (s *Service) Get(ctx context.Context, name string) (*iam.Policy, error) {
	if err := validation.ValidatePolicyName(name); err != nil {
		return nil, err
	}

	policy, err := s.metadataManager.GetPolicy(ctx, name)
	if err != nil {
		if errors.Is(err, metadata.ErrPolicyNotFound) {
			return nil, svcerrors.ErrPolicyNotFound
		}
		return nil, err
	}

	return policy, nil
}

// Update updates the policy document of an existing policy
func (s *Service) Update(ctx context.Context, name string, req *UpdatePolicyRequest) (*iam.Policy, error) {
	// Validate inputs
	if err := validation.ValidatePolicyName(name); err != nil {
		return nil, err
	}
	if err := validation.ValidatePolicyDocument(req.PolicyDocument); err != nil {
		return nil, err
	}

	// Get existing policy
	policy, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}

	// Update policy document and timestamp
	policy.PolicyDocument = req.PolicyDocument
	policy.UpdateDate = time.Now()

	// Persist changes
	if err := s.metadataManager.SavePolicy(ctx, policy); err != nil {
		return nil, err
	}

	return policy, nil
}

// Delete deletes a policy by name
func (s *Service) Delete(ctx context.Context, name string) error {
	if err := validation.ValidatePolicyName(name); err != nil {
		return err
	}

	// Check if policy exists
	if _, err := s.Get(ctx, name); err != nil {
		return err
	}

	return s.metadataManager.DeletePolicy(ctx, name)
}

// List returns all policies
func (s *Service) List(ctx context.Context) (map[string]*metadata.Policy, error) {
	return s.metadataManager.GetPolicies(ctx)
}

// ListNames returns all policy names
func (s *Service) ListNames(ctx context.Context) ([]string, error) {
	return s.metadataManager.ListPolicyNames(ctx)
}
