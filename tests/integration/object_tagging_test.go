package integration

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tagging represents the XML structure for object tagging
type Tagging struct {
	XMLName xml.Name `xml:"Tagging"`
	TagSet  []Tag    `xml:"TagSet>Tag"`
}

// Tag represents a single tag
type Tag struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

func TestPutObjectTagging(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// First, create an object
	content := "test content for tagging"
	putReq, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "test.txt"), strings.NewReader(content))
	bodyBytes := []byte(content)
	ts.SignRequest(putReq, bodyBytes)
	putReq.Header.Set("Content-Type", "text/plain")
	putReq.ContentLength = int64(len(content))

	putResp, err := http.DefaultClient.Do(putReq)
	require.NoError(t, err)
	defer putResp.Body.Close()
	require.Equal(t, http.StatusOK, putResp.StatusCode)

	// Now, put tags on the object
	tagging := Tagging{
		TagSet: []Tag{
			{Key: "env", Value: "test"},
			{Key: "project", Value: "dirio"},
		},
	}
	taggingXML, err := xml.Marshal(tagging)
	require.NoError(t, err)

	tagReq, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "test.txt")+"?tagging", bytes.NewReader(taggingXML))
	ts.SignRequest(tagReq, taggingXML)
	tagReq.Header.Set("Content-Type", "application/xml")
	tagReq.ContentLength = int64(len(taggingXML))

	tagResp, err := http.DefaultClient.Do(tagReq)
	require.NoError(t, err)
	defer tagResp.Body.Close()

	assert.Equal(t, http.StatusOK, tagResp.StatusCode)
}

func TestGetObjectTagging(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Create an object
	content := "test content for getting tags"
	putReq, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "test.txt"), strings.NewReader(content))
	bodyBytes := []byte(content)
	ts.SignRequest(putReq, bodyBytes)
	putReq.Header.Set("Content-Type", "text/plain")
	putReq.ContentLength = int64(len(content))

	putResp, err := http.DefaultClient.Do(putReq)
	require.NoError(t, err)
	defer putResp.Body.Close()
	require.Equal(t, http.StatusOK, putResp.StatusCode)

	// Put tags
	tagging := Tagging{
		TagSet: []Tag{
			{Key: "env", Value: "production"},
			{Key: "team", Value: "backend"},
		},
	}
	taggingXML, err := xml.Marshal(tagging)
	require.NoError(t, err)

	tagReq, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "test.txt")+"?tagging", bytes.NewReader(taggingXML))
	ts.SignRequest(tagReq, taggingXML)
	tagReq.Header.Set("Content-Type", "application/xml")
	tagReq.ContentLength = int64(len(taggingXML))

	tagResp, err := http.DefaultClient.Do(tagReq)
	require.NoError(t, err)
	defer tagResp.Body.Close()
	require.Equal(t, http.StatusOK, tagResp.StatusCode)

	// Get tags back
	getTagReq, _ := http.NewRequest("GET", ts.ObjectURL("test-bucket", "test.txt")+"?tagging", http.NoBody)
	ts.SignRequest(getTagReq, nil)

	getTagResp, err := http.DefaultClient.Do(getTagReq)
	require.NoError(t, err)
	defer getTagResp.Body.Close()

	assert.Equal(t, http.StatusOK, getTagResp.StatusCode)

	// Parse response
	var responseTags Tagging
	body, err := io.ReadAll(getTagResp.Body)
	require.NoError(t, err)
	err = xml.Unmarshal(body, &responseTags)
	require.NoError(t, err)

	// Verify tags
	assert.Len(t, responseTags.TagSet, 2)

	// Create a map for easier assertion
	tagMap := make(map[string]string)
	for _, tag := range responseTags.TagSet {
		tagMap[tag.Key] = tag.Value
	}

	assert.Equal(t, "production", tagMap["env"])
	assert.Equal(t, "backend", tagMap["team"])
}

