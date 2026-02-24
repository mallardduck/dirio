package variables

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mallardduck/go-http-helpers/pkg/headers"

	contextInt "github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/pkg/iam"
)

// FromRequest builds a variable context from an HTTP request
// It extracts user identity, request metadata, and prepares for S3-specific context
func FromRequest(r *http.Request) *Context {
	ctx := &Context{
		CurrentTime: time.Now().UTC(),
	}

	// Extract user from request context (if authenticated)
	if user, err := contextInt.GetUser(r.Context()); err == nil && user != nil {
		ctx.Username = user.Username
		ctx.UserID = user.UUID
	}

	// Extract source IP from request
	ctx.SourceIP = extractSourceIP(r)

	// Extract User-Agent
	ctx.UserAgent = r.Header.Get(headers.UserAgent)

	return ctx
}

// WithS3Context adds S3-specific variables (prefix, delimiter) to the context
// This should be called when evaluating policies for ListObjects operations
func (c *Context) WithS3Context(prefix, delimiter string) *Context {
	// Create a copy to avoid mutating the original
	ctxCopy := *c
	ctxCopy.S3Prefix = prefix
	ctxCopy.S3Delimiter = delimiter
	return &ctxCopy
}

// extractSourceIP gets the client's IP address from the request
// It checks X-Forwarded-For and X-Real-IP headers for proxy scenarios
func extractSourceIP(r *http.Request) net.IP {
	// Check X-Forwarded-For header (comma-separated list, first is client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if parsed := net.ParseIP(ip); parsed != nil {
				return parsed
			}
		}
	}

	// Check X-Real-IP header (single IP)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if parsed := net.ParseIP(strings.TrimSpace(xri)); parsed != nil {
			return parsed
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// If SplitHostPort fails, try parsing RemoteAddr directly
		if parsed := net.ParseIP(r.RemoteAddr); parsed != nil {
			return parsed
		}
		return nil
	}

	return net.ParseIP(host)
}

// ForUser creates a minimal variable context for a specific user
// Useful when you have user info but no HTTP request
func ForUser(user *iam.User) *Context {
	return &Context{
		Username:    user.Username,
		UserID:      user.UUID,
		CurrentTime: time.Now().UTC(),
	}
}

// ForUserSimple creates a context from basic user info (username and UUID)
func ForUserSimple(username string, userID uuid.UUID) *Context {
	return &Context{
		Username:    username,
		UserID:      userID,
		CurrentTime: time.Now().UTC(),
	}
}

// Minimal creates a context with only current time
// Useful for anonymous requests or when user info isn't available
func Minimal() *Context {
	return &Context{
		CurrentTime: time.Now().UTC(),
	}
}
