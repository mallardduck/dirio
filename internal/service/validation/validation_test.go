package validation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/sdk/iam"
)

// ── common ────────────────────────────────────────────────────────────────────

func TestIsAlphanumeric(t *testing.T) {
	assert.True(t, IsAlphanumeric("abc123"))
	assert.True(t, IsAlphanumeric("ABC"))
	assert.False(t, IsAlphanumeric(""))
	assert.False(t, IsAlphanumeric("abc-123"))
	assert.False(t, IsAlphanumeric("abc 123"))
}

func TestIsAlphanumericWithHyphens(t *testing.T) {
	assert.True(t, IsAlphanumericWithHyphens("abc-123"))
	assert.True(t, IsAlphanumericWithHyphens("abc"))
	assert.False(t, IsAlphanumericWithHyphens(""))
	assert.False(t, IsAlphanumericWithHyphens("abc.123"))
	assert.False(t, IsAlphanumericWithHyphens("abc 123"))
}

func TestInRange(t *testing.T) {
	assert.True(t, InRange("abc", 3, 5))
	assert.True(t, InRange("abc", 3, 3))
	assert.False(t, InRange("ab", 3, 5))
	assert.False(t, InRange("abcdef", 3, 5))
}

// ── user ──────────────────────────────────────────────────────────────────────

func TestValidateAccessKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
		errMsg  string
	}{
		{"valid short", "bob", false, ""},
		{"valid typical", "admin123", false, ""},
		{"valid max length", strings.Repeat("a", MaxAccessKeyLength), false, ""},
		{"empty", "", true, "required"},
		{"too short", "ab", true, "between"},
		{"too long", strings.Repeat("a", MaxAccessKeyLength+1), true, "between"},
		{"contains hyphen", "access-key", true, "alphanumeric"},
		{"contains space", "access key", true, "alphanumeric"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAccessKey(tc.key)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSecretKey(t *testing.T) {
	require.NoError(t, ValidateSecretKey("password1"))
	require.NoError(t, ValidateSecretKey(strings.Repeat("x", 64)))

	err := ValidateSecretKey("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")

	err = ValidateSecretKey("short")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least")
}

func TestValidateStatus(t *testing.T) {
	assert.NoError(t, ValidateStatus(iam.UserStatusActive))
	assert.NoError(t, ValidateStatus(iam.UserStatusDisabled))
	assert.Error(t, ValidateStatus(iam.UserStatus("invalid")))
}

// ── bucket ────────────────────────────────────────────────────────────────────

func TestValidateBucketName(t *testing.T) {
	tests := []struct {
		name    string
		bucket  string
		wantErr bool
		errMsg  string
	}{
		{"valid simple", "my-bucket", false, ""},
		{"valid with dots", "my.bucket.name", false, ""},
		{"valid min length", "abc", false, ""},
		{"valid max length", strings.Repeat("a", MaxBucketNameLength), false, ""},
		{"empty", "", true, "required"},
		{"too short", "ab", true, "between"},
		{"too long", strings.Repeat("a", MaxBucketNameLength+1), true, "between"},
		{"starts with uppercase", "MyBucket", true, "lowercase"},
		{"starts with hyphen", "-bucket", true, "lowercase"},
		{"ends with hyphen", "bucket-", true, "lowercase"},
		{"consecutive dots", "my..bucket", true, "consecutive periods"},
		{"dot-hyphen combo", "my.-bucket", true, "adjacent"},
		{"hyphen-dot combo", "my-.bucket", true, "adjacent"},
		{"IP address", "192.168.1.1", true, "IP address"},
		{"xn-- prefix", "xn--bucket", true, "xn--"},
		{"s3alias suffix", "my-bucket-s3alias", true, "-s3alias"},
		{"uppercase letter", "myBucket", true, "lowercase"},
		{"special chars", "my_bucket", true, "lowercase"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBucketName(tc.bucket)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tc.errMsg))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ── object key ────────────────────────────────────────────────────────────────

func TestValidateObjectKey(t *testing.T) {
	assert.NoError(t, ValidateObjectKey("path/to/object.txt"))
	assert.NoError(t, ValidateObjectKey("unicode-日本語"))
	assert.NoError(t, ValidateObjectKey("with\ttab"))
	assert.NoError(t, ValidateObjectKey("with\nnewline"))

	tests := []struct {
		name    string
		key     string
		wantErr bool
		errMsg  string
	}{
		{"empty", "", true, "empty"},
		{"leading slash", "/bad-key", true, "start with"},
		{"too long", strings.Repeat("a", MaxObjectKeyLength+1), true, "exceed"},
		{"control char NUL", "key\x00bad", true, "control"},
		{"control char BEL", "key\x07bad", true, "control"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateObjectKey(tc.key)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ── policy ────────────────────────────────────────────────────────────────────

func TestValidatePolicyName(t *testing.T) {
	assert.NoError(t, ValidatePolicyName("my-policy"))
	assert.NoError(t, ValidatePolicyName("ReadOnly"))
	assert.NoError(t, ValidatePolicyName(strings.Repeat("a", MaxPolicyNameLength)))

	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"empty", "", true, "required"},
		{"too long", strings.Repeat("a", MaxPolicyNameLength+1), true, "between"},
		{"with dot", "my.policy", true, "alphanumeric"},
		{"with space", "my policy", true, "alphanumeric"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePolicyName(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePolicyDocument(t *testing.T) {
	validDoc := &iam.PolicyDocument{
		Version: RequiredVersion,
		Statement: []iam.Statement{
			{Effect: "Allow", Action: "s3:GetObject"},
		},
	}
	require.NoError(t, ValidatePolicyDocument(validDoc))

	t.Run("nil document", func(t *testing.T) {
		err := ValidatePolicyDocument(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("wrong version", func(t *testing.T) {
		doc := &iam.PolicyDocument{Version: "2008-10-17", Statement: []iam.Statement{{Effect: "Allow", Action: "s3:*"}}}
		err := ValidatePolicyDocument(doc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), RequiredVersion)
	})

	t.Run("no statements", func(t *testing.T) {
		doc := &iam.PolicyDocument{Version: RequiredVersion}
		err := ValidatePolicyDocument(doc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "statement")
	})

	t.Run("invalid effect", func(t *testing.T) {
		doc := &iam.PolicyDocument{
			Version:   RequiredVersion,
			Statement: []iam.Statement{{Effect: "Grant", Action: "s3:*"}},
		}
		err := ValidatePolicyDocument(doc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Effect")
	})

	t.Run("nil action", func(t *testing.T) {
		doc := &iam.PolicyDocument{
			Version:   RequiredVersion,
			Statement: []iam.Statement{{Effect: "Allow", Action: nil}},
		}
		err := ValidatePolicyDocument(doc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Action")
	})

	t.Run("empty string action", func(t *testing.T) {
		doc := &iam.PolicyDocument{
			Version:   RequiredVersion,
			Statement: []iam.Statement{{Effect: "Allow", Action: ""}},
		}
		err := ValidatePolicyDocument(doc)
		require.Error(t, err)
	})

	t.Run("empty []any action", func(t *testing.T) {
		doc := &iam.PolicyDocument{
			Version:   RequiredVersion,
			Statement: []iam.Statement{{Effect: "Allow", Action: []any{}}},
		}
		err := ValidatePolicyDocument(doc)
		require.Error(t, err)
	})

	t.Run("empty []string action", func(t *testing.T) {
		doc := &iam.PolicyDocument{
			Version:   RequiredVersion,
			Statement: []iam.Statement{{Effect: "Allow", Action: []string{}}},
		}
		err := ValidatePolicyDocument(doc)
		require.Error(t, err)
	})

	t.Run("invalid action type", func(t *testing.T) {
		doc := &iam.PolicyDocument{
			Version:   RequiredVersion,
			Statement: []iam.Statement{{Effect: "Allow", Action: 42}},
		}
		err := ValidatePolicyDocument(doc)
		require.Error(t, err)
	})

	t.Run("Deny effect is valid", func(t *testing.T) {
		doc := &iam.PolicyDocument{
			Version:   RequiredVersion,
			Statement: []iam.Statement{{Effect: "Deny", Action: "s3:DeleteObject"}},
		}
		assert.NoError(t, ValidatePolicyDocument(doc))
	})
}
