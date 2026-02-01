package s3

import (
	"context"

	"github.com/mallardduck/dirio/pkg/iam"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// PutBucketPolicyRequest contains parameters for setting a bucket policy
type PutBucketPolicyRequest struct {
	Bucket         string
	PolicyDocument *iam.PolicyDocument
}

// PutBucketPolicy sets or updates a bucket policy
func (s *Service) PutBucketPolicy(ctx context.Context, req *PutBucketPolicyRequest) error {
	// Check if bucket exists
	exists, err := s.storage.BucketExists(ctx, req.Bucket)
	if err != nil {
		return err
	}
	if !exists {
		return s3types.ErrBucketNotFound
	}

	// Store the policy in metadata
	return s.metadata.SetBucketPolicy(ctx, req.Bucket, req.PolicyDocument)
}

// GetBucketPolicy retrieves the bucket policy
func (s *Service) GetBucketPolicy(ctx context.Context, bucket string) (*iam.PolicyDocument, error) {
	// Check if bucket exists
	exists, err := s.storage.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, s3types.ErrBucketNotFound
	}

	// Get the policy from metadata
	policy, err := s.metadata.GetBucketPolicy(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, s3types.ErrNoSuchBucketPolicy
	}

	return policy, nil
}

// DeleteBucketPolicy removes the bucket policy
func (s *Service) DeleteBucketPolicy(ctx context.Context, bucket string) error {
	// Check if bucket exists
	exists, err := s.storage.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		return s3types.ErrBucketNotFound
	}

	// Delete the policy from metadata
	return s.metadata.DeleteBucketPolicy(ctx, bucket)
}
