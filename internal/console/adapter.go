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

	consoleapi "github.com/mallardduck/dirio/api"
	"github.com/mallardduck/dirio/internal/consts"
	contextInt "github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/crypto"
	httpauth "github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/service"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	svcgroup "github.com/mallardduck/dirio/internal/service/group"
	"github.com/mallardduck/dirio/internal/service/observation"
	svcpolicy "github.com/mallardduck/dirio/internal/service/policy"
	svcs3 "github.com/mallardduck/dirio/internal/service/s3"
	svcsacct "github.com/mallardduck/dirio/internal/service/serviceaccount"
	svcuser "github.com/mallardduck/dirio/internal/service/user"
	"github.com/mallardduck/dirio/sdk/iam"
	s3types "github.com/mallardduck/dirio/sdk/s3types"
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
	secretKey := req.SecretKey
	if req.GenerateSecret {
		_, generated, err := crypto.GenerateDirIOKey(crypto.PrefixUser)
		if err != nil {
			return nil, fmt.Errorf("failed to generate secret key: %w", err)
		}
		secretKey = generated
	}
	u, err := a.services.User().Create(ctx, &svcuser.CreateUserRequest{
		AccessKey: req.AccessKey,
		SecretKey: secretKey,
		Status:    iam.UserStatusActive,
	})
	if err != nil {
		return nil, err
	}
	result := iamUserToConsole(u)
	if req.GenerateSecret {
		result.SecretKey = secretKey
	}
	return result, nil
}

func (a *Adapter) UpdateUser(ctx context.Context, uuidStr string, req consoleapi.UpdateUserRequest) (*consoleapi.User, error) {
	uUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID: %w", err)
	}
	secretKey := req.SecretKey
	var generatedSecret string
	if req.GenerateSecret {
		_, generated, err := crypto.GenerateDirIOKey(crypto.PrefixUser)
		if err != nil {
			return nil, fmt.Errorf("failed to generate secret key: %w", err)
		}
		generatedSecret = generated
		secretKey = &generatedSecret
	}
	updated, err := a.services.User().Update(ctx, uUUID, &svcuser.UpdateUserRequest{
		SecretKey: secretKey,
	})
	if err != nil {
		return nil, err
	}
	result := iamUserToConsole(updated)
	if req.GenerateSecret {
		result.SecretKey = generatedSecret
	}
	return result, nil
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

