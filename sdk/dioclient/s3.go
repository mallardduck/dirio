package dioclient

import (
	"context"
	"io"
)

// ListBuckets returns all buckets visible to the configured credentials.
func (c *Client) ListBuckets(ctx context.Context) ([]BucketInfo, error) {
	return c.s3.ListBuckets(ctx)
}

// ListObjects streams the objects in bucket with the given prefix. When
// recursive is false a "/" delimiter is used and common prefixes (virtual
// directories) are returned as ObjectInfo entries with Size == -1. The
// returned channel is closed when all results have been sent or ctx is
// cancelled; check ObjectInfo.Err for per-object errors.
func (c *Client) ListObjects(ctx context.Context, bucket, prefix string, recursive bool) <-chan ObjectInfo {
	return c.s3.ListObjects(ctx, bucket, prefix, recursive)
}

// PutObject uploads r to bucket/key. size is the content length (-1 for unknown).
// minio-go automatically uses multipart when size exceeds the part size (8 MiB).
func (c *Client) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return c.s3.PutObject(ctx, bucket, key, r, size, contentType)
}

// GetObject returns a reader for the object content and its metadata.
// The caller must close the returned ReadCloser.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectInfo, error) {
	return c.s3.GetObject(ctx, bucket, key)
}

// StatObject returns metadata for bucket/key without downloading it.
func (c *Client) StatObject(ctx context.Context, bucket, key string) (ObjectInfo, error) {
	return c.s3.StatObject(ctx, bucket, key)
}

// RemoveObject deletes bucket/key.
func (c *Client) RemoveObject(ctx context.Context, bucket, key string) error {
	return c.s3.RemoveObject(ctx, bucket, key)
}

// CopyObject performs a server-side copy from srcBucket/srcKey to dstBucket/dstKey.
func (c *Client) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	return c.s3.CopyObject(ctx, srcBucket, srcKey, dstBucket, dstKey)
}
