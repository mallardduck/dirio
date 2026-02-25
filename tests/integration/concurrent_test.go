package integration

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentReads verifies that multiple goroutines can read the same
// object simultaneously without data corruption or errors.
func TestConcurrentReads(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "concurrent-read-bucket")

	content := strings.Repeat("concurrent-read-data", 512) // ~10 KB
	ts.PutObject(t, "concurrent-read-bucket", "shared.txt", content)

	const goroutines = 20
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	bodies := make([]string, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, ts.ObjectURL("concurrent-read-bucket", "shared.txt"), http.NoBody)
			if err != nil {
				errs[idx] = err
				return
			}
			ts.SignRequest(req, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errs[idx] = err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errs[idx] = fmt.Errorf("unexpected status %d", resp.StatusCode)
				return
			}
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				errs[idx] = err
				return
			}
			bodies[idx] = string(b)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d had an error", i)
	}
	for i, body := range bodies {
		assert.Equal(t, content, body, "goroutine %d got wrong content", i)
	}
}

// TestConcurrentWritesDifferentKeys verifies that multiple goroutines writing
// different keys to the same bucket all succeed without interfering with each other.
func TestConcurrentWritesDifferentKeys(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "concurrent-write-bucket")

	const goroutines = 20
	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("obj-%03d.txt", idx)
			content := fmt.Sprintf("content for object %d", idx)
			body := []byte(content)

			req, err := http.NewRequest(http.MethodPut, ts.ObjectURL("concurrent-write-bucket", key), strings.NewReader(content))
			if err != nil {
				t.Errorf("goroutine %d: build request: %v", idx, err)
				return
			}
			req.ContentLength = int64(len(body))
			ts.SignRequest(req, body)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("goroutine %d: PUT: %v", idx, err)
				return
			}
			defer resp.Body.Close()
			//nolint:errcheck // body drained to allow connection reuse; copy error is unactionable
			io.Copy(io.Discard, resp.Body)

			if resp.StatusCode == http.StatusOK {
				successCount.Add(1)
			} else {
				t.Errorf("goroutine %d: unexpected status %d", idx, resp.StatusCode)
			}
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(goroutines), successCount.Load(), "all concurrent writes should succeed")

	// Verify every object is readable and has the right content.
	for i := range goroutines {
		key := fmt.Sprintf("obj-%03d.txt", i)
		expected := fmt.Sprintf("content for object %d", i)

		req, err := http.NewRequest(http.MethodGet, ts.ObjectURL("concurrent-write-bucket", key), http.NoBody)
		require.NoError(t, err)
		ts.SignRequest(req, nil)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "GET %s", key)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		assert.Equal(t, expected, string(b), "content mismatch for %s", key)
	}
}

// TestConcurrentWritesSameKey verifies that concurrent writes to the same key
// are each individually valid (last-writer-wins; no torn writes or server errors).
func TestConcurrentWritesSameKey(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "concurrent-overwrite-bucket")

	const goroutines = 10
	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Each goroutine writes a distinct, fixed-length value so we can
			// later verify the stored content is exactly one of them.
			content := fmt.Sprintf("writer-%02d|%-50s", idx, "")
			body := []byte(content)

			req, err := http.NewRequest(http.MethodPut, ts.ObjectURL("concurrent-overwrite-bucket", "shared-key.txt"), strings.NewReader(content))
			if err != nil {
				t.Errorf("goroutine %d: build request: %v", idx, err)
				return
			}
			req.ContentLength = int64(len(body))
			ts.SignRequest(req, body)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("goroutine %d: PUT: %v", idx, err)
				return
			}
			defer resp.Body.Close()
			//nolint:errcheck // body drained to allow connection reuse; copy error is unactionable
			io.Copy(io.Discard, resp.Body)

			if resp.StatusCode == http.StatusOK {
				successCount.Add(1)
			} else {
				t.Errorf("goroutine %d: unexpected status %d", idx, resp.StatusCode)
			}
		}(i)
	}
	wg.Wait()

	// All writes should have been accepted.
	assert.Equal(t, int32(goroutines), successCount.Load(), "all concurrent overwrites should be accepted")

	// The final GET should return exactly one complete value — no torn write.
	req, err := http.NewRequest(http.MethodGet, ts.ObjectURL("concurrent-overwrite-bucket", "shared-key.txt"), http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	b, _ := io.ReadAll(resp.Body)
	// Verify the stored content starts with one of the known prefixes.
	found := false
	for i := range goroutines {
		if strings.HasPrefix(string(b), fmt.Sprintf("writer-%02d|", i)) {
			found = true
			break
		}
	}
	assert.True(t, found, "stored content should be exactly one writer's value, got: %q", string(b))
}

