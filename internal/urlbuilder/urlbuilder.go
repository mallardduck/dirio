package urlbuilder

import (
	"fmt"
	"net/http"
	"strings"
)

// Builder generates URLs for S3 API responses
type Builder struct {
	canonicalDomain string
}

// New creates a new URLBuilder
// If canonicalDomain is empty, URLs will be built from request Host header
func New(canonicalDomain string) *Builder {
	return &Builder{
		canonicalDomain: canonicalDomain,
	}
}

// BucketURL generates a URL for bucket operations
// Format: http://host/bucket or https://domain/bucket
func (b *Builder) BucketURL(r *http.Request, bucket string) string {
	scheme := b.detectScheme(r)
	host := b.detectHost(r)
	return fmt.Sprintf("%s://%s/%s", scheme, host, bucket)
}

// ObjectURL generates a URL for object operations
// Format: http://host/bucket/key or https://domain/bucket/key
func (b *Builder) ObjectURL(r *http.Request, bucket, key string) string {
	scheme := b.detectScheme(r)
	host := b.detectHost(r)
	return fmt.Sprintf("%s://%s/%s/%s", scheme, host, bucket, key)
}

// detectHost returns the host to use for URL generation
// Priority: CanonicalDomain > request Host header
func (b *Builder) detectHost(r *http.Request) string {
	if b.canonicalDomain != "" {
		return b.canonicalDomain
	}

	// Use request Host header (includes port if non-standard)
	return r.Host
}

// detectScheme returns the scheme (http/https) for URL generation
// Strategy:
// - If CanonicalDomain is set → assume HTTPS
// - Otherwise detect from request:
//  1. Check X-Forwarded-Proto header (reverse proxy)
//  2. Check TLS state
//  3. Default to http
func (b *Builder) detectScheme(r *http.Request) string {
	// If canonical domain is configured, assume HTTPS
	if b.canonicalDomain != "" {
		return "https"
	}

	// Check X-Forwarded-Proto header (set by reverse proxies)
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return strings.ToLower(proto)
	}

	// Check if request came over TLS
	if r.TLS != nil {
		return "https"
	}

	// Default to http
	return "http"
}
