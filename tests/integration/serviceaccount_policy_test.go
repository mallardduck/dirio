package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/minio/madmin-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/internal/consts"
)

// createServiceAccount creates a service account via the madmin-encrypted admin API.
// parentUser is the parent user's access key; policyMode is "inherit" or "override".
func createServiceAccount(t *testing.T, ts *TestServer, accessKey, secretKey, parentUser, policyMode string) {
	t.Helper()

	body := map[string]string{
		"accessKey":  accessKey,
		"secretKey":  secretKey,
		"parentUser": parentUser,
		"policyMode": policyMode,
	}
	bodyJSON, err := json.Marshal(body)
	require.NoError(t, err)

	encrypted, err := madmin.EncryptData(ts.SecretKey, bodyJSON)
	require.NoError(t, err)

	url := ts.URL("/minio/admin/v3/add-service-account")
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(encrypted))
	require.NoError(t, err)
	req.ContentLength = int64(len(encrypted))
	req.Header.Set("Content-Type", "application/octet-stream")
	ts.SignRequest(req, encrypted)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to create service account %s", accessKey)
}

// detachIAMPolicy detaches a named policy from an IAM user (by access key).
func detachIAMPolicy(t *testing.T, ts *TestServer, policyName, userAccessKey string) {
	t.Helper()
	url := fmt.Sprintf("%s/minio/admin/v3/idp/builtin/policy/detach?policyName=%s&userOrGroup=%s&isGroup=false",
		ts.URL(""), policyName, userAccessKey)
	req, _ := http.NewRequest(http.MethodPost, url, http.NoBody)
	ts.SignRequest(req, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to detach IAM policy %s from user %s", policyName, userAccessKey)
}

// setSAEmbeddedPolicy patches the service account JSON file on disk to set its
// embeddedPolicyJSON field. Used to test PolicyMode=override with an inline policy
// without requiring an HTTP endpoint that exposes direct SA policy attachment.
func setSAEmbeddedPolicy(t *testing.T, ts *TestServer, accessKey, policyJSON string) {
	t.Helper()

	saPath := filepath.Join(ts.DataDir, consts.DirIOMetadataDir, "iam", "service-accounts", accessKey+".json")
	data, err := os.ReadFile(saPath)
	require.NoError(t, err, "SA file should exist at %s", saPath)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	raw["embeddedPolicyJSON"] = policyJSON

	updated, err := json.Marshal(raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(saPath, updated, 0o644))
}

// setSAExpiresAt patches the service account JSON file on disk to set the expiresAt
// field. This lets tests exercise expiration enforcement without wall-clock advancement.
func setSAExpiresAt(t *testing.T, ts *TestServer, accessKey string, expiresAt time.Time) {
	t.Helper()

	saPath := filepath.Join(ts.DataDir, consts.DirIOMetadataDir, "iam", "service-accounts", accessKey+".json")
	data, err := os.ReadFile(saPath)
	require.NoError(t, err, "SA file should exist at %s", saPath)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	raw["expiresAt"] = expiresAt.UTC().Format(time.RFC3339Nano)

	updated, err := json.Marshal(raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(saPath, updated, 0o644))
}

// saBucketPolicyDoc returns a minimal IAM policy granting read access to one bucket.
func saBucketPolicyDoc(bucket string) string {
	return fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:GetObject"],
			"Resource": ["arn:aws:s3:::%s", "arn:aws:s3:::%s/*"]
		}]
	}`, bucket, bucket)
}

// TestSA_InheritMode verifies that a service account with PolicyMode=inherit gains
// exactly the permissions of its parent user via the parent's attached IAM policies.
func TestSA_InheritMode(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "sainheritbucket"
	ts.CreateBucket(t, bucket)
	ts.PutObject(t, bucket, "obj.txt", "hello")

	// Create a parent user and attach a policy granting bucket access.
	parentAK, parentSK := "saparentuser1", "saparentsecret1"
	createIAMUser(t, ts, parentAK, parentSK)
	createIAMPolicy(t, ts, "saparentpolicy", saBucketPolicyDoc(bucket))
	attachIAMPolicy(t, ts, "saparentpolicy", parentAK)

	// Create the service account in inherit mode.
	saAK, saSK := "sainheritkey1", "sainheritkey1234"
	createServiceAccount(t, ts, saAK, saSK, parentAK, "inherit")

	t.Run("SA inherits parent policy and can read object", func(t *testing.T) {
		code := getObject(t, ts, bucket, "obj.txt", saAK, saSK)
		assert.Equal(t, http.StatusOK, code, "SA should have read access via inherited parent policy")
	})

	// Detach the parent's policy — the SA should lose access immediately.
	detachIAMPolicy(t, ts, "saparentpolicy", parentAK)

	t.Run("SA loses access when parent policy is detached", func(t *testing.T) {
		code := getObject(t, ts, bucket, "obj.txt", saAK, saSK)
		assert.Equal(t, http.StatusForbidden, code, "SA should lose access when parent's policy is removed")
	})
}

// TestSA_OverrideMode_NoAccess verifies that a service account with PolicyMode=override
// and no own policies is denied even though its parent user has broad access.
func TestSA_OverrideMode_NoAccess(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "saoverridedeny"
	ts.CreateBucket(t, bucket)
	ts.PutObject(t, bucket, "obj.txt", "data")

	// Parent user has access.
	parentAK, parentSK := "saovrdparent1", "saovrdparent1234"
	createIAMUser(t, ts, parentAK, parentSK)
	createIAMPolicy(t, ts, "saovrdparentpol", saBucketPolicyDoc(bucket))
	attachIAMPolicy(t, ts, "saovrdparentpol", parentAK)

	// Confirm parent can access the bucket.
	require.Equal(t, http.StatusOK, getObject(t, ts, bucket, "obj.txt", parentAK, parentSK),
		"Setup: parent user must be able to access the bucket")

	// SA in override mode with no own policies attached.
	saAK, saSK := "saovrdnopol1", "saovrdnopol12345"
	createServiceAccount(t, ts, saAK, saSK, parentAK, "override")

	t.Run("SA override with no own policies is denied", func(t *testing.T) {
		code := getObject(t, ts, bucket, "obj.txt", saAK, saSK)
		assert.Equal(t, http.StatusForbidden, code,
			"SA in override mode with no own policies should be denied even though parent has access")
	})
}

// TestSA_OverrideMode_WithOwnPolicies verifies that a service account with
// PolicyMode=override uses only its own attached policies, ignoring the parent's wider access.
func TestSA_OverrideMode_WithOwnPolicies(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucketA := "saoverridebucketa"
	bucketB := "saoverridebucketb"
	ts.CreateBucket(t, bucketA)
	ts.CreateBucket(t, bucketB)
	ts.PutObject(t, bucketA, "obj.txt", "data-a")
	ts.PutObject(t, bucketB, "obj.txt", "data-b")

	// Parent has wide access to both buckets.
	widePolicyDoc := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:GetObject"],
			"Resource": [
				"arn:aws:s3:::%s", "arn:aws:s3:::%s/*",
				"arn:aws:s3:::%s", "arn:aws:s3:::%s/*"
			]
		}]
	}`, bucketA, bucketA, bucketB, bucketB)

	parentAK, parentSK := "saovrdwidepar1", "saovrdwidepar1234"
	createIAMUser(t, ts, parentAK, parentSK)
	createIAMPolicy(t, ts, "sawidepolicy", widePolicyDoc)
	attachIAMPolicy(t, ts, "sawidepolicy", parentAK)

	// SA in override mode with only a narrow inline policy (bucket-a only).
	// Service accounts in override mode carry their policy as embedded JSON,
	// so we create the SA and then patch its embeddedPolicyJSON on disk.
	saAK, saSK := "saovrdnarrow1", "saovrdnarrow12345"
	createServiceAccount(t, ts, saAK, saSK, parentAK, "override")
	setSAEmbeddedPolicy(t, ts, saAK, saBucketPolicyDoc(bucketA))

	t.Run("SA override can access its own allowed bucket", func(t *testing.T) {
		code := getObject(t, ts, bucketA, "obj.txt", saAK, saSK)
		assert.Equal(t, http.StatusOK, code, "SA should access bucketA via its own narrow policy")
	})

	t.Run("SA override cannot access bucket not in its own policy", func(t *testing.T) {
		code := getObject(t, ts, bucketB, "obj.txt", saAK, saSK)
		assert.Equal(t, http.StatusForbidden, code,
			"SA should be denied bucketB even though parent has access to it")
	})
}

