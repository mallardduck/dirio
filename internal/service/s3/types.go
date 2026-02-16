package s3

import (
	"io"
	"time"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// ============================================================================
// Bucket Request/Response Types
// ============================================================================

// CreateBucketRequest represents a request to create a new bucket
type CreateBucketRequest struct {
	Name         string
	Owner        string                   // User who owns the bucket
	BucketPolicy *metadata.PolicyDocument // Optional bucket policy
}

// UpdateBucketRequest represents a request to update bucket metadata
type UpdateBucketRequest struct {
	BucketPolicy *metadata.PolicyDocument // Update bucket policy
}

// ============================================================================
// Object Request/Response Types
// ============================================================================

// PutObjectRequest represents a request to upload an object
type PutObjectRequest struct {
	Bucket         string
	Key            string
	Content        io.Reader
	ContentType    string
	CustomMetadata map[string]string // S3 metadata headers (Cache-Control, x-amz-meta-*, etc.)
}

// GetObjectRequest represents a request to download an object
type GetObjectRequest struct {
	Bucket string
	Key    string
}

// GetObjectResponse represents the response from downloading an object
type GetObjectResponse struct {
	Content        io.ReadCloser
	ContentType    string
	Size           int64
	ETag           string
	LastModified   time.Time
	CustomMetadata map[string]string
}

// HeadObjectRequest represents a request to get object metadata
type HeadObjectRequest struct {
	Bucket string
	Key    string
}

// HeadObjectResponse represents object metadata
type HeadObjectResponse struct {
	ContentType    string
	Size           int64
	ETag           string
	LastModified   time.Time
	CustomMetadata map[string]string
}

// DeleteObjectRequest represents a request to delete an object
type DeleteObjectRequest struct {
	Bucket string
	Key    string
}

// DeleteObjectsRequest represents a request to delete multiple objects
type DeleteObjectsRequest struct {
	Bucket  string
	Objects []s3types.ObjectIdentifier
	Quiet   bool // If true, only return errors in response
}

// DeleteObjectsResponse represents the response from deleting multiple objects
type DeleteObjectsResponse struct {
	Deleted []s3types.DeletedObject
	Errors  []s3types.DeleteError
}

// ListObjectsRequest represents a request to list objects in a bucket
type ListObjectsRequest struct {
	Bucket    string
	Prefix    string
	Delimiter string
	// TODO add marker support
	MaxKeys int
}

// ListObjectsV2Request represents a request to list objects (V2)
type ListObjectsV2Request struct {
	Bucket            string
	Prefix            string
	ContinuationToken string
	StartAfter        string
	Delimiter         string
	MaxKeys           int
	FetchOwner        bool
}

// ============================================================================
// Multipart Upload Request/Response Types
// ============================================================================

// CreateMultipartUploadRequest represents a request to initiate a multipart upload
type CreateMultipartUploadRequest struct {
	Bucket         string
	Key            string
	ContentType    string
	CustomMetadata map[string]string
}

// CreateMultipartUploadResponse represents the response from initiating a multipart upload
type CreateMultipartUploadResponse struct {
	UploadID string
	Bucket   string
	Key      string
}

// UploadPartRequest represents a request to upload a part
type UploadPartRequest struct {
	Bucket     string
	Key        string
	UploadID   string
	PartNumber int
	Content    io.Reader
}

// UploadPartResponse represents the response from uploading a part
type UploadPartResponse struct {
	ETag string
}

// CompletePart represents a part in the complete multipart upload request
type CompletePart struct {
	PartNumber int
	ETag       string
}

// CompleteMultipartUploadRequest represents a request to complete a multipart upload
type CompleteMultipartUploadRequest struct {
	Bucket   string
	Key      string
	UploadID string
	Parts    []CompletePart
}

// CompleteMultipartUploadResponse represents the response from completing a multipart upload
type CompleteMultipartUploadResponse struct {
	Location string
	Bucket   string
	Key      string
	ETag     string
}

// AbortMultipartUploadRequest represents a request to abort a multipart upload
type AbortMultipartUploadRequest struct {
	Bucket   string
	Key      string
	UploadID string
}

// ListPartsRequest represents a request to list parts in a multipart upload
type ListPartsRequest struct {
	Bucket   string
	Key      string
	UploadID string
	MaxParts int
}

// Part represents a single part in a multipart upload
type Part struct {
	PartNumber   int
	ETag         string
	Size         int64
	LastModified time.Time
}

// ListPartsResponse represents the response from listing parts
type ListPartsResponse struct {
	Bucket   string
	Key      string
	UploadID string
	Parts    []Part
}
