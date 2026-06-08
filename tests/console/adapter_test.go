package console_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	consoleapi "github.com/mallardduck/dirio/api"
	consolewire "github.com/mallardduck/dirio/internal/console"
	"github.com/mallardduck/dirio/internal/service"
	"github.com/mallardduck/dirio/internal/testutil"
)

// newAdapterFromTestServer constructs a console Adapter backed by the given test
// server's service components, so adapter methods can be exercised directly
// without going through HTTP.
func newAdapterFromTestServer(ts *testutil.TestServer) *consolewire.Adapter {
	factory := service.NewServiceFactory(
		ts.Server.Storage(),
		ts.Server.Metadata(),
		ts.Server.PolicyEngine(),
		ts.Server.Auth(),
	)
	return consolewire.NewAdapter(factory)
}

// ---------------------------------------------------------------------------
// ListObjects
// ---------------------------------------------------------------------------

func TestAdapter_ListObjects_Empty(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "list-empty")

	objects, err := a.ListObjects(ctx, "list-empty", "", "")
	require.NoError(t, err)
	assert.Empty(t, objects)
}

func TestAdapter_ListObjects_WithObjects(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "list-with")
	ts.PutObject(t, "list-with", "a.txt", "aaa")
	ts.PutObject(t, "list-with", "b.txt", "bbb")

	objects, err := a.ListObjects(ctx, "list-with", "", "")
	require.NoError(t, err)
	require.Len(t, objects, 2)

	keys := make([]string, len(objects))
	for i, o := range objects {
		keys[i] = o.Key
	}
	assert.ElementsMatch(t, []string{"a.txt", "b.txt"}, keys)
	assert.False(t, objects[0].IsPrefix)
}

func TestAdapter_ListObjects_PrefixFilter(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "list-prefix")
	ts.PutObject(t, "list-prefix", "docs/readme.md", "doc")
	ts.PutObject(t, "list-prefix", "docs/guide.md", "guide")
	ts.PutObject(t, "list-prefix", "images/logo.png", "img")

	objects, err := a.ListObjects(ctx, "list-prefix", "docs/", "")
	require.NoError(t, err)
	require.Len(t, objects, 2)
	for _, o := range objects {
		assert.True(t, strings.HasPrefix(o.Key, "docs/"))
	}
}

func TestAdapter_ListObjects_WithDelimiter(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "list-delim")
	ts.PutObject(t, "list-delim", "docs/readme.md", "doc")
	ts.PutObject(t, "list-delim", "docs/guide.md", "guide")
	ts.PutObject(t, "list-delim", "root.txt", "root")

	objects, err := a.ListObjects(ctx, "list-delim", "", "/")
	require.NoError(t, err)

	var prefixes, files []string
	for _, o := range objects {
		if o.IsPrefix {
			prefixes = append(prefixes, o.Key)
		} else {
			files = append(files, o.Key)
		}
	}
	assert.Equal(t, []string{"docs/"}, prefixes)
	assert.Equal(t, []string{"root.txt"}, files)
}

// ---------------------------------------------------------------------------
// GetObjectMetadata
// ---------------------------------------------------------------------------

func TestAdapter_GetObjectMetadata(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "meta-bucket")
	ts.PutObject(t, "meta-bucket", "hello.txt", "hello world")

	meta, err := a.GetObjectMetadata(ctx, "meta-bucket", "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello.txt", meta.Key)
	assert.Equal(t, int64(11), meta.Size)
	assert.NotEmpty(t, meta.ETag)
	assert.False(t, meta.LastModified.IsZero())
}

func TestAdapter_GetObjectMetadata_NotFound(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "meta-nf")

	_, err := a.GetObjectMetadata(ctx, "meta-nf", "nonexistent.txt")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// GetObjectTags / SetObjectTags
// ---------------------------------------------------------------------------

func TestAdapter_GetObjectTags_EmptyByDefault(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "tags-empty")
	ts.PutObject(t, "tags-empty", "obj.txt", "content")

	tags, err := a.GetObjectTags(ctx, "tags-empty", "obj.txt")
	require.NoError(t, err)
	assert.NotNil(t, tags)
	assert.Empty(t, tags)
}

