// Package console provides the adapter that implements consoleapi.API by calling
// the DirIO service layer directly (no HTTP round-trips).
package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mallardduck/dirio/consoleapi"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/policy/variables"
	"github.com/mallardduck/dirio/internal/service"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	svcpolicy "github.com/mallardduck/dirio/internal/service/policy"
	svcs3 "github.com/mallardduck/dirio/internal/service/s3"
	svcuser "github.com/mallardduck/dirio/internal/service/user"
	"github.com/mallardduck/dirio/pkg/iam"
	s3types "github.com/mallardduck/dirio/pkg/s3types"
)

// ErrNotImplemented is returned for API methods not yet backed by the service layer.
var ErrNotImplemented = errors.New("not implemented")

// commonS3Actions is the set of S3 permissions evaluated by GetEffectivePermissions.
var commonS3Actions = []string{
	"s3:GetObject",
	"s3:PutObject",
	"s3:DeleteObject",
	"s3:ListBucket",
	"s3:GetBucketPolicy",
	"s3:PutBucketPolicy",
	"s3:DeleteBucketPolicy",
	"s3:CreateBucket",
	"s3:DeleteBucket",
	"s3:GetObjectTagging",
	"s3:PutObjectTagging",
	"s3:DeleteObjectTagging",
}

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

func (a *Adapter) GetBucket(ctx context.Context, bucket string) (*consoleapi.Bucket, error) {
	meta, err := a.services.Metadata().GetBucketMetadata(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket not found: %s", bucket)
	}
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
	return b, nil
}

func (a *Adapter) GetBucketPolicy(ctx context.Context, bucket string) (string, error) {
	doc, err := a.services.S3().GetBucketPolicy(ctx, bucket)
	if err != nil {
		if errors.Is(err, s3types.ErrNoSuchBucketPolicy) {
			return "", nil // no policy set — return empty string
		}
		return "", err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal bucket policy: %w", err)
	}
	return string(raw), nil
}

func (a *Adapter) SetBucketPolicy(ctx context.Context, bucket, policyJSON string) error {
	if strings.TrimSpace(policyJSON) == "" {
		return a.services.S3().DeleteBucketPolicy(ctx, bucket)
	}
	var doc iam.PolicyDocument
	if err := json.Unmarshal([]byte(policyJSON), &doc); err != nil {
		return fmt.Errorf("invalid policy JSON: %w", err)
	}
	return a.services.S3().PutBucketPolicy(ctx, &svcs3.PutBucketPolicyRequest{
		Bucket:         bucket,
		PolicyDocument: &doc,
	})
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

func (a *Adapter) TransferBucketOwnership(ctx context.Context, bucket, newOwnerAccessKey string) error {
	user, err := a.services.User().Get(ctx, newOwnerAccessKey)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return fmt.Errorf("user not found: %s", newOwnerAccessKey)
		}
		return err
	}
	ownerUUID := user.UUID
	return a.services.Metadata().SetBucketOwner(ctx, bucket, &ownerUUID)
}

func (a *Adapter) GetObjectOwner(ctx context.Context, bucket, key string) (*consoleapi.Owner, error) {
	meta, err := a.services.Metadata().GetObjectMetadata(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	if meta.Owner == nil {
		return &consoleapi.Owner{}, nil // admin-owned
	}
	owner := &consoleapi.Owner{UUID: meta.Owner.String()}
	user, err := a.services.Metadata().GetUserByUUID(ctx, *meta.Owner)
	if err == nil {
		owner.AccessKey = user.AccessKey
		owner.Username = user.Username
	}
	return owner, nil
}

// --- Policy Observability ----------------------------------------------------

func (a *Adapter) GetEffectivePermissions(ctx context.Context, accessKey, bucket string) (*consoleapi.EffectivePermissions, error) {
	iamUser, err := a.services.User().Get(ctx, accessKey)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return nil, fmt.Errorf("user not found: %s", accessKey)
		}
		return nil, err
	}

	bucketMeta, err := a.services.Metadata().GetBucketMetadata(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket not found: %s", bucket)
	}

	engine := a.services.PolicyEngine()
	principal := &policy.Principal{
		User:        iamUser,
		IsAnonymous: false,
		IsAdmin:     false,
	}
	varCtx := variables.ForUser(iamUser)

	allowed := make([]string, 0)
	denied := make([]string, 0)

	for _, action := range commonS3Actions {
		req := &policy.RequestContext{
			Principal:       principal,
			Action:          action,
			Resource:        &policy.Resource{Bucket: bucket},
			VarContext:      varCtx,
			BucketOwnerUUID: bucketMeta.Owner,
		}
		decision := engine.Evaluate(ctx, req)
		if decision.IsAllowed() {
			allowed = append(allowed, action)
		} else {
			denied = append(denied, action)
		}
	}

	return &consoleapi.EffectivePermissions{
		AccessKey:      accessKey,
		Bucket:         bucket,
		AllowedActions: allowed,
		DeniedActions:  denied,
	}, nil
}

func (a *Adapter) SimulateRequest(ctx context.Context, req consoleapi.SimulateRequest) (*consoleapi.SimulateResult, error) {
	iamUser, err := a.services.User().Get(ctx, req.AccessKey)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return nil, fmt.Errorf("user not found: %s", req.AccessKey)
		}
		return nil, err
	}

	bucketMeta, err := a.services.Metadata().GetBucketMetadata(ctx, req.Bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket not found: %s", req.Bucket)
	}

	pReq := &policy.RequestContext{
		Principal: &policy.Principal{
			User:        iamUser,
			IsAnonymous: false,
			IsAdmin:     false,
		},
		Action:          req.Action,
		Resource:        &policy.Resource{Bucket: req.Bucket, Key: req.Key},
		VarContext:      variables.ForUser(iamUser),
		BucketOwnerUUID: bucketMeta.Owner,
	}

	if req.Key != "" {
		objMeta, err := a.services.Metadata().GetObjectMetadata(ctx, req.Bucket, req.Key)
		if err == nil && objMeta.Owner != nil {
			pReq.ObjectOwnerUUID = objMeta.Owner
		}
	}

	decision := a.services.PolicyEngine().Evaluate(ctx, pReq)

	result := &consoleapi.SimulateResult{Allowed: decision.IsAllowed()}
	switch decision {
	case policy.DecisionAllow:
		result.Reason = "Allowed by bucket policy or resource ownership"
	case policy.DecisionExplicitDeny:
		result.Reason = "Explicitly denied by bucket policy"
	default:
		result.Reason = "Default deny — no matching allow rule found"
	}

	return result, nil
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
