package dioclient

import (
	compatminio "github.com/mallardduck/dirio/sdk/dioclient/compat/minio"
)

// S3 types — defined in compat/minio, re-exported as type aliases so callers
// never import the compat package directly.
type BucketInfo = compatminio.BucketInfo
type ObjectInfo = compatminio.ObjectInfo

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
