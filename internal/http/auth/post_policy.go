package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PostPolicyForm holds parsed credential fields from a multipart/form-data POST policy upload.
type PostPolicyForm struct {
	Algorithm       string // x-amz-algorithm
	Credential      string // x-amz-credential (full: ACCESSKEY/DATE/REGION/s3/aws4_request)
	Date            string // x-amz-date (ISO 8601 timestamp)
	Signature       string // x-amz-signature
	PolicyBase64    string // policy (base64-encoded policy document)
	Key             string // key (object key, may contain ${filename})
	AccessKey       string // extracted from Credential
	DateShort       string // extracted date (YYYYMMDD) from Credential
	Region          string // extracted from Credential
	ContentType     string // Content-Type
	SuccessRedirect string // success_action_redirect
	SuccessStatus   string // success_action_status
}

// PostPolicyDocument is the decoded JSON policy document from a POST policy upload.
type PostPolicyDocument struct {
	Expiration time.Time
	Conditions []json.RawMessage
}

// UnmarshalJSON customises JSON decoding to parse the Expiration string.
func (p *PostPolicyDocument) UnmarshalJSON(data []byte) error {
	var raw struct {
		Expiration string            `json:"expiration"`
		Conditions []json.RawMessage `json:"conditions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.Conditions = raw.Conditions
	if raw.Expiration != "" {
		// Try RFC3339Nano first (mc includes milliseconds: "2026-02-23T05:54:53.983Z"),
		// then fall back to plain RFC3339 for policies without fractional seconds.
		t, err := time.Parse(time.RFC3339Nano, raw.Expiration)
		if err != nil {
			t, err = time.Parse(time.RFC3339, raw.Expiration)
			if err != nil {
				return fmt.Errorf("invalid expiration format: %w", err)
			}
		}
		p.Expiration = t
	}
	return nil
}

// ParsePostPolicyForm parses the multipart form fields from a POST policy upload request.
func ParsePostPolicyForm(r *http.Request) (*PostPolicyForm, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, fmt.Errorf("failed to parse multipart form: %w", err)
	}
	if r.MultipartForm == nil {
		return nil, fmt.Errorf("no multipart form data")
	}

	get := func(name string) string {
		vals := r.MultipartForm.Value[name]
		if len(vals) == 0 {
			return ""
		}
		return vals[0]
	}

	// Content-Type may appear as "Content-Type" or "content-type"
	contentType := get("Content-Type")
	if contentType == "" {
		contentType = get("content-type")
	}

	pf := &PostPolicyForm{
		Algorithm:       get("x-amz-algorithm"),
		Credential:      get("x-amz-credential"),
		Date:            get("x-amz-date"),
		Signature:       get("x-amz-signature"),
		PolicyBase64:    get("policy"),
		Key:             get("key"),
		ContentType:     contentType,
		SuccessRedirect: get("success_action_redirect"),
		SuccessStatus:   get("success_action_status"),
	}

	// Credential format: ACCESSKEY/DATE/REGION/s3/aws4_request
	if pf.Credential != "" {
		parts := strings.Split(pf.Credential, "/")
		if len(parts) != 5 {
			return nil, fmt.Errorf("invalid credential format in POST policy form")
		}
		pf.AccessKey = parts[0]
		pf.DateShort = parts[1]
		pf.Region = parts[2]
	}

	if pf.AccessKey == "" {
		return nil, ErrMissingCredential
	}
	if pf.Signature == "" {
		return nil, ErrMissingSignature
	}
	if pf.PolicyBase64 == "" {
		return nil, fmt.Errorf("missing policy field in POST policy form")
	}

	return pf, nil
}

// ParsePostPolicyDocument base64-decodes and JSON-parses the policy document.
func ParsePostPolicyDocument(policyBase64 string) (*PostPolicyDocument, error) {
	decoded, err := base64.StdEncoding.DecodeString(policyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode policy: %w", err)
	}
	var doc PostPolicyDocument
	if err := json.Unmarshal(decoded, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse policy JSON: %w", err)
	}
	return &doc, nil
}

// VerifyPostPolicySignature verifies the HMAC-SHA256 signature of a POST policy upload.
//
// Signing key derivation (same as standard SigV4 ComputeSignature):
//
//	signingKey = HMAC(HMAC(HMAC(HMAC("AWS4"+secretKey, date), region), "s3"), "aws4_request")
//
// The signature is computed over the raw base64-encoded policy string (not the decoded JSON).
func VerifyPostPolicySignature(form *PostPolicyForm, secretKey string) error {
	if form.DateShort == "" || form.Region == "" {
		return fmt.Errorf("missing date or region in credential")
	}

	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(form.DateShort))
	kRegion := hmacSHA256(kDate, []byte(form.Region))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte(requestType))

	sig := hmacSHA256(kSigning, []byte(form.PolicyBase64))
	expectedSig := hex.EncodeToString(sig)

	if subtle.ConstantTimeCompare([]byte(expectedSig), []byte(form.Signature)) != 1 {
		return ErrSignatureMismatch
	}
	return nil
}

// ValidatePostPolicyExpiration checks whether the policy has expired.
// Reuses ErrPresignedURLExpired to keep error mapping consistent.
func ValidatePostPolicyExpiration(doc *PostPolicyDocument) error {
	if doc.Expiration.IsZero() {
		return nil
	}
	if time.Now().After(doc.Expiration) {
		return ErrPresignedURLExpired
	}
	return nil
}

// ValidatePostPolicyConditions enforces the conditions encoded in a POST policy document.
//
// Supported condition forms:
//   - Object condition: {"bucket": "val"}, {"key": "val"}, {"content-type": "val"}
//   - Array eq:              ["eq", "$field", "value"]
//   - Array starts-with:     ["starts-with", "$field", "prefix"]
//   - Array length range:    ["content-length-range", min, max]
//
// Credential/algorithm/date conditions are skipped — they are already validated by the signature.
func ValidatePostPolicyConditions(doc *PostPolicyDocument, bucket, key, contentType string, contentLength int64) error {
	for _, raw := range doc.Conditions {
		// Try array condition first
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err == nil {
			if err := validateArrayCondition(arr, bucket, key, contentType, contentLength); err != nil {
				return err
			}
			continue
		}
		// Fall back to object condition
		var obj map[string]string
		if err := json.Unmarshal(raw, &obj); err == nil {
			if err := validateObjectCondition(obj, bucket, key, contentType); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateObjectCondition handles {"field": "value"} conditions.
func validateObjectCondition(cond map[string]string, bucket, key, contentType string) error {
	for field, value := range cond {
		switch strings.ToLower(field) {
		case "bucket":
			if bucket != value {
				return fmt.Errorf("bucket condition failed: expected %q, got %q", value, bucket)
			}
		case "key":
			if key != value {
				return fmt.Errorf("key condition failed: expected %q, got %q", value, key)
			}
		case "content-type":
			if contentType != value {
				return fmt.Errorf("content-type condition failed: expected %q, got %q", value, contentType)
			}
		// Already validated by signature — skip silently
		case "x-amz-algorithm", "x-amz-credential", "x-amz-date", "x-amz-signature", "policy":
		}
	}
	return nil
}

// validateArrayCondition handles array-style conditions.
func validateArrayCondition(cond []json.RawMessage, bucket, key, contentType string, contentLength int64) error {
	if len(cond) < 2 {
		return nil
	}
	var op string
	if err := json.Unmarshal(cond[0], &op); err != nil {
		return nil
	}

	switch strings.ToLower(op) {
	case "eq":
		if len(cond) != 3 {
			return fmt.Errorf("eq condition requires 3 elements")
		}
		var field, expected string
		if err := json.Unmarshal(cond[1], &field); err != nil {
			return nil
		}
		if err := json.Unmarshal(cond[2], &expected); err != nil {
			return nil
		}
		return checkEq(field, expected, bucket, key, contentType)

	case "starts-with":
		if len(cond) != 3 {
			return fmt.Errorf("starts-with condition requires 3 elements")
		}
		var field, prefix string
		if err := json.Unmarshal(cond[1], &field); err != nil {
			return nil
		}
		if err := json.Unmarshal(cond[2], &prefix); err != nil {
			return nil
		}
		return checkStartsWith(field, prefix, bucket, key, contentType)

	case "content-length-range":
		if len(cond) != 3 {
			return fmt.Errorf("content-length-range requires 3 elements")
		}
		var minSize, maxSize int64
		if err := json.Unmarshal(cond[1], &minSize); err != nil {
			return nil
		}
		if err := json.Unmarshal(cond[2], &maxSize); err != nil {
			return nil
		}
		if contentLength < minSize || contentLength > maxSize {
			return fmt.Errorf("content length %d is outside allowed range [%d, %d]", contentLength, minSize, maxSize)
		}
	}
	return nil
}

// checkEq enforces an exact-match condition.
func checkEq(field, expected, bucket, key, contentType string) error {
	if isSkippedField(field) {
		return nil
	}
	actual := resolveField(field, bucket, key, contentType)
	if actual != expected {
		return fmt.Errorf("eq condition failed for %s: expected %q, got %q", field, expected, actual)
	}
	return nil
}

// checkStartsWith enforces a prefix-match condition.
func checkStartsWith(field, prefix, bucket, key, contentType string) error {
	if isSkippedField(field) {
		return nil
	}
	actual := resolveField(field, bucket, key, contentType)
	if !strings.HasPrefix(actual, prefix) {
		return fmt.Errorf("starts-with condition failed for %s: %q does not start with %q", field, actual, prefix)
	}
	return nil
}

// resolveField maps a form field name (with or without "$" prefix) to its actual request value.
func resolveField(field, bucket, key, contentType string) string {
	field = strings.ToLower(strings.TrimPrefix(field, "$"))
	switch field {
	case "bucket":
		return bucket
	case "key":
		return key
	case "content-type":
		return contentType
	default:
		return ""
	}
}

// isSkippedField returns true for fields already validated by signature verification.
func isSkippedField(field string) bool {
	switch strings.ToLower(strings.TrimPrefix(field, "$")) {
	case "x-amz-algorithm", "x-amz-credential", "x-amz-date", "x-amz-signature", "policy":
		return true
	}
	return false
}