func TestObjectTaggingDoesNotCorruptContent(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Create an object with known content
	originalContent := "this content should NOT be corrupted by tagging"
	putReq, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "test.txt"), strings.NewReader(originalContent))
	bodyBytes := []byte(originalContent)
	ts.SignRequest(putReq, bodyBytes)
	putReq.Header.Set("Content-Type", "text/plain")
	putReq.ContentLength = int64(len(originalContent))

	putResp, err := http.DefaultClient.Do(putReq)
	require.NoError(t, err)
	defer putResp.Body.Close()
	require.Equal(t, http.StatusOK, putResp.StatusCode)

	// Verify content before tagging
	getReq1, _ := http.NewRequest("GET", ts.ObjectURL("test-bucket", "test.txt"), http.NoBody)
	ts.SignRequest(getReq1, nil)

	getResp1, err := http.DefaultClient.Do(getReq1)
	require.NoError(t, err)
	contentBefore, err := io.ReadAll(getResp1.Body)
	getResp1.Body.Close()
	require.NoError(t, err)
	require.Equal(t, originalContent, string(contentBefore))

	// Put tags on the object
	tagging := Tagging{
		TagSet: []Tag{
			{Key: "key1", Value: "value1"},
			{Key: "key2", Value: "value2"},
		},
	}
	taggingXML, err := xml.Marshal(tagging)
	require.NoError(t, err)

	tagReq, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "test.txt")+"?tagging", bytes.NewReader(taggingXML))
	ts.SignRequest(tagReq, taggingXML)
	tagReq.Header.Set("Content-Type", "application/xml")
	tagReq.ContentLength = int64(len(taggingXML))

	tagResp, err := http.DefaultClient.Do(tagReq)
	require.NoError(t, err)
	defer tagResp.Body.Close()
	require.Equal(t, http.StatusOK, tagResp.StatusCode)

	// CRITICAL: Verify content after tagging
	getReq2, _ := http.NewRequest("GET", ts.ObjectURL("test-bucket", "test.txt"), http.NoBody)
	ts.SignRequest(getReq2, nil)

	getResp2, err := http.DefaultClient.Do(getReq2)
	require.NoError(t, err)
	contentAfter, err := io.ReadAll(getResp2.Body)
	getResp2.Body.Close()
	require.NoError(t, err)

	// This is the critical assertion - content should NOT have changed
	assert.Equal(t, originalContent, string(contentAfter), "Object content was corrupted by tagging operation")

	// Also verify the content doesn't contain the XML tagging structure
	assert.NotContains(t, string(contentAfter), "<Tagging>", "Object content was replaced with tagging XML")
	assert.NotContains(t, string(contentAfter), "<TagSet>", "Object content was replaced with tagging XML")
}

func TestGetObjectTaggingOnNonexistentObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Try to get tags from nonexistent object
	getTagReq, _ := http.NewRequest("GET", ts.ObjectURL("test-bucket", "nonexistent.txt")+"?tagging", http.NoBody)
	ts.SignRequest(getTagReq, nil)

	getTagResp, err := http.DefaultClient.Do(getTagReq)
	require.NoError(t, err)
	defer getTagResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, getTagResp.StatusCode)

	body, _ := io.ReadAll(getTagResp.Body)
	assert.Contains(t, string(body), "NoSuchKey")
}

func TestPutObjectTaggingOnNonexistentObject(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "test-bucket")

	// Try to put tags on nonexistent object
	tagging := Tagging{
		TagSet: []Tag{
			{Key: "env", Value: "test"},
		},
	}
	taggingXML, err := xml.Marshal(tagging)
	require.NoError(t, err)

	tagReq, _ := http.NewRequest("PUT", ts.ObjectURL("test-bucket", "nonexistent.txt")+"?tagging", bytes.NewReader(taggingXML))
	ts.SignRequest(tagReq, taggingXML)
	tagReq.Header.Set("Content-Type", "application/xml")
	tagReq.ContentLength = int64(len(taggingXML))

	tagResp, err := http.DefaultClient.Do(tagReq)
	require.NoError(t, err)
	defer tagResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, tagResp.StatusCode)

	body, _ := io.ReadAll(tagResp.Body)
	assert.Contains(t, string(body), "NoSuchKey")
}
