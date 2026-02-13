package context

import (
	"context"
	"fmt"

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
)

// WithAuthzDecision returns a new context with the authorization decision set
func WithAuthzDecision(ctx context.Context, decision interface{}) context.Context {
	return context.WithValue(ctx, AuthzDecisionKey, decision)
}

// GetAuthzDecision extracts the authorization decision from the context
func GetAuthzDecision(ctx context.Context) interface{} {
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

func GetUser(ctx context.Context) (*iam.User, error) {
	if userdata, ok := ctx.Value(RequestUserKey).(*iam.User); ok {
		return userdata, nil
	}
	return nil, fmt.Errorf("cannot get user from request context")
}
