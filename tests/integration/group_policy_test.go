package integration

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupPolicyInheritance(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// 1. Create a bucket for testing
	bucketName := "group-test-bucket"
	ts.CreateBucket(t, bucketName)
	ts.PutObject(t, bucketName, "data.txt", "some data")

	// 2. Create a user (no direct policies)
	userAccessKey := "groupuser"
	userSecretKey := "groupusersecret"
	createIAMUser(t, ts, userAccessKey, userSecretKey)

	// 3. Create a group and add the user to it
	groupName := "testgroup"
	updateGroupMembers(t, ts, groupName, []string{userAccessKey}, false)

	// 4. Create a policy and attach it to the group
	policyName := "group-policy"
	policyDoc := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:GetObject"],
				"Resource": ["arn:aws:s3:::%s", "arn:aws:s3:::%s/*"]
			}
		]
	}`, bucketName, bucketName)
	createIAMPolicy(t, ts, policyName, policyDoc)
	attachGroupPolicy(t, ts, policyName, groupName)

	// 5. Verify user can access the bucket (via group policy inheritance)
	t.Run("User inherits group policy", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.BucketURL(bucketName)+"/data.txt", nil)
		SignRequestWithCredentials(req, nil, userAccessKey, userSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "User should have access via group policy")
	})

	// 6. Remove user from group and verify access is denied
	updateGroupMembers(t, ts, groupName, []string{userAccessKey}, true)

	t.Run("User access denied after removal from group", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.BucketURL(bucketName)+"/data.txt", nil)
		SignRequestWithCredentials(req, nil, userAccessKey, userSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusForbidden, resp.StatusCode, "User should NOT have access after removal from group")
	})
}

func updateGroupMembers(t *testing.T, ts *TestServer, groupName string, members []string, isRemove bool) {
	body := fmt.Sprintf(`{"group": "%s", "members": %s, "isRemove": %v}`,
		groupName, fmt.Sprintf("%v", members), isRemove)
	// Need to fix the slice formatting for JSON
	membersJSON := "["
	for i, m := range members {
		membersJSON += fmt.Sprintf(`"%s"`, m)
		if i < len(members)-1 {
			membersJSON += ","
		}
	}
	membersJSON += "]"

	body = fmt.Sprintf(`{"group": "%s", "members": %s, "isRemove": %v}`,
		groupName, membersJSON, isRemove)

	url := ts.URL("/minio/admin/v3/update-group-members")
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer([]byte(body)))
	ts.SignRequest(req, []byte(body))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to update group members")
}

func attachGroupPolicy(t *testing.T, ts *TestServer, policyName, groupName string) {
	url := fmt.Sprintf("%s/minio/admin/v3/set-policy?policyName=%s&userOrGroup=%s&isGroup=true", ts.URL(""), policyName, groupName)
	req, _ := http.NewRequest(http.MethodPost, url, nil)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to attach IAM policy %s to group %s", policyName, groupName)
}
