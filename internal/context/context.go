package context

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/pkg/iam"
)

type KeyID string

const (
	RequestUserKey KeyID = "requestUser"
	// RequestIDKey is the context key for request IDs
	RequestIDKey KeyID = "requestID"
	// RequestStartTimeKey is the context key for the request start timestamp
	RequestStartTimeKey KeyID = "requestStartTime"
	// TraceIDKey is the context key for trace IDs
	TraceIDKey KeyID = "traceID"

	// AuthzDecisionKey is the context key for the authorization decision
	// Set by authorization middleware, used for logging
	AuthzDecisionKey KeyID = "authzDecision"

	// IsAnonymousRequestKey marks a request as explicitly anonymous
	// Set by auth middleware when no Authorization header is present
	IsAnonymousRequestKey KeyID = "isAnonymousRequest"

	// IsPreSignedRequestKey marks a request authenticated via pre-signed URL
	// Set by auth middleware when request uses query-based authentication
	IsPreSignedRequestKey KeyID = "isPreSignedRequest"

	// PreSignedExpiresAtKey stores the expiration time of the pre-signed URL
	// Used for auditing and logging purposes
	PreSignedExpiresAtKey KeyID = "preSignedExpiresAt"

	// ServiceAccountInfoKey holds service account metadata for policy evaluation.
	// Set by auth middleware when the authenticated access key belongs to a service account.
	ServiceAccountInfoKey KeyID = "serviceAccountInfo"

	// IsPostPolicyRequestKey marks a request authenticated via POST policy form upload.
	// Set by auth middleware when request uses multipart/form-data with a policy field.
	IsPostPolicyRequestKey KeyID = "isPostPolicyRequest"

	// PostPolicyPolicyB64Key stores the base64-encoded policy document from the form.
	// Used by the PostObject handler to validate conditions.
	PostPolicyPolicyB64Key KeyID = "postPolicyPolicyB64"
)

// ServiceAccountInfo holds service account metadata used by the policy engine
// to resolve effective IAM policies at evaluation time.
type ServiceAccountInfo struct {
	ParentUserUUID     *uuid.UUID     // UUID of the parent user (nil if no parent)
	PolicyMode         iam.PolicyMode // "inherit" or "override" (empty = inherit)
	EmbeddedPolicyJSON string         // raw IAM policy JSON; used directly in override mode
}

// WithAuthzDecision returns a new context with the authorization decision set
func WithAuthzDecision(ctx context.Context, decision any) context.Context {
	return context.WithValue(ctx, AuthzDecisionKey, decision)
}

// GetAuthzDecision extracts the authorization decision from the context
func GetAuthzDecision(ctx context.Context) any {
	return ctx.Value(AuthzDecisionKey)
}

// WithAnonymousRequest marks the request as anonymous in the context
func WithAnonymousRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, IsAnonymousRequestKey, true)
}

// IsAnonymousRequest returns true if the request was marked as anonymous
func IsAnonymousRequest(ctx context.Context) bool {
	if v, ok := ctx.Value(IsAnonymousRequestKey).(bool); ok {
		return v
	}
	return false
}

// WithUser returns a new context with the user set
func WithUser(ctx context.Context, user *iam.User) context.Context {
	return context.WithValue(ctx, RequestUserKey, user)
}

// GetUser extracts the user from the context
func GetUser(ctx context.Context) (*iam.User, error) {
	if userdata, ok := ctx.Value(RequestUserKey).(*iam.User); ok {
		return userdata, nil
	}
	return nil, fmt.Errorf("cannot get user from request context")
}

// WithPreSignedUser adds a user authenticated via pre-signed URL to context
// Also marks the request as pre-signed and stores expiration for auditing
func WithPreSignedUser(ctx context.Context, user *iam.User, expiresAt any) context.Context {
	ctx = context.WithValue(ctx, RequestUserKey, user)
	ctx = context.WithValue(ctx, IsPreSignedRequestKey, true)
	ctx = context.WithValue(ctx, PreSignedExpiresAtKey, expiresAt)
	return ctx
}

// IsPreSignedRequest returns true if request was authenticated via pre-signed URL
func IsPreSignedRequest(ctx context.Context) bool {
	if v, ok := ctx.Value(IsPreSignedRequestKey).(bool); ok {
		return v
	}
	return false
}

// GetPreSignedExpiresAt returns the expiration time of the pre-signed URL
func GetPreSignedExpiresAt(ctx context.Context) (any, bool) {
	if t := ctx.Value(PreSignedExpiresAtKey); t != nil {
		return t, true
	}
	return nil, false
}

// WithPostPolicyRequest adds a user authenticated via POST policy form upload to context.
// Also marks the request as a POST policy upload and stores the base64 policy for the handler.
func WithPostPolicyRequest(ctx context.Context, user *iam.User, policyB64 string) context.Context {
	ctx = context.WithValue(ctx, RequestUserKey, user)
	ctx = context.WithValue(ctx, IsPostPolicyRequestKey, true)
	ctx = context.WithValue(ctx, PostPolicyPolicyB64Key, policyB64)
	return ctx
}

// IsPostPolicyRequest returns true if request was authenticated via POST policy form upload.
func IsPostPolicyRequest(ctx context.Context) bool {
	if v, ok := ctx.Value(IsPostPolicyRequestKey).(bool); ok {
		return v
	}
	return false
}

// GetPostPolicyPolicyB64 returns the base64-encoded policy document from the POST policy form.
func GetPostPolicyPolicyB64(ctx context.Context) string {
	if v, ok := ctx.Value(PostPolicyPolicyB64Key).(string); ok {
		return v
	}
	return ""
}

// WithServiceAccountInfo returns a new context with service account metadata set.
func WithServiceAccountInfo(ctx context.Context, info *ServiceAccountInfo) context.Context {
	return context.WithValue(ctx, ServiceAccountInfoKey, info)
}

// GetServiceAccountInfo extracts service account metadata from the context.
// Returns nil if the request was not made by a service account.
func GetServiceAccountInfo(ctx context.Context) *ServiceAccountInfo {
	if info, ok := ctx.Value(ServiceAccountInfoKey).(*ServiceAccountInfo); ok {
		return info
	}
	return nil
}
