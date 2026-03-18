package serviceaccount

import (
	"time"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/pkg/iam"
)

// CreateServiceAccountRequest represents a request to create a new service account
type CreateServiceAccountRequest struct {
	AccessKey          string
	SecretKey          string
	ParentUser         *string        // optional parent user access key (resolved to UUID internally)
	ParentUserUUID     *uuid.UUID     // optional: skip access-key lookup and use this UUID directly (e.g. for admin)
	PolicyMode         iam.PolicyMode // optional policy mode ("inherit" or "override"; empty = inherit)
	EmbeddedPolicyJSON string         // raw IAM policy JSON; required when PolicyMode == "override"
	ExpiresAt          *time.Time     // optional expiration time
}

// UpdateServiceAccountRequest represents a request to update a service account
type UpdateServiceAccountRequest struct {
	SecretKey          *string
	Status             *iam.ServiceAcctStatus
	EmbeddedPolicyJSON *string     // nil = no change; non-nil (including "") = update
	ExpiresAt          **time.Time // double pointer: nil = no change, non-nil = update (can clear with pointer to nil)
}

// ServiceAccount is a service-layer view of an IAM service account
type ServiceAccount = iam.ServiceAccount
