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
	exists, err := s.diskStorage.BucketExists(ctx, req.Bucket)
	if err != nil {
		return err
	}
	if !exists {
		return s3types.ErrBucketNotFound
	}

	// Store the policy in metadataManager
	if err := s.metadataManager.SetBucketPolicy(ctx, req.Bucket, req.PolicyDocument); err != nil {
		return err
	}

	// Notify policy engine of the change
	if s.policyEngine != nil {
		s.policyEngine.UpdateBucketPolicy(req.Bucket, req.PolicyDocument)
	}

	return nil
}

// GetBucketPolicy retrieves the bucket policy
func (s *Service) GetBucketPolicy(ctx context.Context, bucket string) (*iam.PolicyDocument, error) {
	// Check if bucket exists
	exists, err := s.diskStorage.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, s3types.ErrBucketNotFound
	}

	// Get the policy from metadataManager
	policy, err := s.metadataManager.GetBucketPolicy(ctx, bucket)
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
	exists, err := s.diskStorage.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		return s3types.ErrBucketNotFound
	}

	// Delete the policy from metadataManager
	if err := s.metadataManager.DeleteBucketPolicy(ctx, bucket); err != nil {
		return err
	}

	// Notify policy engine of the deletion
	if s.policyEngine != nil {
		s.policyEngine.DeleteBucketPolicy(bucket)
	}

	return nil
}
