// Package console provides the adapter that implements consoleapi.API by calling
// the DirIO service layer directly (no HTTP round-trips).
package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mallardduck/dirio/consoleapi"
	"github.com/mallardduck/dirio/internal/service"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	svcpolicy "github.com/mallardduck/dirio/internal/service/policy"
	svcuser "github.com/mallardduck/dirio/internal/service/user"
	"github.com/mallardduck/dirio/pkg/iam"
)

// ErrNotImplemented is returned for API methods not yet backed by the service layer.
var ErrNotImplemented = errors.New("not implemented")

// Adapter implements consoleapi.API via the service layer.
type Adapter struct {
	services *service.ServicesFactory
}

// NewAdapter creates an Adapter backed by the given service factory.
func NewAdapter(services *service.ServicesFactory) *Adapter {
	return &Adapter{services: services}
}

// --- Users -------------------------------------------------------------------

func (a *Adapter) ListUsers(ctx context.Context) ([]*consoleapi.User, error) {
	keys, err := a.services.User().List(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]*consoleapi.User, 0, len(keys))
	for _, key := range keys {
		u, err := a.services.User().Get(ctx, key)
		if err != nil {
			continue // skip users that can't be fetched
		}
		users = append(users, iamUserToConsole(u))
	}

	return users, nil
}

func (a *Adapter) GetUser(ctx context.Context, accessKey string) (*consoleapi.User, error) {
	u, err := a.services.User().Get(ctx, accessKey)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return nil, fmt.Errorf("user not found: %s", accessKey)
		}
		return nil, err
	}
	return iamUserToConsole(u), nil
}

func (a *Adapter) CreateUser(ctx context.Context, req consoleapi.CreateUserRequest) (*consoleapi.User, error) {
	u, err := a.services.User().Create(ctx, &svcuser.CreateUserRequest{
		AccessKey: req.AccessKey,
		SecretKey: req.SecretKey,
	})
	if err != nil {
		return nil, err
	}
	return iamUserToConsole(u), nil
}

func (a *Adapter) DeleteUser(ctx context.Context, accessKey string) error {
	return a.services.User().Delete(ctx, accessKey)
}

func (a *Adapter) SetUserStatus(ctx context.Context, accessKey string, enabled bool) error {
	status := iam.UserStatusActive
	if !enabled {
		status = iam.UserStatusDisabled
	}
	_, err := a.services.User().Update(ctx, accessKey, &svcuser.UpdateUserRequest{
		Status: &status,
	})
	return err
}

// --- Policies ----------------------------------------------------------------

func (a *Adapter) ListPolicies(ctx context.Context) ([]*consoleapi.Policy, error) {
	all, err := a.services.Policy().List(ctx)
	if err != nil {
		return nil, err
	}

	policies := make([]*consoleapi.Policy, 0, len(all))
	for _, p := range all {
		cp, err := iamPolicyToConsole(p)
		if err != nil {
			continue
		}
		policies = append(policies, cp)
	}

	return policies, nil
}

func (a *Adapter) GetPolicy(ctx context.Context, name string) (*consoleapi.Policy, error) {
	p, err := a.services.Policy().Get(ctx, name)
	if err != nil {
		if errors.Is(err, svcerrors.ErrPolicyNotFound) {
			return nil, fmt.Errorf("policy not found: %s", name)
		}
		return nil, err
	}
	return iamPolicyToConsole(p)
}

func (a *Adapter) CreatePolicy(ctx context.Context, req consoleapi.CreatePolicyRequest) (*consoleapi.Policy, error) {
	var doc iam.PolicyDocument
	if err := json.Unmarshal([]byte(req.PolicyDocument), &doc); err != nil {
		return nil, fmt.Errorf("invalid policy document JSON: %w", err)
	}

	p, err := a.services.Policy().Create(ctx, &svcpolicy.CreatePolicyRequest{
		Name:           req.Name,
		PolicyDocument: &doc,
	})
	if err != nil {
		return nil, err
	}
	return iamPolicyToConsole(p)
}