func (a *Adapter) UpdatePolicy(ctx context.Context, name string, req consoleapi.UpdatePolicyRequest) (*consoleapi.Policy, error) {
	var doc iam.PolicyDocument
	if err := json.Unmarshal([]byte(req.PolicyDocument), &doc); err != nil {
		return nil, fmt.Errorf("invalid policy document JSON: %w", err)
	}
	p, err := a.services.Policy().Update(ctx, name, &svcpolicy.UpdatePolicyRequest{
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

func (a *Adapter) CreateBucket(ctx context.Context, name, ownerAccessKey string) error {
	// metadata.Manager.CreateBucket reads user.UUID from context to set ownership.
	// Inject the appropriate user: the specified owner if provided, otherwise admin.
	// Admin is in-memory only (no metadata file), so inject a stub for that case.
	if ownerAccessKey != "" {
		owner, err := a.services.User().GetByAccessKey(ctx, ownerAccessKey)
		if err != nil {
			return fmt.Errorf("failed to resolve owner: %w", err)
		}
		ctx = contextInt.WithUser(ctx, owner)
	} else {
		ctx = contextInt.WithUser(ctx, &iam.User{UUID: iam.AdminUserUUID})
	}
	_, err := a.services.S3().CreateBucket(ctx, &svcs3.CreateBucketRequest{Name: name})
	return err
}

func (a *Adapter) DeleteBucket(ctx context.Context, name string) error {
	return a.services.S3().DeleteBucket(ctx, name)
}

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
		return nil, err
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

// --- Objects -----------------------------------------------------------------

func (a *Adapter) ListObjects(ctx context.Context, bucket, prefix, delimiter string) ([]*consoleapi.ObjectInfo, error) {
	result, err := a.services.S3().ListObjectsV2(ctx, &svcs3.ListObjectsV2Request{
		Bucket:    bucket,
		Prefix:    prefix,
		Delimiter: delimiter,
		MaxKeys:   1000,
	})
	if err != nil {
		return nil, err
	}

	out := make([]*consoleapi.ObjectInfo, 0, len(result.Objects)+len(result.CommonPrefixes))
	for _, obj := range result.Objects {
		out = append(out, &consoleapi.ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			ETag:         obj.ETag,
			LastModified: obj.LastModified,
		})
	}
	for _, cp := range result.CommonPrefixes {
		out = append(out, &consoleapi.ObjectInfo{
			Key:      cp.Prefix,
			IsPrefix: true,
		})
	}
	return out, nil
}

func (a *Adapter) GetObjectMetadata(ctx context.Context, bucket, key string) (*consoleapi.ObjectMetadata, error) {
	resp, err := a.services.S3().HeadObject(ctx, &svcs3.HeadObjectRequest{
		Bucket: bucket,
		Key:    key,
	})
	if err != nil {
		return nil, err
	}
	return &consoleapi.ObjectMetadata{
		Key:            key,
		Size:           resp.Size,
		ETag:           resp.ETag,
		LastModified:   resp.LastModified,
		ContentType:    resp.ContentType,
		CustomMetadata: resp.CustomMetadata,
	}, nil
}

func (a *Adapter) GetObjectTags(ctx context.Context, bucket, key string) (map[string]string, error) {
	tags, err := a.services.S3().GetObjectTagging(ctx, &svcs3.GetObjectTaggingRequest{
		Bucket: bucket,
		Key:    key,
	})
	if err != nil {
		return nil, err
	}
	if tags == nil {
		return map[string]string{}, nil
	}
	return tags, nil
}

func (a *Adapter) SetObjectTags(ctx context.Context, bucket, key string, tags map[string]string) error {
	return a.services.S3().PutObjectTagging(ctx, &svcs3.PutObjectTaggingRequest{
		Bucket: bucket,
		Key:    key,
		Tags:   tags,
	})
}

func (a *Adapter) DeleteObject(ctx context.Context, bucket, key string) error {
	return a.services.S3().DeleteObject(ctx, &svcs3.DeleteObjectRequest{
		Bucket: bucket,
		Key:    key,
	})
}

func (a *Adapter) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	return a.services.S3().CopyObject(ctx, srcBucket, srcKey, dstBucket, dstKey)
}

func (a *Adapter) GeneratePresignedURL(ctx context.Context, req consoleapi.GeneratePresignedURLRequest) (string, error) {
	user, err := a.services.Authenticator().GetUserForAccessKey(ctx, req.AccessKey)
	if err != nil {
		return "", fmt.Errorf("failed to resolve credentials: %w", err)
	}
	method := req.Method
	if method == "" {
		method = "GET"
	}
	return httpauth.GeneratePresignedURL(
		user.AccessKey,
		user.SecretKey,
		consts.DefaultBucketLocation,
		req.Bucket,
		req.Key,
		req.BaseURL,
		method,
		req.Expiry,
	)
}

// --- Ownership ---------------------------------------------------------------

func (a *Adapter) GetBucketOwner(ctx context.Context, bucket string) (*consoleapi.Owner, error) {
	info, err := a.services.S3().GetBucketOwner(ctx, bucket)
	if err != nil {
		return nil, err
	}
	return ownerInfoToConsole(info), nil
}

func (a *Adapter) TransferBucketOwnership(ctx context.Context, bucket, newOwnerAccessKey string) error {
	if newOwnerAccessKey == "" {
		return a.services.S3().SetBucketOwner(ctx, bucket, nil)
	}
	user, err := a.services.User().GetByAccessKey(ctx, newOwnerAccessKey)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return fmt.Errorf("user %q: %w", newOwnerAccessKey, svcerrors.ErrUserNotFound)
		}
		return err
	}
	ownerUUID := user.UUID
	return a.services.S3().SetBucketOwner(ctx, bucket, &ownerUUID)
}

func (a *Adapter) GetObjectOwner(ctx context.Context, bucket, key string) (*consoleapi.Owner, error) {
	info, err := a.services.S3().GetObjectOwner(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	return ownerInfoToConsole(info), nil
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

func (a *Adapter) AddGroupMember(ctx context.Context, groupName, userAccessKey string) error {
	u, err := a.services.User().GetByAccessKey(ctx, userAccessKey)
	if err != nil {
		return fmt.Errorf("failed to resolve user: %w", err)
	}
	return a.services.Group().AddMember(ctx, groupName, u.UUID)
}

func (a *Adapter) RemoveGroupMember(ctx context.Context, groupName, userUUID string) error {
	uid, err := uuid.Parse(userUUID)
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
	perms, err := a.services.Observation().GetEffectivePermissions(ctx, accessKey, bucket)
	if err != nil {
		return nil, err
	}
	return &consoleapi.EffectivePermissions{
		AccessKey:      perms.AccessKey,
		Bucket:         perms.Bucket,
		AllowedActions: perms.AllowedActions,
		DeniedActions:  perms.DeniedActions,
	}, nil
}

func (a *Adapter) SimulateRequest(ctx context.Context, req consoleapi.SimulateRequest) (*consoleapi.SimulateResult, error) {
	result, err := a.services.Observation().Simulate(ctx, observation.SimulateRequest{
		AccessKey: req.AccessKey,
		Bucket:    req.Bucket,
		Action:    req.Action,
		Key:       req.Key,
	})
	if err != nil {
		return nil, err
	}
	return &consoleapi.SimulateResult{
		Allowed: result.Allowed,
		Reason:  result.Reason,
	}, nil
}

// --- conversion helpers ------------------------------------------------------

// ownerInfoToConsole converts an s3.OwnerInfo to a consoleapi.Owner.
// Nil OwnerUUID means the resource is admin-owned; the well-known admin UUID is used.
func ownerInfoToConsole(info *svcs3.OwnerInfo) *consoleapi.Owner {
	if info.OwnerUUID == nil {
		return &consoleapi.Owner{UUID: consoleapi.AdminUserUUID}
	}
	return &consoleapi.Owner{
		UUID:      info.OwnerUUID.String(),
		AccessKey: info.AccessKey,
		Username:  info.Username,
	}
}

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
