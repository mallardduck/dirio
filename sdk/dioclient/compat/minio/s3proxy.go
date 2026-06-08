package minio

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Proxy wraps minio.Client with methods that return native dioclient types.
// Construct one via NewS3Proxy; it is safe for concurrent use.
type S3Proxy struct {
	mc *minio.Client
}

// NewS3Proxy creates an S3Proxy from an endpoint URL and credentials.
func NewS3Proxy(endpoint, accessKey, secretKey, region string, pathStyle bool) (*S3Proxy, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("s3 proxy: invalid endpoint %q: %w", endpoint, err)
	}
	secure := u.Scheme == "https"

	lookup := minio.BucketLookupAuto
	if pathStyle {
		lookup = minio.BucketLookupPath
	}

	mc, err := minio.New(u.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       secure,
		Region:       region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 proxy: %w", err)
	}
	return &S3Proxy{mc: mc}, nil
}

func (p *S3Proxy) ListBuckets(ctx context.Context) ([]BucketInfo, error) {
	buckets, err := p.mc.ListBuckets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BucketInfo, len(buckets))
	for i, b := range buckets {
		out[i] = mapBucketInfo(b)
	}
	return out, nil
}

func (p *S3Proxy) ListObjects(ctx context.Context, bucket, prefix string, recursive bool) <-chan ObjectInfo {
	out := make(chan ObjectInfo)
	go func() {
		defer close(out)
		opts := minio.ListObjectsOptions{Prefix: prefix, Recursive: recursive}
		for obj := range p.mc.ListObjects(ctx, bucket, opts) {
			info := mapObjectInfo(obj)
			select {
			case out <- info:
			case <-ctx.Done():
				return
			}
			if obj.Err != nil {
				return
			}
		}
	}()
	return out
}

func (p *S3Proxy) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	_, err := p.mc.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
		PartSize:    8 * 1024 * 1024,
	})
	return err
}

func (p *S3Proxy) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectInfo, error) {
	obj, err := p.mc.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	stat, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, ObjectInfo{}, err
	}
	return obj, mapObjectInfo(minio.ObjectInfo{
		Key:          stat.Key,
		Size:         stat.Size,
		LastModified: stat.LastModified,
		ETag:         stat.ETag,
		ContentType:  stat.ContentType,
		StorageClass: stat.StorageClass,
	}), nil
}

func (p *S3Proxy) StatObject(ctx context.Context, bucket, key string) (ObjectInfo, error) {
	stat, err := p.mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, err
	}
	return mapObjectInfo(minio.ObjectInfo{
		Key:          stat.Key,
		Size:         stat.Size,
		LastModified: stat.LastModified,
		ETag:         stat.ETag,
		ContentType:  stat.ContentType,
		StorageClass: stat.StorageClass,
	}), nil
}

func (p *S3Proxy) RemoveObject(ctx context.Context, bucket, key string) error {
	return p.mc.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}

func (p *S3Proxy) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	src := minio.CopySrcOptions{Bucket: srcBucket, Object: srcKey}
	dst := minio.CopyDestOptions{Bucket: dstBucket, Object: dstKey}
	_, err := p.mc.CopyObject(ctx, dst, src)
	return err
}
