package auth

// AWS Signature Version 4 chunked transfer encoding decoder.
//
// AWS uses a special chunked encoding format when streaming large uploads:
// https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-streaming.html
//
// Format:
// {chunk-size-hex};chunk-signature={signature}\r\n
// {chunk-data}\r\n
// ...
// 0;chunk-signature={final-signature}\r\n
// \r\n
//
// Chunk Signature Verification:
// Each chunk's signature is calculated as HMAC-SHA256 over a string-to-sign:
//
//	"AWS4-HMAC-SHA256-PAYLOAD\n" +
//	timestamp + "\n" +
//	credential_scope + "\n" +
//	previous_signature + "\n" +
//	hash_of_empty_string + "\n" +
//	hash_of_chunk_data
//
// The first chunk uses the request signature as the previous signature.
// Subsequent chunks use the previous chunk's signature.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mallardduck/dirio/internal/consts"
)

const (
	// chunkSignatureAlgorithm is the algorithm identifier for chunk signatures
	chunkSignatureAlgorithm = "AWS4-HMAC-SHA256-PAYLOAD"

	// emptyPayloadHash is the SHA256 hash of an empty string
	emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

var (
	// ErrInvalidChunkFormat is returned when chunk format is invalid
	ErrInvalidChunkFormat = errors.New("invalid chunk format")
	// ErrChunkSizeMismatch is returned when chunk size doesn't match data
	ErrChunkSizeMismatch = errors.New("chunk size mismatch")
	// ErrChunkSignatureMismatch is returned when chunk signature verification fails
	ErrChunkSignatureMismatch = errors.New("chunk signature mismatch")
)

// ChunkSignatureVerifier holds parameters for verifying chunk signatures
type ChunkSignatureVerifier struct {
	SecretKey         string    // Secret key for HMAC
	Timestamp         time.Time // Request timestamp from X-Amz-Date
	Region            string    // AWS region from credential scope
	PreviousSignature string    // Signature of previous chunk (or request signature for first chunk)
}

// ChunkedReader decodes AWS SigV4 chunked transfer encoding
type ChunkedReader struct {
	reader            *bufio.Reader
	chunkSize         int64 // remaining bytes in current chunk
	finished          bool
	totalRead         int64
	verifier          *ChunkSignatureVerifier // signature verifier (nil = no verification)
	chunkData         *bytes.Buffer           // buffer to accumulate chunk data for signature verification
	currentChunkSig   string                  // signature from current chunk header
	currentChunkStart int64                   // position where current chunk started (for debugging)
}

// NewChunkedReader creates a new chunked encoding decoder without signature verification
func NewChunkedReader(r io.Reader) *ChunkedReader {
	return &ChunkedReader{
		reader:    bufio.NewReader(r),
		chunkSize: 0,
		finished:  false,
		verifier:  nil, // No signature verification by default
	}
}

// WithSignatureVerification enables chunk signature verification
func (cr *ChunkedReader) WithSignatureVerification(verifier *ChunkSignatureVerifier) *ChunkedReader {
	cr.verifier = verifier
	cr.chunkData = &bytes.Buffer{}
	return cr
}

// Read implements io.Reader, decoding chunked data
func (cr *ChunkedReader) Read(p []byte) (int, error) {
	if cr.finished {
		return 0, io.EOF
	}

	// If current chunk is exhausted, read next chunk header
	if cr.chunkSize == 0 {
		if err := cr.readChunkHeader(); err != nil {
			return 0, err
		}

		// Check if this was the final chunk (size 0)
		if cr.chunkSize == 0 {
			cr.finished = true
			return 0, io.EOF
		}
	}

	// Read up to the remaining chunk size or buffer size
	toRead := min(int64(len(p)), cr.chunkSize)

	n, err := io.ReadFull(cr.reader, p[:toRead])
	cr.chunkSize -= int64(n)
	cr.totalRead += int64(n)

	// If verifying signatures, accumulate chunk data
	if cr.verifier != nil && n > 0 {
		cr.chunkData.Write(p[:n])
	}

	// If we've finished this chunk, verify signature and consume trailing \r\n
	if cr.chunkSize == 0 && err == nil {
		// Verify chunk signature if enabled
		if cr.verifier != nil {
			if verifyErr := cr.verifyCurrentChunk(); verifyErr != nil {
				return n, verifyErr
			}
			// Clear buffer for next chunk
			cr.chunkData.Reset()
		}

		if err := cr.consumeTrailingCRLF(); err != nil {
			return n, err
		}
	}

	// Handle errors
	if err != nil {
		if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
			// If we hit EOF but expected more data in chunk, that's an error
			if cr.chunkSize > 0 {
				return n, fmt.Errorf("%w: unexpected EOF in chunk data", ErrChunkSizeMismatch)
			}
		} else {
			return n, err
		}
	}

	return n, nil
}

