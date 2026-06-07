package dioclient

import (
	"context"
	"encoding/json"
	"fmt"

	compatminio "github.com/mallardduck/dirio/sdk/dioclient/compat/minio"
)

type ctxKey int

const ctxUseV1API ctxKey = iota

// WithV1API returns a context that tells InfoCannedPolicy to use the legacy V1 API
// instead of V2. Pass this when connecting to older MinIO or pre-fix DirIO builds.
func WithV1API(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxUseV1API, true)
}

// AdminClient is a connected DirIO/MinIO admin client. It is safe for concurrent use.
type AdminClient struct {
	proxy *compatminio.AdminProxy
}

// NewAdminClient creates an AdminClient for the given Config using the MinIO
// admin API. No network calls are made until the first operation.
func NewAdminClient(cfg Config) (*AdminClient, error) {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:9000"
	}
	proxy, err := compatminio.NewAdminProxy(cfg.Endpoint, cfg.AccessKey, cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("dioclient/admin: %w", err)
	}
	return &AdminClient{proxy: proxy}, nil
}

// --- Service accounts ---

// ListServiceAccounts returns service accounts for the given user.
// Pass user="" to list service accounts for the authenticated user.
func (a *AdminClient) ListServiceAccounts(ctx context.Context, user string) (ServiceAccountsListResp, error) {
	return a.proxy.ListServiceAccounts(ctx, user)
}

// AddServiceAccount creates a new service account and returns its credentials.
func (a *AdminClient) AddServiceAccount(ctx context.Context, opts AddServiceAccountReq) (Credentials, error) {
	return a.proxy.AddServiceAccount(ctx, opts)
}

// InfoServiceAccount returns metadata for a service account by access key.
func (a *AdminClient) InfoServiceAccount(ctx context.Context, accessKey string) (ServiceAccountInfoResp, error) {
	return a.proxy.InfoServiceAccount(ctx, accessKey)
}

// UpdateServiceAccount modifies an existing service account.
func (a *AdminClient) UpdateServiceAccount(ctx context.Context, accessKey string, opts UpdateServiceAccountReq) error {
	return a.proxy.UpdateServiceAccount(ctx, accessKey, opts)
}

// DeleteServiceAccount removes a service account.
func (a *AdminClient) DeleteServiceAccount(ctx context.Context, accessKey string) error {
	return a.proxy.DeleteServiceAccount(ctx, accessKey)
}

// --- IAM users ---

// ListUsers returns all IAM users.
func (a *AdminClient) ListUsers(ctx context.Context) (map[string]UserInfo, error) {
	return a.proxy.ListUsers(ctx)
}

// AddUser creates a new IAM user.
func (a *AdminClient) AddUser(ctx context.Context, accessKey, secretKey string) error {
	return a.proxy.AddUser(ctx, accessKey, secretKey)
}

// RemoveUser deletes an IAM user.
func (a *AdminClient) RemoveUser(ctx context.Context, accessKey string) error {
	return a.proxy.RemoveUser(ctx, accessKey)
}

// GetUserInfo returns info for an IAM user.
func (a *AdminClient) GetUserInfo(ctx context.Context, accessKey string) (UserInfo, error) {
	return a.proxy.GetUserInfo(ctx, accessKey)
}

// SetUserStatus enables or disables an IAM user.
func (a *AdminClient) SetUserStatus(ctx context.Context, accessKey string, status AccountStatus) error {
	return a.proxy.SetUserStatus(ctx, accessKey, status)
}

// --- IAM policies ---

// ListCannedPolicies returns all named policies as raw JSON.
func (a *AdminClient) ListCannedPolicies(ctx context.Context) (map[string]json.RawMessage, error) {
	return a.proxy.ListCannedPolicies(ctx)
}

// AddCannedPolicy creates or replaces a named policy.
func (a *AdminClient) AddCannedPolicy(ctx context.Context, name string, policyJSON []byte) error {
	return a.proxy.AddCannedPolicy(ctx, name, policyJSON)
}

// InfoCannedPolicy returns metadata and raw JSON for a named policy.
// By default it uses the V2 API (includes timestamps and PolicyName).
// Wrap ctx with WithV1API to use the legacy V1 API instead (for older servers).
func (a *AdminClient) InfoCannedPolicy(ctx context.Context, name string) (*PolicyInfo, error) {
	if ctx.Value(ctxUseV1API) == true {
		return a.proxy.InfoCannedPolicyV1(ctx, name)
	}
	return a.proxy.InfoCannedPolicyV2(ctx, name)
}

// DeleteCannedPolicy removes a named IAM policy.
func (a *AdminClient) DeleteCannedPolicy(ctx context.Context, name string) error {
	return a.proxy.DeleteCannedPolicy(ctx, name)
}

// AttachPolicy attaches a named policy to a user or group.
func (a *AdminClient) AttachPolicy(ctx context.Context, req PolicyAssociationReq) (PolicyAssociationResp, error) {
	return a.proxy.AttachPolicy(ctx, req)
}

// DetachPolicy detaches a named policy from a user or group.
func (a *AdminClient) DetachPolicy(ctx context.Context, req PolicyAssociationReq) (PolicyAssociationResp, error) {
	return a.proxy.DetachPolicy(ctx, req)
}
