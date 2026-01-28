package auth

import (
	"context"
	"encoding/xml"
	"net/http"

	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/middleware"
	"github.com/mallardduck/dirio/pkg/s3types"
)

const RequestUserKey = "requestUser"

func (a *Authenticator) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := middleware.GetRequestID(r.Context())

		// Authenticate the request
		user, err := a.AuthenticateRequest(r)
		if err != nil {
			// Map auth errors to S3 error codes
			var errCode s3types.ErrorCode
			switch err {
			case ErrAuthenticationFailed:
				errCode = s3types.ErrAccessDenied
			case ErrUserNotFound:
				errCode = s3types.ErrInvalidAccessKeyID
			case ErrUserInactive:
				errCode = s3types.ErrAccessDenied
			case ErrSignatureMismatch:
				errCode = s3types.ErrSignatureDoesNotMatch
			default:
				// Other signature verification errors
				errCode = s3types.ErrSignatureDoesNotMatch
			}
			writeAuthError(w, requestID, errCode)
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), RequestUserKey, user)
		// Authentication successful - proceed to next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// writeAuthError writes an S3 error response
func writeAuthError(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(errCode.HTTPStatus())

	response := s3types.ErrorResponse{
		Code:      errCode.String(),
		Message:   errCode.Description(),
		RequestID: requestID,
	}

	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(response)
}

func GetRequestUser(ctx context.Context) *metadata.User {
	if user, ok := ctx.Value(RequestUserKey).(*metadata.User); ok {
		return user
	}
	return nil
}