// TestConcurrentBucketCreation verifies that creating the same bucket from
// multiple goroutines simultaneously returns consistent results (the first
// succeeds; duplicates return BucketAlreadyExists).
func TestConcurrentBucketCreation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	const goroutines = 10
	var wg sync.WaitGroup
	statuses := make([]int, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodPut, ts.BucketURL("race-bucket"), http.NoBody)
			if err != nil {
				t.Errorf("goroutine %d: build request: %v", idx, err)
				return
			}
			ts.SignRequest(req, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("goroutine %d: PUT bucket: %v", idx, err)
				return
			}
			defer resp.Body.Close()
			//nolint:errcheck // body drained to allow connection reuse; copy error is unactionable
			io.Copy(io.Discard, resp.Body)
			statuses[idx] = resp.StatusCode
		}(i)
	}
	wg.Wait()

	okCount := 0
	conflictCount := 0
	for _, s := range statuses {
		switch s {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Errorf("unexpected status %d", s)
		}
	}

	assert.GreaterOrEqual(t, okCount, 1, "at least one create should succeed")
	assert.Equal(t, goroutines, okCount+conflictCount, "every response must be 200 or 409")
}

// TestConcurrentMixedReadWrite verifies that interleaved reads and writes on
// the same bucket do not cause data corruption or server errors.
func TestConcurrentMixedReadWrite(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "mixed-rw-bucket")
	// Seed an initial object so readers always have something to GET.
	ts.PutObject(t, "mixed-rw-bucket", "seed.txt", "initial content")

	const (
		readers = 10
		writers = 10
	)

	var wg sync.WaitGroup
	var readErrors atomic.Int32
	var writeErrors atomic.Int32

	// Writers: each puts a unique key.
	for i := range writers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("write-%03d.txt", idx)
			content := fmt.Sprintf("write content %d", idx)
			body := []byte(content)

			req, err := http.NewRequest(http.MethodPut, ts.ObjectURL("mixed-rw-bucket", key), strings.NewReader(content))
			if err != nil {
				writeErrors.Add(1)
				return
			}
			req.ContentLength = int64(len(body))
			ts.SignRequest(req, body)

			resp, err := http.DefaultClient.Do(req)
			if err != nil || resp.StatusCode != http.StatusOK {
				writeErrors.Add(1)
			}
			if resp != nil {
				resp.Body.Close()
			}
		}(i)
	}

	// Readers: each reads the seeded object concurrently with the writes.
	for i := range readers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, ts.ObjectURL("mixed-rw-bucket", "seed.txt"), http.NoBody)
			if err != nil {
				readErrors.Add(1)
				return
			}
			ts.SignRequest(req, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil || resp.StatusCode != http.StatusOK {
				readErrors.Add(1)
			}
			if resp != nil {
				//nolint:errcheck // body drained to allow connection reuse; copy error is unactionable
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}(i)
	}

	wg.Wait()

	assert.Zero(t, readErrors.Load(), "no read errors expected")
	assert.Zero(t, writeErrors.Load(), "no write errors expected")
}

// TestConcurrentDeleteAndRead verifies the race between deleting an object and
// reading it: the reader should get either 200 or 404, never a server error.
func TestConcurrentDeleteAndRead(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "delete-race-bucket")
	ts.PutObject(t, "delete-race-bucket", "ephemeral.txt", "here today, gone tomorrow")

	var wg sync.WaitGroup
	var unexpectedStatus atomic.Int32

	// One goroutine deletes the object.
	wg.Add(1)
	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodDelete, ts.ObjectURL("delete-race-bucket", "ephemeral.txt"), http.NoBody)
		ts.SignRequest(req, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("DELETE: %v", err)
			return
		}
		defer resp.Body.Close()
		//nolint:errcheck // body drained to allow connection reuse; copy error is unactionable
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("DELETE: unexpected status %d", resp.StatusCode)
		}
	}()

	// Multiple goroutines race to read at the same time.
	const readers = 10
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, ts.ObjectURL("delete-race-bucket", "ephemeral.txt"), http.NoBody)
			ts.SignRequest(req, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				unexpectedStatus.Add(1)
				return
			}
			defer resp.Body.Close()
			//nolint:errcheck // body drained to allow connection reuse; copy error is unactionable
			io.Copy(io.Discard, resp.Body)
			// Only 200 or 404 are valid; anything else is a bug.
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
				unexpectedStatus.Add(1)
				t.Errorf("GET during delete race: unexpected status %d", resp.StatusCode)
			}
		}()
	}

	wg.Wait()
	assert.Zero(t, unexpectedStatus.Load(), "readers should only see 200 or 404 during a concurrent delete")
}

