// AWS Signature Version 4 implementation for HTTP request authentication.
//
// This implementation is inspired by:
// - MinIO (https://github.com/minio/minio) - Apache 2.0 License
// - LabStore (https://github.com/IllumiKnowLabs/labstore) - Apache 2.0 License
//
// AWS Signature Version 4 specification:
// https://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-authenticating-requests.html
//
// Pre-signed URL Support Implementation (Phase 3.2 #3)
//
// ✅ IMPLEMENTED: Core pre-signed URL authentication (Feb 16, 2026)
//
// Implemented features:
// 1. ✅ Query string authentication parsing (X-Amz-Algorithm, X-Amz-Credential, X-Amz-Date, X-Amz-Expires, X-Amz-SignedHeaders, X-Amz-Signature)
// 2. ✅ Signature verification for pre-signed requests (VerifyPresignedSignature)
// 3. ✅ Expiration validation (X-Amz-Expires, max 604800 seconds / 7 days)
// 4. ✅ Context tracking (IsPreSignedRequest, GetPreSignedExpiresAt)
// 5. ✅ Middleware integration (AuthMiddleware handles both header and query auth)
//
// Testing commands (to verify implementation):
// - aws s3 presign s3://bucket/object.txt --expires-in 300
// - boto3: generate_presigned_url()
// - mc share download alias/bucket/object.txt
//
// TODO (Future enhancements):
// - [ ] X-Amz-Security-Token support for temporary credentials (STS)
// - [ ] URL generation helper for creating pre-signed URLs (for future CLI/SDK)
//
// References:
// - https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html
// - MinIO: cmd/signature-v4.go (doesPresignedSignatureMatch)
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/mallardduck/go-http-helpers/pkg/headers"

	"github.com/mallardduck/dirio/internal/consts"
)

const (
	authorizationHeader = headers.Authorization
	dateHeader          = consts.HeaderDate
	algorithmID         = "AWS4-HMAC-SHA256"
	iso8601TimeFormat   = "20060102T150405Z"
	shortDateFormat     = "20060102"
	serviceName         = "s3"
	requestType         = "aws4_request"
)

var (
	// ErrMissingAuthHeader is returned when the Authorization header is missing
	ErrMissingAuthHeader = errors.New("missing Authorization header")
	// ErrInvalidAuthFormat is returned when the Authorization header format is invalid
	ErrInvalidAuthFormat = errors.New("invalid Authorization header format")
	// ErrMissingDateHeader is returned when the X-Amz-Date header is missing
	ErrMissingDateHeader = errors.New("missing X-Amz-Date header")
	// ErrInvalidDateFormat is returned when the date format is invalid
	ErrInvalidDateFormat = errors.New("invalid date format")
	// ErrSignatureMismatch is returned when the signature doesn't match
	ErrSignatureMismatch = errors.New("signature mismatch")
	// ErrMissingCredential is returned when credential is missing from auth header
	ErrMissingCredential = errors.New("missing credential in Authorization header")
	// ErrMissingSignature is returned when signature is missing from auth header
	ErrMissingSignature = errors.New("missing signature in Authorization header")
	// ErrMissingSignedHeaders is returned when SignedHeaders is missing from auth header
	ErrMissingSignedHeaders = errors.New("missing SignedHeaders in Authorization header")
	// ErrPresignedURLExpired is returned when a pre-signed URL has expired
	ErrPresignedURLExpired = errors.New("pre-signed URL has expired")
	// ErrInvalidExpiresValue is returned when X-Amz-Expires is invalid
	ErrInvalidExpiresValue = errors.New("invalid X-Amz-Expires value")
	// ErrMissingPresignedParams is returned when required pre-signed URL params are missing
	ErrMissingPresignedParams = errors.New("missing required pre-signed URL parameters")
)

// Credentials represents parsed AWS credentials from the Authorization header
type Credentials struct {
	AccessKey     string
	Date          string
	Region        string
	Service       string
	SignedHeaders []string
	Signature     string
}

