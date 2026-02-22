package policy

import (
	"bytes"
	stdcontext "context"
	"encoding/xml"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mallardduck/go-http-helpers/pkg/headers"
	"github.com/mallardduck/teapot-router/pkg/teapot"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/policy/variables"
	"github.com/mallardduck/dirio/pkg/s3types"
)

var authzLogger = logging.Component("authz")

// AdminKeyChecker provides the current admin access keys for authorization
// bypass decisions. auth.Authenticator implements this interface, allowing
// live credential reloads to propagate without restarting the server.
type AdminKeyChecker interface {
	PrimaryRootAccessKey() string
	AltRootAccessKey() string
}

// AuthorizationConfig holds configuration for the authorization middleware
type AuthorizationConfig struct {
	// Engine is the policy evaluation engine
	Engine *Engine

	// Metadata is the metadata manager for fetching ownership information
	Metadata *metadata.Manager

	// AdminKeys provides the current admin access keys for bypass checks.
	// Using an interface (rather than captured strings) lets the authenticator
	// rotate credentials at runtime and have them reflected immediately.
	AdminKeys AdminKeyChecker
}

// AuthorizationMiddleware creates middleware that enforces policy-based authorization.
//
// This middleware:
// 1. Extracts the S3 action from route context (set by teapot-router)
// 2. Translates the action to required IAM permission(s) using ActionMapper
// 3. Builds a RequestContext with principal, action, and resource
// 4. Evaluates the request against bucket policies
// 5. Returns 403 AccessDenied if the request is denied
//
// For multi-resource operations (CopyObject), it checks permissions on both
// the source and destination resources.
func AuthorizationMiddleware(config *AuthorizationConfig) func(http.Handler) http.Handler {
	mapper := config.Engine.GetActionMapper()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get the S3 action from route context (set by teapot-router)
			routeAction := teapot.GetAction(r)

			// Skip authorization for non-S3 routes (internal, admin, etc.)
			if routeAction == "" || !strings.HasPrefix(routeAction, "s3:") {
				next.ServeHTTP(w, r)
				return
			}

			// Get authenticated user from context (may be nil for anonymous)
			user := getRequestUser(r.Context())

			// Check if explicitly marked as anonymous by auth middleware
			isAnonymous := context.IsAnonymousRequest(r.Context()) || user == nil

			// Build principal
			principal := &Principal{
				User:        user,
				IsAnonymous: isAnonymous,
				IsAdmin:     isAdmin(user, config.AdminKeys.PrimaryRootAccessKey(), config.AdminKeys.AltRootAccessKey()),
			}

			// Populate SA fields from context (set by auth middleware for service account requests)
			if saInfo := context.GetServiceAccountInfo(r.Context()); saInfo != nil {
				principal.IsServiceAccount = true
				principal.ParentUserUUID = saInfo.ParentUserUUID
				principal.PolicyMode = saInfo.PolicyMode
			}

			// Admin bypass - skip all policy checks
			if principal.IsAdmin {
				authzLogger.With("action", routeAction, "user", user.AccessKey).
					Debug("admin bypass - skipping authorization")
				next.ServeHTTP(w, r)
				return
			}

			// Translate route action to required IAM permission(s)
			requiredPermissions := mapper.GetRequiredPermissions(routeAction)

			// Extract resource from route params
			bucket := teapot.URLParam(r, "bucket")
			key := teapot.URLParam(r, "key")

			// Build condition context
			conditions := &ConditionContext{
				SourceIP:        extractClientIP(r),
				UserAgent:       r.UserAgent(),
				SecureTransport: r.TLS != nil,
				CurrentTime:     time.Now(),
			}

			// Build variable context for policy variable substitution
			varCtx := variables.FromRequest(r)

			// Handle multi-resource operations (CopyObject, UploadPartCopy)
			if mapper.IsMultiResourceAction(routeAction) {
				decision := evaluateMultiResourceAction(
					config.Engine,
					principal,
					requiredPermissions,
					bucket,
					key,
					r,
					conditions,
					varCtx,
					config.Metadata,
				)
				if !decision.IsAllowed() {
					writeAccessDenied(w, r)
					return
				}
			} else {
				// Single resource operation
				resource := &Resource{
					Bucket: bucket,
					Key:    key,
				}

				// Use the FIRST required permission (most operations have only one)
				permission := requiredPermissions[0]

				// Fetch ownership information for authorization
				bucketOwnerUUID, objectOwnerUUID := fetchOwnership(r.Context(), config.Metadata, bucket, key)

				reqCtx := &RequestContext{
					Principal:       principal,
					Action:          permission,
					Resource:        resource,
					Conditions:      conditions,
					VarContext:      varCtx,
					BucketOwnerUUID: bucketOwnerUUID,
					ObjectOwnerUUID: objectOwnerUUID,
					OriginalRequest: r,
				}

				decision := config.Engine.Evaluate(r.Context(), reqCtx)

				if !decision.IsAllowed() {
					authzLogger.With(
						"action", routeAction,
						"permission", permission,
						"bucket", bucket,
						"key", key,
						"principal", principalString(principal),
						"decision", decision.String(),
					).Debug("access denied")

					writeAccessDenied(w, r)
					return
				}

				// Store decision in context for logging
				ctx := context.WithAuthzDecision(r.Context(), decision)
				r = r.WithContext(ctx)
			}

			// Authorization passed - proceed to handler
			next.ServeHTTP(w, r)
		})
	}
}

