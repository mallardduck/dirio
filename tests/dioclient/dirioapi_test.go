package dioclient_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/internal/testutil"
	"github.com/mallardduck/dirio/sdk/dioclient"
)

// newDirioClient creates a dioclient.DirioClient pointed at the test server.
func newDirioClient(ts *testutil.TestServer) *dioclient.DirioClient {
	return dioclient.NewDirioClient(dioclient.Config{
		Endpoint:  ts.BaseURL,
		AccessKey: ts.AccessKey,
		SecretKey: ts.SecretKey,
		Region:    "us-east-1",
	})
}

// attachPolicy is a test helper that attaches a named policy to a user.
func attachPolicy(t *testing.T, ac interface {
	AttachPolicy(context.Context, dioclient.PolicyAssociationReq) (dioclient.PolicyAssociationResp, error)
}, ctx context.Context, policy, user string) {
	t.Helper()
	_, err := ac.AttachPolicy(ctx, dioclient.PolicyAssociationReq{
		Policies: []string{policy},
		User:     user,
	})
	require.NoError(t, err)
}

// --- ownership tests ---

func TestDirioAPI_GetBucketOwner_AdminOwned(t *testing.T) {
	ts := testutil.New(t)
	ts.CreateBucket(t, "mybucket")
	dc := newDirioClient(ts)

	info, err := dc.GetBucketOwner(context.Background(), "mybucket")
	require.NoError(t, err)
	// A bucket created by the admin user returns a non-empty UUID.
	assert.NotEmpty(t, info.UUID)
}

func TestDirioAPI_GetBucketOwner_NotFound(t *testing.T) {
	ts := testutil.New(t)
	dc := newDirioClient(ts)

	_, err := dc.GetBucketOwner(context.Background(), "nosuchbucket")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestDirioAPI_TransferBucketOwner(t *testing.T) {
	ts := testutil.New(t)
	ctx := context.Background()
	ts.CreateBucket(t, "transferbucket")
	dc := newDirioClient(ts)

	ac := newAdminClient(t, ts)
	require.NoError(t, ac.AddUser(ctx, "newowner", "newownersecret1"))

	info, err := dc.TransferBucketOwner(ctx, "transferbucket", "newowner")
	require.NoError(t, err)
	assert.Equal(t, "newowner", info.AccessKey)

	got, err := dc.GetBucketOwner(ctx, "transferbucket")
	require.NoError(t, err)
	assert.Equal(t, "newowner", got.AccessKey)
}

func TestDirioAPI_GetObjectOwner(t *testing.T) {
	ts := testutil.New(t)
	ctx := context.Background()
	ts.CreateBucket(t, "objbucket")
	ts.PutObject(t, "objbucket", "docs/readme.txt", "hello")
	dc := newDirioClient(ts)

	info, err := dc.GetObjectOwner(ctx, "objbucket", "docs/readme.txt")
	require.NoError(t, err)
	assert.NotEmpty(t, info.UUID)
}

func TestDirioAPI_GetObjectOwner_NotFound(t *testing.T) {
	ts := testutil.New(t)
	ts.CreateBucket(t, "objbucket2")
	dc := newDirioClient(ts)

	_, err := dc.GetObjectOwner(context.Background(), "objbucket2", "nosuchkey.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// --- simulate tests ---

func TestDirioAPI_Simulate_Allowed(t *testing.T) {
	ts := testutil.New(t)
	ctx := context.Background()
	ts.CreateBucket(t, "simbucket")
	dc := newDirioClient(ts)

	ac := newAdminClient(t, ts)
	require.NoError(t, ac.AddUser(ctx, "rwuser", "rwusersecret1"))
	attachPolicy(t, ac, ctx, "readwrite", "rwuser")

	result, err := dc.Simulate(ctx, dioclient.SimulateRequest{
		AccessKey: "rwuser",
		Bucket:    "simbucket",
		Action:    "s3:ListBucket",
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed, "user with readwrite policy should be allowed to ListBucket")
	assert.NotEmpty(t, result.Reason)
}

func TestDirioAPI_Simulate_DeniedForUnprivilegedUser(t *testing.T) {
	ts := testutil.New(t)
	ctx := context.Background()
	ts.CreateBucket(t, "denybucket")
	dc := newDirioClient(ts)

	ac := newAdminClient(t, ts)
	require.NoError(t, ac.AddUser(ctx, "noperm", "nopermsecret1"))

	result, err := dc.Simulate(ctx, dioclient.SimulateRequest{
		AccessKey: "noperm",
		Bucket:    "denybucket",
		Action:    "s3:PutObject",
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed, "user with no policies should be denied")
}

func TestDirioAPI_Simulate_WithKey(t *testing.T) {
	ts := testutil.New(t)
	ctx := context.Background()
	ts.CreateBucket(t, "keybucket")
	dc := newDirioClient(ts)

	ac := newAdminClient(t, ts)
	require.NoError(t, ac.AddUser(ctx, "readuser", "readusersecret1"))
	attachPolicy(t, ac, ctx, "readonly", "readuser")

	result, err := dc.Simulate(ctx, dioclient.SimulateRequest{
		AccessKey: "readuser",
		Bucket:    "keybucket",
		Action:    "s3:GetObject",
		Key:       "path/to/file.txt",
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed, "readonly user should be allowed s3:GetObject")
}

// --- effective permissions tests ---

func TestDirioAPI_GetEffectivePermissions(t *testing.T) {
	ts := testutil.New(t)
	ctx := context.Background()
	ts.CreateBucket(t, "permbucket")
	dc := newDirioClient(ts)

	ac := newAdminClient(t, ts)
	require.NoError(t, ac.AddUser(ctx, "permuser", "permusersecret1"))
	attachPolicy(t, ac, ctx, "readwrite", "permuser")

	perms, err := dc.GetEffectivePermissions(ctx, "permbucket", "permuser")
	require.NoError(t, err)

	assert.Equal(t, "permuser", perms.AccessKey)
	assert.Equal(t, "permbucket", perms.Bucket)
	assert.NotEmpty(t, perms.AllowedActions, "user with readwrite policy should have allowed actions")
}

func TestDirioAPI_GetEffectivePermissions_UnprivilegedUser(t *testing.T) {
	ts := testutil.New(t)
	ctx := context.Background()
	ts.CreateBucket(t, "permbucket2")
	dc := newDirioClient(ts)

	ac := newAdminClient(t, ts)
	require.NoError(t, ac.AddUser(ctx, "limiteduser", "limitedsecret1"))

	perms, err := dc.GetEffectivePermissions(ctx, "permbucket2", "limiteduser")
	require.NoError(t, err)

	assert.Equal(t, "limiteduser", perms.AccessKey)
	assert.Empty(t, perms.AllowedActions)
	assert.NotEmpty(t, perms.DeniedActions)
}
