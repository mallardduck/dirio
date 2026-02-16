package middleware

import (
	"bytes"
	"io"
	"net/http"
	"regexp"

	"github.com/mallardduck/dirio/internal/consts"
)

// ChunkedDecoderFactory creates chunked readers - injected to avoid import cycles
type ChunkedDecoderFactory func(io.Reader) io.Reader

// ChunkedEncoding is a middleware that decodes AWS SigV4 chunked transfer encoding
// if present in the request body. It should run AFTER authentication middleware.
//
// This middleware:
// - Checks multiple indicators for AWS SigV4 chunked encoding
// - If detected, wraps r.Body with a decoder provided by the factory
// - The handler receives the decoded body transparently
// - Applies to all operations that accept request bodies (PutObject, UploadPart, etc.)
func ChunkedEncoding(decoderFactory ChunkedDecoderFactory) func(http.Handler) http.Handler {
	// Pre-compile regex for detecting chunked encoding pattern
	// Pattern: hex digits, semicolon, "chunk-signature="
	chunkPattern := regexp.MustCompile(`^[0-9a-fA-F]+;chunk-signature=`)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentSHA256 := r.Header.Get(consts.HeaderContentSHA256)
			decodedContentLength := r.Header.Get("X-Amz-Decoded-Content-Length")
			contentEncoding := r.Header.Get("Content-Encoding")

			needsDecoding := false

			// Method 1: Check X-Amz-Content-Sha256 header (standard AWS SDK method)
			if contentSHA256 == consts.ContentSHA256Streaming {
				needsDecoding = true
			}

			// Method 2: Check for X-Amz-Decoded-Content-Length header
			// This header is present when AWS clients use chunked encoding
			if !needsDecoding && decodedContentLength != "" {
				needsDecoding = true
			}

			// Method 3: Check Content-Encoding header (alternative marker)
			if !needsDecoding && contentEncoding == "aws-chunked" {
				needsDecoding = true
			}

			// Method 4: Content sniffing - peek at request body to detect chunked format
			// Only peek if we have a body and haven't detected chunked encoding yet
			if !needsDecoding && r.Body != nil && r.Method != "GET" && r.Method != "HEAD" && r.Method != "DELETE" {
				// Peek at first 100 bytes to detect format
				peekSize := 100
				peeked := make([]byte, peekSize)
				n, _ := io.ReadFull(r.Body, peeked)

				// Check if it matches AWS SigV4 chunked encoding pattern
				if n > 0 && chunkPattern.Match(peeked[:n]) {
					needsDecoding = true
				}

				// Reconstruct body with peeked bytes
				r.Body = &readCloser{
					Reader: io.MultiReader(bytes.NewReader(peeked[:n]), r.Body),
					closer: r.Body,
				}
			}

			if needsDecoding {
				// Decode the chunked body using the provided factory
				decodedReader := decoderFactory(r.Body)

				// Replace request body with decoded reader
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
