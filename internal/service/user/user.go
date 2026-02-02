package user

import (
	"context"
	"errors"
	"time"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	validation2 "github.com/mallardduck/dirio/internal/service/validation"
	"github.com/mallardduck/dirio/pkg/iam"
)

// Service provides user management operations
type Service struct {
	metadata *metadata.Manager
}

// NewService creates a new user service
func NewService(metadata *metadata.Manager) *Service {
	return &Service{
		metadata: metadata,
	}
}

// Create creates a new user with validation
func (s *Service) Create(ctx context.Context, req *CreateUserRequest) (*iam.User, error) {
	// Validate inputs
	if err := validation2.ValidateAccessKey(req.AccessKey); err != nil {
		return nil, err
	}
	if err := validation2.ValidateSecretKey(req.SecretKey); err != nil {
		return nil, err
	}
	if err := validation2.ValidateStatus(req.Status); err != nil {
		return nil, err
	}

	// Check if user already exists
	existing, err := s.metadata.GetUser(ctx, req.AccessKey)
	if err == nil && existing != nil {
		return nil, svcerrors.ErrUserAlreadyExists
	}

	// Set default status if not provided
	status := req.Status
	if status == "" {
		status = "on"
	}

	// Create user with automatic field management
	user := &iam.User{
		Version:          iam.UserMetadataVersion,
		AccessKey:        req.AccessKey,
		SecretKey:        req.SecretKey,
		Username:         req.AccessKey,
		Status:           status,
		UpdatedAt:        time.Now(),
		AttachedPolicies: []string{},
	}

	// Persist user
	if err := s.metadata.CreateOrUpdateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Get retrieves a user by access key
func (s *Service) Get(ctx context.Context, accessKey string) (*iam.User, error) {
	if err := validation2.ValidateAccessKey(accessKey); err != nil {
		return nil, err
	}

	user, err := s.metadata.GetUser(ctx, accessKey)
	if err != nil {
		if errors.Is(err, metadata.ErrUserNotFound) {
			return nil, svcerrors.ErrUserNotFound
		}
		return nil, err
	}

	return user, nil
}

// Update updates mutable fields of an existing user
func (s *Service) Update(ctx context.Context, accessKey string, req *UpdateUserRequest) (*iam.User, error) {
	// Validate access key
	if err := validation2.ValidateAccessKey(accessKey); err != nil {
		return nil, err
	}

	// Get existing user
	user, err := s.Get(ctx, accessKey)
	if err != nil {
		return nil, err
	}

	// Update mutable fields
	if req.SecretKey != nil {
		if err := validation2.ValidateSecretKey(*req.SecretKey); err != nil {
			return nil, err
		}
		user.SecretKey = *req.SecretKey
	}

	if req.Status != nil {
		if err := validation2.ValidateStatus(*req.Status); err != nil {
			return nil, err
		}
		user.Status = *req.Status
	}

	if req.AttachedPolicies != nil {
		user.AttachedPolicies = *req.AttachedPolicies
	}

	// Update timestamp
	user.UpdatedAt = time.Now()

	// Persist changes
	if err := s.metadata.CreateOrUpdateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Delete deletes a user by access key
func (s *Service) Delete(ctx context.Context, accessKey string) error {
	if err := validation2.ValidateAccessKey(accessKey); err != nil {
		return err
	}

	// Check if user exists
	if _, err := s.Get(ctx, accessKey); err != nil {
		return err
	}

	return s.metadata.DeleteUser(ctx, accessKey)
}

// List returns all user access keys
func (s *Service) List(ctx context.Context) ([]string, error) {
	return s.metadata.ListUsers(ctx)
}

// AttachPolicy attaches a policy to a user (idempotent)
func (s *Service) AttachPolicy(ctx context.Context, accessKey, policyName string) error {
	if err := validation2.ValidateAccessKey(accessKey); err != nil {
		return err
	}
	if err := validation2.ValidatePolicyName(policyName); err != nil {
		return err
	}

	// Get existing user
	user, err := s.Get(ctx, accessKey)
	if err != nil {
		return err
	}

	// Check if policy is already attached
	for _, p := range user.AttachedPolicies {
		if p == policyName {
			return nil // Already attached, idempotent
		}
	}

	// Attach policy
	user.AttachedPolicies = append(user.AttachedPolicies, policyName)
	user.UpdatedAt = time.Now()

	return s.metadata.CreateOrUpdateUser(ctx, user)
}

// DetachPolicy detaches a policy from a user
func (s *Service) DetachPolicy(ctx context.Context, accessKey, policyName string) error {
	if err := validation2.ValidateAccessKey(accessKey); err != nil {
		return err
	}
	if err := validation2.ValidatePolicyName(policyName); err != nil {
		return err
	}

	// Get existing user
	user, err := s.Get(ctx, accessKey)
	if err != nil {
		return err
	}

	// Remove policy from attached policies
	newPolicies := make([]string, 0, len(user.AttachedPolicies))
	for _, p := range user.AttachedPolicies {
		if p != policyName {
			newPolicies = append(newPolicies, p)
		}
	}

	user.AttachedPolicies = newPolicies
	user.UpdatedAt = time.Now()

	return s.metadata.CreateOrUpdateUser(ctx, user)
}
