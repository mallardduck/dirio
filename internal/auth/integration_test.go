//go:build integration
// +build integration

package auth

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mallardduck/dirio/internal/consts"
)

// TestAWSCLICompatibility tests that our implementation matches AWS CLI's signature
func TestAWSCLICompatibility(t *testing.T) {
	secretKey := "testsecret"
	accessKey := "testaccess"
	region := consts.DefaultBucketLocation

	// Create a test request similar to what AWS CLI would send
	req := httptest.NewRequest("GET", "http://localhost:9000/", nil)

	// AWS CLI sets these headers
	timestamp := time.Date(2026, 1, 23, 12, 0, 0, 0, time.UTC)
	req.Header.Set("Host", "localhost:9000")
	req.Header.Set("X-Amz-Date", timestamp.Format(iso8601TimeFormat))
	req.Header.Set(consts.HeaderContentSHA256, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855") // empty body

	// Build signature like AWS CLI does
	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	payloadHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	canonicalRequest := BuildCanonicalRequest(req, signedHeaders, payloadHash)
	t.Logf("Canonical Request:\n%s\n", canonicalRequest)

	stringToSign := BuildStringToSign(timestamp, region, canonicalRequest)
	t.Logf("String to Sign:\n%s\n", stringToSign)

	signature := ComputeSignature(secretKey, timestamp, region, stringToSign)
	t.Logf("Computed Signature: %s", signature)

	// Set Authorization header
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s/%s/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=%s",
		accessKey, timestamp.Format(shortDateFormat), region, signature)
	req.Header.Set("Authorization", authHeader)

	// Verify signature
	err := VerifySignature(req, secretKey)
	if err != nil {
		t.Fatalf("Signature verification failed: %v", err)
	}

	t.Log("Signature verified successfully!")
}
