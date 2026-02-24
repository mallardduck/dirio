// Package policy provides the policy evaluation engine for S3 authorization.
// It evaluates bucket policies and IAM policies to determine if a request
// should be allowed or denied.
package policy

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/policy/variables"
	"github.com/mallardduck/dirio/pkg/iam"
)

// RequestContext contains all information needed for policy evaluation
type RequestContext struct {
	Principal       *Principal
	Action          string    // The IAM permission to check (e.g., "s3:GetObject")
	Resource        *Resource // The resource being accessed
	Conditions      *ConditionContext
	VarContext      *variables.Context // Variable substitution context (Phase 3.3)
	OriginalRequest *http.Request      // Access to raw request if needed

	// Ownership information (Phase 3.3) - populated by middleware for ownership-based authorization
	BucketOwnerUUID *uuid.UUID // Owner UUID of the bucket (nil if admin-only or unknown)
	ObjectOwnerUUID *uuid.UUID // Owner UUID of the object (nil if admin-only, unknown, or bucket operation)
}

// Principal represents the requester making the API call
type Principal struct {
	User        *metadata.User // nil for anonymous requests
	IsAnonymous bool           // true if no authentication provided
	IsAdmin     bool           // true if root admin (bypass all policies)

	// Service account fields (populated by authorization middleware when request is from a SA)
	IsServiceAccount bool           // true if this principal is a service account
	ParentUserUUID   *uuid.UUID     // parent user UUID (nil if no parent or not a SA)
	PolicyMode       iam.PolicyMode // "inherit" or "override" (empty string treated as inherit)
}

// Resource represents the S3 resource being accessed
type Resource struct {
	Bucket string // Bucket name
	Key    string // Object key (empty for bucket operations)
}

// ARN returns AWS ARN format for this resource
func (r *Resource) ARN() string {
	if r.Bucket == "" {
		return "*" // Service-level resource (ListBuckets)
	}
	if r.Key == "" {
		return "arn:aws:s3:::" + r.Bucket
	}
	return "arn:aws:s3:::" + r.Bucket + "/" + r.Key
}

// ConditionContext contains request metadata for condition evaluation
type ConditionContext struct {
	SourceIP        string
	UserAgent       string
	SecureTransport bool
	CurrentTime     time.Time
	// Expand in Phase 3.2 for condition operators
}

// Decision is the result of policy evaluation
type Decision int

const (
	DecisionDeny         Decision = iota // Default - no explicit allow found
	DecisionAllow                        // Explicit allow found
	DecisionExplicitDeny                 // Explicit deny (highest precedence, always wins)
)

// String returns a human-readable decision name
func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "Allow"
	case DecisionExplicitDeny:
		return "ExplicitDeny"
	case DecisionDeny:
		fallthrough //nolint:gocritic // emptyFallthrough: needed for exhaustive compliance
	default:
		return "Deny"
	}
}

// IsAllowed returns true if the decision permits the operation
func (d Decision) IsAllowed() bool {
	return d == DecisionAllow
}
