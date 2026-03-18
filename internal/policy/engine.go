package policy

import (
	"context"
	"encoding/json"

	"github.com/mallardduck/dirio/pkg/iam"
)

// Ensure MetadataResolver satisfies PolicyResolver at compile time.
var _ Resolver = (*MetadataResolver)(nil)

// Engine is the core policy evaluation engine.
// It maintains an in-memory cache of policies for fast evaluation
// and provides the main Evaluate() method for authorization decisions.
//
// Design principles:
// - Pure in-memory evaluation (no direct FS access)
// - Thread-safe concurrent access via Cache
// - Service layer notifies Engine of policy changes
type Engine struct {
	cache    *Cache
	mapper   *ActionMapper
	resolver Resolver // may be nil (IAM policy step is skipped when nil)
}

// New creates a new policy engine with the given resolver.
// Pass a MetadataResolver for production use; pass nil to skip IAM policy evaluation
// (useful in tests that only exercise bucket policies or ownership).
func New(resolver Resolver) *Engine {
	return &Engine{
		cache:    NewCache(),
		mapper:   NewActionMapper(),
		resolver: resolver,
	}
}

// Evaluate evaluates a request against all applicable policies.
// This is the main entry point for authorization decisions.
//
// Evaluation order (AWS-like model):
// 1. Admin bypass - authenticated admin can do everything
// 2. Explicit deny in bucket policy - immediately denies (irrevocable)
// 3. IAM Policy Evalutions
// 3.1. Allow in bucket policy - allows if found
// 3.2. IAM user policies (Phase 5) - not implemented yet
// 3.2. IAM group policies (Phase 5) - not implemented yet
// 4. Ownership check - resource owner has implicit access
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
		if d := e.checkBucketPolicy(req); d != DecisionDeny {
			return d
		}
	}

	// 3. IAM policy evaluation
	// Evaluates the effective IAM policies for the principal (user or service account).
	// Explicit deny from IAM beats everything; explicit allow beats ownership/default-deny.
	if req.Principal != nil && !req.Principal.IsAnonymous {
		// SA override mode: evaluate the embedded policy JSON directly (no store lookup).
		if req.Principal.IsServiceAccount && req.Principal.PolicyMode == iam.PolicyModeOverride {
			if req.Principal.EmbeddedPolicyJSON != "" {
				var doc iam.PolicyDocument
				if json.Unmarshal([]byte(req.Principal.EmbeddedPolicyJSON), &doc) == nil {
					d := e.evaluatePolicy(&doc, req)
					if d == DecisionExplicitDeny {
						return DecisionExplicitDeny
					}
					if d == DecisionAllow {
						return DecisionAllow
					}
				}
			}
			// Override mode with no (or unparseable) policy: deny everything.
			return DecisionDeny
		}

		// Inherit mode (and regular users): resolve named policies from the store.
		if e.resolver != nil {
			policyNames := e.resolveEffectivePolicyNames(ctx, req.Principal)
			for _, name := range policyNames {
				doc, err := e.resolver.GetPolicyDocument(ctx, name)
				if err != nil {
					continue // skip missing or inaccessible policies
				}
				d := e.evaluatePolicy(doc, req)
				if d == DecisionExplicitDeny {
					return DecisionExplicitDeny
				}
				if d == DecisionAllow {
					return DecisionAllow
				}
			}
		}
	}

	// 4. Ownership check (Phase 3.3 - AWS-like position 3.5)
	// If the authenticated user owns the resource, grant implicit access
	// This check happens AFTER explicit deny (AWS model: deny beats ownership)
	if req.Principal != nil && !req.Principal.IsAnonymous {
		if checkOwnership(req) {
			return DecisionAllow
		}
	}

	// 5. Default deny
	return DecisionDeny
}

// checkBucketPolicy evaluates the bucket policy for the request's bucket.
// Returns DecisionDeny if no bucket policy is set (caller should continue evaluation).
func (e *Engine) checkBucketPolicy(req *RequestContext) Decision {
	bucketPolicy := e.cache.GetBucketPolicy(req.Resource.Bucket)
	if bucketPolicy == nil {
		return DecisionDeny
	}
	return e.evaluatePolicy(bucketPolicy, req)
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

// ============================================================
// IAM Policy Resolution
// ============================================================

// resolveEffectivePolicyNames returns the named IAM policies to evaluate for the principal.
//
// For regular users: their own AttachedPolicies plus policies from all active groups.
// For SAs in inherit mode: the parent user's AttachedPolicies and group policies.
// SA override mode is handled before this call in Evaluate() via embedded JSON.
func (e *Engine) resolveEffectivePolicyNames(ctx context.Context, principal *Principal) []string {
	if !principal.IsServiceAccount {
		// Regular user: own policies plus group policies.
		if principal.User == nil {
			return nil
		}
		names := make([]string, len(principal.User.AttachedPolicies))
		copy(names, principal.User.AttachedPolicies)
		if groupPolicies, err := e.resolver.GetGroupPoliciesForUser(ctx, principal.User.UUID); err == nil {
			names = append(names, groupPolicies...)
		}
		return names
	}

	// SA inherit mode: fetch parent user's policy names and group policies.
	if principal.ParentUserUUID == nil {
		return nil // no parent, no policies
	}
	parentUUID := *principal.ParentUserUUID
	names, err := e.resolver.GetUserPolicyNamesByUUID(ctx, parentUUID)
	if err != nil {
		return nil // parent not found or error — fail closed
	}
	if groupPolicies, err := e.resolver.GetGroupPoliciesForUser(ctx, parentUUID); err == nil {
		names = append(names, groupPolicies...)
	}
	return names
}

// ============================================================
// Ownership-based Authorization (Phase 3.3)
// ============================================================

// checkOwnership checks if the principal owns the resource being accessed.
// Returns true if the user owns the bucket (for bucket operations) or
// the object (for object operations).
//
// Ownership logic (AWS-like):
// - For object operations: Check if user owns the object
// - For bucket operations: Check if user owns the bucket
// - nil owner means admin-only (not owned by this user)
// - Ownership grants implicit access (unless denied by explicit deny)
func checkOwnership(req *RequestContext) bool {
	if req.Principal == nil || req.Principal.User == nil {
		return false // Anonymous or nil principal
	}

	if req.Resource == nil {
		return false // No resource specified
	}

	userUUID := req.Principal.User.UUID

	// For object operations, check object ownership first
	if req.Resource.Key != "" && req.ObjectOwnerUUID != nil {
		if *req.ObjectOwnerUUID == userUUID {
			return true
		}
		// Object not owned by user, but might still own the bucket
		// (bucket owner has control over all objects in AWS model)
	}

	// For bucket operations or if object not owned, check bucket ownership
	if req.BucketOwnerUUID != nil && *req.BucketOwnerUUID == userUUID {
		return true
	}

	return false
}
