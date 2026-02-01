package errors

import (
	"errors"
	"fmt"
)

// User errors
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrInvalidAccessKey  = errors.New("invalid access key")
	ErrInvalidSecretKey  = errors.New("invalid secret key")
	ErrInvalidStatus     = errors.New("invalid status")
)

// Policy errors
var (
	ErrPolicyNotFound      = errors.New("policy not found")
	ErrPolicyAlreadyExists = errors.New("policy already exists")
	ErrInvalidPolicyName   = errors.New("invalid policy name")
	ErrInvalidPolicyDoc    = errors.New("invalid policy document")
)

// S3 errors (buckets and objects)
var (
	ErrBucketNotFound      = errors.New("bucket not found")
	ErrBucketAlreadyExists = errors.New("bucket already exists")
	ErrBucketNotEmpty      = errors.New("bucket not empty")
	ErrInvalidBucketName   = errors.New("invalid bucket name")
	ErrObjectNotFound      = errors.New("object not found")
	ErrInvalidObjectKey    = errors.New("invalid object key")
)

// ValidationError represents a field-specific validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s", e.Field, e.Message)
}

// NewValidationError creates a new validation error
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// IsNotFound checks if an error is a "not found" error
func IsNotFound(err error) bool {
	return errors.Is(err, ErrUserNotFound) ||
		errors.Is(err, ErrPolicyNotFound) ||
		errors.Is(err, ErrBucketNotFound) ||
		errors.Is(err, ErrObjectNotFound)
}

// IsAlreadyExists checks if an error is an "already exists" error
func IsAlreadyExists(err error) bool {
	return errors.Is(err, ErrUserAlreadyExists) ||
		errors.Is(err, ErrPolicyAlreadyExists) ||
		errors.Is(err, ErrBucketAlreadyExists)
}

// IsValidation checks if an error is a validation error
func IsValidation(err error) bool {
	var valErr *ValidationError
	return errors.As(err, &valErr) ||
		errors.Is(err, ErrInvalidAccessKey) ||
		errors.Is(err, ErrInvalidSecretKey) ||
		errors.Is(err, ErrInvalidStatus) ||
		errors.Is(err, ErrInvalidPolicyName) ||
		errors.Is(err, ErrInvalidPolicyDoc) ||
		errors.Is(err, ErrInvalidBucketName) ||
		errors.Is(err, ErrInvalidObjectKey)
}
