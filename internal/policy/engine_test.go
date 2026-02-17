package policy

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/pkg/iam"
)

func TestEngine_AdminBypass(t *testing.T) {
	engine := New()

	// Admin should always be allowed, even without any policies
	req := &RequestContext{
		Principal: &Principal{
			User:        &metadata.User{AccessKey: "admin"},
			IsAnonymous: false,
			IsAdmin:     true,
		},
		Action:   "s3:DeleteBucket",
		Resource: &Resource{Bucket: "any-bucket"},
	}

	decision := engine.Evaluate(context.Background(), req)
	if decision != DecisionAllow {
		t.Errorf("Admin should be allowed, got %v", decision)
	}
}

func TestEngine_AnonymousDeniedByDefault(t *testing.T) {
	engine := New()

	// Anonymous user with no bucket policy should be denied
	req := &RequestContext{
		Principal: &Principal{IsAnonymous: true},
		Action:    "s3:GetObject",
		Resource:  &Resource{Bucket: "some-bucket", Key: "file.txt"},
	}

	decision := engine.Evaluate(context.Background(), req)
	if decision != DecisionDeny {
		t.Errorf("Anonymous should be denied by default, got %v", decision)
	}
}

func TestEngine_PublicBucketPolicy(t *testing.T) {
	engine := New()

	// Set up a public read policy
	publicPolicy := &iam.PolicyDocument{
		Version: "2012-10-17",
		Statement: []iam.Statement{
			{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:GetObject",
				Resource:  "arn:aws:s3:::public-bucket/*",
			},
		},
	}
	engine.UpdateBucketPolicy("public-bucket", publicPolicy)

	tests := []struct {
		name     string
		req      *RequestContext
		expected Decision
	}{
		{
			name: "anonymous can read from public bucket",
			req: &RequestContext{
				Principal: &Principal{IsAnonymous: true},
				Action:    "s3:GetObject",
				Resource:  &Resource{Bucket: "public-bucket", Key: "file.txt"},
			},
			expected: DecisionAllow,
		},
		{
			name: "anonymous cannot write to public bucket",
			req: &RequestContext{
				Principal: &Principal{IsAnonymous: true},
				Action:    "s3:PutObject",
				Resource:  &Resource{Bucket: "public-bucket", Key: "file.txt"},
			},
			expected: DecisionDeny,
		},
		{
			name: "anonymous cannot read from different bucket",
			req: &RequestContext{
				Principal: &Principal{IsAnonymous: true},
				Action:    "s3:GetObject",
				Resource:  &Resource{Bucket: "private-bucket", Key: "file.txt"},
			},
			expected: DecisionDeny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := engine.Evaluate(context.Background(), tt.req)
			if decision != tt.expected {
				t.Errorf("got %v, want %v", decision, tt.expected)
			}
		})
	}
}

func TestEngine_ExplicitDenyWins(t *testing.T) {
	engine := New()

	// Policy with both allow and deny statements
	mixedPolicy := &iam.PolicyDocument{
		Version: "2012-10-17",
		Statement: []iam.Statement{
			{
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:*",
				Resource:  "arn:aws:s3:::test-bucket/*",
			},
			{
				Effect:    "Deny",
				Principal: "*",
				Action:    "s3:DeleteObject",
				Resource:  "arn:aws:s3:::test-bucket/*",
			},
		},
	}
	engine.UpdateBucketPolicy("test-bucket", mixedPolicy)

	// GetObject should be allowed
	getReq := &RequestContext{
		Principal: &Principal{IsAnonymous: true},
		Action:    "s3:GetObject",
		Resource:  &Resource{Bucket: "test-bucket", Key: "file.txt"},
	}
	if decision := engine.Evaluate(context.Background(), getReq); decision != DecisionAllow {
		t.Errorf("GetObject should be allowed, got %v", decision)
	}

	// DeleteObject should be explicitly denied
	deleteReq := &RequestContext{
		Principal: &Principal{IsAnonymous: true},
		Action:    "s3:DeleteObject",
		Resource:  &Resource{Bucket: "test-bucket", Key: "file.txt"},
	}
	if decision := engine.Evaluate(context.Background(), deleteReq); decision != DecisionExplicitDeny {
		t.Errorf("DeleteObject should be explicitly denied, got %v", decision)
	}
}

