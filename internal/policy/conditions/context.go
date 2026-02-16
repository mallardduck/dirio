package conditions

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/mallardduck/dirio/internal/policy/variables"
)

// FromRequest builds a condition evaluation context from an HTTP request and variable context
// The secureTransport parameter indicates whether the request was made over HTTPS
func FromRequest(r *http.Request, varCtx *variables.Context, secureTransport bool) *Context {
	ctx := &Context{
		VarContext:      varCtx,
		SecureTransport: secureTransport,
	}

	// Populate from VarContext if available
	if varCtx != nil {
		ctx.Username = varCtx.Username
		if varCtx.UserID != uuid.Nil {
			ctx.UserID = varCtx.UserID.String()
		}
		ctx.S3Prefix = varCtx.S3Prefix
		ctx.S3Delimiter = varCtx.S3Delimiter
		ctx.SourceIP = varCtx.SourceIP
		ctx.CurrentTime = varCtx.CurrentTime
		ctx.UserAgent = varCtx.UserAgent
	} else {
		// Fallback to extracting from request if no varCtx
		ctx.CurrentTime = time.Now().UTC()
	}

	// Extract ContentLength from request
	if r != nil {
		ctx.ContentLength = r.ContentLength
	}

	return ctx
}
