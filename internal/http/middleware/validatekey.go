package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/mallardduck/dirio/pkg/s3types"
)

// ValidateS3BucketNameMiddleware returns a middleware that validates S3 bucket names.
// It should be applied to routes that have a {bucket} parameter.
// If validation fails, it writes an error response and stops the request chain.
//
// The getBucket function extracts the bucket name from the request (typically from URL params).
// The writeError function handles writing the S3-formatted error response.
func ValidateS3BucketNameMiddleware(
	getBucket func(*http.Request) string,
	writeError func(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode, err error) error,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bucket := getBucket(r)

			if err := ValidateS3BucketName(bucket); err != nil {
				requestID := GetRequestID(r.Context())
				if writeErr := writeError(w, requestID, s3types.ErrCodeInvalidBucketName, err); writeErr != nil {
					// Error writing error response - headers likely already sent
					return
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

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

	// Check format
	if !isValidBucketName(bucket) {
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

// isValidBucketName checks if a bucket name matches S3 naming rules.
func isValidBucketName(bucket string) bool {
	if len(bucket) < 3 || len(bucket) > 63 {
		return false
	}

	// Check first and last characters
	first := bucket[0]
	last := bucket[len(bucket)-1]
	if !isLowerAlphaNum(first) || !isLowerAlphaNum(last) {
		return false
	}

	// Check all characters
	for i := 0; i < len(bucket); i++ {
		c := bucket[i]
		if !isLowerAlphaNum(c) && c != '.' && c != '-' {
			return false
		}
	}

	return true
}

// isLowerAlphaNum checks if a byte is a lowercase letter or digit.
func isLowerAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

// ValidateS3KeyMiddleware returns a middleware that validates S3 object keys.
// It should be applied to routes that have a {key} parameter.
// If validation fails, it writes an error response and stops the request chain.
//
// The getKey function extracts the key from the request (typically from URL params).
// The writeError function handles writing the S3-formatted error response.
func ValidateS3KeyMiddleware(
	getKey func(*http.Request) string,
	writeError func(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode, err error) error,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := getKey(r)

			if err := ValidateS3Key(key); err != nil {
				requestID := GetRequestID(r.Context())
				if writeErr := writeError(w, requestID, s3types.ErrCodeInvalidObjectKey, err); writeErr != nil {
					// Error writing error response - headers likely already sent
					return
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
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
