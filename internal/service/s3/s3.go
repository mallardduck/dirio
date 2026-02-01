package s3

import (
	"context"
	"errors"
	"io"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/metadata"
	"github.com/mallardduck/dirio/internal/service/validation"
	"github.com/mallardduck/dirio/internal/storage"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// Service provides S3 operations for buckets and objects
type Service struct {
	storage  *storage.Storage
	metadata *metadata.Manager
}

// NewService creates a new S3 service
func NewService(storage *storage.Storage, metadata *metadata.Manager) *Service {
	return &Service{
		storage:  storage,
		metadata: metadata,
	}
}

// ============================================================================
// Bucket Operations
// ============================================================================

// CreateBucket creates a new bucket with validation
func (s *Service) CreateBucket(ctx context.Context, req *CreateBucketRequest) (*metadata.BucketMetadata, error) {
	// Validate bucket name
	if err := validation.ValidateBucketName(req.Name); err != nil {
		return nil, err
	}

	// Check if bucket already exists
	exists, err := s.storage.BucketExists(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, s3types.ErrBucketAlreadyExists
	}

	// Create bucket using storage layer
	if err := s.storage.CreateBucket(ctx, req.Name); err != nil {
		if errors.Is(err, storage.ErrBucketExists) {
			return nil, s3types.ErrBucketAlreadyExists
		}
		return nil, err
	}

	// If bucket policy was provided, update it
	if req.BucketPolicy != nil {
		bucketMeta, err := s.metadata.GetBucketMetadata(ctx, req.Name)
		if err != nil {
			return nil, err
		}
		bucketMeta.BucketPolicy = req.BucketPolicy
		// Note: Need SaveBucketMetadata method to persist
	}

	return s.GetBucket(ctx, req.Name)
}

// GetBucket retrieves bucket metadata
// Note: Assumes bucket name has been validated by the caller
func (s *Service) GetBucket(ctx context.Context, bucket string) (*metadata.BucketMetadata, error) {
	return s.metadata.GetBucketMetadata(ctx, bucket)
}

// HeadBucket checks if a bucket exists
// Note: Assumes bucket name has been validated by the caller
func (s *Service) HeadBucket(ctx context.Context, bucket string) (bool, error) {
	return s.storage.BucketExists(ctx, bucket)
}

// DeleteBucket deletes a bucket
// Note: Assumes bucket name has been validated by the caller
func (s *Service) DeleteBucket(ctx context.Context, bucket string) error {
	if err := s.storage.DeleteBucket(ctx, bucket); err != nil {
		if errors.Is(err, storage.ErrNoSuchBucket) {
			return s3types.ErrBucketNotFound
		}
		if errors.Is(err, storage.ErrBucketNotEmpty) {
			return s3types.ErrBucketNotEmpty
		}
		return err
	}

	return nil
}

// ListBuckets returns all buckets
func (s *Service) ListBuckets(ctx context.Context) ([]s3types.Bucket, error) {
	return s.storage.ListBuckets(ctx)
}

// GetBucketLocation returns the bucket location (region)
// Note: Assumes bucket name has been validated by the caller
func (s *Service) GetBucketLocation(ctx context.Context, bucket string) (string, error) {
	exists, err := s.HeadBucket(ctx, bucket)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", s3types.ErrBucketNotFound
	}

	// Always return default location for now
	return consts.DefaultBucketLocation, nil
}

// ============================================================================
// Object Operations
// ============================================================================

// PutObject uploads an object to a bucket
// Note: Assumes bucket and key have been validated by the caller
func (s *Service) PutObject(ctx context.Context, req *PutObjectRequest) (string, error) {
	// Default content type if not provided
	contentType := req.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Upload object
	etag, err := s.storage.PutObject(ctx, req.Bucket, req.Key, req.Content, contentType, req.CustomMetadata)
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchBucket) {
			return "", s3types.ErrBucketNotFound
		}
		return "", err
	}

	return etag, nil
}

// GetObject downloads an object from a bucket
// Note: Assumes bucket and key have been validated by the caller
func (s *Service) GetObject(ctx context.Context, req *GetObjectRequest) (*GetObjectResponse, error) {
	// Get object
	obj, err := s.storage.GetObject(ctx, req.Bucket, req.Key)
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchKey) {
			return nil, s3types.ErrObjectNotFound
		}
		if errors.Is(err, storage.ErrNoSuchBucket) {
			return nil, s3types.ErrBucketNotFound
		}
		return nil, err
	}

	return &GetObjectResponse{
		Content:        obj.Content,
		ContentType:    obj.ContentType,
		Size:           obj.Size,
		ETag:           obj.ETag,
		LastModified:   obj.LastModified,
		CustomMetadata: obj.CustomMetadata,
	}, nil
}

