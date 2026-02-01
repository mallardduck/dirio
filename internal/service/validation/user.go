package validation

import (
	"fmt"

	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
)

const (
	MinAccessKeyLength = 3 // AWS IAM and MinIO allow short usernames like "bob"
	MaxAccessKeyLength = 20
	MinSecretKeyLength = 8
)

// ValidateAccessKey validates an access key
func ValidateAccessKey(accessKey string) error {
	if accessKey == "" {
		return svcerrors.NewValidationError("AccessKey", "access key is required")
	}
	if !InRange(accessKey, MinAccessKeyLength, MaxAccessKeyLength) {
		return svcerrors.NewValidationError("AccessKey",
			fmt.Sprintf("access key must be between %d and %d characters", MinAccessKeyLength, MaxAccessKeyLength))
	}
	if !IsAlphanumeric(accessKey) {
		return svcerrors.NewValidationError("AccessKey", "access key must contain only alphanumeric characters")
	}
	return nil
}

// ValidateSecretKey validates a secret key
func ValidateSecretKey(secretKey string) error {
	if secretKey == "" {
		return svcerrors.NewValidationError("SecretKey", "secret key is required")
	}
	if len(secretKey) < MinSecretKeyLength {
		return svcerrors.NewValidationError("SecretKey",
			fmt.Sprintf("secret key must be at least %d characters", MinSecretKeyLength))
	}
	return nil
}

// ValidateStatus validates a user status
func ValidateStatus(status string) error {
	if status != "" && status != "on" && status != "off" {
		return svcerrors.NewValidationError("Status", "status must be 'on' or 'off'")
	}
	return nil
}
