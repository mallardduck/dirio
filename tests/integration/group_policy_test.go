package integration

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
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
		req, _ := http.NewRequest(http.MethodGet, ts.BucketURL(bucketName)+"/data.txt", http.NoBody)
		SignRequestWithCredentials(req, nil, userAccessKey, userSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "User should have access via group policy")
	})

	// 6. Remove user from group and verify access is denied
	updateGroupMembers(t, ts, groupName, []string{userAccessKey}, true)

	t.Run("User access denied after removal from group", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.BucketURL(bucketName)+"/data.txt", http.NoBody)
		SignRequestWithCredentials(req, nil, userAccessKey, userSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusForbidden, resp.StatusCode, "User should NOT have access after removal from group")
	})
}

func updateGroupMembers(t *testing.T, ts *TestServer, groupName string, members []string, isRemove bool) {
	// Need to fix the slice formatting for JSON
	var membersJSON strings.Builder
	membersJSON.WriteString("[")
	for i, m := range members {
		_, _ = fmt.Fprintf(&membersJSON, `"%s"`, m)
		if i < len(members)-1 {
			membersJSON.WriteString(",")
		}
	}
	membersJSON.WriteString("]")

	body := fmt.Sprintf(`{"group": %q, "members": %s, "isRemove": %v}`,
		groupName, membersJSON.String(), isRemove)

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
	req, _ := http.NewRequest(http.MethodPost, url, http.NoBody)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to attach IAM policy %s to group %s", policyName, groupName)
}

func detachGroupPolicy(t *testing.T, ts *TestServer, policyName, groupName string) {
	t.Helper()
	url := fmt.Sprintf("%s/minio/admin/v3/idp/builtin/policy/detach?policyName=%s&userOrGroup=%s&isGroup=true", ts.URL(""), policyName, groupName)
	req, _ := http.NewRequest(http.MethodPost, url, http.NoBody)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to detach IAM policy %s from group %s", policyName, groupName)
}

func setGroupStatus(t *testing.T, ts *TestServer, groupName, status string) {
	t.Helper()
	url := fmt.Sprintf("%s/minio/admin/v3/set-group-status?group=%s&status=%s", ts.URL(""), groupName, status)
	req, _ := http.NewRequest(http.MethodPost, url, http.NoBody)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to set group %s status to %s", groupName, status)
}

func getObject(t *testing.T, ts *TestServer, bucket, key, accessKey, secretKey string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.ObjectURL(bucket, key), http.NoBody)
	SignRequestWithCredentials(req, nil, accessKey, secretKey)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	return resp.StatusCode
}

// TestGroupPolicy_MultipleGroupMembership verifies that a user inherits the union
// of all attached policies across multiple groups they belong to.
//
// NOTE: This test is expected to FAIL until group policy evaluation is implemented
// in the policy engine (resolveEffectivePolicyNames does not consult group membership).
func TestGroupPolicy_MultipleGroupMembership(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "bucket-alpha")
	ts.PutObject(t, "bucket-alpha", "obj.txt", "alpha data")
	ts.CreateBucket(t, "bucket-beta")
	ts.PutObject(t, "bucket-beta", "obj.txt", "beta data")

	userAccessKey := "multiuser"
	userSecretKey := "multiusersecret"
	createIAMUser(t, ts, userAccessKey, userSecretKey)

	// group-alpha grants access to bucket-alpha only
	createIAMPolicy(t, ts, "policy-alpha", `{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:GetObject"],
			"Resource": ["arn:aws:s3:::bucket-alpha", "arn:aws:s3:::bucket-alpha/*"]
		}]
	}`)
	updateGroupMembers(t, ts, "group-alpha", []string{userAccessKey}, false)
	attachGroupPolicy(t, ts, "policy-alpha", "group-alpha")

	// group-beta grants access to bucket-beta only
	createIAMPolicy(t, ts, "policy-beta", `{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:GetObject"],
			"Resource": ["arn:aws:s3:::bucket-beta", "arn:aws:s3:::bucket-beta/*"]
		}]
	}`)
	updateGroupMembers(t, ts, "group-beta", []string{userAccessKey}, false)
	attachGroupPolicy(t, ts, "policy-beta", "group-beta")

	// User should be able to reach both buckets via their respective group policies.
	t.Run("User accesses bucket granted by first group", func(t *testing.T) {
		require.Equal(t, http.StatusOK, getObject(t, ts, "bucket-alpha", "obj.txt", userAccessKey, userSecretKey),
			"User should have access to bucket-alpha via group-alpha policy")
	})

	t.Run("User accesses bucket granted by second group", func(t *testing.T) {
		require.Equal(t, http.StatusOK, getObject(t, ts, "bucket-beta", "obj.txt", userAccessKey, userSecretKey),
			"User should have access to bucket-beta via group-beta policy")
	})
}

