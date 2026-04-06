package dirioapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/consoleapi"
)

// ---------------------------------------------------------------------------
// GET /.dirio/api/v1/buckets/{bucket}/owner
// ---------------------------------------------------------------------------

func TestGetBucketOwner_Unauthenticated(t *testing.T) {
	ts := NewTestServer(t)
	req := newUnsignedDirioRequest(t, ts, http.MethodGet, "/buckets/any-bucket/owner", nil)
	resp := do(t, req)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, "Unauthorized", decodeErrorCode(t, resp))
}

func TestGetBucketOwner_NotFound(t *testing.T) {
	ts := NewTestServer(t)
	req := newDirioRequest(t, ts, http.MethodGet, "/buckets/nonexistent/owner", nil)
	resp := do(t, req)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "NoSuchBucket", decodeErrorCode(t, resp))
}

func TestGetBucketOwner_AdminCreatedBucket(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")

	req := newDirioRequest(t, ts, http.MethodGet, "/buckets/my-bucket/owner", nil)
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var owner consoleapi.Owner
	DecodeJSON(t, resp, &owner)

	// Buckets created by the admin have the well-known admin UUID and empty
	// access key / username (the admin has no per-user IAM record).
	assert.Equal(t, consoleapi.AdminUserUUID, owner.UUID)
	assert.Empty(t, owner.AccessKey)
	assert.Empty(t, owner.Username)
}

// ---------------------------------------------------------------------------
// PUT /.dirio/api/v1/buckets/{bucket}/owner (transfer ownership)
// ---------------------------------------------------------------------------

func TestTransferBucketOwner_Unauthenticated(t *testing.T) {
	ts := NewTestServer(t)
	body, _ := json.Marshal(map[string]string{"accessKey": "bob"})
	req := newUnsignedDirioRequest(t, ts, http.MethodPut, "/buckets/my-bucket/owner", body)
	resp := do(t, req)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	DrainAndClose(resp)
}

func TestTransferBucketOwner_NonAdminForbidden(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")
	createUser(t, ts, "alice", "alicesecretkey123")

	body, _ := json.Marshal(map[string]string{"accessKey": "alice"})
	req := newDirioRequestAs(t, ts, http.MethodPut, "/buckets/my-bucket/owner", body, "alice", "alicesecretkey123")
	resp := do(t, req)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, "AccessDenied", decodeErrorCode(t, resp))
}

func TestTransferBucketOwner_MissingBody(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")

	// Empty body — should return 400.
	req := newDirioRequest(t, ts, http.MethodPut, "/buckets/my-bucket/owner", []byte(`{}`))
	resp := do(t, req)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "InvalidRequest", decodeErrorCode(t, resp))
}

func TestTransferBucketOwner_BucketNotFound(t *testing.T) {
	ts := NewTestServer(t)
	createUser(t, ts, "alice", "alicesecretkey123")

	body, _ := json.Marshal(map[string]string{"accessKey": "alice"})
	req := newDirioRequest(t, ts, http.MethodPut, "/buckets/nonexistent/owner", body)
	resp := do(t, req)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "NoSuchBucket", decodeErrorCode(t, resp))
}

func TestTransferBucketOwner_UserNotFound(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")

	body, _ := json.Marshal(map[string]string{"accessKey": "ghost"})
	req := newDirioRequest(t, ts, http.MethodPut, "/buckets/my-bucket/owner", body)
	resp := do(t, req)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "NoSuchUser", decodeErrorCode(t, resp))
}

func TestTransferBucketOwner_Success(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")
	createUser(t, ts, "alice", "alicesecretkey123")

	body, _ := json.Marshal(map[string]string{"accessKey": "alice"})
	req := newDirioRequest(t, ts, http.MethodPut, "/buckets/my-bucket/owner", body)
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var owner consoleapi.Owner
	DecodeJSON(t, resp, &owner)
	assert.Equal(t, "alice", owner.AccessKey)
	assert.NotEmpty(t, owner.UUID)

	// Verify a subsequent GET returns the new owner.
	req2 := newDirioRequest(t, ts, http.MethodGet, "/buckets/my-bucket/owner", nil)
	resp2 := do(t, req2)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	var owner2 consoleapi.Owner
	DecodeJSON(t, resp2, &owner2)
	assert.Equal(t, "alice", owner2.AccessKey)
	assert.Equal(t, owner.UUID, owner2.UUID)
}

// ---------------------------------------------------------------------------
// GET /.dirio/api/v1/buckets/{bucket}/objects/{key}
// ---------------------------------------------------------------------------

func TestGetObjectOwner_Unauthenticated(t *testing.T) {
	ts := NewTestServer(t)
	req := newUnsignedDirioRequest(t, ts, http.MethodGet, "/buckets/my-bucket/objects/file.txt", nil)
	resp := do(t, req)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	DrainAndClose(resp)
}

func TestGetObjectOwner_BucketNotFound(t *testing.T) {
	ts := NewTestServer(t)
	req := newDirioRequest(t, ts, http.MethodGet, "/buckets/nonexistent/objects/file.txt", nil)
	resp := do(t, req)
	// Bucket not found should surface as NoSuchBucket or NoSuchObject depending
	// on how the service layer orders its checks; either 404 is acceptable.
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	DrainAndClose(resp)
}

func TestGetObjectOwner_ObjectNotFound(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")

	req := newDirioRequest(t, ts, http.MethodGet, "/buckets/my-bucket/objects/ghost.txt", nil)
	resp := do(t, req)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "NoSuchObject", decodeErrorCode(t, resp))
}

func TestGetObjectOwner_AdminUploadedObject(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")
	ts.PutObject(t, "my-bucket", "data/report.csv", "hello world")

	req := newDirioRequest(t, ts, http.MethodGet, "/buckets/my-bucket/objects/data/report.csv", nil)
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var owner consoleapi.Owner
	DecodeJSON(t, resp, &owner)
	assert.NotEmpty(t, owner.UUID)
}

func TestGetObjectOwner_KeyWithSlashes(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")
	ts.PutObject(t, "my-bucket", "a/b/c/deep.txt", "deep content")

	// Raw slashes in the path — the router captures with {key:.*}.
	req := newDirioRequest(t, ts, http.MethodGet, "/buckets/my-bucket/objects/a/b/c/deep.txt", nil)
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var owner consoleapi.Owner
	DecodeJSON(t, resp, &owner)
	assert.NotEmpty(t, owner.UUID)
}
