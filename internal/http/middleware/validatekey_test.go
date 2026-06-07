package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mallardduck/dirio/internal/http/response"
	"github.com/mallardduck/dirio/sdk/s3types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── ValidateS3BucketName ─────────────────────────────────────────────────────

func TestValidateS3BucketName(t *testing.T) {
	tests := []struct {
		name    string
		bucket  string
		wantErr bool
		errMsg  string
	}{
		{"valid simple", "my-bucket", false, ""},
		{"valid with dots", "my.bucket", false, ""},
		{"valid with numbers", "bucket123", false, ""},
		{"valid min length", "abc", false, ""},
		{"valid max length", strings.Repeat("a", 63), false, ""},
		{"empty", "", true, "between 3 and 63"},
		{"too short", "ab", true, "between 3 and 63"},
		{"too long", strings.Repeat("a", 64), true, "between 3 and 63"},
		{"starts with hyphen", "-bucket", true, "start and end"},
		{"ends with hyphen", "bucket-", true, "start and end"},
		{"starts with dot", ".bucket", true, "start and end"},
		{"ends with dot", "bucket.", true, "start and end"},
		{"uppercase", "MyBucket", true, "start and end"},
		{"consecutive dots", "my..bucket", true, "consecutive dots"},
		{"IP address", "192.168.1.1", true, "IP address"},
		{"underscore", "my_bucket", true, "start and end"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateS3BucketName(tc.bucket)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ── ValidateS3Key ─────────────────────────────────────────────────────────────

func TestValidateS3Key(t *testing.T) {
	assert.NoError(t, ValidateS3Key("path/to/object.txt"))
	assert.NoError(t, ValidateS3Key("unicode-日本語"))
	assert.NoError(t, ValidateS3Key("with\ttab"))
	assert.NoError(t, ValidateS3Key("with\nnewline"))
	assert.NoError(t, ValidateS3Key("with\rCR"))

	tests := []struct {
		name    string
		key     string
		wantErr bool
		errMsg  string
	}{
		{"empty", "", true, "cannot be empty"},
		{"leading slash", "/bad", true, "not start with '/'"},
		{"too long", strings.Repeat("a", 1025), true, "1024 bytes"},
		{"NUL control char", "key\x00bad", true, "control characters"},
		{"BEL control char", "key\x07bad", true, "control characters"},
		{"SOH control char", "key\x01bad", true, "control characters"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateS3Key(tc.key)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ── ValidateS3BucketNameMiddleware ────────────────────────────────────────────

func TestValidateS3BucketNameMiddleware(t *testing.T) {
	noopWriter := response.XMLErrorWriter(func(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode, mods ...response.ErrorModifier) error {
		w.WriteHeader(http.StatusBadRequest)
		return nil
	})

	t.Run("valid bucket passes through to next", func(t *testing.T) {
		var reached bool
		mw := ValidateS3BucketNameMiddleware(
			func(r *http.Request) string { return "valid-bucket" },
			noopWriter,
		)
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		rec := httptest.NewRecorder()
		mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true })).ServeHTTP(rec, req)
		assert.True(t, reached)
	})

	t.Run("invalid bucket short-circuits with error", func(t *testing.T) {
		var reached bool
		mw := ValidateS3BucketNameMiddleware(
			func(r *http.Request) string { return "ab" }, // too short
			noopWriter,
		)
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		rec := httptest.NewRecorder()
		mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true })).ServeHTTP(rec, req)
		assert.False(t, reached)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// ── ValidateS3KeyMiddleware ───────────────────────────────────────────────────

func TestValidateS3KeyMiddleware(t *testing.T) {
	noopWriter := response.XMLErrorWriter(func(w http.ResponseWriter, requestID string, errCode s3types.ErrorCode, mods ...response.ErrorModifier) error {
		w.WriteHeader(http.StatusBadRequest)
		return nil
	})

	t.Run("valid key passes through", func(t *testing.T) {
		var reached bool
		mw := ValidateS3KeyMiddleware(
			func(r *http.Request) string { return "valid/key.txt" },
			noopWriter,
		)
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		rec := httptest.NewRecorder()
		mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true })).ServeHTTP(rec, req)
		assert.True(t, reached)
	})

	t.Run("invalid key short-circuits with error", func(t *testing.T) {
		var reached bool
		mw := ValidateS3KeyMiddleware(
			func(r *http.Request) string { return "/leading-slash" },
			noopWriter,
		)
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		rec := httptest.NewRecorder()
		mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true })).ServeHTTP(rec, req)
		assert.False(t, reached)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
