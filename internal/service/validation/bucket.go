package validation

import (
	"fmt"
	"strings"
	"unicode"

	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
)

const (
	MinBucketNameLength = 3
	MaxBucketNameLength = 63
)

// ValidateBucketName validates an S3 bucket name according to AWS S3 naming rules
// See: https://docs.aws.amazon.com/AmazonS3/latest/userguide/bucketnamingrules.html
func ValidateBucketName(name string) error {
	if name == "" {
		return svcerrors.NewValidationError("BucketName", "bucket name is required")
	}

	// Check length
	if !InRange(name, MinBucketNameLength, MaxBucketNameLength) {
		return svcerrors.NewValidationError("BucketName",
			fmt.Sprintf("bucket name must be between %d and %d characters", MinBucketNameLength, MaxBucketNameLength))
	}

	// Must start with a lowercase letter or number
	firstChar := rune(name[0])
	if !unicode.IsLower(firstChar) && !unicode.IsDigit(firstChar) {
		return svcerrors.NewValidationError("BucketName", "bucket name must start with a lowercase letter or number")
	}

	// Must end with a lowercase letter or number
	lastChar := rune(name[len(name)-1])
	if !unicode.IsLower(lastChar) && !unicode.IsDigit(lastChar) {
		return svcerrors.NewValidationError("BucketName", "bucket name must end with a lowercase letter or number")
	}

	// Check for invalid characters and patterns
	for i, char := range name {
		if !unicode.IsLower(char) && !unicode.IsDigit(char) && char != '-' && char != '.' {
			return svcerrors.NewValidationError("BucketName", "bucket name can only contain lowercase letters, numbers, hyphens, and periods")
		}

		// Check for consecutive periods
		if char == '.' && i > 0 && rune(name[i-1]) == '.' {
			return svcerrors.NewValidationError("BucketName", "bucket name cannot contain consecutive periods")
		}

		// Check for period-hyphen or hyphen-period combinations
		if i > 0 {
			prev := rune(name[i-1])
			if (char == '.' && prev == '-') || (char == '-' && prev == '.') {
				return svcerrors.NewValidationError("BucketName", "bucket name cannot have periods adjacent to hyphens")
			}
		}
	}

	// Cannot be formatted as an IP address (e.g., 192.168.1.1)
	if isIPAddress(name) {
		return svcerrors.NewValidationError("BucketName", "bucket name cannot be formatted as an IP address")
	}

	// Cannot start with "xn--" (reserved for Punycode)
	if strings.HasPrefix(name, "xn--") {
		return svcerrors.NewValidationError("BucketName", "bucket name cannot start with 'xn--'")
	}

	// Cannot end with "-s3alias" (reserved for S3 access points)
	if strings.HasSuffix(name, "-s3alias") {
		return svcerrors.NewValidationError("BucketName", "bucket name cannot end with '-s3alias'")
	}

	return nil
}

// isIPAddress checks if a string looks like an IPv4 address
func isIPAddress(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		// Check if each part is numeric
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		for _, c := range part {
			if !unicode.IsDigit(c) {
				return false
			}
		}
	}

	return true
}
