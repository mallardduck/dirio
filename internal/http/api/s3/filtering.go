package s3

import (
	stdcontext "context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mallardduck/go-http-helpers/pkg/headers"

	"github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/policy/variables"
	"github.com/mallardduck/dirio/pkg/iam"
	"github.com/mallardduck/dirio/pkg/s3types"
)

var filterLogger = logging.Component("filter")

// filterBuckets filters a list of buckets based on s3:GetBucketLocation permission.
// Returns only buckets the requesting principal has permission to access.
//
// Algorithm:
//  1. Extract principal from request context
//  2. If admin → return all buckets (fast path)
//  3. For each bucket:
//     a. Fetch bucket metadata for ownership
//     b. Build RequestContext for s3:GetBucketLocation
//     c. Evaluate permission via policy engine
//     d. Include if allowed
//  4. Return filtered list
func (h *HTTPHandler) filterBuckets(ctx stdcontext.Context, buckets []s3types.Bucket, r *http.Request) []s3types.Bucket {
	// Extract principal from context
	principal := getRequestPrincipal(ctx, h.adminKeys.PrimaryRootAccessKey(), h.adminKeys.AltRootAccessKey())

	// Admin fast path - return all buckets
	if principal.IsAdmin {
		filterLogger.Debug("admin bypass - returning all buckets")
		return buckets
	}

	// Build condition context for policy evaluation
	conditions := buildConditionContext(r)

	// Build variable context for policy variable substitution
	varCtx := variables.FromRequest(r)

	filtered := make([]s3types.Bucket, 0, len(buckets))
	allowedCount := 0
	deniedCount := 0

	for i := range buckets {
		bucket := &buckets[i]

		// Fetch bucket owner for ownership-based policy evaluation.
		bucketMeta, err := h.s3Service.GetBucket(ctx, bucket.Name)
		var bucketOwnerUUID *uuid.UUID
		if err == nil && bucketMeta != nil {
			bucketOwnerUUID = bucketMeta.Owner
		} else {
			// Metadata fetch failure - log and treat as deny (safe default)
			filterLogger.With("bucket", bucket.Name, "error", err).
				Debug("failed to fetch bucket metadata, treating as deny")
		}

		// Build request context for permission check
		reqCtx := &policy.RequestContext{
			Principal: principal,
			Action:    "s3:GetBucketLocation",
			Resource: &policy.Resource{
				Bucket: bucket.Name,
				Key:    "",
			},
			Conditions:      conditions,
			VarContext:      varCtx,
			BucketOwnerUUID: bucketOwnerUUID,
			ObjectOwnerUUID: nil, // Not applicable for bucket operations
			OriginalRequest: r,
		}

		// Evaluate permission
		decision := h.policyEngine.Evaluate(ctx, reqCtx)

		if decision.IsAllowed() {
			filtered = append(filtered, *bucket)
			allowedCount++
		} else {
			deniedCount++
		}
	}

	// Log filtering results
	principalStr := principalString(principal)
	filterLogger.With(
		"principal", principalStr,
		"total_buckets", len(buckets),
		"allowed", allowedCount,
		"denied", deniedCount,
	).Debug("filtered bucket list")

	return filtered
}

// filterObjects filters a list of objects based on s3:GetObject permission.
// Returns only objects the requesting principal has permission to access.
//
// Algorithm:
//  1. Extract principal from request context
//  2. If admin → return all objects (fast path)
//  3. Fetch bucket owner once (reuse for all objects)
//  4. For each object:
//     a. Fetch object metadata for ownership
//     b. Build RequestContext for s3:GetObject
//     c. Evaluate permission via policy engine
//     d. Include if allowed
//  5. Return filtered list
func (h *HTTPHandler) filterObjects(ctx stdcontext.Context, bucket string, objects []s3types.Object, r *http.Request) []s3types.Object {
	// Extract principal from context
	principal := getRequestPrincipal(ctx, h.adminKeys.PrimaryRootAccessKey(), h.adminKeys.AltRootAccessKey())

	// Admin fast path - return all objects
	if principal.IsAdmin {
		filterLogger.Debug("admin bypass - returning all objects", "bucket", bucket)
		return objects
	}

	// Build condition context for policy evaluation
	conditions := buildConditionContext(r)

	// Build variable context for policy variable substitution
	varCtx := variables.FromRequest(r)

	// Optimization: Fetch bucket owner once and reuse for all objects.
	var bucketOwnerUUID *uuid.UUID
	bucketMeta, err := h.s3Service.GetBucket(ctx, bucket)
	if err == nil && bucketMeta != nil {
		bucketOwnerUUID = bucketMeta.Owner
	} else {
		// Non-fatal: bucket metadata fetch failure
		filterLogger.With("bucket", bucket, "error", err).
			Debug("failed to fetch bucket metadata for filtering")
	}

	filtered := make([]s3types.Object, 0, len(objects))
	allowedCount := 0
	deniedCount := 0

	for i := range objects {
		obj := &objects[i]

		// Fetch object owner for ownership-based policy evaluation.
		objectOwnerUUID, err := h.s3Service.GetObjectOwnerUUID(ctx, bucket, obj.Key)
		if err != nil {
			// Metadata fetch failure - log and treat as deny (safe default)
			filterLogger.With("bucket", bucket, "key", obj.Key, "error", err).
				Debug("failed to fetch object metadata, treating as deny")
			objectOwnerUUID = nil
		}

		// Build request context for permission check
		reqCtx := &policy.RequestContext{
			Principal: principal,
			Action:    "s3:GetObject",
			Resource: &policy.Resource{
				Bucket: bucket,
				Key:    obj.Key,
			},
			Conditions:      conditions,
			VarContext:      varCtx,
			BucketOwnerUUID: bucketOwnerUUID,
			ObjectOwnerUUID: objectOwnerUUID,
			OriginalRequest: r,
		}

		// Evaluate permission
		decision := h.policyEngine.Evaluate(ctx, reqCtx)

		if decision.IsAllowed() {
			filtered = append(filtered, *obj)
			allowedCount++
		} else {
			deniedCount++
		}
	}

	// Log filtering results
	principalStr := principalString(principal)
	filterRatio := 0.0
	if len(objects) > 0 {
		filterRatio = float64(deniedCount) / float64(len(objects))
	}
	filterLogger.With(
		"bucket", bucket,
		"principal", principalStr,
		"total_objects", len(objects),
		"allowed", allowedCount,
		"denied", deniedCount,
		"filter_ratio", filterRatio,
	).Debug("filtered object list")

	return filtered
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
