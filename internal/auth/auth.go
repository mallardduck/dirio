package auth

import (
	"errors"
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
}

// New creates a new authenticator
func New(metadata *metadata.Manager, rootAccessKey, rootSecretKey string) *Authenticator {
	return &Authenticator{
		metadata:      metadata,
		rootAccessKey: rootAccessKey,
		rootSecretKey: rootSecretKey,
	}
}

// ValidateCredentials checks if access key and secret key are valid
func (a *Authenticator) ValidateCredentials(accessKey, secretKey string) bool {
	// Check root credentials
	if accessKey == a.rootAccessKey && secretKey == a.rootSecretKey {
		return true
	}

	// Check user credentials
	users, err := a.metadata.GetUsers()
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
func (a *Authenticator) GetUserForAccessKey(accessKey string) (*metadata.User, error) {
	if accessKey == a.rootAccessKey {
		return &metadata.User{
			AccessKey: a.rootAccessKey,
			SecretKey: a.rootSecretKey,
			Status:    "on",
		}, nil
	}

	users, err := a.metadata.GetUsers()
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
		return nil, ErrAuthenticationFailed
	}

	// Look up user and get secret key
	user, err := a.GetUserForAccessKey(accessKey)
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
