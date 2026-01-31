package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/metadata"
)

var authLogger = logging.Component("auth")

var (
	// ErrAuthenticationFailed is returned when authentication fails
	ErrAuthenticationFailed = errors.New("authentication failed")
	// ErrUserNotFound is returned when the user doesn't exist
	ErrUserNotFound = errors.New("user not found")
	// ErrUserInactive is returned when the user account is not active
	ErrUserInactive = errors.New("user account is not active")
)

// Authenticator handles authentication and authorization
type Authenticator struct {
	metadata      *metadata.Manager
	rootAccessKey string
	rootSecretKey string

	// Additional root credentials (e.g., from data config)
	// These provide an alternative admin account that coexists with CLI admin
	altRootAccessKey string
	altRootSecretKey string
}

// New creates a new authenticator with primary root credentials
func New(metadata *metadata.Manager, rootAccessKey, rootSecretKey string) *Authenticator {
	return &Authenticator{
		metadata:      metadata,
		rootAccessKey: rootAccessKey,
		rootSecretKey: rootSecretKey,
	}
}

// WithAlternativeRoot adds alternative root credentials (e.g., from data config)
// This allows both CLI admin and data config admin to coexist
func (a *Authenticator) WithAlternativeRoot(accessKey, secretKey string) *Authenticator {
	a.altRootAccessKey = accessKey
	a.altRootSecretKey = secretKey
	return a
}

// ValidateCredentials checks if access key and secret key are valid
func (a *Authenticator) ValidateCredentials(ctx context.Context, accessKey, secretKey string) bool {
	// Check primary root credentials (CLI admin)
	if accessKey == a.rootAccessKey && secretKey == a.rootSecretKey {
		return true
	}

	// Check alternative root credentials (data config admin)
	if a.altRootAccessKey != "" && accessKey == a.altRootAccessKey && secretKey == a.altRootSecretKey {
		return true
	}

	// Check user credentials
	users, err := a.metadata.GetUsers(ctx)
	if err != nil {
		return false
	}

	user, exists := users[accessKey]
	if !exists {
		return false
	}

	return user.SecretKey == secretKey && user.Status == "on"
}

// GetUserForAccessKey retrieves user information for an access key
func (a *Authenticator) GetUserForAccessKey(ctx context.Context, accessKey string) (*metadata.User, error) {
	// Check primary root (CLI admin)
	if accessKey == a.rootAccessKey {
		return &metadata.User{
			AccessKey: a.rootAccessKey,
			SecretKey: a.rootSecretKey,
			Status:    "on",
		}, nil
	}

	// Check alternative root (data config admin)
	if a.altRootAccessKey != "" && accessKey == a.altRootAccessKey {
		return &metadata.User{
			AccessKey: a.altRootAccessKey,
			SecretKey: a.altRootSecretKey,
			Status:    "on",
		}, nil
	}

	users, err := a.metadata.GetUsers(ctx)
	if err != nil {
		return nil, err
	}

	user, exists := users[accessKey]
	if !exists {
		return nil, nil
	}

	return user, nil
}

// AuthenticateRequest validates an HTTP request using AWS Signature V4 authentication.
// This is the main entry point for request authentication.
//
// Returns the authenticated user if successful, or an error:
// - ErrAuthenticationFailed: Missing or invalid Authorization header
// - ErrUserNotFound: Access key doesn't exist
// - ErrUserInactive: User account is not active
// - ErrSignatureMismatch: Signature verification failed
func (a *Authenticator) AuthenticateRequest(r *http.Request) (*metadata.User, error) {
	// Extract access key from Authorization header
	accessKey, err := GetAccessKey(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAuthenticationFailed, err)
	}

	// Look up user and get secret key
	user, err := a.GetUserForAccessKey(r.Context(), accessKey)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	// Check if user account is active
	if user.Status != "on" {
		return nil, ErrUserInactive
	}

	// Verify AWS Signature V4
	if err := VerifySignature(r, user.SecretKey); err != nil {
		return nil, err
	}

	return user, nil
}