// ParseAuthorizationHeader parses the AWS Signature V4 Authorization header
// Format: AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-date, Signature=fe5f80f77d5fa3beca038a248ff027d0445342fe2855ddc963176630326f1024
func ParseAuthorizationHeader(authHeader string) (*Credentials, error) {
	if authHeader == "" {
		return nil, ErrMissingAuthHeader
	}

	// Check algorithm prefix
	if !strings.HasPrefix(authHeader, algorithmID+" ") {
		return nil, ErrInvalidAuthFormat
	}

	// Remove algorithm prefix
	authHeader = strings.TrimPrefix(authHeader, algorithmID+" ")

	creds := &Credentials{}

	// Parse comma-separated parts
	parts := strings.Split(authHeader, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "Credential":
			// Parse credential: AccessKey/Date/Region/Service/RequestType
			credParts := strings.Split(value, "/")
			if len(credParts) != 5 {
				return nil, ErrInvalidAuthFormat
			}
			creds.AccessKey = credParts[0]
			creds.Date = credParts[1]
			creds.Region = credParts[2]
			creds.Service = credParts[3]

		case "SignedHeaders":
			// Parse signed headers (semicolon-separated)
			creds.SignedHeaders = strings.Split(value, ";")

		case "Signature":
			creds.Signature = value
		}
	}

	// Validate required fields
	if creds.AccessKey == "" {
		return nil, ErrMissingCredential
	}
	if creds.Signature == "" {
		return nil, ErrMissingSignature
	}
	if len(creds.SignedHeaders) == 0 {
		return nil, ErrMissingSignedHeaders
	}

	return creds, nil
}

// ParsePresignedURLAuth parses AWS Signature V4 authentication from query parameters
// Used for pre-signed URLs with format:
// ?X-Amz-Algorithm=AWS4-HMAC-SHA256
// &X-Amz-Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request
// &X-Amz-Date=20130524T000000Z
// &X-Amz-Expires=86400
// &X-Amz-SignedHeaders=host
// &X-Amz-Signature=...
func ParsePresignedURLAuth(r *http.Request) (*Credentials, time.Time, error) {
	query := r.URL.Query()

	// Check for required algorithm parameter
	algorithm := query.Get("X-Amz-Algorithm")
	if algorithm != algorithmID {
		return nil, time.Time{}, ErrMissingPresignedParams
	}

	// Parse credential
	credentialStr := query.Get("X-Amz-Credential")
	if credentialStr == "" {
		return nil, time.Time{}, ErrMissingCredential
	}

	credParts := strings.Split(credentialStr, "/")
	if len(credParts) != 5 {
		return nil, time.Time{}, ErrInvalidAuthFormat
	}

	creds := &Credentials{
		AccessKey: credParts[0],
		Date:      credParts[1],
		Region:    credParts[2],
		Service:   credParts[3],
	}

	// Parse signed headers
	signedHeadersStr := query.Get("X-Amz-SignedHeaders")
	if signedHeadersStr == "" {
		return nil, time.Time{}, ErrMissingSignedHeaders
	}
	creds.SignedHeaders = strings.Split(signedHeadersStr, ";")

	// Parse signature
	creds.Signature = query.Get("X-Amz-Signature")
	if creds.Signature == "" {
		return nil, time.Time{}, ErrMissingSignature
	}

	// Parse date
	dateStr := query.Get("X-Amz-Date")
	if dateStr == "" {
		return nil, time.Time{}, ErrMissingDateHeader
	}

	timestamp, err := time.Parse(iso8601TimeFormat, dateStr)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("%w: %v", ErrInvalidDateFormat, err)
	}

	// Parse expiration (in seconds)
	expiresStr := query.Get("X-Amz-Expires")
	if expiresStr == "" {
		return nil, time.Time{}, ErrInvalidExpiresValue
	}

	var expiresSeconds int
	if _, err := fmt.Sscanf(expiresStr, "%d", &expiresSeconds); err != nil {
		return nil, time.Time{}, ErrInvalidExpiresValue
	}

	// AWS allows max 7 days (604800 seconds)
	if expiresSeconds < 1 || expiresSeconds > 604800 {
		return nil, time.Time{}, ErrInvalidExpiresValue
	}

	expiresAt := timestamp.Add(time.Duration(expiresSeconds) * time.Second)

	return creds, expiresAt, nil
}

