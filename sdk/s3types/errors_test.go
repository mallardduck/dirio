package s3types

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorCode_String(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want string
	}{
		{ErrCodeNone, ""},
		{ErrCodeInternalError, "InternalError"},
		{ErrCodeNoSuchBucket, "NoSuchBucket"},
		{ErrCodeNoSuchKey, "NoSuchKey"},
		{ErrCodeBucketAlreadyExists, "BucketAlreadyExists"},
		{ErrCodeBucketNotEmpty, "BucketNotEmpty"},
		{ErrCodeMethodNotAllowed, "MethodNotAllowed"},
		{ErrCodeInvalidAccessKeyID, "InvalidAccessKeyId"},
		{ErrCodeSignatureDoesNotMatch, "SignatureDoesNotMatch"},
		{ErrCodeInvalidBucketName, "InvalidBucketName"},
		{ErrCodeInvalidObjectKey, "KeyTooLongError"},
		{ErrCodeAccessDenied, "AccessDenied"},
		{ErrCodeNoSuchBucketPolicy, "NoSuchBucketPolicy"},
		{ErrCodeMalformedPolicy, "MalformedPolicy"},
		{ErrCodeInvalidRequest, "InvalidRequest"},
		{ErrCodeMalformedXML, "MalformedXML"},
		{ErrCodeNoSuchUpload, "NoSuchUpload"},
		{ErrCodeInvalidPart, "InvalidPart"},
		{ErrCodeNotImplemented, "NotImplemented"},
		// unknown code falls back to InternalError
		{ErrorCode(9999), "InternalError"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.code.String(), "ErrorCode(%d).String()", tc.code)
	}
}

func TestErrorCode_Description(t *testing.T) {
	// Spot-check a few known descriptions.
	assert.NotEmpty(t, ErrCodeNoSuchBucket.Description())
	assert.NotEmpty(t, ErrCodeAccessDenied.Description())
	assert.NotEmpty(t, ErrCodeInternalError.Description())

	// All defined codes must have a non-empty description.
	definedCodes := []ErrorCode{
		ErrCodeInternalError, ErrCodeNoSuchBucket, ErrCodeNoSuchKey,
		ErrCodeBucketAlreadyExists, ErrCodeBucketNotEmpty, ErrCodeMethodNotAllowed,
		ErrCodeInvalidAccessKeyID, ErrCodeSignatureDoesNotMatch, ErrCodeInvalidBucketName,
		ErrCodeInvalidObjectKey, ErrCodeAccessDenied, ErrCodeNoSuchBucketPolicy,
		ErrCodeMalformedPolicy, ErrCodeInvalidRequest, ErrCodeMalformedXML,
		ErrCodeNoSuchUpload, ErrCodeInvalidPart, ErrCodeNotImplemented,
	}
	for _, c := range definedCodes {
		assert.NotEmpty(t, c.Description(), "ErrorCode(%d).Description() should not be empty", c)
	}

	// Unknown code returns fallback.
	assert.Equal(t, "Internal error", ErrorCode(9999).Description())
}

func TestErrorCode_HTTPStatus(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want int
	}{
		{ErrCodeNoSuchBucket, http.StatusNotFound},
		{ErrCodeNoSuchKey, http.StatusNotFound},
		{ErrCodeBucketAlreadyExists, http.StatusConflict},
		{ErrCodeBucketNotEmpty, http.StatusConflict},
		{ErrCodeMethodNotAllowed, http.StatusMethodNotAllowed},
		{ErrCodeInvalidAccessKeyID, http.StatusForbidden},
		{ErrCodeSignatureDoesNotMatch, http.StatusForbidden},
		{ErrCodeAccessDenied, http.StatusForbidden},
		{ErrCodeInvalidBucketName, http.StatusBadRequest},
		{ErrCodeInvalidObjectKey, http.StatusBadRequest},
		{ErrCodeMalformedPolicy, http.StatusBadRequest},
		{ErrCodeInvalidRequest, http.StatusBadRequest},
		{ErrCodeMalformedXML, http.StatusBadRequest},
		{ErrCodeNoSuchBucketPolicy, http.StatusNotFound},
		{ErrCodeNoSuchUpload, http.StatusNotFound},
		{ErrCodeInvalidPart, http.StatusBadRequest},
		{ErrCodeNotImplemented, http.StatusNotImplemented},
		{ErrCodeInternalError, http.StatusInternalServerError},
		// ErrCodeNone and unknown both fall back to 500
		{ErrCodeNone, http.StatusInternalServerError},
		{ErrorCode(9999), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.code.HTTPStatus(), "ErrorCode(%d).HTTPStatus()", tc.code)
	}
}
