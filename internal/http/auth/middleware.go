package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/mallardduck/go-http-helpers/pkg/headers"

	contextInt "github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/http/middleware"
	httpresponse "github.com/mallardduck/dirio/internal/http/response"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/pkg/s3types"
)

func (a *Authenticator) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for header-based authentication (standard AWS SigV4)
		authHeader := r.Header.Get(headers.Authorization)
		if authHeader != "" {
			// Authenticate using Authorization header
			user, err := a.AuthenticateRequest(r)
			if err != nil {
				// Map auth errors to S3 error codes
				var errCode s3types.ErrorCode
				switch {
				case errors.Is(err, ErrAuthenticationFailed):
					errCode = s3types.ErrCodeAccessDenied
				case errors.Is(err, ErrUserNotFound):
					errCode = s3types.ErrCodeInvalidAccessKeyID
				case errors.Is(err, ErrUserInactive):
					errCode = s3types.ErrCodeAccessDenied
				case errors.Is(err, ErrSignatureMismatch):
					errCode = s3types.ErrCodeSignatureDoesNotMatch
				default:
					// Other signature verification errors
					errCode = s3types.ErrCodeSignatureDoesNotMatch
				}
				requestID := middleware.GetRequestID(r.Context())
				if writeErr := httpresponse.WriteErrorResponse(w, requestID, errCode); writeErr != nil {
					authLogger.With("err", err, "error_code", errCode, "write_err", writeErr).Warn("encountered error authenticating request and additional error writing XML error response")
					return
				}
				authLogger.With("err", err, "error_code", errCode).Warn("encountered error authenticating request")
				return
			}

			// Add user to context (standard auth)
			ctx := contextInt.WithUser(r.Context(), user)
			// Store SA metadata in context for policy evaluation (non-admin users only)
			if !a.isRootAdminKey(user.AccessKey) {
				if sa, ok := a.IsServiceAccount(r.Context(), user.AccessKey); ok {
					ctx = contextInt.WithServiceAccountInfo(ctx, &contextInt.ServiceAccountInfo{
						ParentUserUUID: sa.ParentUserUUID,
						PolicyMode:     sa.PolicyMode,
					})
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Check for pre-signed URL authentication (query-based AWS SigV4)
		if r.URL.Query().Get("X-Amz-Algorithm") != "" {
			// Authenticate using pre-signed URL
			user, expiresAt, err := a.AuthenticatePresignedRequest(r)
			if err != nil {
				// Map auth errors to S3 error codes
				var errCode s3types.ErrorCode
				switch {
				case errors.Is(err, ErrPresignedURLExpired):
					errCode = s3types.ErrCodeAccessDenied // AWS returns AccessDenied for expired URLs
				case errors.Is(err, ErrAuthenticationFailed):
					errCode = s3types.ErrCodeAccessDenied
				case errors.Is(err, ErrUserNotFound):
					errCode = s3types.ErrCodeInvalidAccessKeyID
				case errors.Is(err, ErrUserInactive):
					errCode = s3types.ErrCodeAccessDenied
				case errors.Is(err, ErrSignatureMismatch):
					errCode = s3types.ErrCodeSignatureDoesNotMatch
				case errors.Is(err, ErrMissingPresignedParams):
					errCode = s3types.ErrCodeAccessDenied
				default:
					// Other signature verification errors
					errCode = s3types.ErrCodeSignatureDoesNotMatch
				}
				requestID := middleware.GetRequestID(r.Context())
				if writeErr := httpresponse.WriteErrorResponse(w, requestID, errCode); writeErr != nil {
					authLogger.With("err", err, "error_code", errCode, "write_err", writeErr).Warn("encountered error authenticating pre-signed URL request and additional error writing XML error response")
					return
				}
				authLogger.With("err", err, "error_code", errCode).Warn("encountered error authenticating pre-signed URL request")
				return
			}

			// Add user to context with pre-signed marker
			ctx := contextInt.WithPreSignedUser(r.Context(), user, expiresAt)
			// Store SA metadata in context for policy evaluation (non-admin users only)
			if !a.isRootAdminKey(user.AccessKey) {
				if sa, ok := a.IsServiceAccount(r.Context(), user.AccessKey); ok {
					ctx = contextInt.WithServiceAccountInfo(ctx, &contextInt.ServiceAccountInfo{
						ParentUserUUID: sa.ParentUserUUID,
						PolicyMode:     sa.PolicyMode,
					})
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// POST policy form uploads (browser-based multipart/form-data with embedded credentials)
		if r.Method == "POST" && strings.Contains(r.Header.Get(headers.ContentType), "multipart/form-data") {
			user, form, _, err := a.AuthenticatePostPolicyRequest(r)
			if err != nil {
				var errCode s3types.ErrorCode
				switch {
				case errors.Is(err, ErrPresignedURLExpired):
					errCode = s3types.ErrCodeAccessDenied
				case errors.Is(err, ErrAuthenticationFailed):
					errCode = s3types.ErrCodeAccessDenied
				case errors.Is(err, ErrUserNotFound):
					errCode = s3types.ErrCodeInvalidAccessKeyID
				case errors.Is(err, ErrUserInactive):
					errCode = s3types.ErrCodeAccessDenied
				case errors.Is(err, ErrSignatureMismatch):
					errCode = s3types.ErrCodeSignatureDoesNotMatch
				default:
					errCode = s3types.ErrCodeAccessDenied
				}
				requestID := middleware.GetRequestID(r.Context())
				if writeErr := httpresponse.WriteErrorResponse(w, requestID, errCode); writeErr != nil {
					authLogger.With("err", err, "error_code", errCode, "write_err", writeErr).Warn("encountered error authenticating POST policy request and additional error writing XML error response")
					return
				}
				authLogger.With("err", err, "error_code", errCode).Warn("encountered error authenticating POST policy request")
				return
			}

			ctx := contextInt.WithPostPolicyRequest(r.Context(), user, form.PolicyBase64)
			if !a.isRootAdminKey(user.AccessKey) {
				if sa, ok := a.IsServiceAccount(r.Context(), user.AccessKey); ok {
					ctx = contextInt.WithServiceAccountInfo(ctx, &contextInt.ServiceAccountInfo{
						ParentUserUUID: sa.ParentUserUUID,
						PolicyMode:     sa.PolicyMode,
					})
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// No authentication credentials - mark as anonymous
		// Authorization middleware will decide based on bucket policies
		ctx := contextInt.WithAnonymousRequest(r.Context())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetRequestUser(ctx context.Context) *metadata.User {
	if user, ok := ctx.Value(contextInt.RequestUserKey).(*metadata.User); ok {
		return user
	}
	return nil
}
