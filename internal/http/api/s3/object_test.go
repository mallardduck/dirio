package s3

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	svcs3 "github.com/mallardduck/dirio/internal/service/s3"
)

// testReader wraps bytes.Reader to satisfy ObjectReader (io.ReadCloser + io.ReaderAt).
type testReader struct{ *bytes.Reader }

func (testReader) Close() error { return nil }

func newTestObject(content string) *svcs3.GetObjectResponse {
	data := []byte(content)
	return &svcs3.GetObjectResponse{
		Content:      testReader{bytes.NewReader(data)},
		ContentType:  "text/plain",
		Size:         int64(len(data)),
		ETag:         `"test-etag"`,
		LastModified: time.Unix(0, 0),
	}
}

// ============================================================================
// parseRangeHeader
// ============================================================================

func TestParseRangeHeader(t *testing.T) {
	const fileSize = 100

	tests := []struct {
		name      string
		header    string
		wantStart int64
		wantEnd   int64
		wantErr   bool
	}{
		{
			name:      "standard range",
			header:    "bytes=0-49",
			wantStart: 0,
			wantEnd:   49,
		},
		{
			name:      "middle range",
			header:    "bytes=25-74",
			wantStart: 25,
			wantEnd:   74,
		},
		{
			name:      "open-ended range",
			header:    "bytes=50-",
			wantStart: 50,
			wantEnd:   99,
		},
		{
			name:      "suffix range",
			header:    "bytes=-10",
			wantStart: 90,
			wantEnd:   99,
		},
		{
			name:      "suffix larger than file clamps to file size",
			header:    "bytes=-200",
			wantStart: 0,
			wantEnd:   99,
		},
		{
			name:      "end beyond file clamps to last byte",
			header:    "bytes=0-999",
			wantStart: 0,
			wantEnd:   99,
		},
		{
			name:    "missing bytes= prefix",
			header:  "0-49",
			wantErr: true,
		},
		{
			name:    "invalid format no dash",
			header:  "bytes=0",
			wantErr: true,
		},
		{
			name:    "start out of range",
			header:  "bytes=100-199",
			wantErr: true,
		},
		{
			name:    "start greater than end",
			header:  "bytes=50-10",
			wantErr: true,
		},
		{
			name:    "non-numeric start",
			header:  "bytes=abc-49",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := parseRangeHeader(tt.header, fileSize)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseRangeHeader() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if start != tt.wantStart {
				t.Errorf("start = %d, want %d", start, tt.wantStart)
			}
			if end != tt.wantEnd {
				t.Errorf("end = %d, want %d", end, tt.wantEnd)
			}
		})
	}
}

// ============================================================================
// serveRangeResponse
// ============================================================================

func TestServeRangeResponse(t *testing.T) {
	const body = "0123456789abcdefghij" // 20 bytes

	tests := []struct {
		name          string
		rangeHeader   string
		wantStatus    int
		wantBody      string
		wantRangeHdr  string
		wantLengthHdr string
	}{
		{
			name:          "first 5 bytes",
			rangeHeader:   "bytes=0-4",
			wantStatus:    http.StatusPartialContent,
			wantBody:      "01234",
			wantRangeHdr:  "bytes 0-4/20",
			wantLengthHdr: "5",
		},
		{
			name:          "middle bytes",
			rangeHeader:   "bytes=5-9",
			wantStatus:    http.StatusPartialContent,
			wantBody:      "56789",
			wantRangeHdr:  "bytes 5-9/20",
			wantLengthHdr: "5",
		},
		{
			name:          "open-ended range from byte 15",
			rangeHeader:   "bytes=15-",
			wantStatus:    http.StatusPartialContent,
			wantBody:      "fghij",
			wantRangeHdr:  "bytes 15-19/20",
			wantLengthHdr: "5",
		},
		{
			name:          "suffix range last 3 bytes",
			rangeHeader:   "bytes=-3",
			wantStatus:    http.StatusPartialContent,
			wantBody:      "hij",
			wantRangeHdr:  "bytes 17-19/20",
			wantLengthHdr: "3",
		},
		{
			name:        "invalid range returns 416",
			rangeHeader: "bytes=50-99",
			wantStatus:  http.StatusRequestedRangeNotSatisfiable,
		},
	}

	h := &HTTPHandler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := newTestObject(body)
			r := httptest.NewRequest(http.MethodGet, "/bucket/key", http.NoBody)
			w := httptest.NewRecorder()

			h.serveRangeResponse(w, r, obj, "bucket", "key", tt.rangeHeader)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusPartialContent {
				return
			}
			if got := w.Body.String(); got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
			if got := w.Header().Get("Content-Range"); got != tt.wantRangeHdr {
				t.Errorf("Content-Range = %q, want %q", got, tt.wantRangeHdr)
			}
			if got := w.Header().Get("Content-Length"); got != tt.wantLengthHdr {
				t.Errorf("Content-Length = %q, want %q", got, tt.wantLengthHdr)
			}
		})
	}
}