// TestSA_Expiration verifies that a service account whose expiresAt is in the past
// is rejected for S3 requests with a 403.
func TestSA_Expiration(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Cleanup()

	bucket := "saexpirebucket"
	ts.CreateBucket(t, bucket)
	ts.PutObject(t, bucket, "obj.txt", "secret data")

	// Create a parent user with bucket access.
	parentAK, parentSK := "saexpireparent1", "saexpireparent123"
	createIAMUser(t, ts, parentAK, parentSK)
	_ = parentSK
	createIAMPolicy(t, ts, "saexpirepolicy", saBucketPolicyDoc(bucket))
	attachIAMPolicy(t, ts, "saexpirepolicy", parentAK)

	// Create SA in inherit mode (so it would have access if not expired).
	saAK, saSK := "saexpirekey1", "saexpirekey12345"
	createServiceAccount(t, ts, saAK, saSK, parentAK, "inherit")

	t.Run("SA can access object before expiration", func(t *testing.T) {
		code := getObject(t, ts, bucket, "obj.txt", saAK, saSK)
		assert.Equal(t, http.StatusOK, code, "SA should have access before it is expired")
	})

	// Patch the on-disk JSON to set expiresAt to one hour in the past.
	// The server reads SA JSON on every request so no cache invalidation is needed.
	setSAExpiresAt(t, ts, saAK, time.Now().Add(-1*time.Hour))

	t.Run("SA is denied after expiration", func(t *testing.T) {
		code := getObject(t, ts, bucket, "obj.txt", saAK, saSK)
		assert.Equal(t, http.StatusForbidden, code, "Expired SA should be rejected with 403")
	})
}
