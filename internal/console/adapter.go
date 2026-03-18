// Package console provides the adapter that implements consoleapi.API by calling
// the DirIO service layer directly (no HTTP round-trips).
package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/consoleapi"
	"github.com/mallardduck/dirio/internal/crypto"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/policy/variables"
	"github.com/mallardduck/dirio/internal/service"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	svcgroup "github.com/mallardduck/dirio/internal/service/group"
	svcpolicy "github.com/mallardduck/dirio/internal/service/policy"
	svcs3 "github.com/mallardduck/dirio/internal/service/s3"
	svcsacct "github.com/mallardduck/dirio/internal/service/serviceaccount"
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
	uids, err := a.services.User().List(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]*consoleapi.User, 0, len(uids)+2)

	// Prepend the admin account(s) — they live in config, not the metadata store.
	if auth := a.services.Authenticator(); auth != nil {
		adminUUID := iam.AdminUserUUID.String()
		if pk := auth.PrimaryRootAccessKey(); pk != "" {
			users = append(users, &consoleapi.User{
				UUID:      adminUUID,
				AccessKey: pk,
				Username:  "admin",
				Status:    "on",
			})
		}
		if ak := auth.AltRootAccessKey(); ak != "" && ak != auth.PrimaryRootAccessKey() {
			users = append(users, &consoleapi.User{
				UUID:      adminUUID,
				AccessKey: ak,
				Username:  "admin (alt)",
				Status:    "on",
			})
		}
	}

	for _, uid := range uids {
		u, err := a.services.User().Get(ctx, uid)
		if err != nil {
			continue // skip users that can't be fetched
		}
		users = append(users, iamUserToConsole(u))
	}

	return users, nil
}

func (a *Adapter) GetUser(ctx context.Context, uuidStr string) (*consoleapi.User, error) {
	uUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID: %w", err)
	}
	u, err := a.services.User().Get(ctx, uUUID)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return nil, fmt.Errorf("user not found: %s", uuidStr)
		}
		return nil, err
	}
	return iamUserToConsole(u), nil
}

func (a *Adapter) GetUserSecret(ctx context.Context, uuidStr string) (string, error) {
	uUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return "", fmt.Errorf("invalid UUID: %w", err)
	}
	u, err := a.services.User().Get(ctx, uUUID)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return "", fmt.Errorf("user not found: %s", uuidStr)
		}
		return "", err
	}
	return u.SecretKey, nil
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

func (a *Adapter) UpdateUser(ctx context.Context, uuidStr string, req consoleapi.UpdateUserRequest) (*consoleapi.User, error) {
	uUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID: %w", err)
	}
	updated, err := a.services.User().Update(ctx, uUUID, &svcuser.UpdateUserRequest{
		SecretKey: req.SecretKey,
	})
	if err != nil {
		return nil, err
	}
	return iamUserToConsole(updated), nil
}

func (a *Adapter) DeleteUser(ctx context.Context, uuidStr string) error {
	uUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}
	return a.services.User().Delete(ctx, uUUID)
}