// getRequestUser extracts the user from request context.
func getRequestUser(ctx stdcontext.Context) *metadata.User {
	if ctx == nil {
		return nil
	}
	if user, ok := ctx.Value(context.RequestUserKey).(*metadata.User); ok {
		return user
	}
	return nil
}

// evaluateMultiResourceAction handles operations that require checking
// permissions on multiple resources (e.g., CopyObject needs GetObject on
// source AND PutObject on destination).
func evaluateMultiResourceAction(
	engine *Engine,
	principal *Principal,
	permissions []string,
	destBucket, destKey string,
	r *http.Request,
	conditions *ConditionContext,
	varCtx *variables.Context,
	metadata *metadata.Manager,
) Decision {
	// Parse source from X-Amz-Copy-Source header
	sourceBucket, sourceKey := parseCopySource(r)

	if sourceBucket == "" {
		// No copy source header - this shouldn't happen for CopyObject
		authzLogger.Warn("CopyObject without X-Amz-Copy-Source header")
		return DecisionDeny
	}

	// Fetch ownership for source resource
	sourceBucketOwnerUUID, sourceObjectOwnerUUID := fetchOwnership(r.Context(), metadata, sourceBucket, sourceKey)

	// Check source permission (GetObject)
	sourceCtx := &RequestContext{
		Principal:       principal,
		Action:          permissions[0], // s3:GetObject
		Resource:        &Resource{Bucket: sourceBucket, Key: sourceKey},
		Conditions:      conditions,
		VarContext:      varCtx,
		BucketOwnerUUID: sourceBucketOwnerUUID,
		ObjectOwnerUUID: sourceObjectOwnerUUID,
		OriginalRequest: r,
	}
	sourceDecision := engine.Evaluate(r.Context(), sourceCtx)
	if !sourceDecision.IsAllowed() {
		authzLogger.With(
			"permission", permissions[0],
			"bucket", sourceBucket,
			"key", sourceKey,
			"principal", principalString(principal),
		).Debug("copy source access denied")
		return sourceDecision
	}

	// Fetch ownership for destination resource
	destBucketOwnerUUID, destObjectOwnerUUID := fetchOwnership(r.Context(), metadata, destBucket, destKey)

	// Check destination permission (PutObject)
	destCtx := &RequestContext{
		Principal:       principal,
		Action:          permissions[1], // s3:PutObject
		Resource:        &Resource{Bucket: destBucket, Key: destKey},
		Conditions:      conditions,
		VarContext:      varCtx,
		BucketOwnerUUID: destBucketOwnerUUID,
		ObjectOwnerUUID: destObjectOwnerUUID,
		OriginalRequest: r,
	}
	destDecision := engine.Evaluate(r.Context(), destCtx)
	if !destDecision.IsAllowed() {
		authzLogger.With(
			"permission", permissions[1],
			"bucket", destBucket,
			"key", destKey,
			"principal", principalString(principal),
		).Debug("copy destination access denied")
		return destDecision
	}

	return DecisionAllow
}

