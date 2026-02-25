package integration

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Large-file tests
// ---------------------------------------------------------------------------

// TestLargeObjectIntegrity uploads a 10 MB random binary and verifies the
// downloaded content is bit-for-bit identical via MD5.
func TestLargeObjectIntegrity(t *testing.T) {
	const size = 10 * 1024 * 1024 // 10 MB

	ts := NewTestServer(t)
	defer ts.Cleanup()
	ts.CreateBucket(t, "large-object-bucket")

	data := make([]byte, size)
	_, err := rand.Read(data)
	require.NoError(t, err)

	wantMD5 := md5hex(data)

	// Upload
	req, err := http.NewRequest(http.MethodPut, ts.ObjectURL("large-object-bucket", "big.bin"), bytes.NewReader(data))
	require.NoError(t, err)
	req.ContentLength = int64(size)
	ts.SignRequest(req, data)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "PUT 10 MB object")

	// Download and verify
	getReq, err := http.NewRequest(http.MethodGet, ts.ObjectURL("large-object-bucket", "big.bin"), http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(getReq, nil)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode)
	assert.Equal(t, fmt.Sprintf("%d", size), getResp.Header.Get("Content-Length"))

	downloaded, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	assert.Len(t, downloaded, size, "downloaded size mismatch")
	assert.Equal(t, wantMD5, md5hex(downloaded), "content integrity check failed")
}

// TestLargeObjectRangeReads uploads a 5 MB object and verifies that several
// non-overlapping range requests each return the exact expected slice.
func TestLargeObjectRangeReads(t *testing.T) {
	const size = 5 * 1024 * 1024 // 5 MB

	ts := NewTestServer(t)
	defer ts.Cleanup()
	ts.CreateBucket(t, "range-bucket")

	data := make([]byte, size)
	_, err := rand.Read(data)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, ts.ObjectURL("range-bucket", "range.bin"), bytes.NewReader(data))
	require.NoError(t, err)
	req.ContentLength = int64(size)
	ts.SignRequest(req, data)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	ranges := []struct {
		name       string
		header     string
		start, end int64
	}{
		{"first-64KB", "bytes=0-65535", 0, 65535},
		{"middle-1MB", fmt.Sprintf("bytes=%d-%d", 1*1024*1024, 2*1024*1024-1), int64(1 * 1024 * 1024), int64(2*1024*1024 - 1)},
		{"last-64KB", fmt.Sprintf("bytes=%d-%d", int64(size-65536), int64(size-1)), int64(size - 65536), int64(size - 1)},
		{"suffix-256KB", fmt.Sprintf("bytes=-%d", 256*1024), int64(size - 256*1024), int64(size - 1)},
	}

	for _, tc := range ranges {
		t.Run(tc.name, func(t *testing.T) {
			rangeReq, err := http.NewRequest(http.MethodGet, ts.ObjectURL("range-bucket", "range.bin"), http.NoBody)
			require.NoError(t, err)
			rangeReq.Header.Set("Range", tc.header)
			ts.SignRequest(rangeReq, nil)

			rResp, err := http.DefaultClient.Do(rangeReq)
			require.NoError(t, err)
			defer rResp.Body.Close()

			assert.Equal(t, http.StatusPartialContent, rResp.StatusCode, "expected 206 for range %s", tc.header)

			got, err := io.ReadAll(rResp.Body)
			require.NoError(t, err)

			want := data[tc.start : tc.end+1]
			assert.Len(t, got, len(want), "range %s: length mismatch", tc.name)
			assert.Equal(t, md5hex(want), md5hex(got), "range %s: content mismatch", tc.name)
		})
	}
}