func (a *Adapter) SetUserStatus(ctx context.Context, uuidStr string, enabled bool) error {
	uUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}
	status := iam.UserStatusActive
	if !enabled {
		status = iam.UserStatusDisabled
	}
	_, err = a.services.User().Update(ctx, uUUID, &svcuser.UpdateUserRequest{
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
	u, err := a.services.User().GetByAccessKey(ctx, accessKey)
	if err != nil {
		return err
	}
	return a.services.User().AttachPolicy(ctx, u.UUID, policyName)
}

func (a *Adapter) DetachPolicy(ctx context.Context, policyName, accessKey string) error {
	u, err := a.services.User().GetByAccessKey(ctx, accessKey)
	if err != nil {
		return err
	}
	return a.services.User().DetachPolicy(ctx, u.UUID, policyName)
}

// --- Buckets -----------------------------------------------------------------

func (a *Adapter) ListBuckets(ctx context.Context) ([]*consoleapi.Bucket, error) {
	metas, err := a.services.S3().ListBucketsWithMetadata(ctx)
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
			user, err := a.services.User().Get(ctx, *meta.Owner)
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
	meta, err := a.services.S3().GetBucket(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket not found: %s", bucket)
	}
	b := &consoleapi.Bucket{
		Name:      meta.Name,
		CreatedAt: meta.Created,
	}
	if meta.Owner != nil {
		b.OwnerUUID = meta.Owner.String()
		user, err := a.services.User().Get(ctx, *meta.Owner)
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
	meta, err := a.services.S3().GetBucket(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if meta.Owner == nil {
		return &consoleapi.Owner{}, nil // admin-owned, no UUID
	}
	owner := &consoleapi.Owner{UUID: meta.Owner.String()}
	user, err := a.services.User().Get(ctx, *meta.Owner)
	if err == nil {
		owner.AccessKey = user.AccessKey
		owner.Username = user.Username
	}
	return owner, nil
}

func (a *Adapter) TransferBucketOwnership(ctx context.Context, bucket, newOwnerAccessKey string) error {
	user, err := a.services.User().GetByAccessKey(ctx, newOwnerAccessKey)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return fmt.Errorf("user not found: %s", newOwnerAccessKey)
		}
		return err
	}
	ownerUUID := user.UUID
	return a.services.S3().SetBucketOwner(ctx, bucket, &ownerUUID)
}

func (a *Adapter) GetObjectOwner(ctx context.Context, bucket, key string) (*consoleapi.Owner, error) {
	ownerUUID, err := a.services.S3().GetObjectOwnerUUID(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	if ownerUUID == nil {
		return &consoleapi.Owner{}, nil // admin-owned
	}
	owner := &consoleapi.Owner{UUID: ownerUUID.String()}
	user, err := a.services.User().Get(ctx, *ownerUUID)
	if err == nil {
		owner.AccessKey = user.AccessKey
		owner.Username = user.Username
	}
	return owner, nil
}

// --- Groups ------------------------------------------------------------------

func (a *Adapter) ListGroups(ctx context.Context) ([]*consoleapi.Group, error) {
	names, err := a.services.Group().List(ctx)
	if err != nil {
		return nil, err
	}
	groups := make([]*consoleapi.Group, 0, len(names))
	for _, name := range names {
		g, err := a.services.Group().Get(ctx, name)
		if err != nil {
			continue
		}
		groups = append(groups, iamGroupToConsole(g))
	}
	return groups, nil
}

func (a *Adapter) GetGroup(ctx context.Context, name string) (*consoleapi.Group, error) {
	g, err := a.services.Group().Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("group not found: %s", name)
	}
	return iamGroupToConsole(g), nil
}

func (a *Adapter) CreateGroup(ctx context.Context, req consoleapi.CreateGroupRequest) (*consoleapi.Group, error) {
	g, err := a.services.Group().Create(ctx, &svcgroup.CreateGroupRequest{Name: req.Name})
	if err != nil {
		return nil, err
	}
	return iamGroupToConsole(g), nil
}

func (a *Adapter) DeleteGroup(ctx context.Context, name string) error {
	return a.services.Group().Delete(ctx, name)
}

func (a *Adapter) AddGroupMember(ctx context.Context, groupName, userUID string) error {
	uid, err := uuid.Parse(userUID)
	if err != nil {
		return fmt.Errorf("invalid user UUID: %w", err)
	}
	return a.services.Group().AddMember(ctx, groupName, uid)
}

func (a *Adapter) RemoveGroupMember(ctx context.Context, groupName, userUID string) error {
	uid, err := uuid.Parse(userUID)
	if err != nil {
		return fmt.Errorf("invalid user UUID: %w", err)
	}
	return a.services.Group().RemoveMember(ctx, groupName, uid)
}

func (a *Adapter) AttachGroupPolicy(ctx context.Context, groupName, policyName string) error {
	return a.services.Group().AttachPolicy(ctx, groupName, policyName)
}

func (a *Adapter) DetachGroupPolicy(ctx context.Context, groupName, policyName string) error {
	return a.services.Group().DetachPolicy(ctx, groupName, policyName)
}

func (a *Adapter) SetGroupStatus(ctx context.Context, groupName string, enabled bool) error {
	status := iam.GroupStatusActive
	if !enabled {
		status = iam.GroupStatusDisabled
	}
	return a.services.Group().SetStatus(ctx, groupName, status)
}

// --- Service Accounts --------------------------------------------------------

func (a *Adapter) ListServiceAccounts(ctx context.Context) ([]*consoleapi.ServiceAccount, error) {
	keys, err := a.services.ServiceAccount().List(ctx)
	if err != nil {
		return nil, err
	}

	sas := make([]*consoleapi.ServiceAccount, 0, len(keys))
	for _, key := range keys {
		sa, err := a.services.ServiceAccount().Get(ctx, key)
		if err != nil {
			continue
		}
		sas = append(sas, iamServiceAccountToConsole(ctx, a.services, sa))
	}

	return sas, nil
}

func (a *Adapter) GetServiceAccount(ctx context.Context, uuidStr string) (*consoleapi.ServiceAccount, error) {
	saUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID: %w", err)
	}
	sa, err := a.services.ServiceAccount().GetByUUID(ctx, saUUID)
	if err != nil {
		return nil, err
	}
	return iamServiceAccountToConsole(ctx, a.services, sa), nil
}

