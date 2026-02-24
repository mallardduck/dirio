package policy

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/internal/policy/variables"
	"github.com/mallardduck/dirio/pkg/iam"
)

func TestEvaluateStatementWithConditions(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	anonymousPrincipal := &Principal{IsAnonymous: true}
	userPrincipal := &Principal{
		User:        &metadata.User{AccessKey: "alice", UUID: testUUID, Username: "alice"},
		IsAnonymous: false,
	}

	allowedIP := net.ParseIP("192.168.1.100")
	deniedIP := net.ParseIP("10.0.0.100")
	currentTime := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		stmt     *iam.Statement
		req      *RequestContext
		expected Decision
	}{
		{
			name: "allow with IP restriction - allowed IP",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:GetObject",
				Resource:  "arn:aws:s3:::bucket/*",
				Condition: map[string]any{
					"IpAddress": map[string]any{
						"aws:SourceIp": "192.168.1.0/24",
					},
				},
			},
			req: &RequestContext{
				Principal:       anonymousPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{SourceIP: allowedIP},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionAllow,
		},
		{
			name: "allow with IP restriction - denied IP",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:GetObject",
				Resource:  "arn:aws:s3:::bucket/*",
				Condition: map[string]any{
					"IpAddress": map[string]any{
						"aws:SourceIp": "192.168.1.0/24",
					},
				},
			},
			req: &RequestContext{
				Principal:       anonymousPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{SourceIP: deniedIP},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionDeny,
		},
		{
			name: "allow with time window - within window",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"DateGreaterThan": map[string]any{
						"aws:CurrentTime": "2026-01-01T00:00:00Z",
					},
					"DateLessThan": map[string]any{
						"aws:CurrentTime": "2026-12-31T23:59:59Z",
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{CurrentTime: currentTime},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionAllow,
		},
		{
			name: "allow with time window - outside window",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"DateGreaterThan": map[string]any{
						"aws:CurrentTime": "2027-01-01T00:00:00Z",
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{CurrentTime: currentTime},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionDeny,
		},
		{
			name: "allow with user-agent filter - matching",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"StringLike": map[string]any{
						"aws:UserAgent": []any{"aws-cli/*", "Boto3/*"},
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{UserAgent: "aws-cli/2.0.0"},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionAllow,
		},
		{
			name: "allow with user-agent filter - not matching",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"StringLike": map[string]any{
						"aws:UserAgent": []any{"aws-cli/*", "Boto3/*"},
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{UserAgent: "Mozilla/5.0"},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionDeny,
		},
		{
			name: "allow with content size limit - within limit",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:PutObject",
				Resource:  "*",
				Condition: map[string]any{
					"NumericLessThanEquals": map[string]any{
						"s3:content-length": float64(10 * 1024 * 1024), // 10MB
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:PutObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{},
				OriginalRequest: &http.Request{ContentLength: 1024 * 1024}, // 1MB
			},
			expected: DecisionAllow,
		},
		{
			name: "allow with content size limit - over limit",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:PutObject",
				Resource:  "*",
				Condition: map[string]any{
					"NumericLessThanEquals": map[string]any{
						"s3:content-length": float64(10 * 1024 * 1024), // 10MB
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:PutObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{},
				OriginalRequest: &http.Request{ContentLength: 11 * 1024 * 1024}, // 11MB
			},
			expected: DecisionDeny,
		},
		{
			name: "allow with HTTPS requirement - HTTPS used",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"Bool": map[string]any{
						"aws:SecureTransport": true,
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				Conditions:      &ConditionContext{SecureTransport: true},
				VarContext:      &variables.Context{},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionAllow,
		},
		{
			name: "allow with HTTPS requirement - HTTP used",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"Bool": map[string]any{
						"aws:SecureTransport": true,
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				Conditions:      &ConditionContext{SecureTransport: false},
				VarContext:      &variables.Context{},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionDeny,
		},
		{
			name: "explicit deny with conditions - condition matches",
			stmt: &iam.Statement{
				Effect:    "Deny",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"IpAddress": map[string]any{
						"aws:SourceIp": "10.0.0.0/8",
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{SourceIP: deniedIP},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionExplicitDeny,
		},
		{
			name: "explicit deny with conditions - condition does not match",
			stmt: &iam.Statement{
				Effect:    "Deny",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"IpAddress": map[string]any{
						"aws:SourceIp": "10.0.0.0/8",
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{SourceIP: allowedIP},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionDeny, // Deny because condition doesn't match, not explicit deny
		},
		{
			name: "multiple conditions (AND logic) - all match",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"IpAddress": map[string]any{
						"aws:SourceIp": "192.168.1.0/24",
					},
					"StringEquals": map[string]any{
						"aws:username": "alice",
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{SourceIP: allowedIP, Username: "alice"},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionAllow,
		},
		{
			name: "multiple conditions (AND logic) - one fails",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"IpAddress": map[string]any{
						"aws:SourceIp": "192.168.1.0/24",
					},
					"StringEquals": map[string]any{
						"aws:username": "bob",
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "bucket", Key: "file.txt"},
				VarContext:      &variables.Context{SourceIP: allowedIP, Username: "alice"},
				OriginalRequest: &http.Request{},
			},
			expected: DecisionDeny,
		},
		{
			name: "condition with variable substitution",
			stmt: &iam.Statement{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "*",
				Condition: map[string]any{
					"StringEquals": map[string]any{
						"s3:prefix": "${aws:username}/",
					},
				},
			},
			req: &RequestContext{
				Principal:       userPrincipal,
				Action:          "s3:ListBucket",
				Resource:        &Resource{Bucket: "bucket"},
				VarContext:      &variables.Context{Username: "alice", S3Prefix: "alice/"},
				OriginalRequest: &http.Request{},
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
