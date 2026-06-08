package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationError_Error(t *testing.T) {
	err := NewValidationError("AccessKey", "must be alphanumeric")
	assert.Equal(t, "AccessKey", err.Field)
	assert.Equal(t, "must be alphanumeric", err.Message)
	assert.Contains(t, err.Error(), "AccessKey")
	assert.Contains(t, err.Error(), "must be alphanumeric")
}

func TestIsNotFound(t *testing.T) {
	notFoundErrs := []error{
		ErrUserNotFound,
		ErrPolicyNotFound,
		ErrBucketNotFound,
		ErrObjectNotFound,
		ErrGroupNotFound,
		ErrServiceAccountNotFound,
	}
	for _, err := range notFoundErrs {
		assert.True(t, IsNotFound(err), "expected IsNotFound(%v) = true", err)
	}

	assert.False(t, IsNotFound(ErrUserAlreadyExists))
	assert.False(t, IsNotFound(ErrInvalidAccessKey))
	assert.False(t, IsNotFound(nil))
	assert.False(t, IsNotFound(errors.New("random error")))
}

func TestIsAlreadyExists(t *testing.T) {
	alreadyExistsErrs := []error{
		ErrUserAlreadyExists,
		ErrPolicyAlreadyExists,
		ErrBucketAlreadyExists,
		ErrGroupAlreadyExists,
		ErrServiceAccountAlreadyExists,
	}
	for _, err := range alreadyExistsErrs {
		assert.True(t, IsAlreadyExists(err), "expected IsAlreadyExists(%v) = true", err)
	}

	assert.False(t, IsAlreadyExists(ErrUserNotFound))
	assert.False(t, IsAlreadyExists(nil))
}

func TestIsValidation(t *testing.T) {
	// ValidationError struct
	assert.True(t, IsValidation(NewValidationError("Field", "msg")))

	// Sentinel validation errors
	validationErrs := []error{
		ErrInvalidAccessKey,
		ErrInvalidSecretKey,
		ErrInvalidStatus,
		ErrInvalidPolicyName,
		ErrInvalidPolicyDoc,
		ErrInvalidBucketName,
		ErrInvalidObjectKey,
	}
	for _, err := range validationErrs {
		assert.True(t, IsValidation(err), "expected IsValidation(%v) = true", err)
	}

	assert.False(t, IsValidation(ErrUserNotFound))
	assert.False(t, IsValidation(nil))
	assert.False(t, IsValidation(errors.New("random")))
}

func TestIsValidation_WrappedValidationError(t *testing.T) {
	valErr := NewValidationError("Field", "message")
	wrapped := fmt.Errorf("outer: %w", valErr)
	assert.True(t, IsValidation(wrapped))
}

func TestSentinelErrors_AreDistinct(t *testing.T) {
	// All sentinel errors must be distinct values (not the same pointer).
	allErrs := []error{
		ErrUserNotFound, ErrUserAlreadyExists, ErrUserIsSystemAdmin,
		ErrInvalidAccessKey, ErrInvalidSecretKey, ErrInvalidStatus,
		ErrPolicyNotFound, ErrPolicyAlreadyExists, ErrPolicyIsBuiltin,
		ErrInvalidPolicyName, ErrInvalidPolicyDoc,
		ErrGroupNotFound, ErrGroupAlreadyExists, ErrInvalidGroupName,
		ErrServiceAccountNotFound, ErrServiceAccountAlreadyExists,
		ErrBucketNotFound, ErrBucketAlreadyExists, ErrBucketNotEmpty,
		ErrInvalidBucketName, ErrObjectNotFound, ErrInvalidObjectKey,
	}
	seen := make(map[string]bool)
	for _, err := range allErrs {
		msg := err.Error()
		require.False(t, seen[msg], "duplicate sentinel error message: %q", msg)
		seen[msg] = true
	}
}