func (a *Adapter) GetServiceAccountSecret(ctx context.Context, uuidStr string) (string, error) {
	saUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return "", fmt.Errorf("invalid UUID: %w", err)
	}

	sa, err := a.services.ServiceAccount().GetByUUID(ctx, saUUID)
	if err != nil {
		return "", err
	}
	return sa.SecretKey, nil
}

func (a *Adapter) CreateServiceAccount(ctx context.Context, req consoleapi.CreateServiceAccountRequest) (*consoleapi.ServiceAccount, error) {
	var parentUserUUID *uuid.UUID
	if req.ParentUserUUID != "" {
		uid, err := uuid.Parse(req.ParentUserUUID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent user UUID: %w", err)
		}
		parentUserUUID = &uid
	}

	if req.AccessKey == "" {
		accessKey, secretKey, err := crypto.GenerateDirIOKey(crypto.PrefixService)
		if err != nil {
			return nil, fmt.Errorf("generating service account key: %w", err)
		}
		req.AccessKey = accessKey
		req.SecretKey = secretKey
	}

	sa, err := a.services.ServiceAccount().Create(ctx, &svcsacct.CreateServiceAccountRequest{
		AccessKey:          req.AccessKey,
		SecretKey:          req.SecretKey,
		ParentUserUUID:     parentUserUUID,
		PolicyMode:         iam.PolicyMode(req.PolicyMode),
		EmbeddedPolicyJSON: req.EmbeddedPolicyJSON,
		ExpiresAt:          req.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	// Expose the secret key once at creation time so the UI can display it.
	// iamServiceAccountToConsole intentionally omits SecretKey for list/get operations.
	result := iamServiceAccountToConsole(ctx, a.services, sa)
	result.SecretKey = sa.SecretKey
	return result, nil
}

func (a *Adapter) DeleteServiceAccount(ctx context.Context, uuidStr string) error {
	saUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}
	sa, err := a.services.ServiceAccount().GetByUUID(ctx, saUUID)
	if err != nil {
		return err
	}
	return a.services.ServiceAccount().Delete(ctx, sa.AccessKey)
}

func (a *Adapter) UpdateServiceAccount(ctx context.Context, uuidStr string, req consoleapi.UpdateServiceAccountRequest) (*consoleapi.ServiceAccount, error) {
	saUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID: %w", err)
	}
	sa, err := a.services.ServiceAccount().GetByUUID(ctx, saUUID)
	if err != nil {
		return nil, err
	}

	updated, err := a.services.ServiceAccount().Update(ctx, sa.AccessKey, &svcsacct.UpdateServiceAccountRequest{
		SecretKey:          req.SecretKey,
		EmbeddedPolicyJSON: req.EmbeddedPolicyJSON,
		ExpiresAt:          req.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	return iamServiceAccountToConsole(ctx, a.services, updated), nil
}

func (a *Adapter) SetServiceAccountStatus(ctx context.Context, uuidStr string, enabled bool) error {
	saUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}
	sa, err := a.services.ServiceAccount().GetByUUID(ctx, saUUID)
	if err != nil {
		return err
	}

	status := iam.ServiceAcctStatusActive
	if !enabled {
		status = iam.ServiceAcctStatusDisabled
	}
	return a.services.ServiceAccount().SetStatus(ctx, sa.AccessKey, status)
}

// --- Policy Observability ----------------------------------------------------

func (a *Adapter) GetEffectivePermissions(ctx context.Context, accessKey, bucket string) (*consoleapi.EffectivePermissions, error) {
	iamUser, err := a.services.User().GetByAccessKey(ctx, accessKey)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return nil, fmt.Errorf("user not found: %s", accessKey)
		}
		return nil, err
	}

	bucketMeta, err := a.services.S3().GetBucket(ctx, bucket)
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
	iamUser, err := a.services.User().GetByAccessKey(ctx, req.AccessKey)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return nil, fmt.Errorf("user not found: %s", req.AccessKey)
		}
		return nil, err
	}

	bucketMeta, err := a.services.S3().GetBucket(ctx, req.Bucket)
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
		ownerUUID, err := a.services.S3().GetObjectOwnerUUID(ctx, req.Bucket, req.Key)
		if err == nil && ownerUUID != nil {
			pReq.ObjectOwnerUUID = ownerUUID
		}
	}

	decision := a.services.PolicyEngine().Evaluate(ctx, pReq)

	result := &consoleapi.SimulateResult{Allowed: decision.IsAllowed()}
	switch decision {
	case policy.DecisionAllow:
		result.Reason = "Allowed by bucket policy or resource ownership"
	case policy.DecisionExplicitDeny:
		result.Reason = "Explicitly denied by bucket policy"
	case policy.DecisionDeny:
		fallthrough //nolint:gocritic // emptyFallthrough: needed for exhaustive compliance
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
		UUID:             u.UUID.String(),
		AccessKey:        u.AccessKey,
		Username:         u.Username,
		Status:           string(u.Status),
		AttachedPolicies: policies,
		UpdatedAt:        u.UpdatedAt,
	}
}

