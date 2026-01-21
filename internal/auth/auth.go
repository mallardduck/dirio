package auth

import (
	"github.com/mallardduck/dirio/internal/metadata"
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

// TODO: Implement AWS Signature V4 authentication
// This will be added in a future phase
