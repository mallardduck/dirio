package auth

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mallardduck/dirio/internal/consts"
)

func TestChunkedReader_SimpleChunk(t *testing.T) {
	// Create chunked data: single chunk with "hello world"
	// Format: {size-hex};chunk-signature={sig}\r\n{data}\r\n0;chunk-signature={sig}\r\n\r\n
	chunkedData := "b;chunk-signature=1234567890abcdef\r\nhello world\r\n0;chunk-signature=final1234567890\r\n\r\n"

	reader := NewChunkedReader(strings.NewReader(chunkedData))
	result, err := io.ReadAll(reader)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "hello world"
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
}

func TestChunkedReader_MultipleChunks(t *testing.T) {
	// Create chunked data with multiple chunks
	chunkedData := "5;chunk-signature=abc123\r\nhello\r\n1;chunk-signature=def456\r\n \r\n5;chunk-signature=ghi789\r\nworld\r\n0;chunk-signature=final\r\n\r\n"

	reader := NewChunkedReader(strings.NewReader(chunkedData))
	result, err := io.ReadAll(reader)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "hello world"
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
}

func TestChunkedReader_EmptyChunk(t *testing.T) {
	// Test with just the final chunk (no data)
	chunkedData := "0;chunk-signature=final1234567890\r\n\r\n"

	reader := NewChunkedReader(strings.NewReader(chunkedData))
	result, err := io.ReadAll(reader)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d bytes: %q", len(result), string(result))
	}
}

func TestChunkedReader_BufferedReads(t *testing.T) {
	// Test reading in small increments
	chunkedData := "d;chunk-signature=abc\r\ntest content!\r\n0;chunk-signature=final\r\n\r\n"

	reader := NewChunkedReader(strings.NewReader(chunkedData))

	// Read in 4-byte chunks
	var result bytes.Buffer
	buf := make([]byte, 4)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			result.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}

	expected := "test content!"
	if result.String() != expected {
		t.Errorf("Expected %q, got %q", expected, result.String())
	}
}

func TestChunkedReader_LargeChunk(t *testing.T) {
	// Create a large chunk (1024 bytes)
	data := strings.Repeat("x", 1024)
	chunkedData := "400;chunk-signature=large\r\n" + data + "\r\n0;chunk-signature=final\r\n\r\n"

	reader := NewChunkedReader(strings.NewReader(chunkedData))
	result, err := io.ReadAll(reader)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if string(result) != data {
		t.Errorf("Data mismatch. Expected %d bytes, got %d bytes", len(data), len(result))
	}
}

func TestChunkedReader_InvalidFormat(t *testing.T) {
	tests := []struct {
		name        string
		chunkedData string
	}{
		{
			name:        "Missing chunk size",
			chunkedData: "chunk-signature=abc\r\ndata\r\n",
		},
		{
			name:        "Invalid hex size",
			chunkedData: "xyz;chunk-signature=abc\r\ndata\r\n",
		},
		{
			name:        "Missing trailing CRLF",
			chunkedData: "4;chunk-signature=abc\r\ndata0;chunk-signature=final\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewChunkedReader(strings.NewReader(tt.chunkedData))
			_, err := io.ReadAll(reader)

			if err == nil {
				t.Error("Expected error for invalid format, got nil")
			}
		})
	}
}

