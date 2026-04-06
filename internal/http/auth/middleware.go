package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/mallardduck/go-http-helpers/pkg/headers"

	contextInt "github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/http/middleware"
	loggingHttp "github.com/mallardduck/dirio/internal/http/middleware/logging"
	httpresponse "github.com/mallardduck/dirio/internal/http/response"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/pkg/iam"
	"github.com/mallardduck/dirio/pkg/s3types"
)

func (a *Authenticator) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.handleHeaderAuth(w, r, next) {
			return
		}
		if a.handlePresignedAuth(w, r, next) {
			return
		}
		if a.handlePostPolicyAuth(w, r, next) {
			return
		}
		// No authentication credentials - mark as anonymous.
		// Authorization middleware will decide based on bucket policies.
		setLogUser(r, "anonymous")
		ctx := contextInt.WithAnonymousRequest(r.Context())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// setLogUser writes the principal identifier into the access log metadata so
// the outer PrepareAccessLogMiddleware can include it in the log line.
func setLogUser(r *http.Request, user string) {
	if logData, ok := loggingHttp.GetLogData(r.Context()); ok {
		logData.User = user
	}
}

// userIdentifier returns the most human-readable identifier for a user.
// Prefers Username; falls back to AccessKey for service accounts and admins.
func userIdentifier(u *iam.User) string {
	if u.Username != "" {
		return u.Username
	}
	return u.AccessKey
}

// withSAContext enriches ctx with service-account info when the user is a non-admin SA.
func (a *Authenticator) withSAContext(ctx context.Context, accessKey string) context.Context {
	if a.isRootAdminKey(accessKey) {
		return ctx
	}
	sa, ok := a.IsServiceAccount(ctx, accessKey)
	if !ok {
		return ctx
	}
	return contextInt.WithServiceAccountInfo(ctx, &contextInt.ServiceAccountInfo{
		ParentUserUUID:     sa.ParentUserUUID,
		PolicyMode:         sa.PolicyMode,
		EmbeddedPolicyJSON: sa.EmbeddedPolicyJSON,
	})
}

// mapAuthErrCode maps authentication errors to S3 error codes.
func mapAuthErrCode(err error) s3types.ErrorCode {
	switch {
	case errors.Is(err, ErrPresignedURLExpired), errors.Is(err, ErrMissingPresignedParams),
		errors.Is(err, ErrAuthenticationFailed), errors.Is(err, ErrUserInactive):
		return s3types.ErrCodeAccessDenied
	case errors.Is(err, ErrUserNotFound):
		return s3types.ErrCodeInvalidAccessKeyID
	default:
		return s3types.ErrCodeSignatureDoesNotMatch
	}
}

// writeAuthError writes an S3 error response for an auth failure and logs it.
func writeAuthError(w http.ResponseWriter, r *http.Request, err error, msg string) {
	errCode := mapAuthErrCode(err)
	requestID := middleware.GetRequestID(r.Context())
	if writeErr := httpresponse.WriteErrorResponse(w, requestID, errCode); writeErr != nil {
		authLogger.With("err", err, "error_code", errCode, "write_err", writeErr).Warn(msg + " and additional error writing XML error response")
		return
	}
	authLogger.With("err", err, "error_code", errCode).Warn(msg)
}

// handleHeaderAuth handles standard AWS SigV4 Authorization-header auth.
// Returns true if this auth path was attempted (regardless of success/failure).
func (a *Authenticator) handleHeaderAuth(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	if r.Header.Get(headers.Authorization) == "" {
		return false
	}
	user, err := a.AuthenticateRequest(r)
	if err != nil {
		writeAuthError(w, r, err, "encountered error authenticating request")
		return true
	}
	setLogUser(r, userIdentifier(user))
	ctx := contextInt.WithUser(r.Context(), user)
	ctx = a.withSAContext(ctx, user.AccessKey)
	next.ServeHTTP(w, r.WithContext(ctx))
	return true
}

// handlePresignedAuth handles query-based AWS SigV4 pre-signed URL auth.
// Returns true if this auth path was attempted (regardless of success/failure).
func (a *Authenticator) handlePresignedAuth(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	if r.URL.Query().Get("X-Amz-Algorithm") == "" {
		return false
	}
	user, expiresAt, err := a.AuthenticatePresignedRequest(r)
	if err != nil {
		writeAuthError(w, r, err, "encountered error authenticating pre-signed URL request")
		return true
	}
	setLogUser(r, userIdentifier(user))
	ctx := contextInt.WithPreSignedUser(r.Context(), user, expiresAt)
	ctx = a.withSAContext(ctx, user.AccessKey)
	next.ServeHTTP(w, r.WithContext(ctx))
	return true
}

// handlePostPolicyAuth handles browser-based multipart/form-data POST policy auth.
// Returns true if this auth path was attempted (regardless of success/failure).
func (a *Authenticator) handlePostPolicyAuth(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	if r.Method != "POST" || !strings.Contains(r.Header.Get(headers.ContentType), "multipart/form-data") {
		return false
	}
	user, form, _, err := a.AuthenticatePostPolicyRequest(r)
	if err != nil {
		writeAuthError(w, r, err, "encountered error authenticating POST policy request")
		return true
	}
	setLogUser(r, userIdentifier(user))
	ctx := contextInt.WithPostPolicyRequest(r.Context(), user, form.PolicyBase64)
	ctx = a.withSAContext(ctx, user.AccessKey)
	next.ServeHTTP(w, r.WithContext(ctx))
	return true
}

func GetRequestUser(ctx context.Context) *metadata.User {
	if user, ok := ctx.Value(contextInt.RequestUserKey).(*metadata.User); ok {
		return user
	}
	return nil
}
