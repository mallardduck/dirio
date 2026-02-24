package integration

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"testing"

	"github.com/minio/madmin-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/pkg/s3types"
)

func TestListFiltering(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	// 1. Create buckets and objects
	// alice-bucket/
	//   public/data.txt
	//   private/secret.txt
	// bob-bucket/
	//   public/info.txt
	//   private/confidential.txt
	ts.CreateBucket(t, "alice-bucket")
	ts.PutObject(t, "alice-bucket", "public/data.txt", "alice public data")
	ts.PutObject(t, "alice-bucket", "private/secret.txt", "alice private secret")

	ts.CreateBucket(t, "bob-bucket")
	ts.PutObject(t, "bob-bucket", "public/info.txt", "bob public info")
	ts.PutObject(t, "bob-bucket", "private/confidential.txt", "bob private confidential")

	// 2. Create users and policies via Admin API
	// Alice: access to alice-bucket/* and bob-bucket/public/*
	aliceAccessKey := "alice"
	aliceSecretKey := "alicesecret123"
	createIAMUser(t, ts, aliceAccessKey, aliceSecretKey)

	alicePolicyDoc := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": "s3:ListAllMyBuckets",
				"Resource": "*"
			},
			{
				"Effect": "Allow",
				"Action": ["s3:GetBucketLocation", "s3:ListBucket"],
				"Resource": ["arn:aws:s3:::alice-bucket", "arn:aws:s3:::bob-bucket"]
			},
			{
				"Effect": "Allow",
				"Action": "s3:GetObject",
				"Resource": ["arn:aws:s3:::alice-bucket/*", "arn:aws:s3:::bob-bucket/public/*"]
			}
		]
	}`
	createIAMPolicy(t, ts, "alice-policy", alicePolicyDoc)
	attachIAMPolicy(t, ts, "alice-policy", aliceAccessKey)

	// Bob: access to bob-bucket/* only
	bobAccessKey := "bob"
	bobSecretKey := "bobsecret123"
	createIAMUser(t, ts, bobAccessKey, bobSecretKey)

	bobPolicyDoc := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": "s3:ListAllMyBuckets",
				"Resource": "*"
			},
			{
				"Effect": "Allow",
				"Action": ["s3:GetBucketLocation", "s3:ListBucket"],
				"Resource": "arn:aws:s3:::bob-bucket"
			},
			{
				"Effect": "Allow",
				"Action": "s3:GetObject",
				"Resource": "arn:aws:s3:::bob-bucket/*"
			}
		]
	}`
	createIAMPolicy(t, ts, "bob-policy", bobPolicyDoc)
	attachIAMPolicy(t, ts, "bob-policy", bobAccessKey)

	t.Run("Alice ListBuckets", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL("/"), http.NoBody)
		SignRequestWithCredentials(req, nil, aliceAccessKey, aliceSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var listResp s3types.ListBucketsResponse
		err = xml.NewDecoder(resp.Body).Decode(&listResp)
		require.NoError(t, err)

		// Alice should see both buckets because she has GetBucketLocation on both
		bucketNames := getBucketNames(listResp.Buckets)
		assert.Contains(t, bucketNames, "alice-bucket")
		assert.Contains(t, bucketNames, "bob-bucket")
	})

	t.Run("Bob ListBuckets", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL("/"), http.NoBody)
		SignRequestWithCredentials(req, nil, bobAccessKey, bobSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var listResp s3types.ListBucketsResponse
		err = xml.NewDecoder(resp.Body).Decode(&listResp)
		require.NoError(t, err)

		// Bob should only see bob-bucket
		bucketNames := getBucketNames(listResp.Buckets)
		assert.NotContains(t, bucketNames, "alice-bucket")
		assert.Contains(t, bucketNames, "bob-bucket")
	})

	t.Run("Alice ListObjects alice-bucket", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.BucketURL("alice-bucket")+"?list-type=2", http.NoBody)
		SignRequestWithCredentials(req, nil, aliceAccessKey, aliceSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var listResp s3types.ListBucketV2Result
		err = xml.NewDecoder(resp.Body).Decode(&listResp)
		require.NoError(t, err)

		// Alice should see all objects in alice-bucket
		objectKeys := getObjectKeys(listResp.Contents)
		assert.Contains(t, objectKeys, "public/data.txt")
		assert.Contains(t, objectKeys, "private/secret.txt")
	})

	t.Run("Alice ListObjects bob-bucket", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.BucketURL("bob-bucket")+"?list-type=2", http.NoBody)
		SignRequestWithCredentials(req, nil, aliceAccessKey, aliceSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var listResp s3types.ListBucketV2Result
		err = xml.NewDecoder(resp.Body).Decode(&listResp)
		require.NoError(t, err)

		// Alice should only see public/info.txt in bob-bucket
		objectKeys := getObjectKeys(listResp.Contents)
		assert.Contains(t, objectKeys, "public/info.txt")
		assert.NotContains(t, objectKeys, "private/confidential.txt")
	})

	t.Run("Bob ListObjects bob-bucket", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.BucketURL("bob-bucket")+"?list-type=2", http.NoBody)
		SignRequestWithCredentials(req, nil, bobAccessKey, bobSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var listResp s3types.ListBucketV2Result
		err = xml.NewDecoder(resp.Body).Decode(&listResp)
		require.NoError(t, err)

		// Bob should see all objects in bob-bucket
		objectKeys := getObjectKeys(listResp.Contents)
		assert.Contains(t, objectKeys, "public/info.txt")
		assert.Contains(t, objectKeys, "private/confidential.txt")
	})
}

// Helpers for Admin API calls within integration tests

func createIAMUser(t *testing.T, ts *TestServer, accessKey, secretKey string) {
	body := fmt.Sprintf(`{"secretKey": "%q", "status": "enabled"}`, secretKey)
	encrypted, err := madmin.EncryptData(ts.SecretKey, []byte(body))
	require.NoError(t, err)

	url := ts.URL("/minio/admin/v3/add-user?accessKey=" + accessKey)
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(encrypted))
	ts.SignRequest(req, encrypted)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to create IAM user %s", accessKey)
}

func createIAMPolicy(t *testing.T, ts *TestServer, name, policyDoc string) {
	url := ts.URL("/minio/admin/v3/add-canned-policy?name=" + name)
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewBuffer([]byte(policyDoc)))
	ts.SignRequest(req, []byte(policyDoc))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to create IAM policy %s", name)
}

func attachIAMPolicy(t *testing.T, ts *TestServer, policyName, userAccessKey string) {
	url := fmt.Sprintf("%s/minio/admin/v3/set-policy?policyName=%s&userOrGroup=%s&isGroup=false", ts.URL(""), policyName, userAccessKey)
	req, _ := http.NewRequest(http.MethodPost, url, http.NoBody)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to attach IAM policy %s to user %s", policyName, userAccessKey)
}

func getBucketNames(buckets []s3types.Bucket) []string {
	names := make([]string, len(buckets))
	for i, b := range buckets {
		names[i] = b.Name
	}
	return names
}

func getObjectKeys(objects []s3types.Object) []string {
	keys := make([]string, len(objects))
	for i, o := range objects {
		keys[i] = o.Key
	}
	return keys
}
