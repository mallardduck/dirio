package conditions

import (
	"net"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mallardduck/dirio/internal/policy/variables"
)

func TestEvaluator_Evaluate_EmptyConditions(t *testing.T) {
	ctx := &Context{}
	eval := NewEvaluator(ctx)

	// Empty conditions should always pass
	match, err := eval.Evaluate(map[string]any{})
	if err != nil {
		t.Errorf("Evaluate() error = %v, want nil", err)
	}
	if !match {
		t.Error("Evaluate() with empty conditions should return true")
	}
}

func TestEvaluator_Evaluate_SingleOperator(t *testing.T) {
	ctx := &Context{
		Username: "alice",
	}
	eval := NewEvaluator(ctx)

	conditions := map[string]any{
		"StringEquals": map[string]any{
			"aws:username": "alice",
		},
	}

	match, err := eval.Evaluate(conditions)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if !match {
		t.Error("Evaluate() should match")
	}
}

func TestEvaluator_Evaluate_MultipleOperatorsAND(t *testing.T) {
	sourceIP := net.ParseIP("192.168.1.100")
	ctx := &Context{
		Username: "alice",
		SourceIP: sourceIP,
	}
	eval := NewEvaluator(ctx)

	// Both conditions must be true (AND logic)
	conditions := map[string]any{
		"StringEquals": map[string]any{
			"aws:username": "alice",
		},
		"IpAddress": map[string]any{
			"aws:SourceIp": "192.168.1.0/24",
		},
	}

	match, err := eval.Evaluate(conditions)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if !match {
		t.Error("Evaluate() both conditions should match")
	}

	// One condition fails - entire evaluation fails
	conditions2 := map[string]any{
		"StringEquals": map[string]any{
			"aws:username": "bob", // Different user
		},
		"IpAddress": map[string]any{
			"aws:SourceIp": "192.168.1.0/24",
		},
	}

	match2, err := eval.Evaluate(conditions2)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if match2 {
		t.Error("Evaluate() should not match when one condition fails")
	}
}

func TestEvaluator_Evaluate_MultipleValuesOR(t *testing.T) {
	sourceIP := net.ParseIP("10.0.5.100")
	ctx := &Context{
		SourceIP: sourceIP,
	}
	eval := NewEvaluator(ctx)

	// Multiple values - OR logic (any match succeeds)
	conditions := map[string]any{
		"IpAddress": map[string]any{
			"aws:SourceIp": []any{"192.168.1.0/24", "10.0.0.0/8"},
		},
	}

	match, err := eval.Evaluate(conditions)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if !match {
		t.Error("Evaluate() should match second IP range")
	}

	// No value matches
	conditions2 := map[string]any{
		"IpAddress": map[string]any{
			"aws:SourceIp": []any{"192.168.1.0/24", "172.16.0.0/12"},
		},
	}

	match2, err := eval.Evaluate(conditions2)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if match2 {
		t.Error("Evaluate() should not match when no values match")
	}
}

func TestEvaluator_Evaluate_VariableSubstitution(t *testing.T) {
	varCtx := &variables.Context{
		Username: "alice",
		UserID:   uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
	}

	ctx := &Context{
		Username:   "alice",
		UserID:     varCtx.UserID.String(),
		S3Prefix:   "alice/data/",
		VarContext: varCtx,
	}
	eval := NewEvaluator(ctx)

	// Variable substitution in condition value
	conditions := map[string]any{
		"StringEquals": map[string]any{
			"s3:prefix": "${aws:username}/data/",
		},
	}

	match, err := eval.Evaluate(conditions)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if !match {
		t.Error("Evaluate() should match with variable substitution")
	}
}

func TestEvaluator_Evaluate_ComplexRealWorld(t *testing.T) {
	// Simulate real-world policy condition:
	// Allow access only from specific IP range, during business hours, for specific user
	sourceIP := net.ParseIP("192.168.1.100")
	businessHoursTime := time.Date(2026, 1, 15, 14, 0, 0, 0, time.UTC) // 2PM on a weekday

	ctx := &Context{
		Username:    "alice",
		SourceIP:    sourceIP,
		CurrentTime: businessHoursTime,
	}
	eval := NewEvaluator(ctx)

	conditions := map[string]any{
		"StringEquals": map[string]any{
			"aws:username": "alice",
		},
		"IpAddress": map[string]any{
			"aws:SourceIp": "192.168.1.0/24",
		},
		"DateGreaterThan": map[string]any{
			"aws:CurrentTime": "2026-01-01T00:00:00Z",
		},
	}

	match, err := eval.Evaluate(conditions)
	if err != nil {
		t.Errorf("Evaluate() error = %v", err)
	}
	if !match {
		t.Error("Evaluate() complex real-world conditions should match")
	}
}

