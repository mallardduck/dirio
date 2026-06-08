package dioclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	compatminio "github.com/mallardduck/dirio/sdk/dioclient/compat/minio"
)

// DirioClient makes authenticated requests to the DirIO-specific REST API at
// /.dirio/api/v1/. These endpoints are exclusive to DirIO and are not
// available on generic S3 or MinIO servers.
//
// It is safe for concurrent use.
type DirioClient struct {
	endpoint   string
	accessKey  string
	secretKey  string
	region     string
	httpClient *http.Client
}

// NewDirioClient creates a DirioClient from a Config.
// No network calls are made until the first operation.
func NewDirioClient(cfg Config) *DirioClient {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = "http://localhost:9000"
	}
	return &DirioClient{
		endpoint:   endpoint,
		accessKey:  cfg.AccessKey,
		secretKey:  cfg.SecretKey,
		region:     region,
		httpClient: &http.Client{},
	}
}

// OwnerInfo is the ownership record returned by the DirIO ownership endpoints.
type OwnerInfo struct {
	UUID      string `json:"uuid"`
	AccessKey string `json:"accessKey"`
	Username  string `json:"username"`
}

// SimulateRequest is the payload for POST /.dirio/api/v1/simulate.
type SimulateRequest struct {
	AccessKey string `json:"accessKey"`
	Bucket    string `json:"bucket"`
	Action    string `json:"action"`
	Key       string `json:"key,omitempty"`
}

// SimulateResult is the response from POST /.dirio/api/v1/simulate.
type SimulateResult struct {
	Allowed     bool   `json:"allowed"`
	Reason      string `json:"reason"`
	MatchedRule string `json:"matchedRule"`
}

// EffectivePermissions is the response from
// GET /.dirio/api/v1/buckets/{bucket}/permissions/{accessKey}.
type EffectivePermissions struct {
	AccessKey      string   `json:"accessKey"`
	Bucket         string   `json:"bucket"`
	AllowedActions []string `json:"allowedActions"`
	DeniedActions  []string `json:"deniedActions"`
}

// GetBucketOwner returns the ownership record for the named bucket.
func (c *DirioClient) GetBucketOwner(ctx context.Context, bucket string) (*OwnerInfo, error) {
	path := fmt.Sprintf("/.dirio/api/v1/buckets/%s/owner", bucket)
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkDirioStatus(resp); err != nil {
		return nil, fmt.Errorf("get bucket owner: %w", err)
	}
	var out OwnerInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("get bucket owner: decode response: %w", err)
	}
	return &out, nil
}

// TransferBucketOwner transfers ownership of bucket to the user identified by
// newAccessKey. The calling credential must be an admin.
func (c *DirioClient) TransferBucketOwner(ctx context.Context, bucket, newAccessKey string) (*OwnerInfo, error) {
	path := fmt.Sprintf("/.dirio/api/v1/buckets/%s/owner", bucket)
	body, _ := json.Marshal(map[string]string{"accessKey": newAccessKey})
	resp, err := c.do(ctx, http.MethodPut, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkDirioStatus(resp); err != nil {
		return nil, fmt.Errorf("transfer bucket owner: %w", err)
	}
	var out OwnerInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("transfer bucket owner: decode response: %w", err)
	}
	return &out, nil
}

// GetObjectOwner returns the ownership record for the named object.
func (c *DirioClient) GetObjectOwner(ctx context.Context, bucket, key string) (*OwnerInfo, error) {
	path := fmt.Sprintf("/.dirio/api/v1/buckets/%s/objects/%s", bucket, key)
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkDirioStatus(resp); err != nil {
		return nil, fmt.Errorf("get object owner: %w", err)
	}
	var out OwnerInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("get object owner: decode response: %w", err)
	}
	return &out, nil
}

// Simulate evaluates whether accessKey would be allowed to perform action on
// bucket (and optionally key). Returns a SimulateResult with the decision and
// reason.
func (c *DirioClient) Simulate(ctx context.Context, req SimulateRequest) (*SimulateResult, error) {
	body, _ := json.Marshal(req)
	resp, err := c.do(ctx, http.MethodPost, "/.dirio/api/v1/simulate", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkDirioStatus(resp); err != nil {
		return nil, fmt.Errorf("simulate: %w", err)
	}
	var out SimulateResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("simulate: decode response: %w", err)
	}
	return &out, nil
}

// GetEffectivePermissions returns all known S3 actions partitioned into allowed
// and denied lists for accessKey on bucket.
func (c *DirioClient) GetEffectivePermissions(ctx context.Context, bucket, accessKey string) (*EffectivePermissions, error) {
	path := fmt.Sprintf("/.dirio/api/v1/buckets/%s/permissions/%s", bucket, accessKey)
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkDirioStatus(resp); err != nil {
		return nil, fmt.Errorf("get effective permissions: %w", err)
	}
	var out EffectivePermissions
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("get effective permissions: decode response: %w", err)
	}
	return &out, nil
}

// do builds, signs, and executes an HTTP request. body may be nil.
func (c *DirioClient) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	u := c.endpoint + path

	var bodyHash string
	if len(body) == 0 {
		// SHA256("") — required for DirIO's SigV4 payload-hash validation.
		bodyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	} else {
		h := sha256.Sum256(body)
		bodyHash = hex.EncodeToString(h[:])
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("dioclient/dirio: build request %s %s: %w", method, path, err)
	}

	req.Header.Set("X-Amz-Content-Sha256", bodyHash)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(len(body))
	}

	// SignV4 takes the request by value; re-attach the body to the returned pointer
	// because the value copy does not carry the io.Reader.
	signed := compatminio.SignRequestV4(*req, c.accessKey, c.secretKey, c.region)
	if len(body) > 0 {
		signed.Body = io.NopCloser(bytes.NewReader(body))
		signed.ContentLength = int64(len(body))
	}

	return c.httpClient.Do(signed)
}

// dirioAPIError is the server's JSON error envelope.
type dirioAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// checkDirioStatus returns a descriptive error when the response status is not 2xx.
func checkDirioStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	var env struct {
		Err dirioAPIError `json:"error"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &env); err == nil && env.Err.Code != "" {
		return fmt.Errorf("HTTP %d %s: %s", resp.StatusCode, env.Err.Code, env.Err.Message)
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
