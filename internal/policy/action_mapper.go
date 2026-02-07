package policy

// ActionMapper translates S3 API action names (from routes) to the actual
// IAM permissions required to perform those operations.
//
// This is critical because S3 action names do NOT always match 1:1 with
// the IAM permissions required:
//   - HeadObject requires s3:GetObject (not s3:HeadObject)
//   - CopyObject requires BOTH s3:GetObject AND s3:PutObject
//   - All multipart operations (except Abort) require s3:PutObject
//
// See docs/action-permission-mapping.md for complete specification.
type ActionMapper struct {
	// Static mapping: route action → required permission(s)
	mappings map[string][]string

	// Multi-resource actions that require checking multiple resources
	multiResource map[string]bool
}

// NewActionMapper creates a new action mapper with static mappings
func NewActionMapper() *ActionMapper {
	return &ActionMapper{
		mappings:      buildActionMappings(),
		multiResource: buildMultiResourceActions(),
	}
}

// GetRequiredPermissions returns the IAM permission(s) needed for an S3 action.
//
// Examples:
//   - "s3:HeadObject"  → ["s3:GetObject"]
//   - "s3:CopyObject"  → ["s3:GetObject", "s3:PutObject"]
//   - "s3:GetObject"   → ["s3:GetObject"] (1:1 mapping, falls through)
//
// For multi-resource operations (CopyObject, UploadPartCopy), the returned
// permissions should be checked against different resources:
//   - permissions[0] → source resource
//   - permissions[1] → destination resource
func (m *ActionMapper) GetRequiredPermissions(action string) []string {
	if perms, ok := m.mappings[action]; ok {
		return perms
	}
	// Default: assume 1:1 mapping if action not in table
	return []string{action}
}

// IsMultiResourceAction returns true if the action requires checking
// permissions on multiple resources (e.g., CopyObject needs source and dest).
func (m *ActionMapper) IsMultiResourceAction(action string) bool {
	return m.multiResource[action]
}

// buildActionMappings creates the static action-to-permission mapping table.
// Only non-1:1 mappings are included; actions not in the table are assumed
// to have 1:1 mapping (action name = permission name).
func buildActionMappings() map[string][]string {
	return map[string][]string{
		// ============================================================
		// Service Level - Different Names
		// ============================================================
		"s3:ListBuckets": {"s3:ListAllMyBuckets"},

		// ============================================================
		// Bucket Operations - Different Names
		// ============================================================
		// HeadBucket requires ListBucket permission
		"s3:HeadBucket": {"s3:ListBucket"},

		// Both ListObjects versions use ListBucket permission
		"s3:ListObjects":   {"s3:ListBucket"},
		"s3:ListObjectsV2": {"s3:ListBucket"},

		// ListObjectVersions uses ListBucketVersions (different name)
		"s3:ListObjectVersions": {"s3:ListBucketVersions"},

		// ListMultipartUploads uses ListBucketMultipartUploads (different name)
		"s3:ListMultipartUploads": {"s3:ListBucketMultipartUploads"},

		// Bulk delete uses singular DeleteObject permission
		"s3:DeleteObjects": {"s3:DeleteObject"},

		// ============================================================
		// Object Operations - Different Names
		// ============================================================
		// HeadObject requires GetObject permission (cannot grant metadata-only access)
		"s3:HeadObject": {"s3:GetObject"},

		// ============================================================
		// Multi-Resource Operations (require TWO permissions on different resources)
		// ============================================================
		// CopyObject requires GetObject on source AND PutObject on destination
		"s3:CopyObject": {"s3:GetObject", "s3:PutObject"},

		// UploadPartCopy requires GetObject on source AND PutObject on destination
		"s3:UploadPartCopy": {"s3:GetObject", "s3:PutObject"},

		// ============================================================
		// Multipart Upload - All use PutObject except Abort and ListParts
		// ============================================================
		"s3:CreateMultipartUpload":   {"s3:PutObject"},
		"s3:UploadPart":              {"s3:PutObject"},
		"s3:CompleteMultipartUpload": {"s3:PutObject"},
		// AbortMultipartUpload has its own permission (1:1)
		// ListParts uses ListMultipartUploadParts (different name)
		"s3:ListParts": {"s3:ListMultipartUploadParts"},

		// ============================================================
		// All other operations are 1:1 (same name), not listed here
		// Examples: GetObject, PutObject, DeleteObject, CreateBucket,
		//           DeleteBucket, GetBucketPolicy, etc.
		// ============================================================
	}
}

// buildMultiResourceActions identifies actions that require permission
// checks on multiple resources (source and destination).
func buildMultiResourceActions() map[string]bool {
	return map[string]bool{
		"s3:CopyObject":     true,
		"s3:UploadPartCopy": true,
	}
}
