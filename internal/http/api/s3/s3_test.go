package s3

import (
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpresponse "github.com/mallardduck/dirio/internal/http/response"
	"github.com/mallardduck/dirio/pkg/s3types"
)

func TestWriteXMLResponse(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		data           any
		wantStatusCode int
		wantHeader     string
		wantContains   []string
		wantErr        bool
	}{
		{
			name:           "successful simple response",
			statusCode:     http.StatusOK,
			data:           s3types.ListBucketsResponse{Owner: s3types.Owner{ID: "test-id", DisplayName: "test-name"}},
			wantStatusCode: http.StatusOK,
			wantHeader:     "application/xml",
			wantContains:   []string{xml.Header, "<Owner>", "<ID>test-id</ID>", "<DisplayName>test-name</DisplayName>"},
		},
		{
			name:           "successful with buckets",
			statusCode:     http.StatusOK,
			data:           s3types.ListBucketsResponse{Buckets: []s3types.Bucket{{Name: "bucket1"}, {Name: "bucket2"}}},
			wantStatusCode: http.StatusOK,
			wantHeader:     "application/xml",
			wantContains:   []string{xml.Header, "<Bucket>", "<Name>bucket1</Name>", "<Name>bucket2</Name>"},
		},
		{
			name:           "created status code",
			statusCode:     http.StatusCreated,
			data:           s3types.ListBucketsResponse{Owner: s3types.Owner{ID: "owner"}},
			wantStatusCode: http.StatusCreated,
			wantHeader:     "application/xml",
			wantContains:   []string{xml.Header, "<Owner>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			err := WriteXMLResponse(w, tt.statusCode, tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("WriteXMLResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if w.Code != tt.wantStatusCode {
				t.Errorf("WriteXMLResponse() status code = %v, want %v", w.Code, tt.wantStatusCode)
			}

			if got := w.Header().Get("Content-Type"); got != tt.wantHeader {
				t.Errorf("WriteXMLResponse() Content-Type = %v, want %v", got, tt.wantHeader)
			}

			body := w.Body.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(body, want) {
					t.Errorf("WriteXMLResponse() body does not contain %q\nGot: %s", want, body)
				}
			}
		})
	}
}

