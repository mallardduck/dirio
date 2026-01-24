package sigv4

import (
	"fmt"
	"net/http"
	"time"
)

// DebugVerifySignature is like VerifySignature but prints debug information
func DebugVerifySignature(r *http.Request, secretKey string) error {
	// Parse Authorization header
	authHeader := r.Header.Get(authorizationHeader)
	fmt.Printf("DEBUG: Authorization header: %s\n", authHeader)

	creds, err := ParseAuthorizationHeader(authHeader)
	if err != nil {
		fmt.Printf("DEBUG: Failed to parse auth header: %v\n", err)
		return err
	}

	fmt.Printf("DEBUG: Parsed credentials - AccessKey: %s, Region: %s, SignedHeaders: %v\n",
		creds.AccessKey, creds.Region, creds.SignedHeaders)
	fmt.Printf("DEBUG: Client signature: %s\n", creds.Signature)

	// Get timestamp from X-Amz-Date header
	dateStr := r.Header.Get(dateHeader)
	if dateStr == "" {
		fmt.Printf("DEBUG: Missing X-Amz-Date header\n")
		return ErrMissingDateHeader
	}
	fmt.Printf("DEBUG: X-Amz-Date: %s\n", dateStr)

	timestamp, err := time.Parse(iso8601TimeFormat, dateStr)
	if err != nil {
		fmt.Printf("DEBUG: Failed to parse timestamp: %v\n", err)
		return fmt.Errorf("%w: %v", ErrInvalidDateFormat, err)
	}

	// Get payload hash from header
	payloadHash := r.Header.Get(contentSHA256Header)
	if payloadHash == "" {
		payloadHash = "UNSIGNED-PAYLOAD"
	}
	fmt.Printf("DEBUG: Payload hash: %s\n", payloadHash)

	// Build canonical request
	canonicalRequest := BuildCanonicalRequest(r, creds.SignedHeaders, payloadHash)
	fmt.Printf("DEBUG: Canonical request:\n%s\n", canonicalRequest)

	// Build string to sign
	stringToSign := BuildStringToSign(timestamp, creds.Region, canonicalRequest)
	fmt.Printf("DEBUG: String to sign:\n%s\n", stringToSign)

	// Compute expected signature
	expectedSignature := ComputeSignature(secretKey, timestamp, creds.Region, stringToSign)
	fmt.Printf("DEBUG: Expected signature: %s\n", expectedSignature)

	// Compare signatures
	if expectedSignature != creds.Signature {
		fmt.Printf("DEBUG: Signature mismatch!\n")
		fmt.Printf("DEBUG:   Expected: %s\n", expectedSignature)
		fmt.Printf("DEBUG:   Got:      %s\n", creds.Signature)
		return ErrSignatureMismatch
	}

	fmt.Printf("DEBUG: Signature verified successfully!\n")
	return nil
}