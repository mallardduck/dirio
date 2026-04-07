// Package observation provides policy simulation and effective-permission
// evaluation. It is the authoritative service-layer implementation for both
// the console UI and the DirIO CLI client (dio), so neither has to duplicate
// this logic in its own adapter or command layer.
package observation

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/policy/variables"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/s3"
	svcuser "github.com/mallardduck/dirio/internal/service/user"
	"github.com/mallardduck/dirio/pkg/s3types"
)

var filterLogger = logging.Component("filter")

// commonS3Actions is the fixed set of S3 permissions evaluated by
// GetEffectivePermissions. Extend this list when new S3 actions are supported.
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

// Service evaluates IAM policies against S3 resources.
type Service struct {
	s3     *s3.Service
	users  *svcuser.Service
	engine *policy.Engine
}

// NewService creates an observation service backed by the given dependencies.
func NewService(s3Svc *s3.Service, userSvc *svcuser.Service, engine *policy.Engine) *Service {
	return &Service{s3: s3Svc, users: userSvc, engine: engine}
}

// ============================================================================
// Types
// ============================================================================

// SimulateRequest describes a single IAM evaluation to perform.
type SimulateRequest struct {
	// AccessKey of the IAM user whose permissions are being simulated.
	AccessKey string
	// Bucket is the target S3 bucket.
	Bucket string
	// Action is the S3 action to evaluate (e.g. "s3:GetObject").
	Action string
	// Key is the optional object key. Leave empty for bucket-level actions.
	Key string
}

// SimulateResult is the outcome of a policy simulation.
type SimulateResult struct {
	Allowed bool
	Reason  string
}

// EffectivePermissions lists which S3 actions a user is allowed or denied on a bucket.
type EffectivePermissions struct {
	AccessKey      string
	Bucket         string
	AllowedActions []string
	DeniedActions  []string
}

// ============================================================================
// Methods
// ============================================================================

// Simulate evaluates a single IAM request and returns the policy decision.
// Returns svcerrors.ErrUserNotFound when the access key is unknown,
// and s3types.ErrBucketNotFound when the bucket does not exist.
func (s *Service) Simulate(ctx context.Context, req SimulateRequest) (*SimulateResult, error) {
	iamUser, err := s.users.GetByAccessKey(ctx, req.AccessKey)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return nil, fmt.Errorf("user %q: %w", req.AccessKey, svcerrors.ErrUserNotFound)
		}
		return nil, err
	}

	bucketMeta, err := s.s3.GetBucket(ctx, req.Bucket)
	if err != nil {
		return nil, err // already s3types.ErrBucketNotFound from service layer
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
		ownerUUID, err := s.s3.GetObjectOwnerUUID(ctx, req.Bucket, req.Key)
		if err == nil && ownerUUID != nil {
			pReq.ObjectOwnerUUID = ownerUUID
		}
	}

	decision := s.engine.Evaluate(ctx, pReq)

	result := &SimulateResult{Allowed: decision.IsAllowed()}
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

// GetEffectivePermissions evaluates all common S3 actions for a user on a bucket
// and returns which are allowed and which are denied.
// Returns svcerrors.ErrUserNotFound or s3types.ErrBucketNotFound as appropriate.
func (s *Service) GetEffectivePermissions(ctx context.Context, accessKey, bucket string) (*EffectivePermissions, error) {
	iamUser, err := s.users.GetByAccessKey(ctx, accessKey)
	if err != nil {
		if errors.Is(err, svcerrors.ErrUserNotFound) {
			return nil, fmt.Errorf("user %q: %w", accessKey, svcerrors.ErrUserNotFound)
		}
		return nil, err
	}

	bucketMeta, err := s.s3.GetBucket(ctx, bucket)
	if err != nil {
		return nil, err // already s3types.ErrBucketNotFound from service layer
	}

	principal := &policy.Principal{
		User:        iamUser,
		IsAnonymous: false,
		IsAdmin:     false,
	}
	varCtx := variables.ForUser(iamUser)

	allowed := make([]string, 0)
	denied := make([]string, 0)

	for _, action := range commonS3Actions {
		pReq := &policy.RequestContext{
			Principal:       principal,
			Action:          action,
			Resource:        &policy.Resource{Bucket: bucket},
			VarContext:      varCtx,
			BucketOwnerUUID: bucketMeta.Owner,
		}
		if s.engine.Evaluate(ctx, pReq).IsAllowed() {
			allowed = append(allowed, action)
		} else {
			denied = append(denied, action)
		}
	}

	return &EffectivePermissions{
		AccessKey:      accessKey,
		Bucket:         bucket,
		AllowedActions: allowed,
		DeniedActions:  denied,
	}, nil
}