func TestWriteXMLResponse_LargeResponse(t *testing.T) {
	// Create a response with many buckets to test large response handling
	buckets := make([]s3types.Bucket, 0, 15000)
	// Add enough buckets to create > 10MB response
	// Each bucket with a reasonably long name should help reach the threshold
	longName := strings.Repeat("a", 1000)
	for range 15000 {
		buckets = append(buckets, s3types.Bucket{Name: longName})
	}

	data := s3types.ListBucketsResponse{Buckets: buckets}
	w := httptest.NewRecorder()

	err := WriteXMLResponse(w, http.StatusOK, data)
	if err != nil {
		t.Errorf("WriteXMLResponse() unexpected error for large response: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("WriteXMLResponse() status code = %v, want %v", w.Code, http.StatusOK)
	}

	// Verify the response was actually large
	if w.Body.Len() <= 10*1024*1024 {
		t.Logf("Response size: %d bytes (expected > 10MB)", w.Body.Len())
	}
}

func TestWriteXMLResponse_InvalidData(t *testing.T) {
	// Create data that cannot be marshaled to XML
	type InvalidStruct struct {
		Channel chan int `xml:"channel"` // channels cannot be marshaled
	}

	w := httptest.NewRecorder()
	err := WriteXMLResponse(w, http.StatusOK, InvalidStruct{Channel: make(chan int)})

	if err == nil {
		t.Error("WriteXMLResponse() expected error for unmarshalable data, got nil")
	}
}

func TestWriteErrorResponse(t *testing.T) {
	tests := []struct {
		name           string
		requestID      string
		errCode        s3types.ErrorCode
		err            error
		wantStatusCode int
		wantContains   []string
	}{
		{
			name:           "no such bucket error",
			requestID:      "req-123",
			errCode:        s3types.ErrCodeNoSuchBucket,
			err:            nil,
			wantStatusCode: http.StatusNotFound,
			wantContains: []string{
				xml.Header,
				"<Error>",
				"<Code>NoSuchBucket</Code>",
				"<Message>The specified bucket does not exist.</Message>",
				"<RequestId>req-123</RequestId>",
			},
		},
		{
			name:           "no such bucket with custom error message",
			requestID:      "req-456",
			errCode:        s3types.ErrCodeNoSuchBucket,
			err:            errors.New("bucket 'test-bucket' not found"),
			wantStatusCode: http.StatusNotFound,
			wantContains: []string{
				xml.Header,
				"<Error>",
				"<Code>NoSuchBucket</Code>",
				"<Message>bucket &#39;test-bucket&#39; not found</Message>",
				"<RequestId>req-456</RequestId>",
			},
		},
		{
			name:           "internal error",
			requestID:      "req-789",
			errCode:        s3types.ErrCodeInternalError,
			err:            errors.New("database connection failed"),
			wantStatusCode: http.StatusInternalServerError,
			wantContains: []string{
				xml.Header,
				"<Error>",
				"<Code>InternalError</Code>",
				"<Message>database connection failed</Message>",
				"<RequestId>req-789</RequestId>",
			},
		},
		{
			name:           "bucket already exists",
			requestID:      "req-999",
			errCode:        s3types.ErrCodeBucketAlreadyExists,
			err:            nil,
			wantStatusCode: http.StatusConflict,
			wantContains: []string{
				xml.Header,
				"<Error>",
				"<Code>BucketAlreadyExists</Code>",
				"<Message>The requested bucket name is not available.</Message>",
				"<RequestId>req-999</RequestId>",
			},
		},
		{
			name:           "access denied",
			requestID:      "req-111",
			errCode:        s3types.ErrCodeAccessDenied,
			err:            nil,
			wantStatusCode: http.StatusForbidden,
			wantContains: []string{
				xml.Header,
				"<Error>",
				"<Code>AccessDenied</Code>",
				"<Message>Access Denied</Message>",
				"<RequestId>req-111</RequestId>",
			},
		},
		{
			name:           "invalid bucket name",
			requestID:      "req-222",
			errCode:        s3types.ErrCodeInvalidBucketName,
			err:            errors.New("bucket name contains invalid characters"),
			wantStatusCode: http.StatusBadRequest,
			wantContains: []string{
				xml.Header,
				"<Error>",
				"<Code>InvalidBucketName</Code>",
				"<Message>bucket name contains invalid characters</Message>",
				"<RequestId>req-222</RequestId>",
			},
		},
		{
			name:           "empty request ID",
			requestID:      "",
			errCode:        s3types.ErrCodeNoSuchKey,
			err:            nil,
			wantStatusCode: http.StatusNotFound,
			wantContains: []string{
				xml.Header,
				"<Error>",
				"<Code>NoSuchKey</Code>",
				"<Message>The specified key does not exist.</Message>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			err := WriteErrorResponse(w, tt.requestID, tt.errCode, httpresponse.SetErrAsMessage(tt.err))

			if err != nil {
				t.Errorf("WriteErrorResponse() unexpected error = %v", err)
				return
			}

			if w.Code != tt.wantStatusCode {
				t.Errorf("WriteErrorResponse() status code = %v, want %v", w.Code, tt.wantStatusCode)
			}

			if got := w.Header().Get("Content-Type"); got != "application/xml" {
				t.Errorf("WriteErrorResponse() Content-Type = %v, want application/xml", got)
			}

			body := w.Body.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(body, want) {
					t.Errorf("WriteErrorResponse() body does not contain %q\nGot: %s", want, body)
				}
			}
		})
	}
}

func TestWriteErrorResponse_XMLStructure(t *testing.T) {
	// Test that the response can be unmarshaled back to the struct
	w := httptest.NewRecorder()
	requestID := "test-request-id"
	testErr := errors.New("test error message")

	err := WriteErrorResponse(w, requestID, s3types.ErrCodeNoSuchBucket, httpresponse.SetErrAsMessage(testErr))
	if err != nil {
		t.Fatalf("WriteErrorResponse() unexpected error = %v", err)
	}

	// Parse the XML response
	var response s3types.ErrorResponse
	if err := xml.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	// Verify the structure
	if response.Code != "NoSuchBucket" {
		t.Errorf("ErrorResponse.Code = %v, want NoSuchBucket", response.Code)
	}
	if response.Message != "test error message" {
		t.Errorf("ErrorResponse.Message = %v, want 'test error message'", response.Message)
	}
	if response.RequestID != requestID {
		t.Errorf("ErrorResponse.RequestID = %v, want %v", response.RequestID, requestID)
	}
}
