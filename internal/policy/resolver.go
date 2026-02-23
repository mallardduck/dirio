package policy

import (
	"context"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/pkg/iam"
)

// Resolver fetches IAM policy documents and user policy lists on demand.
// It is called by the Engine during [Engine.Evaluate] to implement IAM policy evaluation.
type Resolver interface {
	// GetPolicyDocument returns the parsed policy document for the named policy.
	GetPolicyDocument(ctx context.Context, name string) (*iam.PolicyDocument, error)

	// GetUserPolicyNamesByUUID returns the list of policy names attached to the user with the given UUID.
	GetUserPolicyNamesByUUID(ctx context.Context, userUUID uuid.UUID) ([]string, error)

	// GetGroupPoliciesForUser returns the union of all policy names attached to every active group
	// the user with the given UUID belongs to.
	GetGroupPoliciesForUser(ctx context.Context, userUUID uuid.UUID) ([]string, error)
}

// MetadataResolver implements Resolver using the metadata.Manager.
type MetadataResolver struct {
	manager *metadata.Manager
}

// NewMetadataResolver creates a new MetadataResolver backed by the given metadata.Manager.
func NewMetadataResolver(manager *metadata.Manager) *MetadataResolver {
	return &MetadataResolver{manager: manager}
}

// GetPolicyDocument returns the policy document for the named policy.
func (r *MetadataResolver) GetPolicyDocument(ctx context.Context, name string) (*iam.PolicyDocument, error) {
	policy, err := r.manager.GetPolicy(ctx, name)
	if err != nil {
		return nil, err
	}
	return policy.PolicyDocument, nil
}

// GetUserPolicyNamesByUUID returns the attached policy names for the user with the given UUID.
func (r *MetadataResolver) GetUserPolicyNamesByUUID(ctx context.Context, userUUID uuid.UUID) ([]string, error) {
	user, err := r.manager.GetUserByUUID(ctx, userUUID)
	if err != nil {
		return nil, err
	}
	return user.AttachedPolicies, nil
}

// GetGroupPoliciesForUser returns all policy names from every active group the user belongs to.
func (r *MetadataResolver) GetGroupPoliciesForUser(ctx context.Context, userUUID uuid.UUID) ([]string, error) {
	groupNames, err := r.manager.GetGroupNamesForUser(ctx, userUUID)
	if err != nil {
		return nil, err
	}

	var policyNames []string
	for _, groupName := range groupNames {
		g, err := r.manager.GetGroup(ctx, groupName)
		if err != nil || !g.Status.IsActive() {
			continue // skip missing or disabled groups
		}
		policyNames = append(policyNames, g.AttachedPolicies...)
	}
	return policyNames, nil
}
