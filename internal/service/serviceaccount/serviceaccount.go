package serviceaccount

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/validation"
	"github.com/mallardduck/dirio/pkg/iam"
)

// Service provides service account management operations
type Service struct {
	metadata *metadata.Manager
}

// NewService creates a new service account service
func NewService(m *metadata.Manager) *Service {
	return &Service{metadata: m}
}

// Create creates a new service account
func (s *Service) Create(ctx context.Context, req *CreateServiceAccountRequest) (*iam.ServiceAccount, error) {
	if err := validation.ValidateAccessKey(req.AccessKey); err != nil {
		return nil, err
	}
	if err := validation.ValidateSecretKey(req.SecretKey); err != nil {
		return nil, err
	}

	// Check uniqueness across users and service accounts
	// TODO what about overlap with Admin accounts?
	if _, err := s.metadata.GetUserByAccessKey(ctx, req.AccessKey); err == nil {
		return nil, svcerrors.ErrServiceAccountAlreadyExists
	}
	if _, err := s.metadata.GetServiceAccount(ctx, req.AccessKey); err == nil {
		return nil, svcerrors.ErrServiceAccountAlreadyExists
	}

	// Resolve parent user → UUID.
	// ParentUserUUID takes precedence.
	var parentUserUUID *uuid.UUID
	if req.ParentUserUUID != nil {
		parentUserUUID = req.ParentUserUUID
	} else if req.ParentUser != nil && *req.ParentUser != "" {
		parentUser, err := s.metadata.GetUserByAccessKey(ctx, *req.ParentUser)
		if err != nil {
			return nil, fmt.Errorf("parent user %q not found: %w", *req.ParentUser, err)
		}
		parentUserUUID = &parentUser.UUID
	}

	sa := iam.NewServiceAccount(
		uuid.New(),
		req.AccessKey,
		req.SecretKey,
		req.AccessKey,
		parentUserUUID,
		req.PolicyMode,
		iam.ServiceAcctStatusActive,
		req.EmbeddedPolicyJSON,
		req.ExpiresAt,
	)

	if err := s.metadata.CreateServiceAccount(ctx, sa); err != nil {
		return nil, err
	}

	return sa, nil
}

// Get retrieves a service account by access key
func (s *Service) Get(ctx context.Context, accessKey string) (*iam.ServiceAccount, error) {
	if err := validation.ValidateAccessKey(accessKey); err != nil {
		return nil, err
	}

	sa, err := s.metadata.GetServiceAccount(ctx, accessKey)
	if err != nil {
		if errors.Is(err, metadata.ErrServiceAccountNotFound) {
			return nil, svcerrors.ErrServiceAccountNotFound
		}
		return nil, err
	}

	return sa, nil
}

// Delete deletes a service account by access key
func (s *Service) Delete(ctx context.Context, accessKey string) error {
	if err := validation.ValidateAccessKey(accessKey); err != nil {
		return err
	}

	// Verify it exists
	if _, err := s.Get(ctx, accessKey); err != nil {
		return err
	}

	return s.metadata.DeleteServiceAccount(ctx, accessKey)
}

// List returns all service account access keys
func (s *Service) List(ctx context.Context) ([]string, error) {
	return s.metadata.ListServiceAccountKeys(ctx)
}

// Update updates mutable fields of a service account
func (s *Service) Update(ctx context.Context, accessKey string, req *UpdateServiceAccountRequest) (*iam.ServiceAccount, error) {
	sa, err := s.Get(ctx, accessKey)
	if err != nil {
		return nil, err
	}

	if req.SecretKey != nil {
		if err := validation.ValidateSecretKey(*req.SecretKey); err != nil {
			return nil, err
		}
		sa.SecretKey = *req.SecretKey
	}

	if req.Status != nil {
		if err := req.Status.Validate(); err != nil {
			return nil, svcerrors.NewValidationError("Status", err.Error())
		}
		sa.Status = *req.Status
	}

	if req.ExpiresAt != nil {
		sa.ExpiresAt = *req.ExpiresAt
	}

	if req.EmbeddedPolicyJSON != nil {
		sa.EmbeddedPolicyJSON = *req.EmbeddedPolicyJSON
	}

	sa.UpdatedAt = time.Now()

	if err := s.metadata.SaveServiceAccount(ctx, sa); err != nil {
		return nil, err
	}

	return sa, nil
}

// SetStatus enables or disables a service account
func (s *Service) SetStatus(ctx context.Context, accessKey string, status iam.ServiceAcctStatus) error {
	req := &UpdateServiceAccountRequest{Status: &status}
	_, err := s.Update(ctx, accessKey, req)
	return err
}