func TestAdapter_SetAndGetObjectTags(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "tags-rt")
	ts.PutObject(t, "tags-rt", "obj.txt", "data")

	err := a.SetObjectTags(ctx, "tags-rt", "obj.txt", map[string]string{
		"env":  "test",
		"team": "platform",
	})
	require.NoError(t, err)

	tags, err := a.GetObjectTags(ctx, "tags-rt", "obj.txt")
	require.NoError(t, err)
	assert.Equal(t, "test", tags["env"])
	assert.Equal(t, "platform", tags["team"])
}

// ---------------------------------------------------------------------------
// DeleteObject
// ---------------------------------------------------------------------------

func TestAdapter_DeleteObject(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "del-bucket")
	ts.PutObject(t, "del-bucket", "gone.txt", "bye")

	err := a.DeleteObject(ctx, "del-bucket", "gone.txt")
	require.NoError(t, err)

	_, err = a.GetObjectMetadata(ctx, "del-bucket", "gone.txt")
	assert.Error(t, err, "object should be gone after delete")
}

func TestAdapter_DeleteObject_Idempotent(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "del-idem")

	// S3 spec: deleting a nonexistent object is not an error.
	err := a.DeleteObject(ctx, "del-idem", "never-existed.txt")
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// CopyObject
// ---------------------------------------------------------------------------

func TestAdapter_CopyObject(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	ts.CreateBucket(t, "copy-src")
	ts.CreateBucket(t, "copy-dst")
	ts.PutObject(t, "copy-src", "original.txt", "original content")

	err := a.CopyObject(ctx, "copy-src", "original.txt", "copy-dst", "copy.txt")
	require.NoError(t, err)

	srcMeta, err := a.GetObjectMetadata(ctx, "copy-src", "original.txt")
	require.NoError(t, err)
	assert.NotEmpty(t, srcMeta.ETag)

	dstMeta, err := a.GetObjectMetadata(ctx, "copy-dst", "copy.txt")
	require.NoError(t, err)
	assert.Equal(t, srcMeta.ETag, dstMeta.ETag)
}

// ---------------------------------------------------------------------------
// GeneratePresignedURL
// ---------------------------------------------------------------------------

func TestAdapter_GeneratePresignedURL(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	rawURL, err := a.GeneratePresignedURL(ctx, consoleapi.GeneratePresignedURLRequest{
		AccessKey: ts.AccessKey,
		Bucket:    "my-bucket",
		Key:       "path/to/object.txt",
		Expiry:    15 * time.Minute,
		BaseURL:   ts.BaseURL,
	})
	require.NoError(t, err)

	u, err := url.Parse(rawURL)
	require.NoError(t, err)

	q := u.Query()
	assert.Equal(t, "AWS4-HMAC-SHA256", q.Get("X-Amz-Algorithm"))
	assert.Contains(t, q.Get("X-Amz-Credential"), ts.AccessKey)
	assert.Equal(t, "900", q.Get("X-Amz-Expires"))
	assert.NotEmpty(t, q.Get("X-Amz-Signature"))
}

func TestAdapter_GeneratePresignedURL_UnknownAccessKey(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	_, err := a.GeneratePresignedURL(ctx, consoleapi.GeneratePresignedURLRequest{
		AccessKey: "DOESNOTEXIST",
		Bucket:    "bucket",
		Key:       "key",
		Expiry:    5 * time.Minute,
		BaseURL:   ts.BaseURL,
	})
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// UpdatePolicy
// ---------------------------------------------------------------------------

func TestAdapter_UpdatePolicy(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	initialDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::test-bucket"]}]}`
	_, err := a.CreatePolicy(ctx, consoleapi.CreatePolicyRequest{
		Name:           "test-update-policy",
		PolicyDocument: initialDoc,
	})
	require.NoError(t, err)

	newDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::test-bucket/*"]}]}`
	updated, err := a.UpdatePolicy(ctx, "test-update-policy", consoleapi.UpdatePolicyRequest{
		PolicyDocument: newDoc,
	})
	require.NoError(t, err)
	assert.Equal(t, "test-update-policy", updated.Name)
	assert.Contains(t, updated.PolicyDocument, "s3:GetObject")
}

func TestAdapter_UpdatePolicy_InvalidJSON(t *testing.T) {
	ts := testutil.New(t)
	a := newAdapterFromTestServer(ts)
	ctx := context.Background()

	_, err := a.UpdatePolicy(ctx, "any-policy", consoleapi.UpdatePolicyRequest{
		PolicyDocument: `not valid json`,
	})
	assert.Error(t, err)
}
