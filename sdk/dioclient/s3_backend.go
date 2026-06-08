package dioclient

import (
	"context"
	"io"
)

// s3Backend is the interface satisfied by S3Proxy (and future native implementations).
// It is the seam that keeps minio-go quarantined to sdk/dioclient/compat/minio/.
type s3Backend interface {
	ListBuckets(ctx context.Context) ([]BucketInfo, error)
	ListObjects(ctx context.Context, bucket, prefix string, recursive bool) <-chan ObjectInfo
	PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectInfo, error)
	StatObject(ctx context.Context, bucket, key string) (ObjectInfo, error)
	RemoveObject(ctx context.Context, bucket, key string) error
	CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error
}