// readChunkHeader reads and parses a chunk header
// Format: {size-hex};chunk-signature={signature}\r\n
func (cr *ChunkedReader) readChunkHeader() error {
	// Read line containing chunk size and signature
	line, err := cr.reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("%w: unexpected EOF reading chunk header", ErrInvalidChunkFormat)
		}
		return fmt.Errorf("failed to read chunk header: %w", err)
	}

	// Remove trailing \r\n or \n
	line = strings.TrimSuffix(line, "\r\n")
	line = strings.TrimSuffix(line, "\n")

	// Parse: size;chunk-signature=xxx
	parts := strings.SplitN(line, ";", 2)
	if len(parts) < 1 {
		return fmt.Errorf("%w: missing chunk size", ErrInvalidChunkFormat)
	}

	// Parse hex chunk size
	sizeStr := strings.TrimSpace(parts[0])
	size, err := strconv.ParseInt(sizeStr, 16, 64)
	if err != nil {
		return fmt.Errorf("%w: invalid chunk size '%s': %v", ErrInvalidChunkFormat, sizeStr, err)
	}

	// Parse chunk signature if present
	cr.currentChunkSig = ""
	if len(parts) == 2 {
		sigPart := strings.TrimSpace(parts[1])
		if after, ok := strings.CutPrefix(sigPart, "chunk-signature="); ok {
			cr.currentChunkSig = after
		}
	}

	cr.chunkSize = size
	cr.currentChunkStart = cr.totalRead
	return nil
}

// verifyCurrentChunk verifies the signature of the current chunk
func (cr *ChunkedReader) verifyCurrentChunk() error {
	if cr.verifier == nil {
		return nil // No verification requested
	}

	// Check if chunk had a signature
	if cr.currentChunkSig == "" {
		return fmt.Errorf("%w: chunk missing signature", ErrInvalidChunkFormat)
	}

	// Compute expected signature
	expectedSig := cr.computeChunkSignature(cr.chunkData.Bytes())

	// Compare signatures
	if cr.currentChunkSig != expectedSig {
		// Safely truncate signatures for error message
		gotSig := cr.currentChunkSig
		if len(gotSig) > 16 {
			gotSig = gotSig[:16] + "..."
		}
		expSig := expectedSig
		if len(expSig) > 16 {
			expSig = expSig[:16] + "..."
		}
		return fmt.Errorf("%w: chunk at offset %d (got %s, expected %s)",
			ErrChunkSignatureMismatch, cr.currentChunkStart, gotSig, expSig)
	}

	// Update previous signature for next chunk
	cr.verifier.PreviousSignature = cr.currentChunkSig

	return nil
}

// computeChunkSignature computes the expected signature for a chunk
// According to AWS spec:
// String-to-Sign = "AWS4-HMAC-SHA256-PAYLOAD\n" +
//
//	timestamp + "\n" +
//	credential_scope + "\n" +
//	previous_signature + "\n" +
//	hash_of_empty_string + "\n" +
//	hash_of_chunk_data
func (cr *ChunkedReader) computeChunkSignature(chunkData []byte) string {
	// Hash the chunk data
	chunkHash := sha256.Sum256(chunkData)
	chunkHashHex := hex.EncodeToString(chunkHash[:])

	// Build credential scope: YYYYMMDD/region/s3/aws4_request
	dateStamp := cr.verifier.Timestamp.Format(shortDateFormat)
	credentialScope := fmt.Sprintf("%s/%s/%s/%s", dateStamp, cr.verifier.Region, serviceName, requestType)

	// Build string to sign
	stringToSign := strings.Join([]string{
		chunkSignatureAlgorithm,
		cr.verifier.Timestamp.Format(iso8601TimeFormat),
		credentialScope,
		cr.verifier.PreviousSignature,
		emptyPayloadHash,
		chunkHashHex,
	}, "\n")

	// Compute signature using the same signing key derivation as request signatures
	signature := ComputeSignature(cr.verifier.SecretKey, cr.verifier.Timestamp, cr.verifier.Region, stringToSign)

	return signature
}

// consumeTrailingCRLF consumes the \r\n after chunk data
func (cr *ChunkedReader) consumeTrailingCRLF() error {
	// Read the trailing \r\n
	buf := make([]byte, 2)
	n, err := io.ReadFull(cr.reader, buf)
	if err != nil {
		if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
			return fmt.Errorf("%w: expected CRLF after chunk data", ErrInvalidChunkFormat)
		}
		return fmt.Errorf("failed to read trailing CRLF: %w", err)
	}

	// Verify it's actually \r\n (or tolerate just \n)
	if n == 2 && !bytes.Equal(buf, []byte("\r\n")) {
		// Some implementations might use just \n
		if buf[0] == '\n' {
			// Push back the second byte
			err := cr.reader.UnreadByte()
			if err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("%w: expected CRLF, got %q", ErrInvalidChunkFormat, buf)
	}

	return nil
}

// IsChunkedEncoding checks if the request uses AWS SigV4 chunked encoding
func IsChunkedEncoding(contentSHA256 string) bool {
	return contentSHA256 == consts.ContentSHA256Streaming
}
