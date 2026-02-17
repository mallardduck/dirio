package policy

import (
	"testing"
)

func TestValidatePrincipal(t *testing.T) {
	tests := []struct {
		name      string
		principal any
		wantErr   bool
	}{
		{
			name:      "nil is valid",
			principal: nil,
			wantErr:   false,
		},
		{
			name:      "wildcard string",
			principal: "*",
			wantErr:   false,
		},
		{
			name:      "AWS wildcard map",
			principal: map[string]any{"AWS": "*"},
			wantErr:   false,
		},
		{
			name:      "AWS ARN string",
			principal: map[string]any{"AWS": "arn:aws:iam::123456789012:user/alice"},
			wantErr:   false,
		},
		{
			name:      "AWS ARN array (any)",
			principal: map[string]any{"AWS": []any{"arn:aws:iam::123456789012:user/alice", "arn:aws:iam::123456789012:user/bob"}},
			wantErr:   false,
		},
		{
			name:      "AWS ARN array (string)",
			principal: map[string]any{"AWS": []string{"arn:aws:iam::123456789012:user/alice"}},
			wantErr:   false,
		},
		{
			name:      "interface{} map variant",
			principal: map[string]interface{}{"AWS": "*"},
			wantErr:   false,
		},
		{
			name:      "Service principal",
			principal: map[string]any{"Service": "s3.amazonaws.com"},
			wantErr:   false,
		},
		{
			name:      "invalid - empty map",
			principal: map[string]any{},
			wantErr:   true,
		},
		{
			name:      "invalid - wrong key",
			principal: map[string]any{"InvalidKey": "*"},
			wantErr:   true,
		},
		{
			name:      "invalid - number",
			principal: 123,
			wantErr:   true,
		},
		{
			name:      "invalid - empty array",
			principal: map[string]any{"AWS": []string{}},
			wantErr:   true,
		},
		{
			name:      "invalid - array with non-string",
			principal: map[string]any{"AWS": []any{"arn:aws:iam::123:user/alice", 123}},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePrincipal(tt.principal)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePrincipal() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAction(t *testing.T) {
	tests := []struct {
		name    string
		action  any
		wantErr bool
	}{
		{
			name:    "nil is valid",
			action:  nil,
			wantErr: false,
		},
		{
			name:    "single action string",
			action:  "s3:GetObject",
			wantErr: false,
		},
		{
			name:    "wildcard",
			action:  "*",
			wantErr: false,
		},
		{
			name:    "action array (string)",
			action:  []string{"s3:GetObject", "s3:PutObject"},
			wantErr: false,
		},
		{
			name:    "action array (any)",
			action:  []any{"s3:GetObject", "s3:PutObject"},
			wantErr: false,
		},
		{
			name:    "action array (interface{})",
			action:  []interface{}{"s3:GetObject", "s3:PutObject"},
			wantErr: false,
		},
		{
			name:    "invalid - empty string",
			action:  "",
			wantErr: true,
		},
		{
			name:    "invalid - empty array",
			action:  []string{},
			wantErr: true,
		},
		{
			name:    "invalid - array with empty string",
			action:  []string{"s3:GetObject", ""},
			wantErr: true,
		},
		{
			name:    "invalid - array with non-string",
			action:  []any{"s3:GetObject", 123},
			wantErr: true,
		},
		{
			name:    "invalid - number",
			action:  123,
			wantErr: true,
		},
		{
			name:    "invalid - map",
			action:  map[string]any{"action": "s3:GetObject"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAction(tt.action)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateResource(t *testing.T) {
	tests := []struct {
		name     string
		resource any
		wantErr  bool
	}{
		{
			name:     "nil is valid",
			resource: nil,
			wantErr:  false,
		},
		{
			name:     "wildcard",
			resource: "*",
			wantErr:  false,
		},
		{
			name:     "bucket ARN",
			resource: "arn:aws:s3:::my-bucket",
			wantErr:  false,
		},
		{
			name:     "object ARN",
			resource: "arn:aws:s3:::my-bucket/*",
			wantErr:  false,
		},
		{
			name:     "resource array (string)",
			resource: []string{"arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"},
			wantErr:  false,
		},
		{
			name:     "resource array (any)",
			resource: []any{"arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"},
			wantErr:  false,
		},
		{
			name:     "invalid - empty string",
			resource: "",
			wantErr:  true,
		},
		{
			name:     "invalid - empty array",
			resource: []string{},
			wantErr:  true,
		},
		{
			name:     "invalid - array with empty string",
			resource: []string{"arn:aws:s3:::bucket", ""},
			wantErr:  true,
		},
		{
			name:     "invalid - array with non-string",
			resource: []any{"arn:aws:s3:::bucket", 123},
			wantErr:  true,
		},
		{
			name:     "invalid - number",
			resource: 123,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateResource(tt.resource)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateResource() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCondition(t *testing.T) {
	tests := []struct {
		name       string
		conditions ConditionMap
		wantErr    bool
	}{
		{
			name:       "nil is valid",
			conditions: nil,
			wantErr:    false,
		},
		{
			name:       "empty map is valid",
			conditions: map[string]any{},
			wantErr:    false,
		},
		{
			name: "valid StringEquals",
			conditions: map[string]any{
				"StringEquals": map[string]any{
					"aws:username": "alice",
				},
			},
			wantErr: false,
		},
		{
			name: "valid multiple operators",
			conditions: map[string]any{
				"StringEquals": map[string]any{
					"aws:username": "alice",
				},
				"IpAddress": map[string]any{
					"aws:SourceIp": "192.168.1.0/24",
				},
			},
			wantErr: false,
		},
		{
			name: "valid with array value",
			conditions: map[string]any{
				"IpAddress": map[string]any{
					"aws:SourceIp": []string{"192.168.1.0/24", "10.0.0.0/8"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid with interface{} map",
			conditions: map[string]any{
				"StringEquals": map[string]interface{}{
					"aws:username": "alice",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - empty operator name",
			conditions: map[string]any{
				"": map[string]any{
					"aws:username": "alice",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid - operator value not a map",
			conditions: map[string]any{
				"StringEquals": "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid - empty key-value map",
			conditions: map[string]any{
				"StringEquals": map[string]any{},
			},
			wantErr: true,
		},
		{
			name: "invalid - empty key",
			conditions: map[string]any{
				"StringEquals": map[string]any{
					"": "alice",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid - nil value",
			conditions: map[string]any{
				"StringEquals": map[string]any{
					"aws:username": nil,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCondition(tt.conditions)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCondition() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeAction(t *testing.T) {
	tests := []struct {
		name    string
		action  any
		want    []string
		wantErr bool
	}{
		{
			name:    "single string",
			action:  "s3:GetObject",
			want:    []string{"s3:GetObject"},
			wantErr: false,
		},
		{
			name:    "string array",
			action:  []string{"s3:GetObject", "s3:PutObject"},
			want:    []string{"s3:GetObject", "s3:PutObject"},
			wantErr: false,
		},
		{
			name:    "any array",
			action:  []any{"s3:GetObject", "s3:PutObject"},
			want:    []string{"s3:GetObject", "s3:PutObject"},
			wantErr: false,
		},
		{
			name:    "interface{} array",
			action:  []interface{}{"s3:GetObject", "s3:PutObject"},
			want:    []string{"s3:GetObject", "s3:PutObject"},
			wantErr: false,
		},
		{
			name:    "invalid - nil",
			action:  nil,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid - number",
			action:  123,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid - array with non-string",
			action:  []any{"s3:GetObject", 123},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeAction(tt.action)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeAction() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("NormalizeAction() got length = %v, want length %v", len(got), len(tt.want))
					return
				}
				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("NormalizeAction()[%d] = %v, want %v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestNormalizeResource(t *testing.T) {
	tests := []struct {
		name     string
		resource any
		want     []string
		wantErr  bool
	}{
		{
			name:     "single string",
			resource: "arn:aws:s3:::my-bucket/*",
			want:     []string{"arn:aws:s3:::my-bucket/*"},
			wantErr:  false,
		},
		{
			name:     "string array",
			resource: []string{"arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"},
			want:     []string{"arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"},
			wantErr:  false,
		},
		{
			name:     "any array",
			resource: []any{"arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"},
			want:     []string{"arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"},
			wantErr:  false,
		},
		{
			name:     "invalid - nil",
			resource: nil,
			want:     nil,
			wantErr:  true,
		},
		{
			name:     "invalid - number",
			resource: 123,
			want:     nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeResource(tt.resource)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeResource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("NormalizeResource() got length = %v, want length %v", len(got), len(tt.want))
					return
				}
				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("NormalizeResource()[%d] = %v, want %v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}
