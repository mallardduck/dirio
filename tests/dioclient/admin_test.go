package dioclient_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/minio/madmin-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/internal/testutil"
	"github.com/mallardduck/dirio/pkg/dioclient"
)

// newAdminClient creates a dioclient.AdminClient pointed at the test server.
func newAdminClient(t *testing.T, ts *testutil.TestServer) *dioclient.AdminClient {
	t.Helper()
	ac, err := dioclient.NewAdminClient(dioclient.Config{
		Endpoint:  ts.BaseURL,
		AccessKey: ts.AccessKey,
		SecretKey: ts.SecretKey,
		Region:    "us-east-1",
	})
	require.NoError(t, err)
	return ac
}

// samplePolicy returns a minimal IAM policy granting read access to a bucket.
func samplePolicy(bucket string) []byte {
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Effect":   "Allow",
				"Action":   []string{"s3:GetObject", "s3:ListBucket"},
				"Resource": []string{"arn:aws:s3:::" + bucket, "arn:aws:s3:::" + bucket + "/*"},
			},
		},
	}
	b, _ := json.Marshal(doc)
	return b
}

// --- IAM user tests ---

func TestAdminClient_ListUsers_Empty(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)

	users, err := ac.ListUsers(context.Background())
	require.NoError(t, err)
	assert.Empty(t, users)
}

func TestAdminClient_AddAndRemoveUser(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)
	ctx := context.Background()

	require.NoError(t, ac.AddUser(ctx, "alice", "alicesecretkey1"))

	users, err := ac.ListUsers(ctx)
	require.NoError(t, err)
	assert.Contains(t, users, "alice")

	require.NoError(t, ac.RemoveUser(ctx, "alice"))

	users, err = ac.ListUsers(ctx)
	require.NoError(t, err)
	assert.NotContains(t, users, "alice")
}

func TestAdminClient_GetUserInfo(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)
	ctx := context.Background()

	require.NoError(t, ac.AddUser(ctx, "bob", "bobsecretkey123"))

	info, err := ac.GetUserInfo(ctx, "bob")
	require.NoError(t, err)
	assert.Equal(t, madmin.AccountEnabled, info.Status)
}

func TestAdminClient_SetUserStatus(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)
	ctx := context.Background()

	require.NoError(t, ac.AddUser(ctx, "carol", "carolsecret123"))

	require.NoError(t, ac.SetUserStatus(ctx, "carol", madmin.AccountDisabled))
	info, err := ac.GetUserInfo(ctx, "carol")
	require.NoError(t, err)
	assert.Equal(t, madmin.AccountDisabled, info.Status)

	require.NoError(t, ac.SetUserStatus(ctx, "carol", madmin.AccountEnabled))
	info, err = ac.GetUserInfo(ctx, "carol")
	require.NoError(t, err)
	assert.Equal(t, madmin.AccountEnabled, info.Status)
}

// --- IAM policy tests ---

func TestAdminClient_ListCannedPolicies_BuiltIns(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)

	policies, err := ac.ListCannedPolicies(context.Background())
	require.NoError(t, err)
	// DirIO ships with built-in policies (readonly, readwrite, writeonly, etc.).
	assert.NotEmpty(t, policies)
}

func TestAdminClient_AddAndDeleteCannedPolicy(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)
	ctx := context.Background()

	require.NoError(t, ac.AddCannedPolicy(ctx, "readbucket", samplePolicy("testbucket")))

	policies, err := ac.ListCannedPolicies(ctx)
	require.NoError(t, err)
	assert.Contains(t, policies, "readbucket", "newly created policy should appear in list")

	require.NoError(t, ac.DeleteCannedPolicy(ctx, "readbucket"))

	policies, err = ac.ListCannedPolicies(ctx)
	require.NoError(t, err)
	assert.NotContains(t, policies, "readbucket", "deleted policy should not appear in list")
}

func TestAdminClient_InfoCannedPolicy(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)
	ctx := context.Background()

	require.NoError(t, ac.AddCannedPolicy(ctx, "mypolicy", samplePolicy("mybucket")))

	info, err := ac.InfoCannedPolicy(ctx, "mypolicy")
	require.NoError(t, err)
	assert.Equal(t, "mypolicy", info.PolicyName)
	assert.NotEmpty(t, info.Policy)
}

func TestAdminClient_AttachAndDetachPolicy(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)
	ctx := context.Background()

	require.NoError(t, ac.AddCannedPolicy(ctx, "attachpol", samplePolicy("bucket")))
	require.NoError(t, ac.AddUser(ctx, "dave", "davesecret1234"))

	_, err := ac.AttachPolicy(ctx, madmin.PolicyAssociationReq{
		Policies: []string{"attachpol"},
		User:     "dave",
	})
	require.NoError(t, err)

	info, err := ac.GetUserInfo(ctx, "dave")
	require.NoError(t, err)
	assert.Equal(t, "attachpol", info.PolicyName)

	_, err = ac.DetachPolicy(ctx, madmin.PolicyAssociationReq{
		Policies: []string{"attachpol"},
		User:     "dave",
	})
	require.NoError(t, err)

	info, err = ac.GetUserInfo(ctx, "dave")
	require.NoError(t, err)
	assert.Empty(t, info.PolicyName)
}

// --- Service account tests ---

func TestAdminClient_ListServiceAccounts_Empty(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)

	resp, err := ac.ListServiceAccounts(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, resp.Accounts)
}

func TestAdminClient_AddAndDeleteServiceAccount(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)
	ctx := context.Background()

	creds, err := ac.AddServiceAccount(ctx, madmin.AddServiceAccountReq{
		Name: "ci-bot",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, creds.AccessKey)
	assert.NotEmpty(t, creds.SecretKey)

	resp, err := ac.ListServiceAccounts(ctx, "")
	require.NoError(t, err)
	found := false
	for _, a := range resp.Accounts {
		if a.AccessKey == creds.AccessKey {
			found = true
			assert.Equal(t, "ci-bot", a.Name)
		}
	}
	assert.True(t, found, "created service account should appear in list")

	require.NoError(t, ac.DeleteServiceAccount(ctx, creds.AccessKey))

	resp, err = ac.ListServiceAccounts(ctx, "")
	require.NoError(t, err)
	for _, a := range resp.Accounts {
		assert.NotEqual(t, creds.AccessKey, a.AccessKey, "deleted SA should not appear in list")
	}
}

func TestAdminClient_InfoServiceAccount(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)
	ctx := context.Background()

	creds, err := ac.AddServiceAccount(ctx, madmin.AddServiceAccountReq{
		Name:        "info-test-sa",
		Description: "for info test",
	})
	require.NoError(t, err)

	info, err := ac.InfoServiceAccount(ctx, creds.AccessKey)
	require.NoError(t, err)
	assert.Equal(t, "info-test-sa", info.Name)
	assert.Equal(t, "for info test", info.Description)
	assert.Equal(t, "on", info.AccountStatus)
}

func TestAdminClient_UpdateServiceAccount(t *testing.T) {
	ts := testutil.New(t)
	ac := newAdminClient(t, ts)
	ctx := context.Background()

	creds, err := ac.AddServiceAccount(ctx, madmin.AddServiceAccountReq{
		Name: "original-name",
	})
	require.NoError(t, err)

	require.NoError(t, ac.UpdateServiceAccount(ctx, creds.AccessKey, madmin.UpdateServiceAccountReq{
		NewName: "updated-name",
	}))

	info, err := ac.InfoServiceAccount(ctx, creds.AccessKey)
	require.NoError(t, err)
	assert.Equal(t, "updated-name", info.Name)
}