// BuildCanonicalRequest constructs the canonical request string
// Format:
// HTTPMethod + "\n" +
// CanonicalURI + "\n" +
// CanonicalQueryString + "\n" +
// CanonicalHeaders + "\n" +
// SignedHeaders + "\n" +
// HashedPayload
func BuildCanonicalRequest(r *http.Request, signedHeaders []string, payloadHash string) string {
	// HTTP Method (uppercase)
	method := r.Method

	// Canonical URI (URL-encoded path, or "/" if empty)
	uri := r.URL.EscapedPath()
	if uri == "" {
		uri = "/"
	}

	// Canonical Query String (sorted, URL-encoded)
	queryString := buildCanonicalQueryString(r.URL.Query())

	// Canonical Headers (lowercase, sorted, trimmed)
	// Pass r.Host separately since it's not in r.Header
	canonicalHeaders := buildCanonicalHeaders(r.Header, signedHeaders, r.Host)

	// Signed Headers (lowercase, sorted, semicolon-separated)
	// Make a copy and sort to ensure consistent ordering
	sortedHeaders := make([]string, len(signedHeaders))
	copy(sortedHeaders, signedHeaders)
	sort.Strings(sortedHeaders)
	signedHeadersList := strings.Join(sortedHeaders, ";")

	// Combine all parts
	return strings.Join([]string{
		method,
		uri,
		queryString,
		canonicalHeaders,
		signedHeadersList,
		payloadHash,
	}, "\n")
}

// buildCanonicalQueryString builds the canonical query string
func buildCanonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}

	// Sort keys
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build encoded query string
	var parts []string
	for _, k := range keys {
		// Get values for this key and sort them
		vals := values[k]
		sort.Strings(vals)

		encodedKey := url.QueryEscape(k)
		for _, v := range vals {
			encodedValue := url.QueryEscape(v)
			parts = append(parts, encodedKey+"="+encodedValue)
		}
	}

	return strings.Join(parts, "&")
}

// buildCanonicalHeaders builds the canonical headers string
// Note: The Host header must be passed separately via hostHeader parameter
// because in Go's net/http, r.Host is not in r.Header
func buildCanonicalHeaders(headers http.Header, signedHeaders []string, hostHeader string) string {
	// Create a map for quick lookup
	signedMap := make(map[string]bool)
	for _, h := range signedHeaders {
		signedMap[strings.ToLower(h)] = true
	}

	// Build header lines
	var lines []string
	for key, values := range headers {
		lowerKey := strings.ToLower(key)
		if signedMap[lowerKey] {
			// Trim and join multiple values with comma
			trimmedValues := make([]string, len(values))
			for i, v := range values {
				trimmedValues[i] = strings.TrimSpace(v)
			}
			headerValue := strings.Join(trimmedValues, ",")
			lines = append(lines, lowerKey+":"+headerValue)
		}
	}

	// Handle Host header specially - it's in r.Host not r.Header
	if signedMap["host"] && hostHeader != "" {
		// Check if host wasn't already added from headers
		hasHost := false
		for _, line := range lines {
			if strings.HasPrefix(line, "host:") {
				hasHost = true
				break
			}
		}
		if !hasHost {
			lines = append(lines, "host:"+hostHeader)
		}
	}

	// Sort header lines
	sort.Strings(lines)

	// Join with newline and add trailing newline (only if there are lines)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// BuildStringToSign constructs the string to sign
// Format:
// Algorithm + "\n" +
// RequestDateTime + "\n" +
// CredentialScope + "\n" +
// HashedCanonicalRequest
func BuildStringToSign(timestamp time.Time, region, canonicalRequest string) string {
	// Credential scope: Date/Region/Service/RequestType
	dateStamp := timestamp.Format(shortDateFormat)
	credentialScope := fmt.Sprintf("%s/%s/%s/%s", dateStamp, region, serviceName, requestType)

	// Hash the canonical request
	hashedCanonicalRequest := hashSHA256([]byte(canonicalRequest))

	return strings.Join([]string{
		algorithmID,
		timestamp.Format(iso8601TimeFormat),
		credentialScope,
		hashedCanonicalRequest,
	}, "\n")
}

// ComputeSignature computes the AWS Signature V4 signature
func ComputeSignature(secretKey string, timestamp time.Time, region, stringToSign string) string {
	dateStamp := timestamp.Format(shortDateFormat)

	// Derive signing key through a series of HMAC operations
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte(requestType))

	// Compute final signature
	signature := hmacSHA256(kSigning, []byte(stringToSign))
	return hex.EncodeToString(signature)
}

