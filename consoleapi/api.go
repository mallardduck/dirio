// Package consoleapi defines the interface contract between the DirIO web console
// and the DirIO server. This package is the only coupling point: the console/
// package imports only this package, never internal/.
package consoleapi

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// API defines the surface the console can call into the server.
// The adapter in internal/console/adapter.go implements this interface
// by calling the service layer directly (no HTTP round-trips).
type API interface {
	// Users
	ListUsers(ctx context.Context) ([]*User, error)
	GetUser(ctx context.Context, uuid string) (*User, error)
	GetUserSecret(ctx context.Context, uuid string) (string, error)
	CreateUser(ctx context.Context, req CreateUserRequest) (*User, error)
	UpdateUser(ctx context.Context, uuid string, req UpdateUserRequest) (*User, error)
	DeleteUser(ctx context.Context, uuid string) error
	SetUserStatus(ctx context.Context, uuid string, enabled bool) error

	// Policies
	ListPolicies(ctx context.Context) ([]*Policy, error)
	GetPolicy(ctx context.Context, name string) (*Policy, error)
	CreatePolicy(ctx context.Context, req CreatePolicyRequest) (*Policy, error)
	DeletePolicy(ctx context.Context, name string) error
	AttachPolicy(ctx context.Context, policyName, accessKey string) error
	DetachPolicy(ctx context.Context, policyName, accessKey string) error

	// Groups
	ListGroups(ctx context.Context) ([]*Group, error)
	GetGroup(ctx context.Context, name string) (*Group, error)
	CreateGroup(ctx context.Context, req CreateGroupRequest) (*Group, error)
	DeleteGroup(ctx context.Context, name string) error
	AddGroupMember(ctx context.Context, groupName, userUID string) error
	RemoveGroupMember(ctx context.Context, groupName, userUID string) error
	AttachGroupPolicy(ctx context.Context, groupName, policyName string) error
	DetachGroupPolicy(ctx context.Context, groupName, policyName string) error
	SetGroupStatus(ctx context.Context, groupName string, enabled bool) error

	// Service Accounts
	ListServiceAccounts(ctx context.Context) ([]*ServiceAccount, error)
	GetServiceAccount(ctx context.Context, uuid string) (*ServiceAccount, error)
	GetServiceAccountSecret(ctx context.Context, uuid string) (string, error)
	CreateServiceAccount(ctx context.Context, req CreateServiceAccountRequest) (*ServiceAccount, error)
	DeleteServiceAccount(ctx context.Context, uuid string) error
	UpdateServiceAccount(ctx context.Context, uuid string, req UpdateServiceAccountRequest) (*ServiceAccount, error)
	SetServiceAccountStatus(ctx context.Context, uuid string, enabled bool) error

	// Buckets
	ListBuckets(ctx context.Context) ([]*Bucket, error)
	GetBucket(ctx context.Context, bucket string) (*Bucket, error)
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
	UUID             string    `json:"uuid"`
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
	IsBuiltin      bool      `json:"isBuiltin,omitempty"` // true for system-defined policies
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

// UpdateUserRequest is the input for updating a user.
type UpdateUserRequest struct {
	SecretKey *string `json:"secretKey,omitempty"`
}

// CreatePolicyRequest is the input for CreatePolicy.
type CreatePolicyRequest struct {
	Name           string `json:"name"`
	PolicyDocument string `json:"policyDocument"` // raw JSON string
}

// Group represents an IAM group as seen by the console.
type Group struct {
	Name             string      `json:"name"`
	Members          []uuid.UUID `json:"members"`
	AttachedPolicies []string    `json:"attachedPolicies"`
	Status           string      `json:"status"` // "on" or "off"
	CreatedAt        time.Time   `json:"createdAt"`
	UpdatedAt        time.Time   `json:"updatedAt"`
}

// CreateGroupRequest is the input for CreateGroup.
type CreateGroupRequest struct {
	Name string `json:"name"`
}

// ServiceAccount represents a service account as seen by the console.
type ServiceAccount struct {
	UUID             string     `json:"uuid"`
	AccessKey        string     `json:"accessKey"`
	SecretKey        string     `json:"secretKey,omitempty"`
	Username         string     `json:"username"`
	ParentUserUUID   string     `json:"parentUserUUID,omitempty"`
	ParentAccessKey  string     `json:"parentAccessKey,omitempty"`
	PolicyMode       string     `json:"policyMode"` // "inherit" or "override"
	Status           string     `json:"status"`     // "on" or "off"
	AttachedPolicies []string   `json:"attachedPolicies,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
}

// CreateServiceAccountRequest is the input for CreateServiceAccount.
type CreateServiceAccountRequest struct {
	AccessKey  string     `json:"accessKey,omitempty"`
	SecretKey  string     `json:"secretKey,omitempty"`
	ParentUser string     `json:"parentUser,omitempty"` // Parent access key
	PolicyMode string     `json:"policyMode,omitempty"` // "inherit" or "override"
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
}

// UpdateServiceAccountRequest is the input for UpdateServiceAccount.
type UpdateServiceAccountRequest struct {
	SecretKey *string     `json:"secretKey,omitempty"`
	ExpiresAt **time.Time `json:"expiresAt,omitempty"`
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