// parseCopySource extracts bucket and key from X-Amz-Copy-Source header.
// Format: /bucket/key or bucket/key (URL-encoded)
func parseCopySource(r *http.Request) (bucket, key string) {
	copySource := r.Header.Get(consts.HeaderCopySource)
	if copySource == "" {
		return "", ""
	}

	// Remove leading slash if present
	copySource = strings.TrimPrefix(copySource, "/")

	// Split into bucket and key
	parts := strings.SplitN(copySource, "/", 2)
	if len(parts) < 1 {
		return "", ""
	}

	bucket = parts[0]
	if len(parts) == 2 {
		key = parts[1]
	}

	return bucket, key
}

// isAdmin checks if the user is a root admin
func isAdmin(user *metadata.User, rootAccessKey, altRootAccessKey string) bool {
	if user == nil {
		return false
	}
	return user.AccessKey == rootAccessKey ||
		(altRootAccessKey != "" && user.AccessKey == altRootAccessKey)
}

// extractClientIP gets the client IP from X-Forwarded-For or RemoteAddr
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

// principalString returns a string representation of the principal for logging
func principalString(p *Principal) string {
	if p.IsAnonymous {
		return "anonymous"
	}
	if p.User != nil {
		return p.User.AccessKey
	}
	return "unknown"
}

// getRequestID extracts the request ID from context
func getRequestID(ctx stdcontext.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(context.RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// fetchOwnership fetches bucket and object ownership UUIDs for authorization.
// Returns nil for owners if metadata not found (admin-only resources or errors).
func fetchOwnership(ctx stdcontext.Context, metadataMgr *metadata.Manager, bucket, key string) (bucketOwnerUUID, objectOwnerUUID *uuid.UUID) {
	if metadataMgr == nil {
		return nil, nil
	}

	// Fetch bucket metadata for bucket owner UUID
	if bucket != "" {
		bucketMeta, err := metadataMgr.GetBucketMetadata(ctx, bucket)
		if err == nil && bucketMeta != nil {
			bucketOwnerUUID = bucketMeta.Owner // Already a *uuid.UUID
		}
	}

	// Fetch object metadata for object owner UUID (only for object operations)
	if bucket != "" && key != "" {
		objectMeta, err := metadataMgr.GetObjectMetadata(ctx, bucket, key)
		if err == nil && objectMeta != nil {
			objectOwnerUUID = objectMeta.Owner // Already a *uuid.UUID
		}
	}

	return bucketOwnerUUID, objectOwnerUUID
}

// writeAccessDenied writes an S3 AccessDenied error response
func writeAccessDenied(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestID(r.Context())

	response := s3types.ErrorResponse{
		Code:      s3types.ErrCodeAccessDenied.String(),
		Message:   s3types.ErrCodeAccessDenied.Description(),
		Resource:  r.URL.Path,
		RequestID: requestID,
	}

	var buf bytes.Buffer
	buf.Write([]byte(xml.Header))

	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encoder.Encode(response); err != nil {
		authzLogger.With("error", err).Error("failed to encode access denied response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set(headers.ContentType, "application/xml")
	w.WriteHeader(http.StatusForbidden)
	_, err := w.Write(buf.Bytes())
	if err != nil {
		authzLogger.With("error", err).Error("failed to write access denied response")
		return
	}
}
