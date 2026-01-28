package auth

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/mallardduck/dirio/internal/consts"
)

func TestParseAuthorizationHeader(t *testing.T) {
	tests := []struct {
		name          string
		authHeader    string
		wantErr       error
		wantAccessKey string
	}{
		{
			name:       "empty header",
			authHeader: "",
			wantErr:    ErrMissingAuthHeader,
		},
		{
			name:       "invalid algorithm",
			authHeader: "AWS4-HMAC-SHA1 Credential=test",
			wantErr:    ErrInvalidAuthFormat,
		},
		{
			name:       "missing credential",
			authHeader: "AWS4-HMAC-SHA256 SignedHeaders=host, Signature=abc123",
			wantErr:    ErrMissingCredential,
		},
		{
			name:       "missing signature",
			authHeader: "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host",
			wantErr:    ErrMissingSignature,
		},
		{
			name:       "missing signed headers",
			authHeader: "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, Signature=abc123",
			wantErr:    ErrMissingSignedHeaders,
		},
		{
			name:          "valid header",
			authHeader:    "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=abc123",
			wantErr:       nil,
			wantAccessKey: "AKIAIOSFODNN7EXAMPLE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := ParseAuthorizationHeader(tt.authHeader)
			if err != tt.wantErr {
				t.Errorf("ParseAuthorizationHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && creds.AccessKey != tt.wantAccessKey {
				t.Errorf("ParseAuthorizationHeader() AccessKey = %v, want %v", creds.AccessKey, tt.wantAccessKey)
			}
		})
	}
}

