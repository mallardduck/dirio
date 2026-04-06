// Package http implements the MinIO Admin API v3 compatibility layer
// (prefix /minio/admin/v3/).
//
// This package is NOT related to the MinIO data importer in minio/import/.
// Its sole purpose is to expose a subset of the MinIO Admin API so that
// standard MinIO clients (mc, madmin-go) can manage DirIO's IAM objects —
// users, groups, policies, and service accounts — without any client-side
// changes.
//
// The handlers delegate entirely to the DirIO service layer via
// service.ServicesFactory; there is no MinIO code in this path.
//
// Relationship to other packages:
//   - minio/import  — one-time data importer from legacy MinIO installations
//   - minio/middleware — detects MinIO SDK User-Agent and injects compat headers
//   - internal/http/server/dirioapi — DirIO-native REST API (/.dirio/api/v1/)
package http
