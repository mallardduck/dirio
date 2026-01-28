package s3

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	// S3 bucket name must be 3-63 characters
	bucketNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
)

// ValidateS3BucketName validates a bucket name according to S3 naming rules.
//
// S3 bucket naming rules:
// - Between 3 and 63 characters long
// - Consist only of lowercase letters, numbers, dots (.), and hyphens (-)
// - Begin and end with a letter or number
// - Must not contain two adjacent periods
// - Must not be formatted as an IP address (e.g., 192.168.5.4)
func ValidateS3BucketName(bucket string) error {
	// Check length
	if len(bucket) < 3 || len(bucket) > 63 {
		return fmt.Errorf("bucket name must be between 3 and 63 characters long")
	}

	// Check format with regex
	if !bucketNameRegex.MatchString(bucket) {
		return fmt.Errorf("bucket name must start and end with a lowercase letter or number, and contain only lowercase letters, numbers, dots, and hyphens")
	}

	// Check for consecutive dots
	if strings.Contains(bucket, "..") {
		return fmt.Errorf("bucket name must not contain consecutive dots")
	}

	// Check if it's formatted like an IP address
	if net.ParseIP(bucket) != nil {
		return fmt.Errorf("bucket name must not be formatted as an IP address")
	}

	return nil
}

// ValidateS3Key validates an object key according to S3 naming rules.
//
// S3 key naming rules:
// - Can be up to 1024 bytes long (UTF-8 encoded)
// - Can contain any UTF-8 character
// - Should avoid certain characters that require special handling
// - Must not start with '/' (not a valid S3 key)
func ValidateS3Key(key string) error {
	// Check if empty
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	// Check UTF-8 encoding
	if !utf8.ValidString(key) {
		return fmt.Errorf("key must be valid UTF-8")
	}

	// Check byte length (UTF-8 encoded)
	if len(key) > 1024 {
		return fmt.Errorf("key must not exceed 1024 bytes")
	}

	// Check for leading slash (not valid in S3)
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("key must not start with '/'")
	}

	// Check for control characters (ASCII 0-31 except tab, newline, carriage return)
	for _, r := range key {
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return fmt.Errorf("key contains invalid control characters")
		}
	}

	return nil
}
