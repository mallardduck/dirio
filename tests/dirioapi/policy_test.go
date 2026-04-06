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
// POST /.dirio/api/v1/simulate
// ---------------------------------------------------------------------------

func TestSimulate_Unauthenticated(t *testing.T) {
	ts := NewTestServer(t)
	body, _ := json.Marshal(consoleapi.SimulateRequest{
		AccessKey: "alice", Bucket: "b", Action: "s3:GetObject",
	})
	req := newUnsignedDirioRequest(t, ts, http.MethodPost, "/simulate", body)
	resp := do(t, req)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	DrainAndClose(resp)
}

func TestSimulate_MissingFields(t *testing.T) {
	ts := NewTestServer(t)

	cases := []struct {
		name string
		body consoleapi.SimulateRequest
	}{
		{"missing accessKey", consoleapi.SimulateRequest{Bucket: "b", Action: "s3:GetObject"}},
		{"missing bucket", consoleapi.SimulateRequest{AccessKey: "alice", Action: "s3:GetObject"}},
		{"missing action", consoleapi.SimulateRequest{AccessKey: "alice", Bucket: "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := newDirioRequest(t, ts, http.MethodPost, "/simulate", body)
			resp := do(t, req)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			assert.Equal(t, "InvalidRequest", decodeErrorCode(t, resp))
		})
	}
}

func TestSimulate_BucketNotFound(t *testing.T) {
	ts := NewTestServer(t)
	body, _ := json.Marshal(consoleapi.SimulateRequest{
		AccessKey: ts.AccessKey, Bucket: "nonexistent", Action: "s3:GetObject",
	})
	req := newDirioRequest(t, ts, http.MethodPost, "/simulate", body)
	resp := do(t, req)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	DrainAndClose(resp)
}

func TestSimulate_NonAdminCannotSimulateOthers(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")
	createUser(t, ts, "alice", "alicesecretkey123")
	createUser(t, ts, "bob", "bobsecretkey1234")

	// alice tries to simulate as bob — should be denied.
	body, _ := json.Marshal(consoleapi.SimulateRequest{
		AccessKey: "bob", Bucket: "my-bucket", Action: "s3:GetObject",
	})
	req := newDirioRequestAs(t, ts, http.MethodPost, "/simulate", body, "alice", "alicesecretkey123")
	resp := do(t, req)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, "AccessDenied", decodeErrorCode(t, resp))
}

func TestSimulate_NonAdminCanSimulateSelf(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")
	createUser(t, ts, "alice", "alicesecretkey123")

	body, _ := json.Marshal(consoleapi.SimulateRequest{
		AccessKey: "alice", Bucket: "my-bucket", Action: "s3:GetObject",
	})
	req := newDirioRequestAs(t, ts, http.MethodPost, "/simulate", body, "alice", "alicesecretkey123")
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result consoleapi.SimulateResult
	DecodeJSON(t, resp, &result)
	assert.NotEmpty(t, result.Reason)
	// No policy attached, so alice should be denied by default.
	assert.False(t, result.Allowed)
}

func TestSimulate_AdminCanSimulateAnyUser(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")
	createUser(t, ts, "alice", "alicesecretkey123")

	body, _ := json.Marshal(consoleapi.SimulateRequest{
		AccessKey: "alice", Bucket: "my-bucket", Action: "s3:GetObject",
	})
	req := newDirioRequest(t, ts, http.MethodPost, "/simulate", body)
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result consoleapi.SimulateResult
	DecodeJSON(t, resp, &result)
	assert.NotEmpty(t, result.Reason)
}

func TestSimulate_PublicBucketAllowed(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "public-bucket")
	ts.SetBucketPolicy(t, "public-bucket", `{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": "*",
			"Action": ["s3:GetObject"],
			"Resource": ["arn:aws:s3:::public-bucket/*"]
		}]
	}`)
	createUser(t, ts, "alice", "alicesecretkey123")

	body, _ := json.Marshal(consoleapi.SimulateRequest{
		AccessKey: "alice", Bucket: "public-bucket", Action: "s3:GetObject", Key: "file.txt",
	})
	req := newDirioRequest(t, ts, http.MethodPost, "/simulate", body)
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result consoleapi.SimulateResult
	DecodeJSON(t, resp, &result)
	assert.True(t, result.Allowed, "s3:GetObject should be allowed on a public bucket")
	assert.NotEmpty(t, result.Reason)
}

// ---------------------------------------------------------------------------
// GET /.dirio/api/v1/buckets/{bucket}/permissions/{accessKey}
// ---------------------------------------------------------------------------

func TestGetEffectivePermissions_Unauthenticated(t *testing.T) {
	ts := NewTestServer(t)
	req := newUnsignedDirioRequest(t, ts, http.MethodGet, "/buckets/my-bucket/permissions/alice", nil)
	resp := do(t, req)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	DrainAndClose(resp)
}

func TestGetEffectivePermissions_NonAdminCannotQueryOthers(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")
	createUser(t, ts, "alice", "alicesecretkey123")
	createUser(t, ts, "bob", "bobsecretkey1234")

	req := newDirioRequestAs(t, ts, http.MethodGet, "/buckets/my-bucket/permissions/bob", nil, "alice", "alicesecretkey123")
	resp := do(t, req)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, "AccessDenied", decodeErrorCode(t, resp))
}

func TestGetEffectivePermissions_NonAdminCanQuerySelf(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")
	createUser(t, ts, "alice", "alicesecretkey123")

	req := newDirioRequestAs(t, ts, http.MethodGet, "/buckets/my-bucket/permissions/alice", nil, "alice", "alicesecretkey123")
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var perms consoleapi.EffectivePermissions
	DecodeJSON(t, resp, &perms)
	assert.Equal(t, "alice", perms.AccessKey)
	assert.Equal(t, "my-bucket", perms.Bucket)
	assert.NotNil(t, perms.AllowedActions)
	assert.NotNil(t, perms.DeniedActions)
}

func TestGetEffectivePermissions_AdminCanQueryAnyUser(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "my-bucket")
	createUser(t, ts, "alice", "alicesecretkey123")

	req := newDirioRequest(t, ts, http.MethodGet, "/buckets/my-bucket/permissions/alice", nil)
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var perms consoleapi.EffectivePermissions
	DecodeJSON(t, resp, &perms)
	assert.Equal(t, "alice", perms.AccessKey)
	assert.Equal(t, "my-bucket", perms.Bucket)
}

func TestGetEffectivePermissions_BucketNotFound(t *testing.T) {
	ts := NewTestServer(t)
	createUser(t, ts, "alice", "alicesecretkey123")

	req := newDirioRequest(t, ts, http.MethodGet, "/buckets/nonexistent/permissions/alice", nil)
	resp := do(t, req)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "NoSuchBucket", decodeErrorCode(t, resp))
}

func TestGetEffectivePermissions_WithPublicPolicy(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateBucket(t, "public-bucket")
	ts.SetBucketPolicy(t, "public-bucket", `{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": "*",
			"Action": ["s3:GetObject", "s3:ListObjectsV2"],
			"Resource": ["arn:aws:s3:::public-bucket/*", "arn:aws:s3:::public-bucket"]
		}]
	}`)
	createUser(t, ts, "alice", "alicesecretkey123")

	req := newDirioRequest(t, ts, http.MethodGet, "/buckets/public-bucket/permissions/alice", nil)
	resp := do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var perms consoleapi.EffectivePermissions
	DecodeJSON(t, resp, &perms)
	assert.Contains(t, perms.AllowedActions, "s3:GetObject",
		"s3:GetObject should be allowed via bucket policy")
}
