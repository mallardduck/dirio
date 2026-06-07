package profile

import "strings"

// ParsedPath holds the decomposed result of a user-supplied path argument.
type ParsedPath struct {
	// Profile is the profile name prefix, or "" if none was specified.
	Profile string
	// Bucket is the bucket name, or "" when only listing buckets.
	Bucket string
	// Prefix is the optional object key prefix within the bucket.
	Prefix string
}

// ParsePath parses a user-supplied path argument into its components.
//
// Recognised forms:
//
//	""                   → list all buckets in default profile
//	"bucket"             → list objects in bucket (default profile)
//	"bucket/prefix/"     → list objects under prefix (default profile)
//	"profile/bucket"     → list objects in bucket (named profile)
//	"profile/bucket/key" → list objects under prefix (named profile)
//
// Disambiguation: if the first path segment matches a known profile name in
// cfg, it is treated as a profile name; otherwise it is treated as a bucket.
func ParsePath(s string, cfg Config) ParsedPath {
	if s == "" {
		return ParsedPath{}
	}

	parts := strings.SplitN(s, "/", 3)

	// If the first segment is a known profile, treat it as such.
	if _, ok := cfg.Profiles[parts[0]]; ok {
		p := ParsedPath{Profile: parts[0]}
		if len(parts) >= 2 {
			p.Bucket = parts[1]
		}
		if len(parts) == 3 {
			p.Prefix = parts[2]
		}
		return p
	}

	// Otherwise: first segment is the bucket.
	p := ParsedPath{Bucket: parts[0]}
	if len(parts) >= 2 {
		p.Prefix = strings.Join(parts[1:], "/")
	}
	return p
}
