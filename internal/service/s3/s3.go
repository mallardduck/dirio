package s3

import (
	"context"
	"errors"
	"io"
	"io/fs"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/consts"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/persistence/storage"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/service/validation"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// Service provides S3 operations for buckets and objects
type Service struct {
	diskStorage     *storage.Storage
	metadataManager *metadata.Manager
	policyEngine    *policy.Engine
}

// NewService creates a new S3 service
func NewService(diskStorage *storage.Storage, metadataManager *metadata.Manager, policyEngine *policy.Engine) *Service {
	return &Service{
		diskStorage:     diskStorage,
		metadataManager: metadataManager,
		policyEngine:    policyEngine,
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

	// Check if the bucket already exists
	exists, err := s.diskStorage.BucketExists(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, s3types.ErrBucketAlreadyExists
	}

	// Create bucket using diskStorage layer
	if err := s.diskStorage.CreateBucket(ctx, req.Name); err != nil {
		if errors.Is(err, storage.ErrBucketExists) {
			return nil, s3types.ErrBucketAlreadyExists
		}
		return nil, err
	}

	// If a bucket policy was provided, update it
	if req.BucketPolicy != nil {
		bucketMeta, err := s.metadataManager.GetBucketMetadata(ctx, req.Name)
		if err != nil {
			return nil, err
		}
		bucketMeta.BucketPolicy = req.BucketPolicy
		// Note: Need SaveBucketMetadata method to persist
	}

	return s.GetBucket(ctx, req.Name)
}

// GetBucket retrieves bucket metadata.
// Note: Assumes the caller has validated bucket name
func (s *Service) GetBucket(ctx context.Context, bucket string) (*metadata.BucketMetadata, error) {
	meta, err := s.metadataManager.GetBucketMetadata(ctx, bucket)
	if err != nil {
		if isMetadataNotFound(err) {
			return nil, s3types.ErrBucketNotFound
		}
		return nil, err
	}
	return meta, nil
}

// HeadBucket checks if a bucket exists
// Note: Assumes the caller has validated bucket name
func (s *Service) HeadBucket(ctx context.Context, bucket string) (bool, error) {
	return s.diskStorage.BucketExists(ctx, bucket)
}

// DeleteBucket deletes a bucket
// Note: Assumes the caller has validated bucket name
func (s *Service) DeleteBucket(ctx context.Context, bucket string) error {
	if err := s.diskStorage.DeleteBucket(ctx, bucket); err != nil {
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
	return s.diskStorage.ListBuckets(ctx)
}

// ListBucketsWithMetadata returns full bucket metadata for every bucket.
// Used by the console to display ownership information alongside bucket names.
func (s *Service) ListBucketsWithMetadata(ctx context.Context) ([]*metadata.BucketMetadata, error) {
	return s.metadataManager.ListBucketMetadatas(ctx)
}

// SetBucketOwner updates the owner UUID of an existing bucket.
func (s *Service) SetBucketOwner(ctx context.Context, bucket string, ownerUUID *uuid.UUID) error {
	if err := s.metadataManager.SetBucketOwner(ctx, bucket, ownerUUID); err != nil {
		if isMetadataNotFound(err) {
			return s3types.ErrBucketNotFound
		}
		return err
	}
	return nil
}

// GetBucketOwner resolves full ownership information for a bucket.
// OwnerInfo.OwnerUUID is nil for admin-owned buckets; AccessKey and Username
// are populated when the owner is a known IAM user.
func (s *Service) GetBucketOwner(ctx context.Context, bucket string) (*OwnerInfo, error) {
	meta, err := s.GetBucket(ctx, bucket)
	if err != nil {
		return nil, err
	}
	return s.resolveOwner(ctx, meta.Owner), nil
}

// GetObjectOwner resolves full ownership information for an object.
// Returns s3types.ErrBucketNotFound or s3types.ErrObjectNotFound as appropriate.
func (s *Service) GetObjectOwner(ctx context.Context, bucket, key string) (*OwnerInfo, error) {
	ownerUUID, err := s.GetObjectOwnerUUID(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	return s.resolveOwner(ctx, ownerUUID), nil
}

// resolveOwner maps an owner UUID to an OwnerInfo, looking up IAM user details
// when the UUID is non-nil. Unknown users are returned with UUID only.
func (s *Service) resolveOwner(ctx context.Context, ownerUUID *uuid.UUID) *OwnerInfo {
	info := &OwnerInfo{OwnerUUID: ownerUUID}
	if ownerUUID == nil {
		return info
	}
	if user, err := s.metadataManager.GetUser(ctx, *ownerUUID); err == nil {
		info.AccessKey = user.AccessKey
		info.Username = user.Username
	}
	return info
}

// GetObjectOwnerUUID returns the owner UUID of an object, or nil if unset (admin-owned).
// Returns s3types.ErrBucketNotFound when the bucket does not exist,
// and s3types.ErrObjectNotFound when the key does not exist within the bucket.
func (s *Service) GetObjectOwnerUUID(ctx context.Context, bucket, key string) (*uuid.UUID, error) {
	meta, err := s.metadataManager.GetObjectMetadata(ctx, bucket, key)
	if err != nil {
		if isMetadataNotFound(err) {
			// Disambiguate: bucket missing vs key missing.
			exists, headErr := s.diskStorage.BucketExists(ctx, bucket)
			if headErr == nil && !exists {
				return nil, s3types.ErrBucketNotFound
			}
			return nil, s3types.ErrObjectNotFound
		}
		return nil, err
	}
	return meta.Owner, nil
}

// isMetadataNotFound reports whether err from the metadata layer represents a
// not-found condition. GetBucketMetadata passes through *os.PathError wrapping
// fs.ErrNotExist; GetObjectMetadata converts it to a plain string.
func isMetadataNotFound(err error) bool {
	return errors.Is(err, fs.ErrNotExist) ||
		(err != nil && (err.Error() == "file does not exist" || err.Error() == "object metadata not found"))
}

// GetBucketLocation returns the bucket location (region)
// Note: Assumes the caller has validated bucket name
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
// Note: Assumes the caller has validated bucket and key
func (s *Service) PutObject(ctx context.Context, req *PutObjectRequest) (string, error) {
	// Default content type if not provided
	contentType := req.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Upload object
	etag, err := s.diskStorage.PutObject(ctx, req.Bucket, req.Key, req.Content, contentType, req.CustomMetadata)
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
	obj, err := s.diskStorage.GetObject(ctx, req.Bucket, req.Key)
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

// HeadObject retrieves object metadataManager without downloading the object
// Note: Assumes bucket and key have been validated by the caller
func (s *Service) HeadObject(ctx context.Context, req *HeadObjectRequest) (*HeadObjectResponse, error) {
	// Get metadataManager
	meta, err := s.diskStorage.GetObjectMetadata(ctx, req.Bucket, req.Key)
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
	err := s.diskStorage.DeleteObject(ctx, req.Bucket, req.Key)
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
func (s *Service) ListObjects(ctx context.Context, req *ListObjectsRequest) (storage.InternalResult, error) {
	result, err := s.diskStorage.ListObjects(ctx, req.Bucket, req.Prefix, req.Marker, req.Delimiter, req.MaxKeys)
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchBucket) {
			return storage.InternalResult{}, s3types.ErrBucketNotFound
		}
		return storage.InternalResult{}, err
	}

	return result, nil
}

// ListObjectsV2 lists objects in a bucket (V2)
// Note: Assumes bucket name has been validated by the caller
func (s *Service) ListObjectsV2(ctx context.Context, req *ListObjectsV2Request) (storage.InternalResult, error) {
	// List objects
	result, err := s.diskStorage.ListObjectsV2(ctx, req.Bucket, req.Prefix, req.ContinuationToken, req.StartAfter, req.Delimiter, req.MaxKeys, req.FetchOwner)
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

	// TODO: Implement range request support in diskStorage layer
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

// DeleteObjects deletes multiple objects in a single request
// Returns a top-level error for request-level failures (invalid request, bucket not found, etc.)
// Per-object deletion failures are collected in the response's Errors slice
func (s *Service) DeleteObjects(ctx context.Context, request *DeleteObjectsRequest) (*DeleteObjectsResponse, error) {
	// Validate bucket name
	if err := validation.ValidateBucketName(request.Bucket); err != nil {
		return nil, err
	}

	objectCount := len(request.Objects)
	// Validate objects list is not empty
	if objectCount == 0 {
		return nil, errors.New("delete request must contain at least one object")
	}

	// AWS S3 supports up to 1000 objects per request
	if objectCount > 1000 {
		return nil, errors.New("cannot delete more than 1000 objects in a single request")
	}

	// Validate each object key
	for _, obj := range request.Objects {
		if err := validation.ValidateObjectKey(obj.Key); err != nil {
			return nil, err
		}
	}

	// Check if bucket exists (early return error for bucket-level issues)
	exists, err := s.HeadBucket(ctx, request.Bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, s3types.ErrBucketNotFound
	}

	// Initialize response with pre-allocated slices
	response := &DeleteObjectsResponse{
		Deleted: make([]s3types.DeletedObject, 0, len(request.Objects)),
		Errors:  make([]s3types.DeleteError, 0),
	}

	// Delete each object
	for _, obj := range request.Objects {
		err := s.diskStorage.DeleteObject(ctx, request.Bucket, obj.Key)

		if err != nil {
			// Per S3 spec, DeleteObject is idempotent - treat not-found as success
			if errors.Is(err, storage.ErrNoSuchKey) {
				// Add to deleted list unless in quiet mode
				if !request.Quiet {
					response.Deleted = append(response.Deleted, s3types.DeletedObject{
						Key:       obj.Key,
						VersionId: obj.VersionId,
					})
				}
				continue
			}

			// For other errors, add to errors list (always, regardless of Quiet mode)
			response.Errors = append(response.Errors, s3types.DeleteError{
				Key:       obj.Key,
				VersionId: obj.VersionId,
				Code:      "InternalError",
				Message:   err.Error(),
			})
			continue
		}

		// Successfully deleted - add to deleted list unless in quiet mode
		if !request.Quiet {
			response.Deleted = append(response.Deleted, s3types.DeletedObject{
				Key:       obj.Key,
				VersionId: obj.VersionId,
			})
		}
	}

	return response, nil
}

// ============================================================================
// Object Tagging Operations
// ============================================================================

// PutObjectTagging sets tags on an object
func (s *Service) PutObjectTagging(ctx context.Context, req *PutObjectTaggingRequest) error {
	// Validate inputs
	if err := validation.ValidateBucketName(req.Bucket); err != nil {
		return err
	}
	if err := validation.ValidateObjectKey(req.Key); err != nil {
		return err
	}

	// Check if object exists
	exists, err := s.ObjectExists(ctx, req.Bucket, req.Key)
	if err != nil {
		return err
	}
	if !exists {
		return s3types.ErrObjectNotFound
	}

	// Get existing metadataManager
	meta, err := s.metadataManager.GetObjectMetadata(ctx, req.Bucket, req.Key)
	if err != nil {
		return err
	}

	// Update tags
	meta.Tags = req.Tags

	// Save metadataManager
	return s.metadataManager.PutObjectMetadata(ctx, req.Bucket, req.Key, meta)
}

// GetObjectTagging retrieves tags from an object
func (s *Service) GetObjectTagging(ctx context.Context, req *GetObjectTaggingRequest) (map[string]string, error) {
	// Validate inputs
	if err := validation.ValidateBucketName(req.Bucket); err != nil {
		return nil, err
	}
	if err := validation.ValidateObjectKey(req.Key); err != nil {
		return nil, err
	}

	// Check if object exists
	exists, err := s.ObjectExists(ctx, req.Bucket, req.Key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, s3types.ErrObjectNotFound
	}

	// Get metadataManager
	meta, err := s.metadataManager.GetObjectMetadata(ctx, req.Bucket, req.Key)
	if err != nil {
		return nil, err
	}

	// Return tags (empty map if no tags)
	if meta.Tags == nil {
		return make(map[string]string), nil
	}

	return meta.Tags, nil
}
