package dioclient

import (
	"context"
	"encoding/json"
)

// adminBackend abstracts the wire protocol used to talk to the admin API.
// The only current implementation is *compat/minio.AdminProxy (madmin-go).
// Replacing madmin-go means writing a new struct that satisfies this interface;
// AdminClient and all callers above it are unaffected.
type adminBackend interface {
	// Service accounts
	ListServiceAccounts(ctx context.Context, user string) (ServiceAccountsListResp, error)
	AddServiceAccount(ctx context.Context, req AddServiceAccountReq) (Credentials, error)
	InfoServiceAccount(ctx context.Context, accessKey string) (ServiceAccountInfoResp, error)
	UpdateServiceAccount(ctx context.Context, accessKey string, req UpdateServiceAccountReq) error
	DeleteServiceAccount(ctx context.Context, accessKey string) error

	// IAM users
	ListUsers(ctx context.Context) (map[string]UserInfo, error)
	AddUser(ctx context.Context, accessKey, secretKey string) error
	RemoveUser(ctx context.Context, accessKey string) error
	GetUserInfo(ctx context.Context, accessKey string) (UserInfo, error)
	SetUserStatus(ctx context.Context, accessKey string, status AccountStatus) error

	// IAM policies
	ListCannedPolicies(ctx context.Context) (map[string]json.RawMessage, error)
	AddCannedPolicy(ctx context.Context, name string, policyJSON []byte) error
	InfoCannedPolicyV1(ctx context.Context, name string) (*PolicyInfo, error)
	InfoCannedPolicyV2(ctx context.Context, name string) (*PolicyInfo, error)
	DeleteCannedPolicy(ctx context.Context, name string) error
	AttachPolicy(ctx context.Context, req PolicyAssociationReq) (PolicyAssociationResp, error)
	DetachPolicy(ctx context.Context, req PolicyAssociationReq) (PolicyAssociationResp, error)
}
