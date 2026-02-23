package s3types

import (
	"encoding/xml"
	"errors"
	"net/http"
)

// ErrorResponse represents an S3 error
type ErrorResponse struct {
	XMLName          xml.Name `xml:"Error" json:"-"`
	Code             string   `xml:"Code"`
	Message          string   `xml:"Message"`
	Key              string   `xml:"Key,omitempty" json:"Key,omitempty"`
	BucketName       string   `xml:"BucketName,omitempty" json:"BucketName,omitempty"`
	Resource         string
	Region           string `xml:"Region,omitempty" json:"Region,omitempty"`
	RequestID        string `xml:"RequestId" json:"RequestId"`
	HostID           string `xml:"HostId" json:"HostId"`
	ActualObjectSize string `xml:"ActualObjectSize,omitempty" json:"ActualObjectSize,omitempty"`
	RangeRequested   string `xml:"RangeRequested,omitempty" json:"RangeRequested,omitempty"`
}

// ErrorCode represents S3 error codes
type ErrorCode int

const (
	ErrCodeNone ErrorCode = iota
	ErrCodeInternalError
	ErrCodeNoSuchBucket
	ErrCodeNoSuchKey
	ErrCodeBucketAlreadyExists
	ErrCodeBucketNotEmpty
	ErrCodeMethodNotAllowed
	ErrCodeInvalidAccessKeyID
	ErrCodeSignatureDoesNotMatch
	ErrCodeInvalidBucketName
	ErrCodeInvalidObjectKey
	ErrCodeAccessDenied
	ErrCodeNoSuchBucketPolicy
	ErrCodeMalformedPolicy
	ErrCodeInvalidRequest
	ErrCodeMalformedXML
	ErrCodeNoSuchUpload
	ErrCodeInvalidPart
	ErrCodeNotImplemented
)

// String returns the string representation of error code
func (e ErrorCode) String() string {
	switch e {
	case ErrCodeNone:
		return ""
	case ErrCodeInternalError:
		return "InternalError"
	case ErrCodeNoSuchBucket:
		return "NoSuchBucket"
	case ErrCodeNoSuchKey:
		return "NoSuchKey"
	case ErrCodeBucketAlreadyExists:
		return "BucketAlreadyExists"
	case ErrCodeBucketNotEmpty:
		return "BucketNotEmpty"
	case ErrCodeMethodNotAllowed:
		return "MethodNotAllowed"
	case ErrCodeInvalidAccessKeyID:
		return "InvalidAccessKeyId"
	case ErrCodeSignatureDoesNotMatch:
		return "SignatureDoesNotMatch"
	case ErrCodeInvalidBucketName:
		return "InvalidBucketName"
	case ErrCodeInvalidObjectKey:
		return "KeyTooLongError"
	case ErrCodeAccessDenied:
		return "AccessDenied"
	case ErrCodeNoSuchBucketPolicy:
		return "NoSuchBucketPolicy"
	case ErrCodeMalformedPolicy:
		return "MalformedPolicy"
	case ErrCodeInvalidRequest:
		return "InvalidRequest"
	case ErrCodeMalformedXML:
		return "MalformedXML"
	case ErrCodeNoSuchUpload:
		return "NoSuchUpload"
	case ErrCodeInvalidPart:
		return "InvalidPart"
	case ErrCodeNotImplemented:
		return "NotImplemented"
	default:
		return "InternalError"
	}
}

// Description returns the error description
func (e ErrorCode) Description() string {
	switch e {
	case ErrCodeInternalError:
		return "We encountered an internal error. Please try again."
	case ErrCodeNoSuchBucket:
		return "The specified bucket does not exist."
	case ErrCodeNoSuchKey:
		return "The specified key does not exist."
	case ErrCodeBucketAlreadyExists:
		return "The requested bucket name is not available."
	case ErrCodeBucketNotEmpty:
		return "The bucket you tried to delete is not empty."
	case ErrCodeMethodNotAllowed:
		return "The specified method is not allowed against this resource."
	case ErrCodeInvalidAccessKeyID:
		return "The AWS access key ID you provided does not exist in our records."
	case ErrCodeSignatureDoesNotMatch:
		return "The request signature we calculated does not match the signature you provided."
	case ErrCodeInvalidBucketName:
		return "The specified bucket is not valid."
	case ErrCodeInvalidObjectKey:
		return "Your key is too long or contains invalid characters."
	case ErrCodeAccessDenied:
		return "Access Denied"
	case ErrCodeNoSuchBucketPolicy:
		return "The bucket policy does not exist"
	case ErrCodeMalformedPolicy:
		return "Policy has invalid resource"
	case ErrCodeInvalidRequest:
		return "Invalid request"
	case ErrCodeMalformedXML:
		return "The XML provided was not well-formed or did not validate against our published schema"
	case ErrCodeNoSuchUpload:
		return "The specified multipart upload does not exist."
	case ErrCodeInvalidPart:
		return "One or more of the specified parts could not be found."
	case ErrCodeNotImplemented:
		return "A header you provided implies functionality that is not implemented."
	default:
		return "Internal error"
	}
}

// HTTPStatus returns the HTTP status code for the error
func (e ErrorCode) HTTPStatus() int {
	switch e {
	case ErrCodeNoSuchBucket, ErrCodeNoSuchKey:
		return http.StatusNotFound
	case ErrCodeBucketAlreadyExists:
		return http.StatusConflict
	case ErrCodeBucketNotEmpty:
		return http.StatusConflict
	case ErrCodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case ErrCodeInvalidAccessKeyID, ErrCodeSignatureDoesNotMatch, ErrCodeAccessDenied:
		return http.StatusForbidden
	case ErrCodeInvalidBucketName, ErrCodeInvalidObjectKey, ErrCodeMalformedPolicy, ErrCodeInvalidRequest, ErrCodeMalformedXML:
		return http.StatusBadRequest
	case ErrCodeNoSuchBucketPolicy, ErrCodeNoSuchUpload:
		return http.StatusNotFound
	case ErrCodeInvalidPart:
		return http.StatusBadRequest
	case ErrCodeNotImplemented:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

// ============================================================================
// Internal S3 Errors (shared across storage, service, and API layers)
// ============================================================================

var (
	// Bucket errors
	ErrBucketNotFound      = errors.New("bucket not found")
	ErrBucketAlreadyExists = errors.New("bucket already exists")
	ErrBucketNotEmpty      = errors.New("bucket not empty")
	ErrInvalidBucketName   = errors.New("invalid bucket name")
	ErrNoSuchBucketPolicy  = errors.New("bucket policy does not exist")

	// Object errors
	ErrObjectNotFound   = errors.New("object not found")
	ErrInvalidObjectKey = errors.New("invalid object key")
)
