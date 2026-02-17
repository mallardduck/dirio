package iam

import (
	"testing"
)

func TestStatement_Validate(t *testing.T) {
	tests := []struct {
		name    string
		stmt    Statement
		wantErr bool
	}{
		{
			name: "valid statement with string action and resource",
			stmt: Statement{
				Effect:   "Allow",
				Action:   "s3:GetObject",
				Resource: "arn:aws:s3:::bucket/*",
			},
			wantErr: false,
		},
		{
			name: "valid statement with array action and resource",
			stmt: Statement{
				Effect:   "Allow",
				Action:   []string{"s3:GetObject", "s3:PutObject"},
				Resource: []string{"arn:aws:s3:::bucket1/*", "arn:aws:s3:::bucket2/*"},
			},
			wantErr: false,
		},
		{
			name: "valid statement with principal",
			stmt: Statement{
				Effect:    "Allow",
				Principal: map[string]any{"AWS": "*"},
				Action:    "s3:GetObject",
				Resource:  "arn:aws:s3:::bucket/*",
			},
			wantErr: false,
		},
		{
			name: "valid statement with NotAction and NotResource",
			stmt: Statement{
				Effect:      "Deny",
				NotAction:   "s3:DeleteBucket",
				NotResource: "arn:aws:s3:::protected-bucket",
			},
			wantErr: false,
		},
		{
			name: "valid statement with NotPrincipal",
			stmt: Statement{
				Effect:       "Deny",
				NotPrincipal: map[string]any{"AWS": "arn:aws:iam::123456789012:user/admin"},
				Action:       "s3:*",
				Resource:     "*",
			},
			wantErr: false,
		},
		{
			name: "invalid - missing Effect",
			stmt: Statement{
				Action:   "s3:GetObject",
				Resource: "arn:aws:s3:::bucket/*",
			},
			wantErr: true,
		},
		{
			name: "invalid - invalid Effect",
			stmt: Statement{
				Effect:   "Maybe",
				Action:   "s3:GetObject",
				Resource: "arn:aws:s3:::bucket/*",
			},
			wantErr: true,
		},
		{
			name: "invalid - missing Action and NotAction",
			stmt: Statement{
				Effect:   "Allow",
				Resource: "arn:aws:s3:::bucket/*",
			},
			wantErr: true,
		},
		{
			name: "invalid - missing Resource and NotResource",
			stmt: Statement{
				Effect: "Allow",
				Action: "s3:GetObject",
			},
			wantErr: true,
		},
		{
			name: "invalid - both Action and NotAction",
			stmt: Statement{
				Effect:    "Allow",
				Action:    "s3:GetObject",
				NotAction: "s3:DeleteObject",
				Resource:  "arn:aws:s3:::bucket/*",
			},
			wantErr: true,
		},
		{
			name: "invalid - both Resource and NotResource",
			stmt: Statement{
				Effect:      "Allow",
				Action:      "s3:GetObject",
				Resource:    "arn:aws:s3:::bucket/*",
				NotResource: "arn:aws:s3:::other/*",
			},
			wantErr: true,
		},
		{
			name: "invalid - both Principal and NotPrincipal",
			stmt: Statement{
				Effect:       "Allow",
				Principal:    "*",
				NotPrincipal: map[string]any{"AWS": "arn:aws:iam::123:user/bob"},
				Action:       "s3:GetObject",
				Resource:     "arn:aws:s3:::bucket/*",
			},
			wantErr: true,
		},
		{
			name: "invalid - empty string Action",
			stmt: Statement{
				Effect:   "Allow",
				Action:   "",
				Resource: "arn:aws:s3:::bucket/*",
			},
			wantErr: true,
		},
		{
			name: "invalid - empty array Action",
			stmt: Statement{
				Effect:   "Allow",
				Action:   []string{},
				Resource: "arn:aws:s3:::bucket/*",
			},
			wantErr: true,
		},
		{
			name: "invalid - Action array with empty string",
			stmt: Statement{
				Effect:   "Allow",
				Action:   []string{"s3:GetObject", ""},
				Resource: "arn:aws:s3:::bucket/*",
			},
			wantErr: true,
		},
		{
			name: "invalid - Action with wrong type (number)",
			stmt: Statement{
				Effect:   "Allow",
				Action:   123,
				Resource: "arn:aws:s3:::bucket/*",
			},
			wantErr: true,
		},
		{
			name: "invalid - Resource array with non-string",
			stmt: Statement{
				Effect:   "Allow",
				Action:   "s3:GetObject",
				Resource: []any{"arn:aws:s3:::bucket/*", 123},
			},
			wantErr: true,
		},
		{
			name: "invalid - Action with map (not allowed)",
			stmt: Statement{
				Effect:   "Allow",
				Action:   map[string]any{"S3": "GetObject"},
				Resource: "arn:aws:s3:::bucket/*",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.stmt.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Statement.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPolicyDocument_Validate(t *testing.T) {
	tests := []struct {
		name    string
		pd      PolicyDocument
		wantErr bool
	}{
		{
			name: "valid policy document",
			pd: PolicyDocument{
				Version: "2012-10-17",
				Statement: []Statement{
					{
						Effect:   "Allow",
						Action:   "s3:GetObject",
						Resource: "arn:aws:s3:::bucket/*",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid with multiple statements",
			pd: PolicyDocument{
				Version: "2012-10-17",
				Statement: []Statement{
					{
						Sid:      "AllowRead",
						Effect:   "Allow",
						Action:   "s3:GetObject",
						Resource: "arn:aws:s3:::bucket/*",
					},
					{
						Sid:      "AllowWrite",
						Effect:   "Allow",
						Action:   "s3:PutObject",
						Resource: "arn:aws:s3:::bucket/*",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - missing Version",
			pd: PolicyDocument{
				Statement: []Statement{
					{
						Effect:   "Allow",
						Action:   "s3:GetObject",
						Resource: "arn:aws:s3:::bucket/*",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid - empty statements",
			pd: PolicyDocument{
				Version:   "2012-10-17",
				Statement: []Statement{},
			},
			wantErr: true,
		},
		{
			name: "invalid - statement with error",
			pd: PolicyDocument{
				Version: "2012-10-17",
				Statement: []Statement{
					{
						Effect:   "Allow",
						Action:   "s3:GetObject",
						Resource: "arn:aws:s3:::bucket/*",
					},
					{
						Effect: "Allow",
						Action: "s3:PutObject",
						// Missing Resource - should fail
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("PolicyDocument.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPolicy_Validate(t *testing.T) {
	tests := []struct {
		name    string
		policy  Policy
		wantErr bool
	}{
		{
			name: "valid policy",
			policy: Policy{
				Version: "1.0.0",
				Name:    "ReadOnlyPolicy",
				PolicyDocument: &PolicyDocument{
					Version: "2012-10-17",
					Statement: []Statement{
						{
							Effect:   "Allow",
							Action:   "s3:GetObject",
							Resource: "arn:aws:s3:::bucket/*",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - missing version",
			policy: Policy{
				Name: "ReadOnlyPolicy",
				PolicyDocument: &PolicyDocument{
					Version: "2012-10-17",
					Statement: []Statement{
						{
							Effect:   "Allow",
							Action:   "s3:GetObject",
							Resource: "arn:aws:s3:::bucket/*",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid - missing name",
			policy: Policy{
				Version: "1.0.0",
				PolicyDocument: &PolicyDocument{
					Version: "2012-10-17",
					Statement: []Statement{
						{
							Effect:   "Allow",
							Action:   "s3:GetObject",
							Resource: "arn:aws:s3:::bucket/*",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid - missing policy document",
			policy: Policy{
				Version: "1.0.0",
				Name:    "ReadOnlyPolicy",
			},
			wantErr: true,
		},
		{
			name: "invalid - invalid policy document",
			policy: Policy{
				Version: "1.0.0",
				Name:    "ReadOnlyPolicy",
				PolicyDocument: &PolicyDocument{
					Version:   "2012-10-17",
					Statement: []Statement{},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Policy.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