// FilterBuckets returns only the buckets in the list that the principal is
// allowed to see (s3:GetBucketLocation). Admin principals bypass evaluation.
func (s *Service) FilterBuckets(
	ctx context.Context,
	principal *policy.Principal,
	buckets []s3types.Bucket,
	conditions *policy.ConditionContext,
	varCtx *variables.Context,
) []s3types.Bucket {
	if principal.IsAdmin {
		return buckets
	}

	filtered := make([]s3types.Bucket, 0, len(buckets))
	allowed, denied := 0, 0

	for i := range buckets {
		bucket := &buckets[i]

		var bucketOwnerUUID *uuid.UUID
		meta, err := s.s3.GetBucket(ctx, bucket.Name)
		if err == nil {
			bucketOwnerUUID = meta.Owner
		} else {
			filterLogger.With("bucket", bucket.Name, "error", err).
				Debug("failed to fetch bucket metadata, treating as deny")
		}

		decision := s.engine.Evaluate(ctx, &policy.RequestContext{
			Principal:       principal,
			Action:          "s3:GetBucketLocation",
			Resource:        &policy.Resource{Bucket: bucket.Name},
			Conditions:      conditions,
			VarContext:      varCtx,
			BucketOwnerUUID: bucketOwnerUUID,
		})

		if decision.IsAllowed() {
			filtered = append(filtered, *bucket)
			allowed++
		} else {
			denied++
		}
	}

	filterLogger.With(
		"total_buckets", len(buckets),
		"allowed", allowed,
		"denied", denied,
	).Debug("filtered bucket list")

	return filtered
}

// FilterObjects returns only the objects in the list that the principal is
// allowed to read (s3:GetObject). Admin principals bypass evaluation.
func (s *Service) FilterObjects(
	ctx context.Context,
	principal *policy.Principal,
	bucket string,
	objects []s3types.Object,
	conditions *policy.ConditionContext,
	varCtx *variables.Context,
) []s3types.Object {
	if principal.IsAdmin {
		return objects
	}

	// Fetch bucket owner once and reuse for every object evaluation.
	var bucketOwnerUUID *uuid.UUID
	if meta, err := s.s3.GetBucket(ctx, bucket); err == nil {
		bucketOwnerUUID = meta.Owner
	} else {
		filterLogger.With("bucket", bucket, "error", err).
			Debug("failed to fetch bucket metadata for filtering")
	}

	filtered := make([]s3types.Object, 0, len(objects))
	allowed, denied := 0, 0

	for i := range objects {
		obj := &objects[i]

		objectOwnerUUID, err := s.s3.GetObjectOwnerUUID(ctx, bucket, obj.Key)
		if err != nil {
			filterLogger.With("bucket", bucket, "key", obj.Key, "error", err).
				Debug("failed to fetch object metadata, treating as deny")
			objectOwnerUUID = nil
		}

		decision := s.engine.Evaluate(ctx, &policy.RequestContext{
			Principal:       principal,
			Action:          "s3:GetObject",
			Resource:        &policy.Resource{Bucket: bucket, Key: obj.Key},
			Conditions:      conditions,
			VarContext:      varCtx,
			BucketOwnerUUID: bucketOwnerUUID,
			ObjectOwnerUUID: objectOwnerUUID,
		})

		if decision.IsAllowed() {
			filtered = append(filtered, *obj)
			allowed++
		} else {
			denied++
		}
	}

	filterRatio := 0.0
	if len(objects) > 0 {
		filterRatio = float64(denied) / float64(len(objects))
	}
	filterLogger.With(
		"bucket", bucket,
		"total_objects", len(objects),
		"allowed", allowed,
		"denied", denied,
		"filter_ratio", filterRatio,
	).Debug("filtered object list")

	return filtered
}
