package minio

import (
	"encoding/json"
	"time"
)

// AccountStatus mirrors madmin.AccountStatus — "enabled" | "disabled"
type AccountStatus string

const (
	AccountEnabled  AccountStatus = "enabled"
	AccountDisabled AccountStatus = "disabled"
)

// ServiceAccountInfo describes a single service account as returned by ListServiceAccounts.
type ServiceAccountInfo struct {
	AccessKey     string
	ParentUser    string
	AccountStatus string // "on" or "off"
	Name          string
	Expiration    *time.Time
}

// ServiceAccountsListResp is the response from ListServiceAccounts.
type ServiceAccountsListResp struct {
	Accounts []ServiceAccountInfo
}

// AddServiceAccountReq carries parameters for creating a service account.
type AddServiceAccountReq struct {
	TargetUser  string
	Name        string
	Description string
	Policy      json.RawMessage
	Expiration  *time.Time
}

// Credentials holds short-lived or service-account credentials.
type Credentials struct {
	AccessKey string
	SecretKey string
}

// ServiceAccountInfoResp is the response from InfoServiceAccount.
type ServiceAccountInfoResp struct {
	ParentUser    string
	AccountStatus string // "on" or "off"
	Name          string
	Description   string
	ImpliedPolicy bool
	Expiration    *time.Time
}

// UpdateServiceAccountReq carries parameters for modifying a service account.
type UpdateServiceAccountReq struct {
	NewName       string
	NewPolicy     json.RawMessage
	NewExpiration *time.Time
}

// UserInfo describes an IAM user.
type UserInfo struct {
	Status     AccountStatus
	PolicyName string
	MemberOf   []string
	UpdatedAt  time.Time
}

// PolicyInfo holds metadata and raw JSON for a named IAM policy.
type PolicyInfo struct {
	PolicyName string
	Policy     json.RawMessage
	CreateDate time.Time
	UpdateDate time.Time
}

// PolicyAssociationReq carries parameters for attaching or detaching a policy.
type PolicyAssociationReq struct {
	Policies []string
	User     string
	Group    string
}

// PolicyAssociationResp is returned by AttachPolicy and DetachPolicy.
type PolicyAssociationResp struct {
	PoliciesDetached []string
	PoliciesAttached []string
}

// BucketInfo describes a single S3 bucket.
type BucketInfo struct {
	Name      string
	CreatedAt time.Time
}

// ObjectInfo describes a single S3 object or common prefix.
// Size is -1 for common prefixes (virtual directories). Err is non-nil when
// a per-object error occurred during listing.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
	ContentType  string
	StorageClass string
	Err          error
}
