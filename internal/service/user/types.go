package user

// CreateUserRequest represents a request to create a new user
type CreateUserRequest struct {
	AccessKey string
	SecretKey string
	Status    string // "on" or "off"
}

// UpdateUserRequest represents a request to update an existing user
type UpdateUserRequest struct {
	SecretKey        *string
	Status           *string
	AttachedPolicies *[]string
}
