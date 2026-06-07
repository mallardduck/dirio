package dioclient

import (
	"time"

	compatminio "github.com/mallardduck/dirio/sdk/dioclient/compat/minio"
)

// BucketInfo describes a single S3 bucket.
type BucketInfo struct {
	Name      string
	CreatedAt time.Time
}

// ObjectInfo describes a single S3 object.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
	ContentType  string
	StorageClass string
}

// Admin and IAM types — defined in compat/minio, re-exported as type aliases so
// callers never need to import the compat package directly. When DirIO-native admin
// types replace madmin-go, only this file and compat/minio need to change.

type AccountStatus = compatminio.AccountStatus

const (
	AccountEnabled  = compatminio.AccountEnabled
	AccountDisabled = compatminio.AccountDisabled
)

type ServiceAccountInfo = compatminio.ServiceAccountInfo
type ServiceAccountsListResp = compatminio.ServiceAccountsListResp
type AddServiceAccountReq = compatminio.AddServiceAccountReq
type Credentials = compatminio.Credentials
type ServiceAccountInfoResp = compatminio.ServiceAccountInfoResp
type UpdateServiceAccountReq = compatminio.UpdateServiceAccountReq
type UserInfo = compatminio.UserInfo
type PolicyInfo = compatminio.PolicyInfo
type PolicyAssociationReq = compatminio.PolicyAssociationReq
type PolicyAssociationResp = compatminio.PolicyAssociationResp
