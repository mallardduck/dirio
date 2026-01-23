// Package minio provides read-only MinIO data import functionality.
//
// This package handles importing data from MinIO single-node filesystem mode
// installations. It maintains a clean boundary between read-only MinIO import
// operations and DirIO's read/write metadata management.
//
// Supported MinIO Format:
//
// Only single-node filesystem mode (format: "fs") is supported. Distributed
// or erasure-coded MinIO installations are not supported.
//
// MinIO data structure:
//   - .minio.sys/format.json - Format validation (must be fs mode)
//   - .minio.sys/config/iam/users/ - User credentials
//   - .minio.sys/buckets/ - Bucket metadata
//
// Example usage:
//
//	result, err := minio.Import("/path/to/minio-data")
//	if err != nil {
//		log.Fatal(err)
//	}
//	// Use result.Users and result.Buckets
//
// The import process validates the format first (ValidateFormat), then reads
// users and bucket metadata without modifying any MinIO files.
package minio
