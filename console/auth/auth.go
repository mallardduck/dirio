// Package auth provides authentication primitives for the DirIO admin console.
// It defines the AdminAuth interface (satisfied by an adapter in the server wiring)
// and the Session type for HMAC-signed, cookie-based login sessions.
package auth

import "context"

// AdminAuth validates that a given access-key/secret-key pair belongs to an
// admin user. The implementation lives in the server wiring layer and wraps
// internal/http/auth.Authenticator so this package stays free of internal/
// imports.
type AdminAuth interface {
	AuthenticateAdmin(ctx context.Context, accessKey, secretKey string) bool
}
