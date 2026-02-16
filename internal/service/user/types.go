package user

import "github.com/mallardduck/dirio/pkg/iam"

// CreateUserRequest represents a request to create a new user
type CreateUserRequest struct {
	AccessKey string
	SecretKey string
	Status    iam.UserStatus
}

// UpdateUserRequest represents a request to update an existing user
type UpdateUserRequest struct {
	SecretKey        *string
	Status           *iam.UserStatus
	AttachedPolicies *[]string
}
