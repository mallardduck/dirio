package auth

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"

	contextInt "github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/middleware"
	"github.com/mallardduck/dirio/pkg/s3types"
)

func (a *Authenticator) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Authenticate the request
		user, err := a.AuthenticateRequest(r)
		if err != nil {
			// Map auth errors to S3 error codes
			var errCode s3types.ErrorCode
			switch {
			case errors.Is(err, ErrAuthenticationFailed):
				errCode = s3types.ErrAccessDenied
			case errors.Is(err, ErrUserNotFound):
				errCode = s3types.ErrInvalidAccessKeyID
			case errors.Is(err, ErrUserInactive):
				errCode = s3types.ErrAccessDenied
			case errors.Is(err, ErrSignatureMismatch):
				errCode = s3types.ErrSignatureDoesNotMatch
			default:
				// Other signature verification errors
				errCode = s3types.ErrSignatureDoesNotMatch
			}
			requestID := middleware.GetRequestID(r.Context())
			if writeErr := writeAuthError(w, requestID, errCode); writeErr != nil {
				authLogger.With("err", err, "error_code", errCode, "write_err", writeErr).Warn("encountered error authenticating request and additional error writing XML error response")
				return
			}
			authLogger.With("err", err, "error_code", errCode).Warn("encountered error authenticating request")
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), contextInt.RequestUserKey, user)
		// Authentication successful - proceed to next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// writeAuthError writes an S3 error response
func writeAuthError(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode) error {
	response := s3types.ErrorResponse{
		Code:      errCode.String(),
		Message:   errCode.Description(),
		RequestID: requestID,
	}

	var buf bytes.Buffer
	buf.Write([]byte(xml.Header))

	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encoder.Encode(response); err != nil {
		return fmt.Errorf("failed to encode error response: %w", err)
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(errCode.HTTPStatus())
	_, err := w.Write(buf.Bytes())
	return err
}

func GetRequestUser(ctx context.Context) *metadata.User {
	if user, ok := ctx.Value(contextInt.RequestUserKey).(*metadata.User); ok {
		return user
	}
	return nil
}