func TestEngine_UserSpecificPolicy(t *testing.T) {
	engine := New()

	// Policy that allows a specific user
	userPolicy := &iam.PolicyDocument{
		Version: "2012-10-17",
		Statement: []iam.Statement{
			{
				Effect:    "Allow",
				Principal: map[string]interface{}{"AWS": "arn:aws:iam::123456789012:user/alice"},
				Action:    "s3:*",
				Resource:  "arn:aws:s3:::alice-bucket/*",
			},
		},
	}
	engine.UpdateBucketPolicy("alice-bucket", userPolicy)

	alicePrincipal := &Principal{
		User:        &metadata.User{AccessKey: "alice"},
		IsAnonymous: false,
	}
	bobPrincipal := &Principal{
		User:        &metadata.User{AccessKey: "bob"},
		IsAnonymous: false,
	}

	// Alice can access
	aliceReq := &RequestContext{
		Principal: alicePrincipal,
		Action:    "s3:GetObject",
		Resource:  &Resource{Bucket: "alice-bucket", Key: "file.txt"},
	}
	if decision := engine.Evaluate(context.Background(), aliceReq); decision != DecisionAllow {
		t.Errorf("Alice should be allowed, got %v", decision)
	}

	// Bob cannot access (no matching policy)
	bobReq := &RequestContext{
		Principal: bobPrincipal,
		Action:    "s3:GetObject",
		Resource:  &Resource{Bucket: "alice-bucket", Key: "file.txt"},
	}
	if decision := engine.Evaluate(context.Background(), bobReq); decision != DecisionDeny {
		t.Errorf("Bob should be denied, got %v", decision)
	}
}

func TestEngine_PolicyLifecycle(t *testing.T) {
	engine := New()

	bucket := "lifecycle-test"

	// Initially no policy
	if engine.HasBucketPolicy(bucket) {
		t.Error("Should not have policy initially")
	}

	// Add policy
	policy := &iam.PolicyDocument{
		Version: "2012-10-17",
		Statement: []iam.Statement{
			{Effect: "Allow", Principal: "*", Action: "s3:GetObject", Resource: "arn:aws:s3:::lifecycle-test/*"},
		},
	}
	engine.UpdateBucketPolicy(bucket, policy)

	if !engine.HasBucketPolicy(bucket) {
		t.Error("Should have policy after update")
	}

	// Anonymous can now read
	req := &RequestContext{
		Principal: &Principal{IsAnonymous: true},
		Action:    "s3:GetObject",
		Resource:  &Resource{Bucket: bucket, Key: "file.txt"},
	}
	if decision := engine.Evaluate(context.Background(), req); decision != DecisionAllow {
		t.Errorf("Should be allowed with policy, got %v", decision)
	}

	// Delete policy
	engine.DeleteBucketPolicy(bucket)

	if engine.HasBucketPolicy(bucket) {
		t.Error("Should not have policy after delete")
	}

	// Anonymous denied again
	if decision := engine.Evaluate(context.Background(), req); decision != DecisionDeny {
		t.Errorf("Should be denied without policy, got %v", decision)
	}
}

func TestEngine_LoadBucketPolicies(t *testing.T) {
	engine := New()

	policies := map[string]*iam.PolicyDocument{
		"bucket1": {
			Version: "2012-10-17",
			Statement: []iam.Statement{
				{Effect: "Allow", Principal: "*", Action: "s3:GetObject", Resource: "arn:aws:s3:::bucket1/*"},
			},
		},
		"bucket2": {
			Version: "2012-10-17",
			Statement: []iam.Statement{
				{Effect: "Allow", Principal: "*", Action: "s3:GetObject", Resource: "arn:aws:s3:::bucket2/*"},
			},
		},
	}

	engine.LoadBucketPolicies(context.Background(), policies)

	if !engine.HasBucketPolicy("bucket1") {
		t.Error("Should have bucket1 policy")
	}
	if !engine.HasBucketPolicy("bucket2") {
		t.Error("Should have bucket2 policy")
	}
	if engine.HasBucketPolicy("bucket3") {
		t.Error("Should not have bucket3 policy")
	}
}

func TestEngine_GetActionMapper(t *testing.T) {
	engine := New()

	mapper := engine.GetActionMapper()
	if mapper == nil {
		t.Error("ActionMapper should not be nil")
	}

	// Verify it works
	perms := mapper.GetRequiredPermissions("s3:HeadObject")
	if len(perms) != 1 || perms[0] != "s3:GetObject" {
		t.Errorf("ActionMapper should translate HeadObject, got %v", perms)
	}
}