// VerifySignature verifies the AWS Signature V4 signature for the given request
func VerifySignature(r *http.Request, secretKey string) error {
	// Parse Authorization header
	authHeader := r.Header.Get(authorizationHeader)
	creds, err := ParseAuthorizationHeader(authHeader)
	if err != nil {
		return err
	}

	// Get timestamp from X-Amz-Date header
	dateStr := r.Header.Get(dateHeader)
	if dateStr == "" {
		return ErrMissingDateHeader
	}

	timestamp, err := time.Parse(iso8601TimeFormat, dateStr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidDateFormat, err)
	}

	// Get payload hash from header (or use "UNSIGNED-PAYLOAD" for unsigned)
	payloadHash := r.Header.Get(consts.HeaderContentSHA256)
	if payloadHash == "" {
		payloadHash = consts.ContentSHA256Unsigned
	}

	// Build canonical request
	canonicalRequest := BuildCanonicalRequest(r, creds.SignedHeaders, payloadHash)

	// Build string to sign
	stringToSign := BuildStringToSign(timestamp, creds.Region, canonicalRequest)

	// Compute expected signature
	expectedSignature := ComputeSignature(secretKey, timestamp, creds.Region, stringToSign)

	// Compare signatures using constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(expectedSignature), []byte(creds.Signature)) != 1 {
		return ErrSignatureMismatch
	}

	return nil
}

// VerifyPresignedSignature verifies the AWS Signature V4 signature for a pre-signed URL request
func VerifyPresignedSignature(r *http.Request, secretKey string) (time.Time, error) {
	// Parse pre-signed URL authentication from query parameters
	creds, expiresAt, err := ParsePresignedURLAuth(r)
	if err != nil {
		return time.Time{}, err
	}

	// Check if URL has expired
	if time.Now().After(expiresAt) {
		return time.Time{}, ErrPresignedURLExpired
	}

	// Get timestamp from query parameter
	dateStr := r.URL.Query().Get("X-Amz-Date")
	timestamp, err := time.Parse(iso8601TimeFormat, dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %v", ErrInvalidDateFormat, err)
	}

	// For pre-signed URLs, we need to build the canonical request without the signature
	// Clone the URL and remove the signature parameter
	urlCopy := *r.URL
	queryCopy := urlCopy.Query()
	queryCopy.Del("X-Amz-Signature")
	urlCopy.RawQuery = queryCopy.Encode()

	// Create a temporary request with the modified URL
	rCopy := *r
	rCopy.URL = &urlCopy

	// Get payload hash - for pre-signed URLs it's always UNSIGNED-PAYLOAD
	payloadHash := consts.ContentSHA256Unsigned

	// Build canonical request (using the URL without signature)
	canonicalRequest := BuildCanonicalRequest(&rCopy, creds.SignedHeaders, payloadHash)

	// Build string to sign
	stringToSign := BuildStringToSign(timestamp, creds.Region, canonicalRequest)

	// Compute expected signature
	expectedSignature := ComputeSignature(secretKey, timestamp, creds.Region, stringToSign)

	// Compare signatures using constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(expectedSignature), []byte(creds.Signature)) != 1 {
		return time.Time{}, ErrSignatureMismatch
	}

	return expiresAt, nil
}

// GetAccessKey extracts the access key from the Authorization header without verifying the signature
func GetAccessKey(r *http.Request) (string, error) {
	authHeader := r.Header.Get(authorizationHeader)
	creds, err := ParseAuthorizationHeader(authHeader)
	if err != nil {
		return "", err
	}
	return creds.AccessKey, nil
}

// GetAccessKeyFromPresignedURL extracts the access key from pre-signed URL query parameters
func GetAccessKeyFromPresignedURL(r *http.Request) (string, error) {
	credentialStr := r.URL.Query().Get("X-Amz-Credential")
	if credentialStr == "" {
		return "", ErrMissingCredential
	}

	credParts := strings.Split(credentialStr, "/")
	if len(credParts) != 5 {
		return "", ErrInvalidAuthFormat
	}

	return credParts[0], nil
}

// hmacSHA256 computes HMAC-SHA256
func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// hashSHA256 computes SHA256 hash and returns hex-encoded string
func hashSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