// TestMultipartUploadLargeObject uploads a 15 MB object via multipart with
// 5 MB parts (three parts) and verifies the assembled content is intact.
func TestMultipartUploadLargeObject(t *testing.T) {
	const (
		partSize  = 5 * 1024 * 1024 // 5 MB per part
		numParts  = 3
		totalSize = partSize * numParts // 15 MB
	)

	ts := NewTestServer(t)
	defer ts.Cleanup()
	ts.CreateBucket(t, "mpu-large-bucket")

	// Build the full payload up front so we can verify integrity.
	payload := make([]byte, totalSize)
	_, err := rand.Read(payload)
	require.NoError(t, err)

	// 1. Initiate multipart upload.
	initURL := ts.ObjectURL("mpu-large-bucket", "large-mpu.bin") + "?uploads"
	initReq, err := http.NewRequest(http.MethodPost, initURL, http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(initReq, nil)

	initResp, err := http.DefaultClient.Do(initReq)
	require.NoError(t, err)
	initBody, _ := io.ReadAll(initResp.Body)
	initResp.Body.Close()
	require.Equal(t, http.StatusOK, initResp.StatusCode, "initiate multipart upload")

	uploadID := extractXMLValue(string(initBody), "UploadId")
	require.NotEmpty(t, uploadID, "UploadId missing from initiate response")

	// 2. Upload each part.
	type uploadedPart struct {
		number int
		etag   string
	}
	parts := make([]uploadedPart, numParts)

	for i := range numParts {
		partNum := i + 1
		start := i * partSize
		end := start + partSize
		chunk := payload[start:end]

		partURL := fmt.Sprintf("%s?partNumber=%d&uploadId=%s",
			ts.ObjectURL("mpu-large-bucket", "large-mpu.bin"), partNum, uploadID)

		partReq, err := http.NewRequest(http.MethodPut, partURL, bytes.NewReader(chunk))
		require.NoError(t, err)
		partReq.ContentLength = int64(len(chunk))
		ts.SignRequest(partReq, chunk)

		partResp, err := http.DefaultClient.Do(partReq)
		require.NoError(t, err)
		partResp.Body.Close()
		require.Equal(t, http.StatusOK, partResp.StatusCode, "upload part %d", partNum)

		etag := partResp.Header.Get("ETag")
		require.NotEmpty(t, etag, "part %d: missing ETag", partNum)
		parts[i] = uploadedPart{partNum, etag}
	}

	// 3. Complete multipart upload.
	var sb strings.Builder
	sb.WriteString("<CompleteMultipartUpload>")
	for _, p := range parts {
		fmt.Fprintf(&sb, "<Part><PartNumber>%d</PartNumber><ETag>%s</ETag></Part>", p.number, p.etag)
	}
	sb.WriteString("</CompleteMultipartUpload>")
	completeBody := []byte(sb.String())

	completeURL := fmt.Sprintf("%s?uploadId=%s", ts.ObjectURL("mpu-large-bucket", "large-mpu.bin"), uploadID)
	completeReq, err := http.NewRequest(http.MethodPost, completeURL, bytes.NewReader(completeBody))
	require.NoError(t, err)
	completeReq.ContentLength = int64(len(completeBody))
	ts.SignRequest(completeReq, completeBody)

	completeResp, err := http.DefaultClient.Do(completeReq)
	require.NoError(t, err)
	DrainAndClose(completeResp)
	require.Equal(t, http.StatusOK, completeResp.StatusCode, "complete multipart upload")

	// 4. Download and verify.
	getReq, err := http.NewRequest(http.MethodGet, ts.ObjectURL("mpu-large-bucket", "large-mpu.bin"), http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(getReq, nil)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode)

	downloaded, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	assert.Len(t, downloaded, totalSize, "assembled object size mismatch")
	assert.Equal(t, md5hex(payload), md5hex(downloaded), "assembled content integrity check failed")
}

// ---------------------------------------------------------------------------
// Many-small-files tests
// ---------------------------------------------------------------------------

// TestManySmallObjectsUploadAndList uploads 250 small objects and then pages
// through the listing using a small page size (50), verifying every object
// appears exactly once and has the correct ETag.
func TestManySmallObjectsUploadAndList(t *testing.T) {
	const (
		objectCount = 250
		pageSize    = 50
		objectSize  = 1024 // 1 KB each
	)

	ts := NewTestServer(t)
	defer ts.Cleanup()
	ts.CreateBucket(t, "many-small-bucket")

	// Upload all objects, record expected ETags.
	wantETags := make(map[string]string, objectCount)
	for i := range objectCount {
		key := fmt.Sprintf("obj/%04d.dat", i)
		body := make([]byte, objectSize)
		// Fill with deterministic data derived from index so each object is unique.
		for j := range body {
			body[j] = byte((i + j) & 0xff)
		}
		wantETags[key] = md5hex(body)

		req, err := http.NewRequest(http.MethodPut, ts.ObjectURL("many-small-bucket", key), bytes.NewReader(body))
		require.NoError(t, err)
		req.ContentLength = int64(objectSize)
		ts.SignRequest(req, body)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode, "PUT %s", key)
	}

	// List all pages, collect every key seen.
	seen := make(map[string]string) // key → ETag from listing
	continuationToken := ""

	for {
		url := fmt.Sprintf("%s?list-type=2&max-keys=%d&prefix=obj/",
			ts.BucketURL("many-small-bucket"), pageSize)
		if continuationToken != "" {
			url += "&continuation-token=" + continuationToken
		}

		listReq, err := http.NewRequest(http.MethodGet, url, http.NoBody)
		require.NoError(t, err)
		ts.SignRequest(listReq, nil)

		listResp, err := http.DefaultClient.Do(listReq)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, listResp.StatusCode)

		body, err := io.ReadAll(listResp.Body)
		listResp.Body.Close()
		require.NoError(t, err)

		var result listBucketResult
		require.NoError(t, xml.Unmarshal(body, &result), "unmarshal ListObjectsV2 response")

		for _, obj := range result.Contents {
			// Strip quotes from ETag for comparison.
			etag := strings.Trim(obj.ETag, `"`)
			seen[obj.Key] = etag
		}

		if !result.IsTruncated {
			break
		}
		continuationToken = result.NextContinuationToken
		require.NotEmpty(t, continuationToken, "IsTruncated=true but no NextContinuationToken")
	}

	assert.Len(t, seen, objectCount, "listed object count mismatch")
	for key, wantETag := range wantETags {
		gotETag, ok := seen[key]
		assert.True(t, ok, "object %s missing from listing", key)
		assert.Equal(t, wantETag, gotETag, "ETag mismatch for %s", key)
	}
}