func TestEvaluator_Evaluate_IPRestriction(t *testing.T) {
	// Test case from setup script: policy-ip-test bucket
	allowedIP := net.ParseIP("192.168.1.100")
	deniedIP := net.ParseIP("10.0.0.100")

	tests := []struct {
		name      string
		sourceIP  net.IP
		wantMatch bool
	}{
		{
			name:      "allowed IP range",
			sourceIP:  allowedIP,
			wantMatch: true,
		},
		{
			name:      "denied IP range",
			sourceIP:  deniedIP,
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				SourceIP: tt.sourceIP,
			}
			eval := NewEvaluator(ctx)

			conditions := map[string]any{
				"IpAddress": map[string]any{
					"aws:SourceIp": "192.168.1.0/24",
				},
			}

			match, err := eval.Evaluate(conditions)
			if err != nil {
				t.Errorf("Evaluate() error = %v", err)
			}
			if match != tt.wantMatch {
				t.Errorf("Evaluate() = %v, want %v", match, tt.wantMatch)
			}
		})
	}
}

func TestEvaluator_Evaluate_TimeWindow(t *testing.T) {
	// Test case from setup script: policy-time-test bucket
	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)

	tests := []struct {
		name        string
		currentTime time.Time
		wantMatch   bool
	}{
		{
			name:        "within time window",
			currentTime: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
			wantMatch:   true,
		},
		{
			name:        "before time window",
			currentTime: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
			wantMatch:   false,
		},
		{
			name:        "after time window",
			currentTime: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			wantMatch:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				CurrentTime: tt.currentTime,
			}
			eval := NewEvaluator(ctx)

			conditions := map[string]any{
				"DateGreaterThan": map[string]any{
					"aws:CurrentTime": startTime.Format(time.RFC3339),
				},
				"DateLessThan": map[string]any{
					"aws:CurrentTime": endTime.Format(time.RFC3339),
				},
			}

			match, err := eval.Evaluate(conditions)
			if err != nil {
				t.Errorf("Evaluate() error = %v", err)
			}
			if match != tt.wantMatch {
				t.Errorf("Evaluate() = %v, want %v", match, tt.wantMatch)
			}
		})
	}
}

func TestEvaluator_Evaluate_UserAgentFilter(t *testing.T) {
	// Test case from setup script: policy-string-test bucket
	tests := []struct {
		name      string
		userAgent string
		wantMatch bool
	}{
		{
			name:      "aws-cli matches",
			userAgent: "aws-cli/2.0.0",
			wantMatch: true,
		},
		{
			name:      "boto3 matches",
			userAgent: "Boto3/1.20.0 Python/3.9.0",
			wantMatch: true,
		},
		{
			name:      "minio-mc matches",
			userAgent: "MinIO (linux; amd64) minio-go/v7.0.0 mc/RELEASE.2021-10-07T04-19-58Z",
			wantMatch: true,
		},
		{
			name:      "browser does not match",
			userAgent: "Mozilla/5.0",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				UserAgent: tt.userAgent,
			}
			eval := NewEvaluator(ctx)

			conditions := map[string]any{
				"StringLike": map[string]any{
					"aws:UserAgent": []any{"aws-cli/*", "Boto3/*", "MinIO*"},
				},
			}

			match, err := eval.Evaluate(conditions)
			if err != nil {
				t.Errorf("Evaluate() error = %v", err)
			}
			if match != tt.wantMatch {
				t.Errorf("Evaluate() = %v, want %v", match, tt.wantMatch)
			}
		})
	}
}

func TestEvaluator_Evaluate_ContentSizeLimit(t *testing.T) {
	// Test case from setup script: policy-numeric-test bucket
	tests := []struct {
		name          string
		contentLength int64
		wantMatch     bool
	}{
		{
			name:          "small file allowed",
			contentLength: 1024 * 1024, // 1MB
			wantMatch:     true,
		},
		{
			name:          "exactly at limit",
			contentLength: 10 * 1024 * 1024, // 10MB
			wantMatch:     true,
		},
		{
			name:          "over limit denied",
			contentLength: 11 * 1024 * 1024, // 11MB
			wantMatch:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				ContentLength: tt.contentLength,
			}
			eval := NewEvaluator(ctx)

			conditions := map[string]any{
				"NumericLessThanEquals": map[string]any{
					"s3:content-length": float64(10 * 1024 * 1024), // 10MB limit
				},
			}

			match, err := eval.Evaluate(conditions)
			if err != nil {
				t.Errorf("Evaluate() error = %v", err)
			}
			if match != tt.wantMatch {
				t.Errorf("Evaluate() = %v, want %v", match, tt.wantMatch)
			}
		})
	}
}