func TestBuildCanonicalQueryString(t *testing.T) {
	tests := []struct {
		name   string
		values url.Values
		want   string
	}{
		{
			name:   "empty",
			values: url.Values{},
			want:   "",
		},
		{
			name: "single parameter",
			values: url.Values{
				"foo": {"bar"},
			},
			want: "foo=bar",
		},
		{
			name: "multiple parameters sorted",
			values: url.Values{
				"zebra": {"last"},
				"apple": {"first"},
			},
			want: "apple=first&zebra=last",
		},
		{
			name: "multiple values for same key",
			values: url.Values{
				"key": {"value2", "value1"},
			},
			want: "key=value1&key=value2",
		},
		{
			name: "special characters",
			values: url.Values{
				"key": {"value with spaces"},
			},
			want: "key=value+with+spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCanonicalQueryString(tt.values)
			if got != tt.want {
				t.Errorf("buildCanonicalQueryString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildCanonicalHeaders(t *testing.T) {
	tests := []struct {
		name          string
		headers       http.Header
		signedHeaders []string
		want          string
	}{
		{
			name:          "empty",
			headers:       http.Header{},
			signedHeaders: []string{},
			want:          "",
		},
		{
			name: "single header",
			headers: http.Header{
				"Host": {"example.com"},
			},
			signedHeaders: []string{"host"},
			want:          "host:example.com\n",
		},
		{
			name: "multiple headers sorted",
			headers: http.Header{
				"X-Amz-Date": {"20130524T000000Z"},
				"Host":       {"example.com"},
			},
			signedHeaders: []string{"host", "x-amz-date"},
			want:          "host:example.com\nx-amz-date:20130524T000000Z\n",
		},
		{
			name: "header not in signed list ignored",
			headers: http.Header{
				"Host":       {"example.com"},
				"User-Agent": {"MyClient/1.0"},
			},
			signedHeaders: []string{"host"},
			want:          "host:example.com\n",
		},
		{
			name: "header values trimmed",
			headers: http.Header{
				"Host": {"  example.com  "},
			},
			signedHeaders: []string{"host"},
			want:          "host:example.com\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For testing, pass empty host header since these tests use headers directly
			hostHeader := ""
			if tt.headers.Get("Host") != "" {
				hostHeader = tt.headers.Get("Host")
			}
			got := buildCanonicalHeaders(tt.headers, tt.signedHeaders, hostHeader)
			if got != tt.want {
				t.Errorf("buildCanonicalHeaders() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildCanonicalRequest(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Host", "examplebucket.s3.amazonaws.com")
	req.Header.Set("X-Amz-Date", "20130524T000000Z")

	signedHeaders := []string{"host", "x-amz-date"}
	payloadHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // SHA256 of empty string

	canonical := BuildCanonicalRequest(req, signedHeaders, payloadHash)

	// Verify it contains the expected components
	if !containsString(canonical, "GET") {
		t.Error("Canonical request should contain HTTP method")
	}
	if !containsString(canonical, "/test.txt") {
		t.Error("Canonical request should contain URI path")
	}
	if !containsString(canonical, "host;x-amz-date") {
		t.Error("Canonical request should contain signed headers")
	}
	if !containsString(canonical, payloadHash) {
		t.Error("Canonical request should contain payload hash")
	}
}

func TestBuildStringToSign(t *testing.T) {
	timestamp := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	region := consts.DefaultBucketLocation
	canonicalRequest := "GET\n/\n\nhost:examplebucket.s3.amazonaws.com\n\nhost\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	stringToSign := BuildStringToSign(timestamp, region, canonicalRequest)

	// Verify it contains the expected components
	if !containsString(stringToSign, "AWS4-HMAC-SHA256") {
		t.Error("String to sign should contain algorithm")
	}
	if !containsString(stringToSign, "20130524T000000Z") {
		t.Error("String to sign should contain timestamp")
	}
	if !containsString(stringToSign, "20130524/us-east-1/s3/aws4_request") {
		t.Error("String to sign should contain credential scope")
	}
}

func TestComputeSignature(t *testing.T) {
	secretKey := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	timestamp := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	region := consts.DefaultBucketLocation
	stringToSign := "AWS4-HMAC-SHA256\n20130524T000000Z\n20130524/us-east-1/s3/aws4_request\n3bfa292879f6447bbcda7001decf97f4a54dc650c8942174ae0a9121cf58ad04"

	signature := ComputeSignature(secretKey, timestamp, region, stringToSign)

	// Signature should be 64 hex characters
	if len(signature) != 64 {
		t.Errorf("Signature length = %d, want 64", len(signature))
	}

	// Signature should be deterministic
	signature2 := ComputeSignature(secretKey, timestamp, region, stringToSign)
	if signature != signature2 {
		t.Error("ComputeSignature should be deterministic")
	}
}

func TestVerifySignature(t *testing.T) {
	secretKey := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	timestamp := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	region := consts.DefaultBucketLocation

	// Create a test request
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Host", "examplebucket.s3.amazonaws.com")
	req.Header.Set("X-Amz-Date", timestamp.Format(iso8601TimeFormat))
	req.Header.Set("X-Amz-Content-Sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	// Build the signature
	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	canonicalRequest := BuildCanonicalRequest(req, signedHeaders, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	stringToSign := BuildStringToSign(timestamp, region, canonicalRequest)
	signature := ComputeSignature(secretKey, timestamp, region, stringToSign)

	// Set Authorization header with the computed signature (headers must be sorted)
	authHeader := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=" + signature
	req.Header.Set("Authorization", authHeader)

	// Verify the signature
	err := VerifySignature(req, secretKey)
	if err != nil {
		t.Errorf("VerifySignature() error = %v, want nil", err)
	}

	// Test with wrong signature
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=wrongsignature")
	err = VerifySignature(req, secretKey)
	if err != ErrSignatureMismatch {
		t.Errorf("VerifySignature() with wrong signature should return ErrSignatureMismatch, got %v", err)
	}
}

func TestGetAccessKey(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		want       string
		wantErr    error
	}{
		{
			name:       "valid header",
			authHeader: "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=abc",
			want:       "AKIAIOSFODNN7EXAMPLE",
			wantErr:    nil,
		},
		{
			name:       "missing header",
			authHeader: "",
			want:       "",
			wantErr:    ErrMissingAuthHeader,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "https://example.com", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			got, err := GetAccessKey(req)
			if err != tt.wantErr {
				t.Errorf("GetAccessKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetAccessKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[0:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsString(s[1:len(s)-1], substr)))
}