// HeadObject retrieves object metadata without downloading the object
// Note: Assumes bucket and key have been validated by the caller
func (s *Service) HeadObject(ctx context.Context, req *HeadObjectRequest) (*HeadObjectResponse, error) {
	// Get metadata
	meta, err := s.storage.GetObjectMetadata(ctx, req.Bucket, req.Key)
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchKey) {
			return nil, s3types.ErrObjectNotFound
		}
		if errors.Is(err, storage.ErrNoSuchBucket) {
			return nil, s3types.ErrBucketNotFound
		}
		return nil, err
	}

	return &HeadObjectResponse{
		ContentType:    meta.ContentType,
		Size:           meta.Size,
		ETag:           meta.ETag,
		LastModified:   meta.LastModified,
		CustomMetadata: meta.CustomMetadata,
	}, nil
}

// DeleteObject deletes an object from a bucket
// Per S3 spec, this is idempotent - returns success even if object doesn't exist
// Note: Assumes bucket and key have been validated by the caller
func (s *Service) DeleteObject(ctx context.Context, req *DeleteObjectRequest) error {
	// Delete object
	err := s.storage.DeleteObject(ctx, req.Bucket, req.Key)
	if err != nil {
		// S3 spec: DeleteObject is idempotent, ignore "not found" errors
		if errors.Is(err, storage.ErrNoSuchKey) {
			return nil
		}
		if errors.Is(err, storage.ErrNoSuchBucket) {
			return s3types.ErrBucketNotFound
		}
		return err
	}

	return nil
}

// ListObjects lists objects in a bucket (V1)
// Note: Assumes bucket name has been validated by the caller
func (s *Service) ListObjects(ctx context.Context, req *ListObjectsRequest) ([]s3types.Object, error) {
	// List objects
	objects, err := s.storage.ListObjects(ctx, req.Bucket, req.Prefix, req.Delimiter, req.MaxKeys)
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchBucket) {
			return nil, s3types.ErrBucketNotFound
		}
		return nil, err
	}

	// todo maybe this should return s3types.ListBucketResult?
	return objects, nil
}

// ListObjectsV2 lists objects in a bucket (V2)
// Note: Assumes bucket name has been validated by the caller
func (s *Service) ListObjectsV2(ctx context.Context, req *ListObjectsV2Request) (storage.InternalResult, error) {
	// List objects
	result, err := s.storage.ListObjectsV2(ctx, req.Bucket, req.Prefix, req.ContinuationToken, req.StartAfter, req.Delimiter, req.MaxKeys, req.FetchOwner)
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchBucket) {
			return storage.InternalResult{}, s3types.ErrBucketNotFound
		}
		return storage.InternalResult{}, err
	}

	// TODO maybe should return s3types.ListBucketV2Result?

	return result, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// ObjectExists checks if an object exists in a bucket
func (s *Service) ObjectExists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := s.HeadObject(ctx, &HeadObjectRequest{
		Bucket: bucket,
		Key:    key,
	})

	if err != nil {
		if errors.Is(err, s3types.ErrObjectNotFound) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// CopyObject copies an object from one location to another
// Note: This is a placeholder for future implementation
func (s *Service) CopyObject(ctx context.Context, sourceBucket, sourceKey, destBucket, destKey string) error {
	// Validate all inputs
	if err := validation.ValidateBucketName(sourceBucket); err != nil {
		return err
	}
	if err := validation.ValidateObjectKey(sourceKey); err != nil {
		return err
	}
	if err := validation.ValidateBucketName(destBucket); err != nil {
		return err
	}
	if err := validation.ValidateObjectKey(destKey); err != nil {
		return err
	}

	// Get source object
	obj, err := s.GetObject(ctx, &GetObjectRequest{
		Bucket: sourceBucket,
		Key:    sourceKey,
	})
	if err != nil {
		return err
	}
	defer obj.Content.Close()

	// Copy to destination
	_, err = s.PutObject(ctx, &PutObjectRequest{
		Bucket:         destBucket,
		Key:            destKey,
		Content:        obj.Content,
		ContentType:    obj.ContentType,
		CustomMetadata: obj.CustomMetadata,
	})

	return err
}

// GetObjectWithRange retrieves part of an object (for range requests)
// Note: This is a placeholder for future implementation
func (s *Service) GetObjectWithRange(ctx context.Context, bucket, key string, start, end int64) (io.ReadCloser, error) {
	// Validate inputs
	if err := validation.ValidateBucketName(bucket); err != nil {
		return nil, err
	}
	if err := validation.ValidateObjectKey(key); err != nil {
		return nil, err
	}

	// TODO: Implement range request support in storage layer
	// For now, return the full object
	obj, err := s.GetObject(ctx, &GetObjectRequest{
		Bucket: bucket,
		Key:    key,
	})
	if err != nil {
		return nil, err
	}

	return obj.Content, nil
}