func TestEvaluator_Evaluate_SecureTransport(t *testing.T) {
	tests := []struct {
		name            string
		secureTransport bool
		required        bool
		wantMatch       bool
	}{
		{
			name:            "HTTPS required and used",
			secureTransport: true,
			required:        true,
			wantMatch:       true,
		},
		{
			name:            "HTTPS required but HTTP used",
			secureTransport: false,
			required:        true,
			wantMatch:       false,
		},
		{
			name:            "HTTP allowed and used",
			secureTransport: false,
			required:        false,
			wantMatch:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				SecureTransport: tt.secureTransport,
			}
			eval := NewEvaluator(ctx)

			conditions := map[string]any{
				"Bool": map[string]any{
					"aws:SecureTransport": tt.required,
				},
			}

			match, err := eval.Evaluate(conditions)
			if err != nil {
				t.Errorf("Evaluate() error = %v", err)
			}
			if match != tt.wantMatch {
				t.Errorf("Evaluate() = %v, want %v", match, tt.wantMatch)
			}
		})
	}
}

func TestEvaluator_Evaluate_MissingContextValue(t *testing.T) {
	// Empty context - missing required values should fail
	ctx := &Context{}
	eval := NewEvaluator(ctx)

	conditions := map[string]any{
		"StringEquals": map[string]any{
			"aws:username": "alice",
		},
	}

	match, err := eval.Evaluate(conditions)
	if err == nil {
		t.Error("Evaluate() should return error for missing context value")
	}
	if match {
		t.Error("Evaluate() should not match when context value is missing")
	}
}

func TestEvaluator_Evaluate_UnknownOperator(t *testing.T) {
	ctx := &Context{
		Username: "alice",
	}
	eval := NewEvaluator(ctx)

	conditions := map[string]any{
		"UnknownOperator": map[string]any{
			"aws:username": "alice",
		},
	}

	match, err := eval.Evaluate(conditions)
	if err == nil {
		t.Error("Evaluate() should return error for unknown operator")
	}
	if match {
		t.Error("Evaluate() should not match with unknown operator")
	}
}

func TestEvaluator_Evaluate_InvalidConditionFormat(t *testing.T) {
	ctx := &Context{}
	eval := NewEvaluator(ctx)

	// Invalid format: operator value is not a map
	conditions := map[string]any{
		"StringEquals": "not a map",
	}

	match, err := eval.Evaluate(conditions)
	if err == nil {
		t.Error("Evaluate() should return error for invalid condition format")
	}
	if match {
		t.Error("Evaluate() should not match with invalid format")
	}
}

func TestEvaluator_getContextValue(t *testing.T) {
	sourceIP := net.ParseIP("192.168.1.100")
	currentTime := time.Now()

	ctx := &Context{
		Username:        "alice",
		UserID:          "550e8400-e29b-41d4-a716-446655440000",
		SourceIP:        sourceIP,
		UserAgent:       "aws-cli/2.0.0",
		SecureTransport: true,
		CurrentTime:     currentTime,
		S3Prefix:        "data/",
		S3Delimiter:     "/",
		ContentLength:   1024,
	}
	eval := NewEvaluator(ctx)

	tests := []struct {
		key       string
		wantValue any
		wantErr   bool
	}{
		{"aws:username", "alice", false},
		{"aws:userid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"aws:SourceIp", sourceIP, false},
		{"aws:UserAgent", "aws-cli/2.0.0", false},
		{"aws:SecureTransport", true, false},
		{"aws:CurrentTime", currentTime, false},
		{"s3:prefix", "data/", false},
		{"s3:delimiter", "/", false},
		{"s3:content-length", int64(1024), false},
		{"unknown:key", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := eval.getContextValue(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("getContextValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Special handling for net.IP and time.Time which aren't directly comparable
				switch want := tt.wantValue.(type) {
				case net.IP:
					if gotIP, ok := got.(net.IP); !ok || !gotIP.Equal(want) {
						t.Errorf("getContextValue() = %v, want %v", got, tt.wantValue)
					}
				case time.Time:
					if gotTime, ok := got.(time.Time); !ok || !gotTime.Equal(want) {
						t.Errorf("getContextValue() = %v, want %v", got, tt.wantValue)
					}
				default:
					if got != tt.wantValue {
						t.Errorf("getContextValue() = %v, want %v", got, tt.wantValue)
					}
				}
			}
		})
	}
}
