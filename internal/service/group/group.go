package group

import (
	"context"
	"errors"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/pkg/iam"
)

// Service provides group management operations
type Service struct {
	metadata *metadata.Manager
}

// NewService creates a new group service
func NewService(m *metadata.Manager) *Service {
	return &Service{metadata: m}
}

// Create creates a new empty group
func (s *Service) Create(ctx context.Context, req *CreateGroupRequest) (*iam.Group, error) {
	if req.Name == "" {
		return nil, svcerrors.NewValidationError("Name", "group name is required")
	}

	if err := s.metadata.CreateGroup(ctx, req.Name); err != nil {
		if errors.Is(err, metadata.ErrGroupAlreadyExists) {
			return nil, svcerrors.ErrGroupAlreadyExists
		}
		return nil, err
	}

	return s.metadata.GetGroup(ctx, req.Name)
}

// Get retrieves a group by name
func (s *Service) Get(ctx context.Context, name string) (*iam.Group, error) {
	if name == "" {
		return nil, svcerrors.NewValidationError("Name", "group name is required")
	}

	g, err := s.metadata.GetGroup(ctx, name)
	if err != nil {
		if errors.Is(err, metadata.ErrGroupNotFound) {
			return nil, svcerrors.ErrGroupNotFound
		}
		return nil, err
	}

	return g, nil
}

// Delete deletes a group by name
func (s *Service) Delete(ctx context.Context, name string) error {
	if name == "" {
		return svcerrors.NewValidationError("Name", "group name is required")
	}

	// Verify the group exists first
	if _, err := s.Get(ctx, name); err != nil {
		return err
	}

	return s.metadata.DeleteGroup(ctx, name)
}

// List returns all group names
func (s *Service) List(ctx context.Context) ([]string, error) {
	return s.metadata.ListGroupNames(ctx)
}

// AddMember adds a user to a group (idempotent)
func (s *Service) AddMember(ctx context.Context, groupName, accessKey string) error {
	if groupName == "" {
		return svcerrors.NewValidationError("GroupName", "group name is required")
	}
	if accessKey == "" {
		return svcerrors.NewValidationError("AccessKey", "access key is required")
	}

	// Verify the group exists
	if _, err := s.Get(ctx, groupName); err != nil {
		return err
	}

	// Verify the user exists
	if _, err := s.metadata.GetUser(ctx, accessKey); err != nil {
		if errors.Is(err, metadata.ErrUserNotFound) {
			return svcerrors.ErrUserNotFound
		}
		return err
	}

	return s.metadata.AddUserToGroup(ctx, groupName, accessKey)
}

// RemoveMember removes a user from a group
func (s *Service) RemoveMember(ctx context.Context, groupName, accessKey string) error {
	if groupName == "" {
		return svcerrors.NewValidationError("GroupName", "group name is required")
	}
	if accessKey == "" {
		return svcerrors.NewValidationError("AccessKey", "access key is required")
	}

	// Verify the group exists
	if _, err := s.Get(ctx, groupName); err != nil {
		return err
	}

	return s.metadata.RemoveUserFromGroup(ctx, groupName, accessKey)
}

// AttachPolicy attaches a policy to a group (idempotent)
func (s *Service) AttachPolicy(ctx context.Context, groupName, policyName string) error {
	if groupName == "" {
		return svcerrors.NewValidationError("GroupName", "group name is required")
	}
	if policyName == "" {
		return svcerrors.NewValidationError("PolicyName", "policy name is required")
	}

	// Verify the group exists
	if _, err := s.Get(ctx, groupName); err != nil {
		return err
	}

	// Verify the policy exists
	if _, err := s.metadata.GetPolicy(ctx, policyName); err != nil {
		if errors.Is(err, metadata.ErrPolicyNotFound) {
			return svcerrors.ErrPolicyNotFound
		}
		return err
	}

	return s.metadata.AttachPolicyToGroup(ctx, groupName, policyName)
}

// DetachPolicy detaches a policy from a group
func (s *Service) DetachPolicy(ctx context.Context, groupName, policyName string) error {
	if groupName == "" {
		return svcerrors.NewValidationError("GroupName", "group name is required")
	}
	if policyName == "" {
		return svcerrors.NewValidationError("PolicyName", "policy name is required")
	}

	// Verify the group exists
	if _, err := s.Get(ctx, groupName); err != nil {
		return err
	}

	return s.metadata.DetachPolicyFromGroup(ctx, groupName, policyName)
}

// SetStatus enables or disables a group
func (s *Service) SetStatus(ctx context.Context, groupName string, status iam.GroupStatus) error {
	if groupName == "" {
		return svcerrors.NewValidationError("GroupName", "group name is required")
	}
	if err := status.Validate(); err != nil {
		return svcerrors.NewValidationError("Status", err.Error())
	}

	// Verify the group exists
	if _, err := s.Get(ctx, groupName); err != nil {
		return err
	}

	return s.metadata.SetGroupStatus(ctx, groupName, status)
}
