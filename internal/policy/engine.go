package policy

import (
	"context"

	"github.com/mallardduck/dirio/pkg/iam"
)

// Engine is the core policy evaluation engine.
// It maintains an in-memory cache of policies for fast evaluation
// and provides the main Evaluate() method for authorization decisions.
//
// Design principles:
// - Pure in-memory evaluation (no direct FS access)
// - Thread-safe concurrent access via Cache
// - Service layer notifies Engine of policy changes
type Engine struct {
	cache  *Cache
	mapper *ActionMapper
}

// New creates a new policy engine with empty cache
func New() *Engine {
	return &Engine{
		cache:  NewCache(),
		mapper: NewActionMapper(),
	}
}

// Evaluate evaluates a request against all applicable policies.
// This is the main entry point for authorization decisions.
//
// Evaluation order:
// 1. Admin bypass - authenticated admin can do everything
// 2. Explicit deny in bucket policy - immediately denies
// 3. Allow in bucket policy - allows if found
// 4. IAM user policies (Phase 5) - not implemented yet
// 5. Default deny - if no explicit allow found
//
// The action in req.Action should be the MAPPED permission (from ActionMapper),
// not the route action. The authorization middleware handles this translation.
func (e *Engine) Evaluate(ctx context.Context, req *RequestContext) Decision {
	// 1. Admin bypass (authenticated admin can do everything)
	if req.Principal != nil && req.Principal.IsAdmin {
		return DecisionAllow
	}

	// 2. Bucket policy evaluation (if bucket specified)
	if req.Resource != nil && req.Resource.Bucket != "" {
		bucketPolicy := e.cache.GetBucketPolicy(req.Resource.Bucket)
		if bucketPolicy != nil {
			decision := e.evaluatePolicy(bucketPolicy, req)
			if decision == DecisionExplicitDeny {
				return DecisionExplicitDeny // Explicit deny always wins
			}
			if decision == DecisionAllow {
				return DecisionAllow
			}
		}
	}

	// 3. IAM user policy evaluation (Phase 5 - defer)
	// TODO: Evaluate user's attached IAM policies

	// 4. Default behavior based on authentication status
	if req.Principal != nil && !req.Principal.IsAnonymous {
		// Phase 3.1: Authenticated non-admin users denied by default
		// Phase 5: Will evaluate user's attached IAM policies instead
		return DecisionDeny
	}

	// 5. Default deny for anonymous
	return DecisionDeny
}

// evaluatePolicy evaluates a single policy document against a request.
// Returns DecisionExplicitDeny if any statement explicitly denies.
// Returns DecisionAllow if any statement allows (and no explicit deny).
// Returns DecisionDeny if no statements match.
func (e *Engine) evaluatePolicy(policy *iam.PolicyDocument, req *RequestContext) Decision {
	hasAllow := false

	for i := range policy.Statement {
		stmt := &policy.Statement[i]
		result := evaluateStatement(stmt, req)

		// Explicit deny always wins immediately
		if result == DecisionExplicitDeny {
			return DecisionExplicitDeny
		}

		// Track if we found any allows
		if result == DecisionAllow {
			hasAllow = true
		}
	}

	if hasAllow {
		return DecisionAllow
	}
	return DecisionDeny
}

// ============================================================
// Lifecycle Methods - Called by service layer
// ============================================================

// LoadBucketPolicies loads all bucket policies at startup.
// Called by server during initialization.
func (e *Engine) LoadBucketPolicies(ctx context.Context, policies map[string]*iam.PolicyDocument) {
	e.cache.LoadBucketPolicies(policies)
}

// UpdateBucketPolicy updates a single bucket policy at runtime.
// Called by service layer when PutBucketPolicy is executed.
func (e *Engine) UpdateBucketPolicy(bucket string, policy *iam.PolicyDocument) {
	e.cache.SetBucketPolicy(bucket, policy)
}

// DeleteBucketPolicy removes a bucket policy at runtime.
// Called by service layer when DeleteBucketPolicy is executed.
func (e *Engine) DeleteBucketPolicy(bucket string) {
	e.cache.SetBucketPolicy(bucket, nil)
}

// ============================================================
// Accessors
// ============================================================

// GetActionMapper returns the action mapper for use by authorization middleware
func (e *Engine) GetActionMapper() *ActionMapper {
	return e.mapper
}

// GetCache returns the cache for testing/debugging purposes
func (e *Engine) GetCache() *Cache {
	return e.cache
}

// HasBucketPolicy checks if a bucket has a policy set
func (e *Engine) HasBucketPolicy(bucket string) bool {
	return e.cache.HasBucketPolicy(bucket)
}
