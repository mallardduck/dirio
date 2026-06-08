package dioclient_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/sdk/dioclient"
)

// newClient creates a dioclient.Client pointed at endpoint with the given credentials.
func newClient(t *testing.T, endpoint, accessKey, secretKey string) *dioclient.Client {
	t.Helper()
	c, err := dioclient.New(dioclient.Config{
		Endpoint:  endpoint,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Region:    "us-east-1",
	})
	require.NoError(t, err)
	return c
}

// minioSeedClient returns a raw minio-go client for seeding test data.
// pkg/dioclient has no write methods yet (Phase 7.2), so tests use this to create
// buckets and objects before exercising the read-only dioclient methods.
func minioSeedClient(t *testing.T, endpoint, accessKey, secretKey string, pathStyle bool) *minio.Client {
	t.Helper()

	// Strip scheme — minio-go wants just host:port.
	host := strings.TrimPrefix(endpoint, "https://")
	host = strings.TrimPrefix(host, "http://")
	secure := strings.HasPrefix(endpoint, "https://")

	lookup := minio.BucketLookupAuto
	if pathStyle {
		lookup = minio.BucketLookupPath
	}

	mc, err := minio.New(host, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       secure,
		Region:       "us-east-1",
		BucketLookup: lookup,
	})
	require.NoError(t, err)
	return mc
}

// seedBuckets creates the named buckets via mc. Already-existing buckets are ignored.
func seedBuckets(t *testing.T, mc *minio.Client, names ...string) {
	t.Helper()
	ctx := context.Background()
	for _, name := range names {
		err := mc.MakeBucket(ctx, name, minio.MakeBucketOptions{Region: "us-east-1"})
		if err != nil {
			// Ignore BucketAlreadyOwnedByYou / BucketAlreadyExists.
			resp, ok := err.(minio.ErrorResponse)
			if ok && (resp.Code == "BucketAlreadyOwnedByYou" || resp.Code == "BucketAlreadyExists") {
				continue
			}
			require.NoError(t, err, "create bucket %s", name)
		}
	}
}

// seedObjects puts each key→content pair into bucket. Bucket must already exist.
func seedObjects(t *testing.T, mc *minio.Client, bucket string, objects map[string]string) {
	t.Helper()
	ctx := context.Background()
	for key, content := range objects {
		data := []byte(content)
		_, err := mc.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)),
			minio.PutObjectOptions{ContentType: "text/plain"})
		require.NoError(t, err, "put object %s/%s", bucket, key)
	}
}
