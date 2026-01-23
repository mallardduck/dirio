package urlbuilder

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBucketURL_WithCanonicalDomain(t *testing.T) {
	builder := New("s3.example.com")
	req := &http.Request{
		Host: "localhost:9000",
	}

	url := builder.BucketURL(req, "mybucket")
	assert.Equal(t, "https://s3.example.com/mybucket", url)
}

func TestBucketURL_WithoutCanonicalDomain(t *testing.T) {
	builder := New("")
	req := &http.Request{
		Host: "localhost:9000",
	}

	url := builder.BucketURL(req, "mybucket")
	assert.Equal(t, "http://localhost:9000/mybucket", url)
}

func TestObjectURL_WithCanonicalDomain(t *testing.T) {
	builder := New("s3.example.com")
	req := &http.Request{
		Host: "localhost:9000",
	}

	url := builder.ObjectURL(req, "mybucket", "path/to/file.txt")
	assert.Equal(t, "https://s3.example.com/mybucket/path/to/file.txt", url)
}

func TestObjectURL_WithoutCanonicalDomain(t *testing.T) {
	builder := New("")
	req := &http.Request{
		Host: "localhost:9000",
	}

	url := builder.ObjectURL(req, "mybucket", "path/to/file.txt")
	assert.Equal(t, "http://localhost:9000/mybucket/path/to/file.txt", url)
}

func TestDetectScheme_CanonicalDomain(t *testing.T) {
	builder := New("s3.example.com")
	req := &http.Request{
		Header: http.Header{},
	}

	scheme := builder.detectScheme(req)
	assert.Equal(t, "https", scheme)
}

func TestDetectScheme_XForwardedProto(t *testing.T) {
	builder := New("")
	req := &http.Request{
		Header: http.Header{
			"X-Forwarded-Proto": []string{"https"},
		},
	}

	scheme := builder.detectScheme(req)
	assert.Equal(t, "https", scheme)
}

func TestDetectScheme_XForwardedProto_HTTP(t *testing.T) {
	builder := New("")
	req := &http.Request{
		Header: http.Header{
			"X-Forwarded-Proto": []string{"HTTP"},
		},
	}

	scheme := builder.detectScheme(req)
	assert.Equal(t, "http", scheme)
}

func TestDetectScheme_TLS(t *testing.T) {
	builder := New("")
	req := &http.Request{
		Header: http.Header{},
		TLS:    &tls.ConnectionState{},
	}

	scheme := builder.detectScheme(req)
	assert.Equal(t, "https", scheme)
}

func TestDetectScheme_DefaultHTTP(t *testing.T) {
	builder := New("")
	req := &http.Request{
		Header: http.Header{},
	}

	scheme := builder.detectScheme(req)
	assert.Equal(t, "http", scheme)
}

func TestDetectHost_CanonicalDomain(t *testing.T) {
	builder := New("s3.example.com")
	req := &http.Request{
		Host: "localhost:9000",
	}

	host := builder.detectHost(req)
	assert.Equal(t, "s3.example.com", host)
}

func TestDetectHost_RequestHost(t *testing.T) {
	builder := New("")
	req := &http.Request{
		Host: "dirio-s3.local:9000",
	}

	host := builder.detectHost(req)
	assert.Equal(t, "dirio-s3.local:9000", host)
}

func TestDetectHost_MDNSHostname(t *testing.T) {
	builder := New("")
	req := &http.Request{
		Host: "dirio-s3.local:9000",
	}

	host := builder.detectHost(req)
	assert.Equal(t, "dirio-s3.local:9000", host)
}

func TestBucketURL_WithMDNS(t *testing.T) {
	builder := New("")
	req := &http.Request{
		Host:   "dirio-s3.local:9000",
		Header: http.Header{},
	}

	url := builder.BucketURL(req, "test-bucket")
	assert.Equal(t, "http://dirio-s3.local:9000/test-bucket", url)
}
