package serviceaccount

import (
	"time"

	"github.com/mallardduck/dirio/pkg/iam"
)

// CreateServiceAccountRequest represents a request to create a new service account
type CreateServiceAccountRequest struct {
	AccessKey  string
	SecretKey  string
	ParentUser *string        // optional parent user access key (resolved to UUID internally)
	PolicyMode iam.PolicyMode // optional policy mode ("inherit" or "override"; empty = inherit)
	ExpiresAt  *time.Time     // optional expiration time
}

// UpdateServiceAccountRequest represents a request to update a service account
type UpdateServiceAccountRequest struct {
	SecretKey *string
	Status    *iam.ServiceAcctStatus
	ExpiresAt **time.Time // double pointer: nil = no change, non-nil = update (can clear with pointer to nil)
}

// ServiceAccount is a service-layer view of an IAM service account
type ServiceAccount = iam.ServiceAccount