// TestManySmallObjectsContentIntegrity uploads 100 small objects and downloads
// each one, verifying the content is exactly what was uploaded.
func TestManySmallObjectsContentIntegrity(t *testing.T) {
	const (
		objectCount = 100
		objectSize  = 4 * 1024 // 4 KB each
	)

	ts := NewTestServer(t)
	defer ts.Cleanup()
	ts.CreateBucket(t, "small-integrity-bucket")

	bodies := make([][]byte, objectCount)
	for i := range objectCount {
		body := make([]byte, objectSize)
		for j := range body {
			body[j] = byte((i*7 + j*3) & 0xff)
		}
		bodies[i] = body

		key := fmt.Sprintf("file-%04d.bin", i)
		req, err := http.NewRequest(http.MethodPut, ts.ObjectURL("small-integrity-bucket", key), bytes.NewReader(body))
		require.NoError(t, err)
		req.ContentLength = int64(objectSize)
		ts.SignRequest(req, body)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode, "PUT file-%04d.bin", i)
	}

	// Download and verify every object.
	for i := range objectCount {
		key := fmt.Sprintf("file-%04d.bin", i)
		req, err := http.NewRequest(http.MethodGet, ts.ObjectURL("small-integrity-bucket", key), http.NoBody)
		require.NoError(t, err)
		ts.SignRequest(req, nil)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		got, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "GET %s", key)
		assert.Equal(t, md5hex(bodies[i]), md5hex(got), "content mismatch for %s", key)
	}
}

// TestManySmallObjectsBatchDelete uploads 150 small objects and then deletes
// them all in one DeleteObjects request, verifying the bucket is empty.
func TestManySmallObjectsBatchDelete(t *testing.T) {
	const objectCount = 150

	ts := NewTestServer(t)
	defer ts.Cleanup()
	ts.CreateBucket(t, "batch-delete-bucket")

	for i := range objectCount {
		key := fmt.Sprintf("del/%04d.txt", i)
		body := []byte(fmt.Sprintf("content-%d", i))

		req, err := http.NewRequest(http.MethodPut, ts.ObjectURL("batch-delete-bucket", key), bytes.NewReader(body))
		require.NoError(t, err)
		req.ContentLength = int64(len(body))
		ts.SignRequest(req, body)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode, "PUT %s", key)
	}

	// Build a DeleteObjects request for all 150 keys.
	var sb strings.Builder
	sb.WriteString(`<Delete>`)
	for i := range objectCount {
		fmt.Fprintf(&sb, "<Object><Key>del/%04d.txt</Key></Object>", i)
	}
	sb.WriteString(`</Delete>`)

	deleteBody := []byte(sb.String())
	deleteURL := ts.BucketURL("batch-delete-bucket") + "?delete"
	deleteReq, err := http.NewRequest(http.MethodPost, deleteURL, bytes.NewReader(deleteBody))
	require.NoError(t, err)
	deleteReq.ContentLength = int64(len(deleteBody))
	deleteReq.Header.Set("Content-Type", "application/xml")
	ts.SignRequest(deleteReq, deleteBody)

	deleteResp, err := http.DefaultClient.Do(deleteReq)
	require.NoError(t, err)
	deleteRespBody, _ := io.ReadAll(deleteResp.Body)
	deleteResp.Body.Close()
	require.Equal(t, http.StatusOK, deleteResp.StatusCode, "DeleteObjects: %s", deleteRespBody)

	// Verify no errors in the response.
	assert.NotContains(t, string(deleteRespBody), "<Error>", "DeleteObjects response should contain no errors")

	// Verify the bucket is now empty.
	listReq, err := http.NewRequest(http.MethodGet, ts.BucketURL("batch-delete-bucket")+"?list-type=2", http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(listReq, nil)

	listResp, err := http.DefaultClient.Do(listReq)
	require.NoError(t, err)
	listBody, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var result listBucketResult
	require.NoError(t, xml.Unmarshal(listBody, &result))
	assert.Empty(t, result.Contents, "bucket should be empty after batch delete")
}

