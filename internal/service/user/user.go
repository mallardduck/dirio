package user

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	validation2 "github.com/mallardduck/dirio/internal/service/validation"
	"github.com/mallardduck/dirio/pkg/iam"
)

// Service provides user management operations
type Service struct {
	metadataManager *metadata.Manager
}

// NewService creates a new user service
func NewService(metadataManager *metadata.Manager) *Service {
	return &Service{
		metadataManager: metadataManager,
	}
}

// Create creates a new user with validation
func (s *Service) Create(ctx context.Context, req *CreateUserRequest) (*iam.User, error) {
	if err := validation2.ValidateAccessKey(req.AccessKey); err != nil {
		return nil, err
	}
	if err := validation2.ValidateSecretKey(req.SecretKey); err != nil {
		return nil, err
	}
	if err := validation2.ValidateStatus(req.Status); err != nil {
		return nil, err
	}

	// Check if access key is already taken
	existing, err := s.metadataManager.GetUserByAccessKey(ctx, req.AccessKey)
	if err == nil && existing != nil {
		return nil, svcerrors.ErrUserAlreadyExists
	}

	status := req.Status
	if status == "" {
		status = iam.UserStatusActive
	}

	user := &iam.User{
		Version:          iam.UserMetadataVersion,
		AccessKey:        req.AccessKey,
		SecretKey:        req.SecretKey,
		Username:         req.AccessKey,
		Status:           status,
		UpdatedAt:        time.Now(),
		AttachedPolicies: []string{},
	}

	if err := s.metadataManager.CreateOrUpdateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Get retrieves a user by UUID.
func (s *Service) Get(ctx context.Context, userUID uuid.UUID) (*iam.User, error) {
	user, err := s.metadataManager.GetUser(ctx, userUID)
	if err != nil {
		if errors.Is(err, metadata.ErrUserNotFound) {
			return nil, svcerrors.ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

// GetByAccessKey retrieves a user by access key. This method exists for external
// API boundaries (e.g. the MinIO-compatible HTTP API) where access keys are the
// wire-format identifier. Internal code should prefer Get(uuid).
func (s *Service) GetByAccessKey(ctx context.Context, accessKey string) (*iam.User, error) {
	if err := validation2.ValidateAccessKey(accessKey); err != nil {
		return nil, err
	}
	user, err := s.metadataManager.GetUserByAccessKey(ctx, accessKey)
	if err != nil {
		if errors.Is(err, metadata.ErrUserNotFound) {
			return nil, svcerrors.ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

// Update updates mutable fields of an existing user.
func (s *Service) Update(ctx context.Context, userUID uuid.UUID, req *UpdateUserRequest) (*iam.User, error) {
	user, err := s.Get(ctx, userUID)
	if err != nil {
		return nil, err
	}

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

	user.UpdatedAt = time.Now()

	if err := s.metadataManager.CreateOrUpdateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Delete deletes a user by UUID.
func (s *Service) Delete(ctx context.Context, userUID uuid.UUID) error {
	if _, err := s.Get(ctx, userUID); err != nil {
		return err
	}
	return s.metadataManager.DeleteUser(ctx, userUID)
}

// List returns all user UUIDs.
func (s *Service) List(ctx context.Context) ([]uuid.UUID, error) {
	return s.metadataManager.ListUsers(ctx)
}

// AttachPolicy attaches a policy to a user (idempotent).
func (s *Service) AttachPolicy(ctx context.Context, userUID uuid.UUID, policyName string) error {
	if err := validation2.ValidatePolicyName(policyName); err != nil {
		return err
	}

	if _, err := s.metadataManager.GetPolicy(ctx, policyName); err != nil {
		if errors.Is(err, metadata.ErrPolicyNotFound) {
			return svcerrors.ErrPolicyNotFound
		}
		return err
	}

	user, err := s.Get(ctx, userUID)
	if err != nil {
		return err
	}

	if slices.Contains(user.AttachedPolicies, policyName) {
		return nil // idempotent
	}

	user.AttachedPolicies = append(user.AttachedPolicies, policyName)
	user.UpdatedAt = time.Now()

	return s.metadataManager.CreateOrUpdateUser(ctx, user)
}

// DetachPolicy detaches a policy from a user.
func (s *Service) DetachPolicy(ctx context.Context, userUID uuid.UUID, policyName string) error {
	if err := validation2.ValidatePolicyName(policyName); err != nil {
		return err
	}

	user, err := s.Get(ctx, userUID)
	if err != nil {
		return err
	}

	newPolicies := make([]string, 0, len(user.AttachedPolicies))
	for _, p := range user.AttachedPolicies {
		if p != policyName {
			newPolicies = append(newPolicies, p)
		}
	}

	user.AttachedPolicies = newPolicies
	user.UpdatedAt = time.Now()

	return s.metadataManager.CreateOrUpdateUser(ctx, user)
}

func (s *Service) GetGroups(ctx context.Context, uid uuid.UUID) ([]string, error) {
	return s.metadataManager.GetGroupNamesForUser(ctx, uid)
}
