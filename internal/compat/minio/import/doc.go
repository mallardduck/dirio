// Package minioimport provides read-only data import from MinIO single-node
// filesystem mode installations.
//
// This package is a one-way bridge: it reads an existing MinIO data directory
// and returns parsed users, groups, policies, service accounts, bucket metadata,
// and per-object metadata. It never writes to the MinIO directory.
//
// Supported MinIO formats:
//   - Single-node filesystem mode ("fs") — 2019 legacy and 2022+ modern layouts
//   - Distributed / erasure-coded installations are NOT supported
//
// MinIO data structure read:
//
//	.minio.sys/
//	  format.json               — format validation (must be "fs")
//	  config/iam/users/         — user credentials
//	  config/iam/groups/        — group membership
//	  config/iam/policies/      — IAM policy documents
//	  config/iam/policydb/      — user/group → policy mappings
//	  config/iam/service-accounts/ — service account credentials
//	  buckets/                  — bucket metadata (.metadata.bin, policy.json, …)
//
// Typical usage:
//
//	result, err := minioimport.Import(minioFS)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// result.Users, result.Buckets, result.Policies, …
package minioimport
