package s3types

import (
	"encoding/xml"
	"net/http"
	"time"
)

// Bucket represents an S3 bucket
type Bucket struct {
	Name         string    `xml:"Name"`
	CreationDate time.Time `xml:"CreationDate"`
}

// Owner represents bucket/object owner
type Owner struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

// ListBucketsResponse is the response for ListBuckets operation
type ListBucketsResponse struct {
	XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListAllMyBucketsResult"`
	Owner   Owner    `xml:"Owner"`
	Buckets []Bucket `xml:"Buckets>Bucket"`
}

// Object represents an S3 object in listing
type Object struct {
	Key          string    `xml:"Key"`
	LastModified time.Time `xml:"LastModified"`
	ETag         string    `xml:"ETag"`
	Size         int64     `xml:"Size"`
	StorageClass string    `xml:"StorageClass"`
	Owner        *Owner    `xml:"Owner,omitempty"`
}

// CommonPrefix represents a common prefix in listing
type CommonPrefix struct {
	Prefix string `xml:"Prefix"`
}

// ListBucketResult is the response for ListObjects (V1)
type ListBucketResult struct {
	XMLName        xml.Name       `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListBucketResult"`
	Name           string         `xml:"Name"`
	Prefix         string         `xml:"Prefix"`
	Marker         string         `xml:"Marker"`
	NextMarker     string         `xml:"NextMarker,omitempty"`
	Delimiter      string         `xml:"Delimiter,omitempty"`
	MaxKeys        int            `xml:"MaxKeys"`
	IsTruncated    bool           `xml:"IsTruncated"`
	Contents       []Object       `xml:"Contents"`
	CommonPrefixes []CommonPrefix `xml:"CommonPrefixes,omitempty"`
}

// ListBucketV2Result is the response for ListObjectsV2
type ListBucketV2Result struct {
	XMLName               xml.Name       `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListBucketResult"`
	Name                  string         `xml:"Name"`
	Prefix                string         `xml:"Prefix"`
	Delimiter             string         `xml:"Delimiter,omitempty"`
	MaxKeys               int            `xml:"MaxKeys"`
	KeyCount              int            `xml:"KeyCount"`
	IsTruncated           bool           `xml:"IsTruncated"`
	ContinuationToken     string         `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string         `xml:"NextContinuationToken,omitempty"`
	StartAfter            string         `xml:"StartAfter,omitempty"`
	Contents              []Object       `xml:"Contents"`
	CommonPrefixes        []CommonPrefix `xml:"CommonPrefixes,omitempty"`
}

// LocationResponse is the response for GetBucketLocation
type LocationResponse struct {
	XMLName  xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ LocationConstraint"`
	Location string   `xml:",chardata"`
}

// ErrorResponse represents an S3 error
type ErrorResponse struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource,omitempty"`
	RequestID string   `xml:"RequestId,omitempty"`
}

// ErrorCode represents S3 error codes
type ErrorCode int

const (
	ErrNone ErrorCode = iota
	ErrInternalError
	ErrNoSuchBucket
	ErrNoSuchKey
	ErrBucketAlreadyExists
	ErrBucketNotEmpty
	ErrMethodNotAllowed
	ErrInvalidAccessKeyID
	ErrSignatureDoesNotMatch
	ErrInvalidBucketName
	ErrInvalidObjectKey
	ErrAccessDenied
)

// String returns the string representation of error code
func (e ErrorCode) String() string {
	switch e {
	case ErrNone:
		return ""
	case ErrInternalError:
		return "InternalError"
	case ErrNoSuchBucket:
		return "NoSuchBucket"
	case ErrNoSuchKey:
		return "NoSuchKey"
	case ErrBucketAlreadyExists:
		return "BucketAlreadyExists"
	case ErrBucketNotEmpty:
		return "BucketNotEmpty"
	case ErrMethodNotAllowed:
		return "MethodNotAllowed"
	case ErrInvalidAccessKeyID:
		return "InvalidAccessKeyId"
	case ErrSignatureDoesNotMatch:
		return "SignatureDoesNotMatch"
	case ErrInvalidBucketName:
		return "InvalidBucketName"
	case ErrInvalidObjectKey:
		return "KeyTooLongError"
	case ErrAccessDenied:
		return "AccessDenied"
	default:
		return "InternalError"
	}
}

// Description returns the error description
func (e ErrorCode) Description() string {
	switch e {
	case ErrInternalError:
		return "We encountered an internal error. Please try again."
	case ErrNoSuchBucket:
		return "The specified bucket does not exist."
	case ErrNoSuchKey:
		return "The specified key does not exist."
	case ErrBucketAlreadyExists:
		return "The requested bucket name is not available."
	case ErrBucketNotEmpty:
		return "The bucket you tried to delete is not empty."
	case ErrMethodNotAllowed:
		return "The specified method is not allowed against this resource."
	case ErrInvalidAccessKeyID:
		return "The AWS access key ID you provided does not exist in our records."
	case ErrSignatureDoesNotMatch:
		return "The request signature we calculated does not match the signature you provided."
	case ErrInvalidBucketName:
		return "The specified bucket is not valid."
	case ErrInvalidObjectKey:
		return "Your key is too long or contains invalid characters."
	case ErrAccessDenied:
		return "Access Denied"
	default:
		return "Internal error"
	}
}

// HTTPStatus returns the HTTP status code for the error
func (e ErrorCode) HTTPStatus() int {
	switch e {
	case ErrNoSuchBucket, ErrNoSuchKey:
		return http.StatusNotFound
	case ErrBucketAlreadyExists:
		return http.StatusConflict
	case ErrBucketNotEmpty:
		return http.StatusConflict
	case ErrMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case ErrInvalidAccessKeyID, ErrSignatureDoesNotMatch, ErrAccessDenied:
		return http.StatusForbidden
	case ErrInvalidBucketName, ErrInvalidObjectKey:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