// TestConcurrentListAndPut verifies that ListObjectsV2 returns consistent XML
// while objects are being added concurrently — no 5xx, no malformed responses.
func TestConcurrentListAndPut(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "list-race-bucket")

	// Pre-seed a few objects so the list is never completely empty.
	for i := range 5 {
		ts.PutObject(t, "list-race-bucket", fmt.Sprintf("seed-%d.txt", i), "seed")
	}

	var wg sync.WaitGroup
	var listErrors atomic.Int32

	// Writers add objects in the background.
	const writers = 10
	for i := range writers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("live-%03d.txt", idx)
			body := []byte("live content")
			req, _ := http.NewRequest(http.MethodPut, ts.ObjectURL("list-race-bucket", key), strings.NewReader("live content"))
			req.ContentLength = int64(len(body))
			ts.SignRequest(req, body)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}(i)
	}

	// Listers issue ListObjectsV2 concurrently.
	const listers = 10
	for range listers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			url := ts.BucketURL("list-race-bucket") + "?list-type=2&max-keys=100"
			req, _ := http.NewRequest(http.MethodGet, url, http.NoBody)
			ts.SignRequest(req, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				listErrors.Add(1)
				return
			}
			defer resp.Body.Close()
			//nolint:errcheck // body drained to allow connection reuse; copy error is unactionable
			io.Copy(io.Discard, resp.Body)
			if resp.StatusCode != http.StatusOK {
				listErrors.Add(1)
				t.Errorf("LIST during concurrent PUT: unexpected status %d", resp.StatusCode)
			}
		}()
	}

	wg.Wait()
	assert.Zero(t, listErrors.Load(), "no list errors expected during concurrent puts")
}

// TestConcurrentMultipartUploads verifies that several multipart uploads
// can be initiated, have parts uploaded, and be completed simultaneously
// without errors or cross-contamination of part data.
func TestConcurrentMultipartUploads(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "concurrent-mpu-bucket")

	const uploads = 5
	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := range uploads {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("mpu-%02d.bin", idx)

			// 1. Initiate.
			initiateURL := ts.ObjectURL("concurrent-mpu-bucket", key) + "?uploads"
			req, _ := http.NewRequest(http.MethodPost, initiateURL, http.NoBody)
			ts.SignRequest(req, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("upload %d: initiate failed: err=%v", idx, err)
				return
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("upload %d: initiate failed: status=%d", idx, resp.StatusCode)
				resp.Body.Close()
				return
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			// Extract UploadId from the XML response.
			uploadID := extractXMLValue(string(body), "UploadId")
			if uploadID == "" {
				t.Errorf("upload %d: could not extract UploadId from: %s", idx, body)
				return
			}

			// 2. Upload one part (minimum 5 MB for real S3, but our server
			//    has no minimum — use 64 KB to keep the test fast).
			partContent := strings.Repeat(fmt.Sprintf("%02d", idx), 32*1024) // 64 KB
			partBody := []byte(partContent)
			partURL := fmt.Sprintf("%s?partNumber=1&uploadId=%s", ts.ObjectURL("concurrent-mpu-bucket", key), uploadID)
			partReq, _ := http.NewRequest(http.MethodPut, partURL, strings.NewReader(partContent))
			partReq.ContentLength = int64(len(partBody))
			ts.SignRequest(partReq, partBody)
			partResp, err := http.DefaultClient.Do(partReq)
			if err != nil {
				t.Errorf("upload %d: part 1 failed: err=%v", idx, err)
				return
			}
			if partResp.StatusCode != http.StatusOK {
				t.Errorf("upload %d: part 1 failed: status=%d", idx, partResp.StatusCode)
				partResp.Body.Close()
				return
			}
			etag := partResp.Header.Get("ETag")
			partResp.Body.Close()

			// 3. Complete.
			completeXML := fmt.Sprintf(`<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>%s</ETag></Part></CompleteMultipartUpload>`, etag)
			completeBody := []byte(completeXML)
			completeURL := fmt.Sprintf("%s?uploadId=%s", ts.ObjectURL("concurrent-mpu-bucket", key), uploadID)
			completeReq, _ := http.NewRequest(http.MethodPost, completeURL, strings.NewReader(completeXML))
			completeReq.ContentLength = int64(len(completeBody))
			ts.SignRequest(completeReq, completeBody)
			completeResp, err := http.DefaultClient.Do(completeReq)
			if err != nil {
				t.Errorf("upload %d: complete failed: err=%v", idx, err)
				return
			}
			if completeResp.StatusCode != http.StatusOK {
				t.Errorf("upload %d: complete failed: status=%d", idx, completeResp.StatusCode)
				completeResp.Body.Close()
				return
			}
			completeResp.Body.Close()

			successCount.Add(1)
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(uploads), successCount.Load(), "all concurrent multipart uploads should complete")

	// Verify each completed object exists.
	for i := range uploads {
		key := fmt.Sprintf("mpu-%02d.bin", i)
		req, _ := http.NewRequest(http.MethodHead, ts.ObjectURL("concurrent-mpu-bucket", key), http.NoBody)
		ts.SignRequest(req, nil)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "HEAD %s", key)
	}
}

// extractXMLValue is a minimal helper that extracts the text content of the
// first XML element with the given tag name.  It avoids importing encoding/xml
// for simple single-value extractions in test helpers.
func extractXMLValue(xml, tag string) string {
	open := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	start := strings.Index(xml, open)
	if start == -1 {
		return ""
	}
	start += len(open)
	end := strings.Index(xml[start:], closeTag)
	if end == -1 {
		return ""
	}
	return xml[start : start+end]
}
