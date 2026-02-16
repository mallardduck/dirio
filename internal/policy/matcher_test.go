package policy

import (
	"testing"

	"github.com/google/uuid"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/policy/variables"
	"github.com/mallardduck/dirio/pkg/iam"
)

func TestMatchPrincipal(t *testing.T) {
	anonymousPrincipal := &Principal{IsAnonymous: true}
	userPrincipal := &Principal{
		User:        &metadata.User{AccessKey: "alice"},
		IsAnonymous: false,
	}

	tests := []struct {
		name          string
		stmtPrincipal interface{}
		reqPrincipal  *Principal
		expected      bool
	}{
		// Wildcard "*" matches everyone
		{"wildcard string matches anonymous", "*", anonymousPrincipal, true},
		{"wildcard string matches user", "*", userPrincipal, true},

		// Map with AWS: "*"
		{"AWS wildcard matches anonymous", map[string]interface{}{"AWS": "*"}, anonymousPrincipal, true},
		{"AWS wildcard matches user", map[string]interface{}{"AWS": "*"}, userPrincipal, true},

		// Specific user ARN
		{
			"specific ARN matches user",
			map[string]interface{}{"AWS": "arn:aws:iam::123456789012:user/alice"},
			userPrincipal,
			true,
		},
		{
			"specific ARN does not match different user",
			map[string]interface{}{"AWS": "arn:aws:iam::123456789012:user/bob"},
			userPrincipal,
			false,
		},
		{
			"specific ARN does not match anonymous",
			map[string]interface{}{"AWS": "arn:aws:iam::123456789012:user/alice"},
			anonymousPrincipal,
			false,
		},

		// Array of ARNs
		{
			"array includes matching ARN",
			map[string]interface{}{"AWS": []interface{}{"arn:aws:iam::123456789012:user/alice"}},
			userPrincipal,
			true,
		},
		{
			"array includes wildcard",
			map[string]interface{}{"AWS": []interface{}{"arn:aws:iam::123456789012:user/bob", "*"}},
			anonymousPrincipal,
			true,
		},

		// Nil principal
		{"nil does not match", nil, userPrincipal, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPrincipal(tt.stmtPrincipal, tt.reqPrincipal)
			if got != tt.expected {
				t.Errorf("matchPrincipal() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchAction(t *testing.T) {
	tests := []struct {
		name       string
		stmtAction interface{}
		reqAction  string
		expected   bool
	}{
		// Exact match
		{"exact match", "s3:GetObject", "s3:GetObject", true},
		{"exact mismatch", "s3:GetObject", "s3:PutObject", false},

		// Wildcard
		{"wildcard matches all", "*", "s3:GetObject", true},
		{"s3 wildcard matches s3 actions", "s3:*", "s3:GetObject", true},
		{"s3 wildcard matches any s3 action", "s3:*", "s3:PutObject", true},
		{"s3 Get wildcard matches Get actions", "s3:Get*", "s3:GetObject", true},
		{"s3 Get wildcard matches GetBucketPolicy", "s3:Get*", "s3:GetBucketPolicy", true},
		{"s3 Get wildcard does not match Put", "s3:Get*", "s3:PutObject", false},

		// Array of actions
		{"array includes action", []interface{}{"s3:GetObject", "s3:PutObject"}, "s3:GetObject", true},
		{"array does not include action", []interface{}{"s3:GetObject", "s3:PutObject"}, "s3:DeleteObject", false},
		{"array with wildcard", []interface{}{"s3:Get*", "s3:List*"}, "s3:ListBucket", true},

		// String array
		{"string array includes action", []string{"s3:GetObject", "s3:PutObject"}, "s3:GetObject", true},

		// Nil
		{"nil does not match", nil, "s3:GetObject", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchAction(tt.stmtAction, tt.reqAction)
			if got != tt.expected {
				t.Errorf("matchAction() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchResource(t *testing.T) {
	bucketResource := &Resource{Bucket: "my-bucket", Key: ""}
	objectResource := &Resource{Bucket: "my-bucket", Key: "path/to/file.txt"}
	prefixResource := &Resource{Bucket: "my-bucket", Key: "prefix/file.txt"}

	tests := []struct {
		name         string
		stmtResource interface{}
		reqResource  *Resource
		expected     bool
	}{
		// Exact match
		{"exact bucket match", "arn:aws:s3:::my-bucket", bucketResource, true},
		{"exact object match", "arn:aws:s3:::my-bucket/path/to/file.txt", objectResource, true},
		{"bucket ARN does not match object", "arn:aws:s3:::my-bucket", objectResource, false},

		// Wildcard
		{"wildcard matches everything", "*", bucketResource, true},
		{"bucket/* matches objects", "arn:aws:s3:::my-bucket/*", objectResource, true},
		{"bucket/* does not match bucket itself", "arn:aws:s3:::my-bucket/*", bucketResource, false},

		// Prefix matching
		{"prefix/* matches objects with prefix", "arn:aws:s3:::my-bucket/prefix/*", prefixResource, true},
		{"prefix/* does not match other objects", "arn:aws:s3:::my-bucket/prefix/*", objectResource, false},

		// Array of resources
		{
			"array includes resource",
			[]interface{}{"arn:aws:s3:::my-bucket", "arn:aws:s3:::other-bucket"},
			bucketResource,
			true,
		},
		{
			"array with wildcard",
			[]interface{}{"arn:aws:s3:::my-bucket/*", "arn:aws:s3:::other-bucket/*"},
			objectResource,
			true,
		},

		// String array
		{
			"string array includes resource",
			[]string{"arn:aws:s3:::my-bucket", "arn:aws:s3:::other-bucket"},
			bucketResource,
			true,
		},

		// Nil
		{"nil does not match", nil, bucketResource, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchResource(tt.stmtResource, tt.reqResource)
			if got != tt.expected {
				t.Errorf("matchResource() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEvaluateStatement(t *testing.T) {
	anonymousPrincipal := &Principal{IsAnonymous: true}
	userPrincipal := &Principal{
		User:        &metadata.User{AccessKey: "alice"},
		IsAnonymous: false,
	}

	tests := []struct {
		name     string
		stmt     *iam.Statement
		req      *RequestContext
		expected Decision
	}{
		{
			name: "allow public read",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:GetObject",
				Resource:  "arn:aws:s3:::public-bucket/*",
			},
			req: &RequestContext{
				Principal: anonymousPrincipal,
				Action:    "s3:GetObject",
				Resource:  &Resource{Bucket: "public-bucket", Key: "file.txt"},
			},
			expected: DecisionAllow,
		},
		{
			name: "deny for wrong action",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:GetObject",
				Resource:  "arn:aws:s3:::public-bucket/*",
			},
			req: &RequestContext{
				Principal: anonymousPrincipal,
				Action:    "s3:PutObject",
				Resource:  &Resource{Bucket: "public-bucket", Key: "file.txt"},
			},
			expected: DecisionDeny,
		},
		{
			name: "deny for wrong resource",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:GetObject",
				Resource:  "arn:aws:s3:::public-bucket/*",
			},
			req: &RequestContext{
				Principal: anonymousPrincipal,
				Action:    "s3:GetObject",
				Resource:  &Resource{Bucket: "private-bucket", Key: "file.txt"},
			},
			expected: DecisionDeny,
		},
		{
			name: "explicit deny wins",
			stmt: &iam.Statement{
				Effect:    "Deny",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
			},
			req: &RequestContext{
				Principal: userPrincipal,
				Action:    "s3:GetObject",
				Resource:  &Resource{Bucket: "bucket", Key: "file.txt"},
			},
			expected: DecisionExplicitDeny,
		},
		{
			name: "specific user access",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: map[string]interface{}{"AWS": "arn:aws:iam::123456789012:user/alice"},
				Action:    "s3:*",
				Resource:  "arn:aws:s3:::alice-bucket/*",
			},
			req: &RequestContext{
				Principal: userPrincipal,
				Action:    "s3:GetObject",
				Resource:  &Resource{Bucket: "alice-bucket", Key: "file.txt"},
			},
			expected: DecisionAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateStatement(tt.stmt, tt.req)
			if got != tt.expected {
				t.Errorf("evaluateStatement() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestResource_ARN(t *testing.T) {
	tests := []struct {
		name     string
		resource *Resource
		expected string
	}{
		{"service level", &Resource{}, "*"},
		{"bucket", &Resource{Bucket: "my-bucket"}, "arn:aws:s3:::my-bucket"},
		{"object", &Resource{Bucket: "my-bucket", Key: "file.txt"}, "arn:aws:s3:::my-bucket/file.txt"},
		{"nested object", &Resource{Bucket: "my-bucket", Key: "path/to/file.txt"}, "arn:aws:s3:::my-bucket/path/to/file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resource.ARN()
			if got != tt.expected {
				t.Errorf("ARN() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestMatchResourceWithVariables(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	// Create variable context for a user named "alice"
	varCtx := &variables.Context{
		Username: "alice",
		UserID:   testUUID,
	}

	// Test resources
	aliceResource := &Resource{Bucket: "my-bucket", Key: "alice/file.txt"}
	bobResource := &Resource{Bucket: "my-bucket", Key: "bob/file.txt"}
	uuidResource := &Resource{Bucket: "my-bucket", Key: "550e8400-e29b-41d4-a716-446655440000/file.txt"}

	tests := []struct {
		name         string
		stmtResource interface{}
		reqResource  *Resource
		varCtx       *variables.Context
		expected     bool
	}{
		// Variable substitution - username
		{
			name:         "username variable matches user path",
			stmtResource: "arn:aws:s3:::my-bucket/${aws:username}/*",
			reqResource:  aliceResource,
			varCtx:       varCtx,
			expected:     true,
		},
		{
			name:         "username variable does not match other user",
			stmtResource: "arn:aws:s3:::my-bucket/${aws:username}/*",
			reqResource:  bobResource,
			varCtx:       varCtx,
			expected:     false,
		},

		// Variable substitution - userid
		{
			name:         "userid variable matches UUID path",
			stmtResource: "arn:aws:s3:::my-bucket/${aws:userid}/*",
			reqResource:  uuidResource,
			varCtx:       varCtx,
			expected:     true,
		},
		{
			name:         "userid variable does not match username path",
			stmtResource: "arn:aws:s3:::my-bucket/${aws:userid}/*",
			reqResource:  aliceResource,
			varCtx:       varCtx,
			expected:     false,
		},

		// Array with variables
		{
			name: "array with variable includes match",
			stmtResource: []string{
				"arn:aws:s3:::my-bucket/${aws:username}/*",
				"arn:aws:s3:::my-bucket/public/*",
			},
			reqResource: aliceResource,
			varCtx:      varCtx,
			expected:    true,
		},

		// Nil variable context - fall back to regular matching
		{
			name:         "nil context falls back to regular matching",
			stmtResource: "arn:aws:s3:::my-bucket/*",
			reqResource:  aliceResource,
			varCtx:       nil,
			expected:     true,
		},
		{
			name:         "nil context with variable pattern does not match",
			stmtResource: "arn:aws:s3:::my-bucket/${aws:username}/*",
			reqResource:  aliceResource,
			varCtx:       nil,
			expected:     false, // No substitution, so literal pattern doesn't match
		},

		// Unknown variable - fall back to original pattern
		{
			name:         "unknown variable falls back to original pattern",
			stmtResource: "arn:aws:s3:::my-bucket/${aws:unknown}/*",
			reqResource:  aliceResource,
			varCtx:       varCtx,
			expected:     false, // Pattern with variable doesn't match after fallback
		},

		// Static patterns still work
		{
			name:         "static pattern with context",
			stmtResource: "arn:aws:s3:::my-bucket/*",
			reqResource:  aliceResource,
			varCtx:       varCtx,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchResourceWithVariables(tt.stmtResource, tt.reqResource, tt.varCtx)
			if got != tt.expected {
				t.Errorf("matchResourceWithVariables() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEvaluateStatementWithVariables(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	userPrincipal := &Principal{
		User:        &metadata.User{AccessKey: "alice", UUID: testUUID, Username: "alice"},
		IsAnonymous: false,
	}

	varCtx := &variables.Context{
		Username: "alice",
		UserID:   testUUID,
	}

	tests := []struct {
		name     string
		stmt     *iam.Statement
		req      *RequestContext
		expected Decision
	}{
		{
			name: "allow user access to their own path with username variable",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "arn:aws:s3:::shared-bucket/${aws:username}/*",
			},
			req: &RequestContext{
				Principal:  userPrincipal,
				Action:     "s3:GetObject",
				Resource:   &Resource{Bucket: "shared-bucket", Key: "alice/file.txt"},
				VarContext: varCtx,
			},
			expected: DecisionAllow,
		},
		{
			name: "deny user access to other user path with username variable",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "arn:aws:s3:::shared-bucket/${aws:username}/*",
			},
			req: &RequestContext{
				Principal:  userPrincipal,
				Action:     "s3:GetObject",
				Resource:   &Resource{Bucket: "shared-bucket", Key: "bob/file.txt"},
				VarContext: varCtx,
			},
			expected: DecisionDeny,
		},
		{
			name: "allow user access to UUID-based path with userid variable",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "arn:aws:s3:::uuid-bucket/${aws:userid}/*",
			},
			req: &RequestContext{
				Principal:  userPrincipal,
				Action:     "s3:PutObject",
				Resource:   &Resource{Bucket: "uuid-bucket", Key: "550e8400-e29b-41d4-a716-446655440000/data.txt"},
				VarContext: varCtx,
			},
			expected: DecisionAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateStatement(tt.stmt, tt.req)
			if got != tt.expected {
				t.Errorf("evaluateStatement() = %v, want %v", got, tt.expected)
			}
		})
	}
}
