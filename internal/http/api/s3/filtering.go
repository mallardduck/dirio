package s3

import (
	stdcontext "context"
	"net/http"
	"strings"
	"time"

	"github.com/mallardduck/go-http-helpers/pkg/headers"

	"github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/policy/variables"
	"github.com/mallardduck/dirio/pkg/iam"
	"github.com/mallardduck/dirio/pkg/s3types"
)

var filterLogger = logging.Component("filter")

// filterBuckets extracts request context and delegates to the observation service.
func (h *HTTPHandler) filterBuckets(ctx stdcontext.Context, buckets []s3types.Bucket, r *http.Request) []s3types.Bucket {
	principal := getRequestPrincipal(ctx, h.adminKeys.PrimaryRootAccessKey(), h.adminKeys.AltRootAccessKey())
	return h.observationSvc.FilterBuckets(ctx, principal, buckets, buildConditionContext(r), variables.FromRequest(r))
}

// filterObjects extracts request context and delegates to the observation service.
func (h *HTTPHandler) filterObjects(ctx stdcontext.Context, bucket string, objects []s3types.Object, r *http.Request) []s3types.Object {
	principal := getRequestPrincipal(ctx, h.adminKeys.PrimaryRootAccessKey(), h.adminKeys.AltRootAccessKey())
	return h.observationSvc.FilterObjects(ctx, principal, bucket, objects, buildConditionContext(r), variables.FromRequest(r))
}

// getRequestPrincipal extracts the principal from request context and builds a Principal.
// This replicates the logic from policy.AuthorizationMiddleware but for filtering use.
func getRequestPrincipal(ctx stdcontext.Context, rootAccessKey, altRootAccessKey string) *policy.Principal {
	// Get authenticated user from context (may be nil for anonymous)
	user := getRequestUser(ctx)

	// Check if explicitly marked as anonymous by auth middleware
	isAnonymous := context.IsAnonymousRequest(ctx) || user == nil

	// Build principal
	return &policy.Principal{
		User:        user,
		IsAnonymous: isAnonymous,
		IsAdmin:     isAdminUser(user, rootAccessKey, altRootAccessKey),
	}
}

// getRequestUser extracts the user from request context.
// Pattern from policy.AuthorizationMiddleware.
func getRequestUser(ctx stdcontext.Context) *iam.User {
	if ctx == nil {
		return nil
	}
	if user, ok := ctx.Value(context.RequestUserKey).(*iam.User); ok {
		return user
	}
	return nil
}

// isAdminUser checks if the user is a root admin.
// Pattern from policy.AuthorizationMiddleware.
func isAdminUser(user *iam.User, rootAccessKey, altRootAccessKey string) bool {
	if user == nil {
		return false
	}
	return user.AccessKey == rootAccessKey ||
		(altRootAccessKey != "" && user.AccessKey == altRootAccessKey)
}

// buildConditionContext creates a ConditionContext from an HTTP request.
// Pattern from policy.AuthorizationMiddleware.
func buildConditionContext(r *http.Request) *policy.ConditionContext {
	return &policy.ConditionContext{
		SourceIP:        extractClientIP(r),
		UserAgent:       r.UserAgent(),
		SecureTransport: r.TLS != nil,
		CurrentTime:     time.Now(),
		ContentLength:   r.ContentLength,
	}
}

// extractClientIP gets the client IP from X-Forwarded-For or RemoteAddr.
// Pattern from policy.AuthorizationMiddleware.
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (may be set by reverse proxy)
	forwarded := r.Header.Get(headers.XForwardedFor)
	if forwarded != "" {
		// X-Forwarded-For can contain multiple IPs; first is the client
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	// Fall back to RemoteAddr
	addr := r.RemoteAddr
	// Strip port if present
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		// Handle IPv6 addresses like [::1]:8080
		if strings.HasPrefix(addr, "[") {
			if bracketIdx := strings.LastIndex(addr, "]"); bracketIdx != -1 && bracketIdx < idx {
				return addr[1:bracketIdx]
			}
		}
		return addr[:idx]
	}
	return addr
}

// principalString returns a string representation of the principal for logging.
// Pattern from policy.AuthorizationMiddleware.
func principalString(p *policy.Principal) string {
	if p.IsAnonymous {
		return "anonymous"
	}
	if p.User != nil {
		return p.User.AccessKey
	}
	return "unknown"
}

// buildOwnerFromContext extracts the owner information from the request context.
// Returns the authenticated user's access key, or "root" for admin/anonymous users.
func buildOwnerFromContext(ctx stdcontext.Context) s3types.Owner {
	user := getRequestUser(ctx)
	if user == nil {
		// Anonymous or error - use "root" as default
		return s3types.Owner{
			ID:          "root",
			DisplayName: "root",
		}
	}

	// Use the authenticated user's access key as the owner ID
	return s3types.Owner{
		ID:          user.AccessKey,
		DisplayName: user.Username,
	}
}