// ============================================================================
// serveFullResponse
// ============================================================================

func TestServeFullResponse(t *testing.T) {
	const body = "hello world"

	h := &HTTPHandler{}
	obj := newTestObject(body)
	r := httptest.NewRequest(http.MethodGet, "/bucket/key", http.NoBody)
	w := httptest.NewRecorder()

	h.serveFullResponse(w, r, obj, "bucket", "key")

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
	if got := w.Header().Get("Content-Length"); got != strconv.Itoa(len(body)) {
		t.Errorf("Content-Length = %q, want %q", got, strconv.Itoa(len(body)))
	}
}

// ============================================================================
// serveMultiRangeResponse (via serveRangeResponse)
// ============================================================================

func TestServeMultiRangeResponse(t *testing.T) {
	// body: "0123456789abcdefghij" (20 bytes, 0-indexed)
	// bytes=1-3  → "123"
	// bytes=10-12 → "abc"
	const body = "0123456789abcdefghij"

	h := &HTTPHandler{}
	obj := newTestObject(body)
	r := httptest.NewRequest(http.MethodGet, "/bucket/key", http.NoBody)
	w := httptest.NewRecorder()

	h.serveRangeResponse(w, r, obj, "bucket", "key", "bytes=1-3, 10-12")

	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "multipart/byteranges; boundary=") {
		t.Fatalf("Content-Type = %q, want multipart/byteranges", ct)
	}
	boundary := strings.TrimPrefix(ct, "multipart/byteranges; boundary=")

	type part struct {
		contentRange string
		contentType  string
		body         string
	}
	var parts []part
	mr := multipart.NewReader(w.Body, boundary)
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading multipart: %v", err)
		}
		data, _ := io.ReadAll(p)
		parts = append(parts, part{
			contentRange: p.Header.Get("Content-Range"),
			contentType:  p.Header.Get("Content-Type"),
			body:         string(data),
		})
	}

	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2", len(parts))
	}

	if parts[0].contentRange != "bytes 1-3/20" {
		t.Errorf("part[0] Content-Range = %q, want %q", parts[0].contentRange, "bytes 1-3/20")
	}
	if parts[0].body != "123" {
		t.Errorf("part[0] body = %q, want %q", parts[0].body, "123")
	}
	if parts[0].contentType != "text/plain" {
		t.Errorf("part[0] Content-Type = %q, want text/plain", parts[0].contentType)
	}

	if parts[1].contentRange != "bytes 10-12/20" {
		t.Errorf("part[1] Content-Range = %q, want %q", parts[1].contentRange, "bytes 10-12/20")
	}
	if parts[1].body != "abc" {
		t.Errorf("part[1] body = %q, want %q", parts[1].body, "abc")
	}
	if parts[1].contentType != "text/plain" {
		t.Errorf("part[1] Content-Type = %q, want text/plain", parts[1].contentType)
	}
}
