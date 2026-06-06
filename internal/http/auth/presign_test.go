package auth

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPresignAccessKey = "TESTAKEYID"
	testPresignSecretKey = "testSecretKey0123456789012345678901234"
	testPresignRegion    = "us-east-1"
	testPresignBaseURL   = "http://localhost:9000"
)

func TestGeneratePresignedGetURL_HasRequiredParams(t *testing.T) {
	rawURL, err := GeneratePresignedGetURL(
		testPresignAccessKey, testPresignSecretKey, testPresignRegion,
		"my-bucket", "path/to/object.txt",
		testPresignBaseURL, 5*time.Minute,
	)
	require.NoError(t, err)

	u, err := url.Parse(rawURL)
	require.NoError(t, err)

	q := u.Query()
	assert.Equal(t, "AWS4-HMAC-SHA256", q.Get("X-Amz-Algorithm"))
	assert.Contains(t, q.Get("X-Amz-Credential"), testPresignAccessKey)
	assert.NotEmpty(t, q.Get("X-Amz-Date"))
	assert.Equal(t, "300", q.Get("X-Amz-Expires"))
	assert.Equal(t, "host", q.Get("X-Amz-SignedHeaders"))
	assert.NotEmpty(t, q.Get("X-Amz-Signature"))
	assert.Equal(t, "/my-bucket/path/to/object.txt", u.Path)
}

func TestGeneratePresignedGetURL_SignatureVerifies(t *testing.T) {
	expiry := 5 * time.Minute
	rawURL, err := GeneratePresignedGetURL(
		testPresignAccessKey, testPresignSecretKey, testPresignRegion,
		"my-bucket", "object.txt",
		testPresignBaseURL, expiry,
	)
	require.NoError(t, err)

	// Build a synthetic request that mirrors what the server would receive.
	u, err := url.Parse(rawURL)
	require.NoError(t, err)

	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
		Host:   u.Host,
		Header: http.Header{},
	}

	_, err = VerifyPresignedSignature(req, testPresignSecretKey)
	assert.NoError(t, err, "generated presigned URL should pass signature verification")
}

func TestGeneratePresignedGetURL_WrongSecretFails(t *testing.T) {
	rawURL, err := GeneratePresignedGetURL(
		testPresignAccessKey, testPresignSecretKey, testPresignRegion,
		"my-bucket", "object.txt",
		testPresignBaseURL, 5*time.Minute,
	)
	require.NoError(t, err)

	u, err := url.Parse(rawURL)
	require.NoError(t, err)

	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
		Host:   u.Host,
		Header: http.Header{},
	}

	_, err = VerifyPresignedSignature(req, "wrongsecret")
	assert.ErrorIs(t, err, ErrSignatureMismatch)
}

func TestGeneratePresignedGetURL_ExpiredURL(t *testing.T) {
	// Use injectable time to generate a URL that has already expired.
	pastTime := time.Now().UTC().Add(-10 * time.Minute)
	rawURL, err := generatePresignedGetURL(
		testPresignAccessKey, testPresignSecretKey, testPresignRegion,
		"my-bucket", "object.txt",
		testPresignBaseURL, 5*time.Minute, pastTime,
	)
	require.NoError(t, err)

	u, err := url.Parse(rawURL)
	require.NoError(t, err)

	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
		Host:   u.Host,
		Header: http.Header{},
	}

	_, err = VerifyPresignedSignature(req, testPresignSecretKey)
	assert.ErrorIs(t, err, ErrPresignedURLExpired)
}

func TestGeneratePresignedGetURL_InvalidExpiry(t *testing.T) {
	_, err := GeneratePresignedGetURL(
		testPresignAccessKey, testPresignSecretKey, testPresignRegion,
		"my-bucket", "object.txt",
		testPresignBaseURL, 0,
	)
	require.ErrorIs(t, err, ErrInvalidExpiresValue)

	_, err = GeneratePresignedGetURL(
		testPresignAccessKey, testPresignSecretKey, testPresignRegion,
		"my-bucket", "object.txt",
		testPresignBaseURL, 8*24*time.Hour, // > 7 days
	)
	require.ErrorIs(t, err, ErrInvalidExpiresValue)
}
