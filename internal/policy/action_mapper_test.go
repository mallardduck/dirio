package policy

import (
	"testing"
)

func TestActionMapper_GetRequiredPermissions(t *testing.T) {
	mapper := NewActionMapper()

	tests := []struct {
		name     string
		action   string
		expected []string
	}{
		// 1:1 Same Name (fall through)
		{
			name:     "GetObject is 1:1",
			action:   "s3:GetObject",
			expected: []string{"s3:GetObject"},
		},
		{
			name:     "PutObject is 1:1",
			action:   "s3:PutObject",
			expected: []string{"s3:PutObject"},
		},
		{
			name:     "DeleteObject is 1:1",
			action:   "s3:DeleteObject",
			expected: []string{"s3:DeleteObject"},
		},
		{
			name:     "CreateBucket is 1:1",
			action:   "s3:CreateBucket",
			expected: []string{"s3:CreateBucket"},
		},
		{
			name:     "DeleteBucket is 1:1",
			action:   "s3:DeleteBucket",
			expected: []string{"s3:DeleteBucket"},
		},
		{
			name:     "GetBucketPolicy is 1:1",
			action:   "s3:GetBucketPolicy",
			expected: []string{"s3:GetBucketPolicy"},
		},
		{
			name:     "AbortMultipartUpload is 1:1",
			action:   "s3:AbortMultipartUpload",
			expected: []string{"s3:AbortMultipartUpload"},
		},

		// 1:1 Different Name
		{
			name:     "HeadBucket requires ListBucket",
			action:   "s3:HeadBucket",
			expected: []string{"s3:ListBucket"},
		},
		{
			name:     "HeadObject requires GetObject",
			action:   "s3:HeadObject",
			expected: []string{"s3:GetObject"},
		},
		{
			name:     "ListObjects requires ListBucket",
			action:   "s3:ListObjects",
			expected: []string{"s3:ListBucket"},
		},
		{
			name:     "ListObjectsV2 requires ListBucket",
			action:   "s3:ListObjectsV2",
			expected: []string{"s3:ListBucket"},
		},
		{
			name:     "ListBuckets requires ListAllMyBuckets",
			action:   "s3:ListBuckets",
			expected: []string{"s3:ListAllMyBuckets"},
		},
		{
			name:     "ListObjectVersions requires ListBucketVersions",
			action:   "s3:ListObjectVersions",
			expected: []string{"s3:ListBucketVersions"},
		},
		{
			name:     "ListMultipartUploads requires ListBucketMultipartUploads",
			action:   "s3:ListMultipartUploads",
			expected: []string{"s3:ListBucketMultipartUploads"},
		},
		{
			name:     "ListParts requires ListMultipartUploadParts",
			action:   "s3:ListParts",
			expected: []string{"s3:ListMultipartUploadParts"},
		},
		{
			name:     "DeleteObjects requires DeleteObject (singular)",
			action:   "s3:DeleteObjects",
			expected: []string{"s3:DeleteObject"},
		},

		// 1:Many (Multi-Resource)
		{
			name:     "CopyObject requires GetObject AND PutObject",
			action:   "s3:CopyObject",
			expected: []string{"s3:GetObject", "s3:PutObject"},
		},
		{
			name:     "UploadPartCopy requires GetObject AND PutObject",
			action:   "s3:UploadPartCopy",
			expected: []string{"s3:GetObject", "s3:PutObject"},
		},

		// Multipart Operations
		{
			name:     "CreateMultipartUpload requires PutObject",
			action:   "s3:CreateMultipartUpload",
			expected: []string{"s3:PutObject"},
		},
		{
			name:     "UploadPart requires PutObject",
			action:   "s3:UploadPart",
			expected: []string{"s3:PutObject"},
		},
		{
			name:     "CompleteMultipartUpload requires PutObject",
			action:   "s3:CompleteMultipartUpload",
			expected: []string{"s3:PutObject"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapper.GetRequiredPermissions(tt.action)
			if len(got) != len(tt.expected) {
				t.Errorf("GetRequiredPermissions(%q) = %v, want %v", tt.action, got, tt.expected)
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("GetRequiredPermissions(%q)[%d] = %q, want %q", tt.action, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestActionMapper_IsMultiResourceAction(t *testing.T) {
	mapper := NewActionMapper()

	tests := []struct {
		action   string
		expected bool
	}{
		// Multi-resource actions
		{"s3:CopyObject", true},
		{"s3:UploadPartCopy", true},

		// Single-resource actions
		{"s3:GetObject", false},
		{"s3:PutObject", false},
		{"s3:DeleteObject", false},
		{"s3:HeadObject", false},
		{"s3:ListObjects", false},
		{"s3:CreateMultipartUpload", false},
		{"s3:UploadPart", false},
		{"s3:CompleteMultipartUpload", false},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			got := mapper.IsMultiResourceAction(tt.action)
			if got != tt.expected {
				t.Errorf("IsMultiResourceAction(%q) = %v, want %v", tt.action, got, tt.expected)
			}
		})
	}
}

func TestActionMapper_UnknownAction(t *testing.T) {
	mapper := NewActionMapper()

	// Unknown actions should return 1:1 mapping (the action itself)
	unknownAction := "s3:SomeUnknownAction"
	got := mapper.GetRequiredPermissions(unknownAction)

	if len(got) != 1 || got[0] != unknownAction {
		t.Errorf("GetRequiredPermissions(%q) = %v, want [%q]", unknownAction, got, unknownAction)
	}
}
