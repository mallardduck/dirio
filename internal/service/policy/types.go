package policy

import "github.com/mallardduck/dirio/pkg/iam"

// CreatePolicyRequest represents a request to create a new policy
type CreatePolicyRequest struct {
	Name           string
	PolicyDocument *iam.PolicyDocument
}

// UpdatePolicyRequest represents a request to update an existing policy
type UpdatePolicyRequest struct {
	PolicyDocument *iam.PolicyDocument
}