func iamGroupToConsole(g *iam.Group) *consoleapi.Group {
	members := g.Members
	if members == nil {
		members = []uuid.UUID{}
	}
	policies := g.AttachedPolicies
	if policies == nil {
		policies = []string{}
	}
	return &consoleapi.Group{
		Name:             g.Name,
		Members:          members,
		AttachedPolicies: policies,
		Status:           string(g.Status),
		CreatedAt:        g.CreatedAt,
		UpdatedAt:        g.UpdatedAt,
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
		IsBuiltin:      p.IsBuiltin,
	}, nil
}

func iamServiceAccountToConsole(ctx context.Context, svcs *service.ServicesFactory, sa *iam.ServiceAccount) *consoleapi.ServiceAccount {
	parentAccessKey := ""
	parentUsername := ""
	parentUUID := ""
	if sa.ParentUserUUID != nil {
		parentUUID = sa.ParentUserUUID.String()
		if *sa.ParentUserUUID == iam.AdminUserUUID {
			parentAccessKey = "admin"
			parentUsername = "Admin"
		} else if u, err := svcs.User().Get(ctx, *sa.ParentUserUUID); err == nil {
			parentAccessKey = u.AccessKey
			parentUsername = u.Username
		}
	}

	return &consoleapi.ServiceAccount{
		UUID:               sa.UUID.String(),
		AccessKey:          sa.AccessKey,
		Username:           sa.Username,
		ParentUserUUID:     parentUUID,
		ParentAccessKey:    parentAccessKey,
		ParentUsername:     parentUsername,
		PolicyMode:         string(sa.PolicyMode),
		Status:             string(sa.Status),
		EmbeddedPolicyJSON: sa.EmbeddedPolicyJSON,
		CreatedAt:          sa.CreatedAt,
		UpdatedAt:          sa.UpdatedAt,
		ExpiresAt:          sa.ExpiresAt,
	}
}
