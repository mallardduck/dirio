// Package consoleapi defines the interface contract between the DirIO web console
// and the DirIO server. This package is the only coupling point: the console/
// package imports only this package, never internal/.
//
// Module path: github.com/mallardduck/dirio/api
// Package name stays consoleapi so all callers keep using consoleapi.Foo unchanged.
package consoleapi

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AdminUserUUID is the stable UUID for the built-in admin account.
// The console uses this to identify the admin user returned by ListUsers
// and to route SA parent assignments without an access-key lookup.
const AdminUserUUID = "badfc0de-fadd-fc0f-fee0-000dadbeef00"

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
	UpdatePolicy(ctx context.Context, name string, req UpdatePolicyRequest) (*Policy, error)
	DeletePolicy(ctx context.Context, name string) error
	AttachPolicy(ctx context.Context, policyName, accessKey string) error
	DetachPolicy(ctx context.Context, policyName, accessKey string) error

	// Groups
	ListGroups(ctx context.Context) ([]*Group, error)
	GetGroup(ctx context.Context, name string) (*Group, error)
	CreateGroup(ctx context.Context, req CreateGroupRequest) (*Group, error)
	DeleteGroup(ctx context.Context, name string) error
	AddGroupMember(ctx context.Context, groupName, userAccessKey string) error
	RemoveGroupMember(ctx context.Context, groupName, userUUID string) error
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
	CreateBucket(ctx context.Context, name, ownerAccessKey string) error
	DeleteBucket(ctx context.Context, name string) error
	ListBuckets(ctx context.Context) ([]*Bucket, error)
	GetBucket(ctx context.Context, bucket string) (*Bucket, error)
	GetBucketPolicy(ctx context.Context, bucket string) (string, error) // raw JSON
	SetBucketPolicy(ctx context.Context, bucket, policyJSON string) error

	// Objects
	ListObjects(ctx context.Context, bucket, prefix, delimiter string) ([]*ObjectInfo, error)
	GetObjectMetadata(ctx context.Context, bucket, key string) (*ObjectMetadata, error)
	GetObjectTags(ctx context.Context, bucket, key string) (map[string]string, error)
	SetObjectTags(ctx context.Context, bucket, key string, tags map[string]string) error
	DeleteObject(ctx context.Context, bucket, key string) error
	CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error
	GeneratePresignedURL(ctx context.Context, req GeneratePresignedURLRequest) (string, error)

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
	SecretKey        string    `json:"secretKey,omitempty"` // only populated when auto-generated on create
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
	AccessKey      string `json:"accessKey"`
	SecretKey      string `json:"secretKey"`
	GenerateSecret bool   `json:"generateSecret"` // if true, SecretKey is ignored and one is generated
}

// UpdateUserRequest is the input for updating a user.
type UpdateUserRequest struct {
	SecretKey      *string `json:"secretKey,omitempty"`
	GenerateSecret bool    `json:"generateSecret"`
}

// CreatePolicyRequest is the input for CreatePolicy.
type CreatePolicyRequest struct {
	Name           string `json:"name"`
	PolicyDocument string `json:"policyDocument"` // raw JSON string
}

// UpdatePolicyRequest is the input for UpdatePolicy.
type UpdatePolicyRequest struct {
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
	UUID               string     `json:"uuid"`
	AccessKey          string     `json:"accessKey"`
	SecretKey          string     `json:"secretKey,omitempty"`
	Username           string     `json:"username"`
	ParentUserUUID     string     `json:"parentUserUUID,omitempty"`
	ParentAccessKey    string     `json:"parentAccessKey,omitempty"`
	ParentUsername     string     `json:"parentUsername,omitempty"`
	PolicyMode         string     `json:"policyMode"`                   // "inherit" or "override"
	Status             string     `json:"status"`                       // "on" or "off"
	EmbeddedPolicyJSON string     `json:"embeddedPolicyJSON,omitempty"` // Raw IAM policy JSON (override mode)
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
}

// CreateServiceAccountRequest is the input for CreateServiceAccount.
type CreateServiceAccountRequest struct {
	AccessKey          string     `json:"accessKey,omitempty"`
	SecretKey          string     `json:"secretKey,omitempty"`
	ParentUserUUID     string     `json:"parentUserUUID,omitempty"`     // Parent user UUID (works for admin too)
	PolicyMode         string     `json:"policyMode,omitempty"`         // "inherit" or "override"
	EmbeddedPolicyJSON string     `json:"embeddedPolicyJSON,omitempty"` // Raw IAM policy JSON; required when PolicyMode == "override"
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
}

// UpdateServiceAccountRequest is the input for UpdateServiceAccount.
type UpdateServiceAccountRequest struct {
	SecretKey          *string     `json:"secretKey,omitempty"`
	EmbeddedPolicyJSON *string     `json:"embeddedPolicyJSON,omitempty"`
	ExpiresAt          **time.Time `json:"expiresAt,omitempty"`
}

// ObjectInfo represents a single object or common-prefix entry in a bucket listing.
type ObjectInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag"`
	LastModified time.Time `json:"lastModified"`
	ContentType  string    `json:"contentType,omitempty"`
	IsPrefix     bool      `json:"isPrefix,omitempty"` // true for "folder" entries
}

// ObjectMetadata represents the full metadata of a single object.
type ObjectMetadata struct {
	Key            string            `json:"key"`
	Size           int64             `json:"size"`
	ETag           string            `json:"etag"`
	LastModified   time.Time         `json:"lastModified"`
	ContentType    string            `json:"contentType"`
	CustomMetadata map[string]string `json:"customMetadata,omitempty"`
}

// GeneratePresignedURLRequest is the input for GeneratePresignedURL.
type GeneratePresignedURLRequest struct {
	AccessKey string        `json:"accessKey"`
	Bucket    string        `json:"bucket"`
	Key       string        `json:"key"`
	Expiry    time.Duration `json:"expiry"`
	BaseURL   string        `json:"baseURL"`
	Method    string        `json:"method"` // HTTP method, e.g. "GET" or "PUT"; defaults to "GET"
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