func (a *Adapter) DeletePolicy(ctx context.Context, name string) error {
	return a.services.Policy().Delete(ctx, name)
}

func (a *Adapter) AttachPolicy(ctx context.Context, policyName, accessKey string) error {
	return a.services.User().AttachPolicy(ctx, accessKey, policyName)
}

func (a *Adapter) DetachPolicy(ctx context.Context, policyName, accessKey string) error {
	return a.services.User().DetachPolicy(ctx, accessKey, policyName)
}

// --- Buckets -----------------------------------------------------------------

func (a *Adapter) ListBuckets(ctx context.Context) ([]*consoleapi.Bucket, error) {
	metas, err := a.services.Metadata().ListBucketMetadatas(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]*consoleapi.Bucket, 0, len(metas))
	for _, meta := range metas {
		b := &consoleapi.Bucket{
			Name:      meta.Name,
			CreatedAt: meta.Created,
		}
		if meta.Owner != nil {
			b.OwnerUUID = meta.Owner.String()
			user, err := a.services.Metadata().GetUserByUUID(ctx, *meta.Owner)
			if err == nil {
				b.Owner = &consoleapi.Owner{
					UUID:      meta.Owner.String(),
					AccessKey: user.AccessKey,
					Username:  user.Username,
				}
			}
		}
		out = append(out, b)
	}

	return out, nil
}

func (a *Adapter) GetBucketPolicy(_ context.Context, _ string) (string, error) {
	return "", ErrNotImplemented
}

func (a *Adapter) SetBucketPolicy(_ context.Context, _, _ string) error {
	return ErrNotImplemented
}

// --- Ownership ---------------------------------------------------------------

func (a *Adapter) GetBucketOwner(ctx context.Context, bucket string) (*consoleapi.Owner, error) {
	meta, err := a.services.Metadata().GetBucketMetadata(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if meta.Owner == nil {
		return &consoleapi.Owner{}, nil // admin-owned, no UUID
	}
	owner := &consoleapi.Owner{UUID: meta.Owner.String()}
	user, err := a.services.Metadata().GetUserByUUID(ctx, *meta.Owner)
	if err == nil {
		owner.AccessKey = user.AccessKey
		owner.Username = user.Username
	}
	return owner, nil
}

func (a *Adapter) TransferBucketOwnership(_ context.Context, _, _ string) error {
	return ErrNotImplemented
}

func (a *Adapter) GetObjectOwner(_ context.Context, _, _ string) (*consoleapi.Owner, error) {
	return nil, ErrNotImplemented
}

// --- Policy Observability ----------------------------------------------------

func (a *Adapter) GetEffectivePermissions(_ context.Context, _, _ string) (*consoleapi.EffectivePermissions, error) {
	return nil, ErrNotImplemented
}

func (a *Adapter) SimulateRequest(_ context.Context, _ consoleapi.SimulateRequest) (*consoleapi.SimulateResult, error) {
	return nil, ErrNotImplemented
}

// --- conversion helpers ------------------------------------------------------

func iamUserToConsole(u *iam.User) *consoleapi.User {
	policies := u.AttachedPolicies
	if policies == nil {
		policies = []string{}
	}
	return &consoleapi.User{
		AccessKey:        u.AccessKey,
		Username:         u.Username,
		Status:           string(u.Status),
		AttachedPolicies: policies,
		UpdatedAt:        u.UpdatedAt,
	}
}

func iamPolicyToConsole(p *iam.Policy) (*consoleapi.Policy, error) {
	docJSON, err := json.Marshal(p.PolicyDocument)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal policy document: %w", err)
	}
	return &consoleapi.Policy{
		Name:           p.Name,
		PolicyDocument: string(docJSON),
		CreateDate:     p.CreateDate,
		UpdateDate:     p.UpdateDate,
	}, nil
}