// ---------------------------------------------------------------------------
// Mixed-size tests
// ---------------------------------------------------------------------------

// TestMixedLargeAndSmallObjects puts a mix of large (2 MB) and small (1 KB)
// objects into a single bucket and verifies that listing returns all of them
// and that both sizes download with correct content.
func TestMixedLargeAndSmallObjects(t *testing.T) {
	const (
		smallCount = 50
		largeCount = 5
		smallSize  = 1024
		largeSize  = 2 * 1024 * 1024
	)

	ts := NewTestServer(t)
	defer ts.Cleanup()
	ts.CreateBucket(t, "mixed-size-bucket")

	type uploaded struct {
		key  string
		hash string
	}
	all := make([]uploaded, 0, smallCount+largeCount)

	// Upload small objects.
	for i := range smallCount {
		key := fmt.Sprintf("small/%04d.dat", i)
		body := make([]byte, smallSize)
		for j := range body {
			body[j] = byte((i + j) & 0xff)
		}

		req, err := http.NewRequest(http.MethodPut, ts.ObjectURL("mixed-size-bucket", key), bytes.NewReader(body))
		require.NoError(t, err)
		req.ContentLength = int64(smallSize)
		ts.SignRequest(req, body)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode, "PUT %s", key)
		all = append(all, uploaded{key, md5hex(body)})
	}

	// Upload large objects.
	for i := range largeCount {
		key := fmt.Sprintf("large/%04d.dat", i)
		body := make([]byte, largeSize)
		_, err := rand.Read(body)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPut, ts.ObjectURL("mixed-size-bucket", key), bytes.NewReader(body))
		require.NoError(t, err)
		req.ContentLength = int64(largeSize)
		ts.SignRequest(req, body)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode, "PUT %s", key)
		all = append(all, uploaded{key, md5hex(body)})
	}

	// List everything and confirm total count.
	listReq, err := http.NewRequest(http.MethodGet,
		ts.BucketURL("mixed-size-bucket")+"?list-type=2&max-keys=1000", http.NoBody)
	require.NoError(t, err)
	ts.SignRequest(listReq, nil)

	listResp, err := http.DefaultClient.Do(listReq)
	require.NoError(t, err)
	listBody, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()

	var result listBucketResult
	require.NoError(t, xml.Unmarshal(listBody, &result))
	assert.Len(t, result.Contents, smallCount+largeCount, "listed object count mismatch")

	// Spot-check: download 3 large objects and verify content hash.
	for i := range min(3, largeCount) {
		u := all[smallCount+i]

		getReq, err := http.NewRequest(http.MethodGet, ts.ObjectURL("mixed-size-bucket", u.key), http.NoBody)
		require.NoError(t, err)
		ts.SignRequest(getReq, nil)

		getResp, err := http.DefaultClient.Do(getReq)
		require.NoError(t, err)
		downloaded, _ := io.ReadAll(getResp.Body)
		getResp.Body.Close()

		assert.Equal(t, http.StatusOK, getResp.StatusCode, "GET %s", u.key)
		assert.Equal(t, u.hash, md5hex(downloaded), "integrity check for %s", u.key)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// listBucketResult is a minimal struct for unmarshalling ListObjectsV2 XML.
type listBucketResult struct {
	XMLName               xml.Name      `xml:"ListBucketResult"`
	IsTruncated           bool          `xml:"IsTruncated"`
	NextContinuationToken string        `xml:"NextContinuationToken"`
	Contents              []s3ObjectXML `xml:"Contents"`
}

type s3ObjectXML struct {
	Key  string `xml:"Key"`
	ETag string `xml:"ETag"`
	Size int64  `xml:"Size"`
}

// md5hex returns the lowercase hex MD5 digest of b.
func md5hex(b []byte) string {
	h := md5.Sum(b)
	return hex.EncodeToString(h[:])
}