// TestGroupPolicy_PolicyDetachFromGroup verifies that detaching a policy from a group
// removes the access that group members had through that policy.
//
// NOTE: The first assertion (user has access) is expected to FAIL until group policy
// evaluation is implemented in the policy engine.
func TestGroupPolicy_PolicyDetachFromGroup(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "detach-bucket")
	ts.PutObject(t, "detach-bucket", "obj.txt", "some data")

	userAccessKey := "detachuser"
	userSecretKey := "detachusersecret"
	createIAMUser(t, ts, userAccessKey, userSecretKey)

	createIAMPolicy(t, ts, "detach-policy", `{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:GetObject"],
			"Resource": ["arn:aws:s3:::detach-bucket", "arn:aws:s3:::detach-bucket/*"]
		}]
	}`)
	updateGroupMembers(t, ts, "detach-group", []string{userAccessKey}, false)
	attachGroupPolicy(t, ts, "detach-policy", "detach-group")

	t.Run("User has access before policy is detached from group", func(t *testing.T) {
		require.Equal(t, http.StatusOK, getObject(t, ts, "detach-bucket", "obj.txt", userAccessKey, userSecretKey),
			"User should have access via group policy before detach")
	})

	detachGroupPolicy(t, ts, "detach-policy", "detach-group")

	t.Run("User loses access after policy is detached from group", func(t *testing.T) {
		require.Equal(t, http.StatusForbidden, getObject(t, ts, "detach-bucket", "obj.txt", userAccessKey, userSecretKey),
			"User should lose access after group policy is detached")
	})
}

// TestGroupPolicy_DisabledGroupIgnored verifies that a disabled group's policies
// are not applied to its members, while an active group's policies still are.
//
// NOTE: The assertion that the user can access the active-group bucket is expected
// to FAIL until group policy evaluation is implemented in the policy engine.
func TestGroupPolicy_DisabledGroupIgnored(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	ts.CreateBucket(t, "active-bucket")
	ts.PutObject(t, "active-bucket", "obj.txt", "active data")
	ts.CreateBucket(t, "disabled-bucket")
	ts.PutObject(t, "disabled-bucket", "obj.txt", "disabled data")

	userAccessKey := "statususer"
	userSecretKey := "statususersecret"
	createIAMUser(t, ts, userAccessKey, userSecretKey)

	// active-group: enabled, grants access to active-bucket
	createIAMPolicy(t, ts, "active-group-policy", `{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:GetObject"],
			"Resource": ["arn:aws:s3:::active-bucket", "arn:aws:s3:::active-bucket/*"]
		}]
	}`)
	updateGroupMembers(t, ts, "active-group", []string{userAccessKey}, false)
	attachGroupPolicy(t, ts, "active-group-policy", "active-group")

	// disabled-group: disabled, grants access to disabled-bucket — but should be ignored
	createIAMPolicy(t, ts, "disabled-group-policy", `{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:GetObject"],
			"Resource": ["arn:aws:s3:::disabled-bucket", "arn:aws:s3:::disabled-bucket/*"]
		}]
	}`)
	updateGroupMembers(t, ts, "disabled-group", []string{userAccessKey}, false)
	attachGroupPolicy(t, ts, "disabled-group-policy", "disabled-group")
	setGroupStatus(t, ts, "disabled-group", "disabled")

	t.Run("User can access bucket granted by active group", func(t *testing.T) {
		require.Equal(t, http.StatusOK, getObject(t, ts, "active-bucket", "obj.txt", userAccessKey, userSecretKey),
			"User should have access to active-bucket via enabled group policy")
	})

	t.Run("User cannot access bucket from disabled group", func(t *testing.T) {
		require.Equal(t, http.StatusForbidden, getObject(t, ts, "disabled-bucket", "obj.txt", userAccessKey, userSecretKey),
			"User should NOT have access via a disabled group's policy")
	})
}