func TestEngine_OwnershipBasedAuthorization(t *testing.T) {
	engine := New()

	// Create test users with UUIDs
	aliceUUID := uuid.New()
	bobUUID := uuid.New()

	aliceUser := &metadata.User{
		UUID:      aliceUUID,
		Username:  "alice",
		AccessKey: "alice-key",
		Status:    iam.UserStatusActive,
	}

	bobUser := &metadata.User{
		UUID:      bobUUID,
		Username:  "bob",
		AccessKey: "bob-key",
		Status:    iam.UserStatusActive,
	}

	tests := []struct {
		name     string
		req      *RequestContext
		expected Decision
	}{
		{
			name: "bucket owner can access their bucket",
			req: &RequestContext{
				Principal: &Principal{
					User:        aliceUser,
					IsAnonymous: false,
					IsAdmin:     false,
				},
				Action:          "s3:ListBucket",
				Resource:        &Resource{Bucket: "alice-bucket"},
				BucketOwnerUUID: &aliceUUID,
			},
			expected: DecisionAllow,
		},
		{
			name: "object owner can access their object",
			req: &RequestContext{
				Principal: &Principal{
					User:        aliceUser,
					IsAnonymous: false,
					IsAdmin:     false,
				},
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "shared-bucket", Key: "alice/file.txt"},
				BucketOwnerUUID: &bobUUID,   // Bob owns bucket
				ObjectOwnerUUID: &aliceUUID, // Alice owns object
			},
			expected: DecisionAllow,
		},
		{
			name: "bucket owner can access objects in their bucket",
			req: &RequestContext{
				Principal: &Principal{
					User:        bobUser,
					IsAnonymous: false,
					IsAdmin:     false,
				},
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "shared-bucket", Key: "alice/file.txt"},
				BucketOwnerUUID: &bobUUID,   // Bob owns bucket
				ObjectOwnerUUID: &aliceUUID, // Alice owns object
			},
			expected: DecisionAllow, // Bucket owner has implicit access (AWS model)
		},
		{
			name: "non-owner cannot access bucket",
			req: &RequestContext{
				Principal: &Principal{
					User:        bobUser,
					IsAnonymous: false,
					IsAdmin:     false,
				},
				Action:          "s3:ListBucket",
				Resource:        &Resource{Bucket: "alice-bucket"},
				BucketOwnerUUID: &aliceUUID,
			},
			expected: DecisionDeny,
		},
		{
			name: "non-owner cannot access object",
			req: &RequestContext{
				Principal: &Principal{
					User:        bobUser,
					IsAnonymous: false,
					IsAdmin:     false,
				},
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "alice-bucket", Key: "file.txt"},
				BucketOwnerUUID: &aliceUUID,
				ObjectOwnerUUID: &aliceUUID,
			},
			expected: DecisionDeny,
		},
		{
			name: "anonymous has no ownership",
			req: &RequestContext{
				Principal: &Principal{
					IsAnonymous: true,
				},
				Action:          "s3:GetObject",
				Resource:        &Resource{Bucket: "alice-bucket", Key: "file.txt"},
				BucketOwnerUUID: &aliceUUID,
			},
			expected: DecisionDeny,
		},
		{
			name: "admin-only bucket (nil owner) denies regular user",
			req: &RequestContext{
				Principal: &Principal{
					User:        aliceUser,
					IsAnonymous: false,
					IsAdmin:     false,
				},
				Action:          "s3:ListBucket",
				Resource:        &Resource{Bucket: "admin-bucket"},
				BucketOwnerUUID: nil, // Admin-only
			},
			expected: DecisionDeny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := engine.Evaluate(context.Background(), tt.req)
			if decision != tt.expected {
				t.Errorf("got %v, want %v", decision, tt.expected)
			}
		})
	}
}

func TestEngine_ExplicitDenyBeatsOwnership(t *testing.T) {
	engine := New()

	aliceUUID := uuid.New()
	aliceUser := &metadata.User{
		UUID:      aliceUUID,
		Username:  "alice",
		AccessKey: "alice-key",
		Status:    iam.UserStatusActive,
	}

	// Policy that explicitly denies the owner
	denyOwnerPolicy := &iam.PolicyDocument{
		Version: "2012-10-17",
		Statement: []iam.Statement{
			{
				Effect:    "Deny",
				Principal: "*", // Denies everyone, including owner
				Action:    "s3:DeleteObject",
				Resource:  "arn:aws:s3:::alice-bucket/protected/*",
			},
		},
	}
	engine.UpdateBucketPolicy("alice-bucket", denyOwnerPolicy)

	tests := []struct {
		name     string
		key      string
		action   string
		expected Decision
	}{
		{
			name:     "owner denied by explicit deny for protected path",
			key:      "protected/sensitive.txt",
			action:   "s3:DeleteObject",
			expected: DecisionExplicitDeny,
		},
		{
			name:     "owner allowed for unprotected path (no policy match)",
			key:      "public/file.txt",
			action:   "s3:DeleteObject",
			expected: DecisionAllow, // Ownership grants access
		},
		{
			name:     "owner allowed for different action on protected path",
			key:      "protected/sensitive.txt",
			action:   "s3:GetObject",
			expected: DecisionAllow, // Ownership grants access (no deny for GetObject)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &RequestContext{
				Principal: &Principal{
					User:        aliceUser,
					IsAnonymous: false,
					IsAdmin:     false,
				},
				Action:          tt.action,
				Resource:        &Resource{Bucket: "alice-bucket", Key: tt.key},
				BucketOwnerUUID: &aliceUUID,
				ObjectOwnerUUID: &aliceUUID,
			}

			decision := engine.Evaluate(context.Background(), req)
			if decision != tt.expected {
				t.Errorf("got %v, want %v", decision, tt.expected)
			}
		})
	}
}
