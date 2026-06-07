// Package minio is the madmin-go quarantine zone within the dioclient SDK.
//
// This is the ONLY package in sdk/ that may import github.com/minio/madmin-go.
// All other packages in sdk/ use the native types defined in types.go, which are
// re-exported from sdk/dioclient via type aliases. When DirIO-native admin types
// are complete, this package can be dropped and madmin-go removed from sdk/go.mod.
package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// AdminProxy wraps madmin.AdminClient with methods that return native dioclient types.
// Construct one via NewAdminProxy; it is safe for concurrent use.
type AdminProxy struct {
	mc *madmin.AdminClient
}

// NewAdminProxy creates an AdminProxy from an endpoint URL and credentials.
func NewAdminProxy(endpoint, accessKey, secretKey string) (*AdminProxy, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("admin proxy: invalid endpoint %q: %w", endpoint, err)
	}
	secure := u.Scheme == "https"
	mc, err := madmin.NewWithOptions(u.Host, &madmin.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("admin proxy: %w", err)
	}
	return &AdminProxy{mc: mc}, nil
}

// --- Service accounts ---

func (p *AdminProxy) ListServiceAccounts(ctx context.Context, user string) (ServiceAccountsListResp, error) {
	resp, err := p.mc.ListServiceAccounts(ctx, user)
	if err != nil {
		return ServiceAccountsListResp{}, err
	}
	accounts := make([]ServiceAccountInfo, len(resp.Accounts))
	for i, a := range resp.Accounts {
		accounts[i] = ServiceAccountInfo{
			AccessKey:     a.AccessKey,
			ParentUser:    a.ParentUser,
			AccountStatus: a.AccountStatus,
			Name:          a.Name,
			Expiration:    a.Expiration,
		}
	}
	return ServiceAccountsListResp{Accounts: accounts}, nil
}

func (p *AdminProxy) AddServiceAccount(ctx context.Context, req AddServiceAccountReq) (Credentials, error) {
	creds, err := p.mc.AddServiceAccount(ctx, madmin.AddServiceAccountReq{
		TargetUser:  req.TargetUser,
		Name:        req.Name,
		Description: req.Description,
		Policy:      req.Policy,
		Expiration:  req.Expiration,
	})
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{AccessKey: creds.AccessKey, SecretKey: creds.SecretKey}, nil
}

func (p *AdminProxy) InfoServiceAccount(ctx context.Context, accessKey string) (ServiceAccountInfoResp, error) {
	info, err := p.mc.InfoServiceAccount(ctx, accessKey)
	if err != nil {
		return ServiceAccountInfoResp{}, err
	}
	return ServiceAccountInfoResp{
		ParentUser:    info.ParentUser,
		AccountStatus: info.AccountStatus,
		Name:          info.Name,
		Description:   info.Description,
		ImpliedPolicy: info.ImpliedPolicy,
		Expiration:    info.Expiration,
	}, nil
}

func (p *AdminProxy) UpdateServiceAccount(ctx context.Context, accessKey string, req UpdateServiceAccountReq) error {
	return p.mc.UpdateServiceAccount(ctx, accessKey, madmin.UpdateServiceAccountReq{
		NewName:       req.NewName,
		NewPolicy:     req.NewPolicy,
		NewExpiration: req.NewExpiration,
	})
}

func (p *AdminProxy) DeleteServiceAccount(ctx context.Context, accessKey string) error {
	return p.mc.DeleteServiceAccount(ctx, accessKey)
}

// --- IAM users ---

func (p *AdminProxy) ListUsers(ctx context.Context) (map[string]UserInfo, error) {
	raw, err := p.mc.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]UserInfo, len(raw))
	for k, u := range raw {
		out[k] = UserInfo{
			Status:     AccountStatus(u.Status),
			PolicyName: u.PolicyName,
			MemberOf:   u.MemberOf,
			UpdatedAt:  u.UpdatedAt,
		}
	}
	return out, nil
}

func (p *AdminProxy) AddUser(ctx context.Context, accessKey, secretKey string) error {
	return p.mc.AddUser(ctx, accessKey, secretKey)
}

func (p *AdminProxy) RemoveUser(ctx context.Context, accessKey string) error {
	return p.mc.RemoveUser(ctx, accessKey)
}

func (p *AdminProxy) GetUserInfo(ctx context.Context, accessKey string) (UserInfo, error) {
	u, err := p.mc.GetUserInfo(ctx, accessKey)
	if err != nil {
		return UserInfo{}, err
	}
	return UserInfo{
		Status:     AccountStatus(u.Status),
		PolicyName: u.PolicyName,
		MemberOf:   u.MemberOf,
		UpdatedAt:  u.UpdatedAt,
	}, nil
}

func (p *AdminProxy) SetUserStatus(ctx context.Context, accessKey string, status AccountStatus) error {
	return p.mc.SetUserStatus(ctx, accessKey, madmin.AccountStatus(status))
}

// --- IAM policies ---

func (p *AdminProxy) ListCannedPolicies(ctx context.Context) (map[string]json.RawMessage, error) {
	return p.mc.ListCannedPolicies(ctx)
}

func (p *AdminProxy) AddCannedPolicy(ctx context.Context, name string, policyJSON []byte) error {
	return p.mc.AddCannedPolicy(ctx, name, policyJSON)
}

// InfoCannedPolicyV1 uses the deprecated V1 admin API (for legacy server compatibility).
func (p *AdminProxy) InfoCannedPolicyV1(ctx context.Context, name string) (*PolicyInfo, error) {
	raw, err := p.mc.InfoCannedPolicy(ctx, name) //nolint:staticcheck // deprecated V1 API used intentionally for legacy server compatibility
	if err != nil {
		return nil, err
	}
	return &PolicyInfo{PolicyName: name, Policy: json.RawMessage(raw)}, nil
}

// InfoCannedPolicyV2 uses the V2 admin API (includes timestamps and PolicyName).
func (p *AdminProxy) InfoCannedPolicyV2(ctx context.Context, name string) (*PolicyInfo, error) {
	info, err := p.mc.InfoCannedPolicyV2(ctx, name)
	if err != nil {
		return nil, err
	}
	return &PolicyInfo{
		PolicyName: info.PolicyName,
		Policy:     info.Policy,
		CreateDate: info.CreateDate,
		UpdateDate: info.UpdateDate,
	}, nil
}

func (p *AdminProxy) DeleteCannedPolicy(ctx context.Context, name string) error {
	return p.mc.RemoveCannedPolicy(ctx, name)
}

func (p *AdminProxy) AttachPolicy(ctx context.Context, req PolicyAssociationReq) (PolicyAssociationResp, error) {
	resp, err := p.mc.AttachPolicy(ctx, madmin.PolicyAssociationReq{
		Policies: req.Policies,
		User:     req.User,
		Group:    req.Group,
	})
	if err != nil {
		return PolicyAssociationResp{}, err
	}
	return PolicyAssociationResp{
		PoliciesDetached: resp.PoliciesDetached,
		PoliciesAttached: resp.PoliciesAttached,
	}, nil
}

func (p *AdminProxy) DetachPolicy(ctx context.Context, req PolicyAssociationReq) (PolicyAssociationResp, error) {
	resp, err := p.mc.DetachPolicy(ctx, madmin.PolicyAssociationReq{
		Policies: req.Policies,
		User:     req.User,
		Group:    req.Group,
	})
	if err != nil {
		return PolicyAssociationResp{}, err
	}
	return PolicyAssociationResp{
		PoliciesDetached: resp.PoliciesDetached,
		PoliciesAttached: resp.PoliciesAttached,
	}, nil
}
