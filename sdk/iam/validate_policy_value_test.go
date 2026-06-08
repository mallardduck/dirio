package iam

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validatePolicyValue is tested through Statement.Validate for most paths.
// These cases cover the branches not reachable via Statement alone.
func TestValidatePolicyValue(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		fieldName string
		wantErr   bool
		errMsg    string
	}{
		// nil passes through
		{"nil value", nil, "Action", false, ""},

		// string happy path
		{"non-empty string", "s3:GetObject", "Action", false, ""},
		{"empty string", "", "Action", true, "cannot be empty"},

		// []string
		{"non-empty []string", []string{"s3:GetObject"}, "Action", false, ""},
		{"empty []string", []string{}, "Action", true, "cannot be empty"},
		{"[]string with empty element", []string{"s3:GetObject", ""}, "Action", true, "cannot be empty"},

		// []any - Action/Resource must have string items
		{"[]any all strings", []any{"s3:GetObject", "s3:PutObject"}, "Action", false, ""},
		{"[]any empty", []any{}, "Action", true, "cannot be empty"},
		{"[]any with empty string element", []any{"s3:GetObject", ""}, "Action", true, "cannot be empty"},
		{"[]any non-string in Action", []any{"s3:GetObject", 42}, "Action", true, "must be string"},
		// Principal/NotPrincipal allow non-string items in []any
		{"[]any non-string in Principal", []any{"arn:aws:iam::123:root", map[string]any{"AWS": "*"}}, "Principal", false, ""},
		{"[]any non-string in NotPrincipal", []any{map[string]any{"AWS": "*"}}, "NotPrincipal", false, ""},
		{"[]any empty string in Principal", []any{""}, "Principal", true, "cannot be empty"},

		// map[string]any - only valid for Principal/NotPrincipal
		{"non-empty map for Principal", map[string]any{"AWS": "*"}, "Principal", false, ""},
		{"non-empty map for NotPrincipal", map[string]any{"Federated": "cognito-identity.amazonaws.com"}, "NotPrincipal", false, ""},
		{"empty map for Principal", map[string]any{}, "Principal", true, "cannot be empty"},
		{"map for Action (not allowed)", map[string]any{"S3": "GetObject"}, "Action", true, "cannot be a map"},
		{"map for Resource (not allowed)", map[string]any{}, "Resource", true, "cannot be a map"},

		// unknown type
		{"int type", 42, "Action", true, "unsupported type"},
		{"bool type", true, "Resource", true, "unsupported type"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePolicyValue(tc.value, tc.fieldName)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestStatement_Validate_AdditionalCases covers Statement.Validate branches not
// reached by the table in validation_test.go.
func TestStatement_Validate_AdditionalCases(t *testing.T) {
	t.Run("NotAction with valid resource", func(t *testing.T) {
		s := Statement{Effect: "Deny", NotAction: "s3:DeleteBucket", Resource: "*"}
		assert.NoError(t, s.Validate())
	})

	t.Run("NotResource with valid action", func(t *testing.T) {
		s := Statement{Effect: "Allow", Action: "s3:GetObject", NotResource: "arn:aws:s3:::private/*"}
		assert.NoError(t, s.Validate())
	})

	t.Run("valid []any action", func(t *testing.T) {
		s := Statement{Effect: "Allow", Action: []any{"s3:GetObject", "s3:PutObject"}, Resource: "*"}
		assert.NoError(t, s.Validate())
	})

	t.Run("valid Principal map", func(t *testing.T) {
		s := Statement{
			Effect:    "Allow",
			Principal: map[string]any{"AWS": "*"},
			Action:    "s3:GetObject",
			Resource:  "*",
		}
		assert.NoError(t, s.Validate())
	})

	t.Run("empty map Principal", func(t *testing.T) {
		s := Statement{
			Effect:    "Allow",
			Principal: map[string]any{},
			Action:    "s3:GetObject",
			Resource:  "*",
		}
		require.Error(t, s.Validate())
		assert.Contains(t, s.Validate().Error(), "Principal")
	})

	t.Run("invalid type in NotResource", func(t *testing.T) {
		s := Statement{Effect: "Allow", Action: "s3:*", NotResource: 42}
		require.Error(t, s.Validate())
	})
}
