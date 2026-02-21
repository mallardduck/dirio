// Package consoleapi defines the interface contract between the DirIO web console
// and the DirIO server. This package is the only coupling point: the console/
// package imports only this package, never internal/.
package consoleapi

import (
	"context"
	"time"
)

// API defines the surface the console can call into the server.
// The adapter in internal/console/adapter.go implements this interface
// by calling the service layer directly (no HTTP round-trips).
type API interface {
	// Users
	ListUsers(ctx context.Context) ([]*User, error)
	GetUser(ctx context.Context, accessKey string) (*User, error)
	CreateUser(ctx context.Context, req CreateUserRequest) (*User, error)
	DeleteUser(ctx context.Context, accessKey string) error
	SetUserStatus(ctx context.Context, accessKey string, enabled bool) error

	// Policies
	ListPolicies(ctx context.Context) ([]*Policy, error)
	GetPolicy(ctx context.Context, name string) (*Policy, error)
	CreatePolicy(ctx context.Context, req CreatePolicyRequest) (*Policy, error)
	DeletePolicy(ctx context.Context, name string) error
	AttachPolicy(ctx context.Context, policyName, accessKey string) error
	DetachPolicy(ctx context.Context, policyName, accessKey string) error

	// Buckets
	ListBuckets(ctx context.Context) ([]*Bucket, error)
	GetBucketPolicy(ctx context.Context, bucket string) (string, error) // raw JSON
	SetBucketPolicy(ctx context.Context, bucket, policyJSON string) error

	// Ownership (DirIO-specific — not reachable via mc or S3 clients)
	GetBucketOwner(ctx context.Context, bucket string) (*Owner, error)
	TransferBucketOwnership(ctx context.Context, bucket, newOwnerAccessKey string) error
	GetObjectOwner(ctx context.Context, bucket, key string) (*Owner, error)

	// Policy Observability (DirIO-specific)
	GetEffectivePermissions(ctx context.Context, accessKey, bucket string) (*EffectivePermissions, error)
	SimulateRequest(ctx context.Context, req SimulateRequest) (*SimulateResult, error)
}

// --- Request / Response types ------------------------------------------------

// User represents a user as seen by the console.
type User struct {
	AccessKey        string    `json:"accessKey"`
	Username         string    `json:"username"`
	Status           string    `json:"status"` // "on" or "off"
	AttachedPolicies []string  `json:"attachedPolicies"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// Policy represents an IAM policy as seen by the console.
type Policy struct {
	Name           string    `json:"name"`
	PolicyDocument string    `json:"policyDocument"` // raw JSON string
	CreateDate     time.Time `json:"createDate"`
	UpdateDate     time.Time `json:"updateDate"`
}

// Bucket represents a bucket as seen by the console.
type Bucket struct {
	Name      string    `json:"name"`
	OwnerUUID string    `json:"ownerUUID"` // empty string means admin-only
	Owner     *Owner    `json:"owner,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Owner represents the owner of a bucket or object.
type Owner struct {
	UUID      string `json:"uuid"`
	AccessKey string `json:"accessKey"` // empty if admin or unknown
	Username  string `json:"username"`
}

// CreateUserRequest is the input for CreateUser.
type CreateUserRequest struct {
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

// CreatePolicyRequest is the input for CreatePolicy.
type CreatePolicyRequest struct {
	Name           string `json:"name"`
	PolicyDocument string `json:"policyDocument"` // raw JSON string
}

// EffectivePermissions shows the evaluated access for a user on a bucket.
type EffectivePermissions struct {
	AccessKey      string   `json:"accessKey"`
	Bucket         string   `json:"bucket"`
	AllowedActions []string `json:"allowedActions"`
	DeniedActions  []string `json:"deniedActions"`
}

// SimulateRequest is the input for the policy simulator.
type SimulateRequest struct {
	AccessKey string `json:"accessKey"`
	Bucket    string `json:"bucket"`
	Action    string `json:"action"`
	Key       string `json:"key,omitempty"`
}

// SimulateResult is the outcome of a policy simulation.
type SimulateResult struct {
	Allowed     bool   `json:"allowed"`
	Reason      string `json:"reason"`
	MatchedRule string `json:"matchedRule,omitempty"`
}
