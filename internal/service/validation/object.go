package validation

import (
	"fmt"
	"strings"
	"unicode/utf8"

	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
)

const (
	MaxObjectKeyLength = 1024 // bytes
)

// ValidateObjectKey validates an S3 object key according to AWS S3 naming rules
// See: https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-keys.html
func ValidateObjectKey(key string) error {
	if key == "" {
		return svcerrors.NewValidationError("ObjectKey", "object key cannot be empty")
	}

	// Check UTF-8 encoding
	if !utf8.ValidString(key) {
		return svcerrors.NewValidationError("ObjectKey", "object key must be valid UTF-8")
	}

	// Check byte length (UTF-8 encoded)
	if len(key) > MaxObjectKeyLength {
		return svcerrors.NewValidationError("ObjectKey",
			fmt.Sprintf("object key must not exceed %d bytes", MaxObjectKeyLength))
	}

	// Check for leading slash (not valid in S3)
	if strings.HasPrefix(key, "/") {
		return svcerrors.NewValidationError("ObjectKey", "object key must not start with '/'")
	}

	// Check for control characters (ASCII 0-31 except tab, newline, carriage return)
	for _, r := range key {
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return svcerrors.NewValidationError("ObjectKey", "object key contains invalid control characters")
		}
	}

	return nil
}
