package response

import (
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/sdk/s3types"
)

func TestWriteXMLResponse(t *testing.T) {
	type payload struct {
		XMLName xml.Name `xml:"Root"`
		Value   string   `xml:"Value"`
	}

	t.Run("writes XML with correct status and content-type", func(t *testing.T) {
		rec := httptest.NewRecorder()
		err := WriteXMLResponse(rec, http.StatusOK, payload{Value: "hello"})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/xml", rec.Header().Get("Content-Type"))
		body := rec.Body.String()
		assert.Contains(t, body, `<?xml`)
		assert.Contains(t, body, "hello")
	})

	t.Run("includes XML declaration header", func(t *testing.T) {
		rec := httptest.NewRecorder()
		_ = WriteXMLResponse(rec, http.StatusOK, payload{Value: "x"})
		assert.True(t, strings.HasPrefix(rec.Body.String(), "<?xml version="))
	})

	t.Run("non-200 status code is preserved", func(t *testing.T) {
		rec := httptest.NewRecorder()
		_ = WriteXMLResponse(rec, http.StatusNotFound, payload{Value: "missing"})
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestWriteErrorResponse(t *testing.T) {
	t.Run("writes S3 error XML with correct status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		err := WriteErrorResponse(rec, "req-abc", s3types.ErrCodeNoSuchBucket)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, "NoSuchBucket")
		assert.Contains(t, body, "req-abc")
	})

	t.Run("modifier overrides default message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		customErr := errors.New("custom error detail")
		err := WriteErrorResponse(rec, "req-xyz", s3types.ErrCodeInvalidBucketName, SetErrAsMessage(customErr))
		require.NoError(t, err)
		assert.Contains(t, rec.Body.String(), "custom error detail")
	})

	t.Run("nil modifier is safely skipped", func(t *testing.T) {
		rec := httptest.NewRecorder()
		err := WriteErrorResponse(rec, "req-nil", s3types.ErrCodeAccessDenied, nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("request ID is embedded in response", func(t *testing.T) {
		rec := httptest.NewRecorder()
		_ = WriteErrorResponse(rec, "my-request-id-123", s3types.ErrCodeInternalError)
		assert.Contains(t, rec.Body.String(), "my-request-id-123")
	})
}

func TestSetErrAsMessage(t *testing.T) {
	t.Run("sets message from error", func(t *testing.T) {
		mod := SetErrAsMessage(errors.New("something went wrong"))
		resp := s3types.ErrorResponse{Message: "original"}
		got := mod(resp)
		assert.Equal(t, "something went wrong", got.Message)
	})

	t.Run("nil error leaves message unchanged", func(t *testing.T) {
		mod := SetErrAsMessage(nil)
		resp := s3types.ErrorResponse{Message: "keep this"}
		got := mod(resp)
		assert.Equal(t, "keep this", got.Message)
	})
}
