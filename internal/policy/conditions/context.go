package conditions

import (
	"time"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/policy/variables"
)

// Build constructs a condition evaluation context from pre-extracted request attributes.
// All HTTP-specific values must be extracted by the caller before calling Build,
// keeping the policy engine free of any dependency on *http.Request.
func Build(varCtx *variables.Context, secureTransport bool, contentLength int64) *Context {
	ctx := &Context{
		VarContext:      varCtx,
		SecureTransport: secureTransport,
		ContentLength:   contentLength,
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
		ctx.CurrentTime = time.Now().UTC()
	}

	return ctx
}
