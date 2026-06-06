package ui

import (
	"fmt"
)

// CurrentOwnerAccessKey returns the access key of the current bucket owner,
// or "" when the bucket is admin-owned (nil Owner).
func (d BucketDetailData) CurrentOwnerAccessKey() string {
	if d.Owner != nil {
		return d.Owner.AccessKey
	}
	return ""
}

// formatBytes formats a byte count as a human-readable string (e.g. "1.4 MB").
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ParentPrefix returns the parent "folder" prefix for a given object key.
// For "photos/2024/img.jpg" it returns "photos/2024/".
// For a top-level key it returns "".
func ParentPrefix(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			return key[:i+1]
		}
	}
	return ""
}

// parentPrefix is the unexported alias used within the ui package.
func parentPrefix(key string) string { return ParentPrefix(key) }
