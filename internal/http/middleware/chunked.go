package middleware

import (
	"io"
	"net/http"

	"github.com/mallardduck/dirio/internal/consts"
)

// ChunkedDecoderFactory creates chunked readers - injected to avoid import cycles
type ChunkedDecoderFactory func(io.Reader) io.Reader

// ChunkedEncoding is a middleware that decodes AWS SigV4 chunked transfer encoding
// if present in the request body. It should run AFTER authentication middleware.
//
// This middleware:
// - Checks X-Amz-Content-Sha256 header for chunked encoding marker
// - If detected, wraps r.Body with a decoder provided by the factory
// - The handler receives the decoded body transparently
// - Applies to all operations that accept request bodies (PutObject, UploadPart, etc.)
func ChunkedEncoding(decoderFactory ChunkedDecoderFactory) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if chunked encoding is present
			contentSHA256 := r.Header.Get(consts.HeaderContentSHA256)

			if contentSHA256 == consts.ContentSHA256Streaming {
				// Decode the chunked body using the provided factory
				decodedReader := decoderFactory(r.Body)

				// Replace request body with decoded reader
				// The original body's Close will still be called when needed
				r.Body = &readCloser{
					Reader: decodedReader,
					closer: r.Body,
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// readCloser combines an io.Reader with the Close method from another ReadCloser
type readCloser struct {
	io.Reader
	closer io.Closer
}

func (rc *readCloser) Close() error {
	return rc.closer.Close()
}