func TestIsChunkedEncoding(t *testing.T) {
	tests := []struct {
		name          string
		contentSHA256 string
		expected      bool
	}{
		{
			name:          "Streaming chunked encoding",
			contentSHA256: consts.ContentSHA256Streaming,
			expected:      true,
		},
		{
			name:          "Normal SHA256 hash",
			contentSHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			expected:      false,
		},
		{
			name:          "Unsigned payload",
			contentSHA256: consts.ContentSHA256Unsigned,
			expected:      false,
		},
		{
			name:          "Empty string",
			contentSHA256: "",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsChunkedEncoding(tt.contentSHA256)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestChunkedReader_RealWorldFormat tests with actual AWS SDK format
func TestChunkedReader_RealWorldFormat(t *testing.T) {
	// This matches the format from the bug report
	// Note: "tagging test content" is 20 bytes = 0x14, not 0x15
	chunkedData := "14;chunk-signature=27e683aa022df0a0d27ac3e1e28f24e86e861a7f9f83ffd0c64a16f66d0f998f\r\ntagging test content\r\n0;chunk-signature=48193c2564d7c6caee2d81b1234567890abcdef\r\n\r\n"

	reader := NewChunkedReader(strings.NewReader(chunkedData))
	result, err := io.ReadAll(reader)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "tagging test content"
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
}

// TestChunkedReader_SignatureVerification tests chunk signature verification
func TestChunkedReader_SignatureVerification(t *testing.T) {
	// Setup verification parameters
	secretKey := "testsecret"
	timestamp := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	region := "us-east-1"

	// Compute request signature for chunked encoding
	// This would be the signature from the Authorization header
	dateStamp := timestamp.Format("20060102")
	credentialScope := dateStamp + "/" + region + "/s3/aws4_request"

	// For this test, simulate a request signature
	// In reality, this comes from the Authorization header
	stringToSign := "AWS4-HMAC-SHA256\n" +
		timestamp.Format("20060102T150405Z") + "\n" +
		credentialScope + "\n" +
		consts.ContentSHA256Streaming
	requestSignature := ComputeSignature(secretKey, timestamp, region, stringToSign)

	// Prepare test data
	chunkData := []byte("test data")

	// Compute chunk signature using the actual algorithm
	reader := NewChunkedReader(strings.NewReader(""))
	verifier := &ChunkSignatureVerifier{
		SecretKey:         secretKey,
		Timestamp:         timestamp,
		Region:            region,
		PreviousSignature: requestSignature,
	}
	reader.verifier = verifier
	reader.chunkData = &bytes.Buffer{}
	reader.chunkData.Write(chunkData)

	expectedSig := reader.computeChunkSignature(chunkData)

	// Create chunked data with valid signature
	chunkSize := len(chunkData)
	chunkedData := fmt.Sprintf("%x;chunk-signature=%s\r\n%s\r\n0;chunk-signature=",
		chunkSize, expectedSig, string(chunkData))

	// Compute final chunk signature (empty data)
	reader.verifier.PreviousSignature = expectedSig
	reader.chunkData.Reset()
	finalSig := reader.computeChunkSignature([]byte{})
	chunkedData += finalSig + "\r\n\r\n"

	// Test with verification enabled
	testReader := NewChunkedReader(strings.NewReader(chunkedData))
	testReader.WithSignatureVerification(&ChunkSignatureVerifier{
		SecretKey:         secretKey,
		Timestamp:         timestamp,
		Region:            region,
		PreviousSignature: requestSignature,
	})

	result, err := io.ReadAll(testReader)
	if err != nil {
		t.Fatalf("Expected no error with valid signatures, got: %v", err)
	}

	if string(result) != string(chunkData) {
		t.Errorf("Expected %q, got %q", string(chunkData), string(result))
	}
}

// TestChunkedReader_SignatureVerificationFailure tests signature mismatch detection
func TestChunkedReader_SignatureVerificationFailure(t *testing.T) {
	secretKey := "testsecret"
	timestamp := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	region := "us-east-1"
	requestSignature := "badrequestsignature1234567890abcdef1234567890abcdef1234567890abcdef"

	// Create chunked data with INVALID signature
	chunkedData := "9;chunk-signature=invalidsignature1234567890abcdef\r\ntest data\r\n0;chunk-signature=anotherbadsig\r\n\r\n"

	reader := NewChunkedReader(strings.NewReader(chunkedData))
	reader.WithSignatureVerification(&ChunkSignatureVerifier{
		SecretKey:         secretKey,
		Timestamp:         timestamp,
		Region:            region,
		PreviousSignature: requestSignature,
	})

	_, err := io.ReadAll(reader)
	if err == nil {
		t.Fatal("Expected signature mismatch error, got nil")
	}

	if !strings.Contains(err.Error(), "chunk signature mismatch") {
		t.Errorf("Expected 'chunk signature mismatch' error, got: %v", err)
	}
}

// TestChunkedReader_MultipleChunksWithVerification tests multiple chunks with signature verification
func TestChunkedReader_MultipleChunksWithVerification(t *testing.T) {
	secretKey := "testsecret"
	timestamp := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	region := "us-east-1"

	// Compute request signature
	dateStamp := timestamp.Format("20060102")
	credentialScope := dateStamp + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" +
		timestamp.Format("20060102T150405Z") + "\n" +
		credentialScope + "\n" +
		consts.ContentSHA256Streaming
	requestSignature := ComputeSignature(secretKey, timestamp, region, stringToSign)

	// Prepare multiple chunks
	chunk1Data := []byte("hello")
	chunk2Data := []byte(" ")
	chunk3Data := []byte("world")

	// Compute signatures for each chunk
	reader := NewChunkedReader(strings.NewReader(""))
	verifier := &ChunkSignatureVerifier{
		SecretKey:         secretKey,
		Timestamp:         timestamp,
		Region:            region,
		PreviousSignature: requestSignature,
	}
	reader.verifier = verifier
	reader.chunkData = &bytes.Buffer{}

	// Chunk 1
	reader.chunkData.Write(chunk1Data)
	sig1 := reader.computeChunkSignature(chunk1Data)

	// Chunk 2 (previous signature is chunk 1's signature)
	reader.verifier.PreviousSignature = sig1
	reader.chunkData.Reset()
	reader.chunkData.Write(chunk2Data)
	sig2 := reader.computeChunkSignature(chunk2Data)

	// Chunk 3 (previous signature is chunk 2's signature)
	reader.verifier.PreviousSignature = sig2
	reader.chunkData.Reset()
	reader.chunkData.Write(chunk3Data)
	sig3 := reader.computeChunkSignature(chunk3Data)

	// Final chunk (empty)
	reader.verifier.PreviousSignature = sig3
	reader.chunkData.Reset()
	finalSig := reader.computeChunkSignature([]byte{})

	// Build chunked data with all valid signatures
	chunkedData := fmt.Sprintf(
		"%x;chunk-signature=%s\r\n%s\r\n"+
			"%x;chunk-signature=%s\r\n%s\r\n"+
			"%x;chunk-signature=%s\r\n%s\r\n"+
			"0;chunk-signature=%s\r\n\r\n",
		len(chunk1Data), sig1, string(chunk1Data),
		len(chunk2Data), sig2, string(chunk2Data),
		len(chunk3Data), sig3, string(chunk3Data),
		finalSig,
	)

	// Test with verification
	testReader := NewChunkedReader(strings.NewReader(chunkedData))
	testReader.WithSignatureVerification(&ChunkSignatureVerifier{
		SecretKey:         secretKey,
		Timestamp:         timestamp,
		Region:            region,
		PreviousSignature: requestSignature,
	})

	result, err := io.ReadAll(testReader)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "hello world"
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
}

// TestChunkedReader_NoVerificationStillWorks ensures backward compatibility
func TestChunkedReader_NoVerificationStillWorks(t *testing.T) {
	// Should work without signature verification (backward compatible)
	chunkedData := "5;chunk-signature=anysignature\r\nhello\r\n0;chunk-signature=final\r\n\r\n"

	reader := NewChunkedReader(strings.NewReader(chunkedData))
	// Don't enable verification

	result, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Expected no error without verification, got: %v", err)
	}

	if string(result) != "hello" {
		t.Errorf("Expected 'hello', got %q", string(result))
	}
}
