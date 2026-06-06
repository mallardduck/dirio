package auth

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mallardduck/dirio/internal/consts"
)

// GeneratePresignedGetURL builds a pre-signed S3 GET URL for the given object using
// AWS Signature Version 4 query-string authentication.
//
// expiry must be between 1 second and 7 days (604800 seconds); region should match
// the server's configured region (defaults to "us-east-1").
func GeneratePresignedGetURL(accessKey, secretKey, region, bucket, key, baseURL string, expiry time.Duration) (string, error) {
	return generatePresignedGetURL(accessKey, secretKey, region, bucket, key, baseURL, expiry, time.Now().UTC())
}

// generatePresignedGetURL is the inner implementation with an injectable timestamp for testing.
func generatePresignedGetURL(accessKey, secretKey, region, bucket, key, baseURL string, expiry time.Duration, now time.Time) (string, error) {
	expirySeconds := int(expiry.Seconds())
	if expirySeconds < 1 || expirySeconds > 604800 {
		return "", ErrInvalidExpiresValue
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	dateStr := now.Format(iso8601TimeFormat)
	shortDate := now.Format(shortDateFormat)
	credentialScope := fmt.Sprintf("%s/%s/%s/%s", shortDate, region, serviceName, requestType)
	credential := accessKey + "/" + credentialScope

	authParams := url.Values{}
	authParams.Set("X-Amz-Algorithm", algorithmID)
	authParams.Set("X-Amz-Credential", credential)
	authParams.Set("X-Amz-Date", dateStr)
	authParams.Set("X-Amz-Expires", strconv.Itoa(expirySeconds))
	authParams.Set("X-Amz-SignedHeaders", "host")

	objectPath := "/" + bucket + "/" + key

	// Build a synthetic request so we can reuse the canonical-request builder.
	// Host is set explicitly because Go's net/http does not put it in Header.
	syntheticReq := &http.Request{
		Method: http.MethodGet,
		URL: &url.URL{
			Scheme:   u.Scheme,
			Host:     u.Host,
			Path:     objectPath,
			RawQuery: authParams.Encode(),
		},
		Host:   u.Host,
		Header: http.Header{},
	}

	canonicalRequest := BuildCanonicalRequest(syntheticReq, []string{"host"}, consts.ContentSHA256Unsigned)
	stringToSign := BuildStringToSign(now, region, canonicalRequest)
	signature := ComputeSignature(secretKey, now, region, stringToSign)

	authParams.Set("X-Amz-Signature", signature)
	return fmt.Sprintf("%s%s?%s", baseURL, objectPath, authParams.Encode()), nil
}
