package minio

import "github.com/minio/minio-go/v7"

// mapBucketInfo converts a minio bucket entry to our native type.
func mapBucketInfo(b minio.BucketInfo) BucketInfo {
	return BucketInfo{
		Name:      b.Name,
		CreatedAt: b.CreationDate,
	}
}

// mapObjectInfo converts a minio object entry to our native type.
func mapObjectInfo(o minio.ObjectInfo) ObjectInfo {
	return ObjectInfo{
		Key:          o.Key,
		Size:         o.Size,
		LastModified: o.LastModified,
		ETag:         o.ETag,
		ContentType:  o.ContentType,
		StorageClass: o.StorageClass,
		Err:          o.Err,
	}
}
