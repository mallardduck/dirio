package conditions

import (
	"net"
	"testing"
	"time"
)

// String Operator Tests

func TestEvaluateStringEquals(t *testing.T) {
	tests := []struct {
		name         string
		contextValue interface{}
		condValue    interface{}
		ignoreCase   bool
		want         bool
		wantErr      bool
	}{
		{
			name:         "exact match",
			contextValue: "alice",
			condValue:    "alice",
			ignoreCase:   false,
			want:         true,
		},
		{
			name:         "no match",
			contextValue: "alice",
			condValue:    "bob",
			ignoreCase:   false,
			want:         false,
		},
		{
			name:         "case sensitive mismatch",
			contextValue: "Alice",
			condValue:    "alice",
			ignoreCase:   false,
			want:         false,
		},
		{
			name:         "case insensitive match",
			contextValue: "Alice",
			condValue:    "alice",
			ignoreCase:   true,
			want:         true,
		},
		{
			name:         "empty strings match",
			contextValue: "",
			condValue:    "",
			ignoreCase:   false,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateStringEquals(tt.contextValue, tt.condValue, tt.ignoreCase)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluateStringEquals() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateStringEquals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateStringLike(t *testing.T) {
	tests := []struct {
		name         string
		contextValue interface{}
		pattern      interface{}
		negate       bool
		want         bool
		wantErr      bool
	}{
		// Standard matching (negate=false)
		{
			name:         "wildcard match all",
			contextValue: "aws-cli/2.0.0",
			pattern:      "aws-cli/*",
			negate:       false,
			want:         true,
		},
		{
			name:         "wildcard match middle",
			contextValue: "aws-cli/2.0.0",
			pattern:      "aws-*/2.0.0",
			negate:       false,
			want:         true,
		},
		{
			name:         "question mark single char",
			contextValue: "boto3",
			pattern:      "boto?",
			negate:       false,
			want:         true,
		},
		{
			name:         "question mark no match",
			contextValue: "boto",
			pattern:      "boto?",
			negate:       false,
			want:         false,
		},
		{
			name:         "complex pattern",
			contextValue: "user/alice/data.txt",
			pattern:      "user/*/data.txt",
			negate:       false,
			want:         true,
		},
		{
			name:         "no match",
			contextValue: "user/alice/config.txt",
			pattern:      "user/*/data.txt",
			negate:       false,
			want:         false,
		},
		{
			name:         "exact match no wildcards",
			contextValue: "exact",
			pattern:      "exact",
			negate:       false,
			want:         true,
		},
		// Negated matching (negate=true) - inverts results
		{
			name:         "negate: wildcard match should fail",
			contextValue: "aws-cli/2.0.0",
			pattern:      "aws-cli/*",
			negate:       true,
			want:         false,
		},
		{
			name:         "negate: no match should succeed",
			contextValue: "user/alice/config.txt",
			pattern:      "user/*/data.txt",
			negate:       true,
			want:         true,
		},
		{
			name:         "negate: question mark no match should succeed",
			contextValue: "boto",
			pattern:      "boto?",
			negate:       true,
			want:         true,
		},
		{
			name:         "negate: exact match should fail",
			contextValue: "exact",
			pattern:      "exact",
			negate:       true,
			want:         false,
		},
		{
			name:         "negate: different value should succeed",
			contextValue: "python-sdk/1.0.0",
			pattern:      "aws-cli/*",
			negate:       true,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateStringLike(tt.contextValue, tt.pattern, tt.negate)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluateStringLike() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateStringLike(negate=%v) = %v, want %v", tt.negate, got, tt.want)
			}
		})
	}
}

func TestGlobToRegex(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"*", "anything", true},
		{"*.txt", "file.txt", true},
		{"*.txt", "file.csv", false},
		{"test?", "test1", true},
		{"test?", "test", false},
		{"a*b*c", "abc", true},
		{"a*b*c", "aXbYc", true},
		{"a*b*c", "ac", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"->"+tt.input, func(t *testing.T) {
			regex := globToRegex(tt.pattern)
			got, _ := evaluateStringLike(tt.input, tt.pattern, false)
			if got != tt.want {
				t.Errorf("pattern %q, input %q: got %v, want %v (regex: %s)", tt.pattern, tt.input, got, tt.want, regex)
			}
		})
	}
}

// Numeric Operator Tests

func TestEvaluateNumericEquals(t *testing.T) {
	tests := []struct {
		name         string
		contextValue interface{}
		condValue    interface{}
		want         bool
		wantErr      bool
	}{
		{
			name:         "equal floats",
			contextValue: 42.0,
			condValue:    42.0,
			want:         true,
		},
		{
			name:         "equal int and float",
			contextValue: 42,
			condValue:    42.0,
			want:         true,
		},
		{
			name:         "not equal",
			contextValue: 42.0,
			condValue:    43.0,
			want:         false,
		},
		{
			name:         "string number",
			contextValue: "42",
			condValue:    42.0,
			want:         true,
		},
		{
			name:         "int64",
			contextValue: int64(1024),
			condValue:    1024,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateNumericEquals(tt.contextValue, tt.condValue)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluateNumericEquals() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateNumericEquals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateNumericLessThan(t *testing.T) {
	tests := []struct {
		name         string
		contextValue interface{}
		condValue    interface{}
		orEqual      bool
		want         bool
	}{
		{
			name:         "less than",
			contextValue: 10.0,
			condValue:    20.0,
			orEqual:      false,
			want:         true,
		},
		{
			name:         "not less than",
			contextValue: 30.0,
			condValue:    20.0,
			orEqual:      false,
			want:         false,
		},
		{
			name:         "equal not less than",
			contextValue: 20.0,
			condValue:    20.0,
			orEqual:      false,
			want:         false,
		},
		{
			name:         "less than or equal - equal",
			contextValue: 20.0,
			condValue:    20.0,
			orEqual:      true,
			want:         true,
		},
		{
			name:         "less than or equal - less",
			contextValue: 10.0,
			condValue:    20.0,
			orEqual:      true,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateNumericLessThan(tt.contextValue, tt.condValue, tt.orEqual)
			if err != nil {
				t.Errorf("evaluateNumericLessThan() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateNumericLessThan() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateNumericGreaterThan(t *testing.T) {
	tests := []struct {
		name         string
		contextValue interface{}
		condValue    interface{}
		orEqual      bool
		want         bool
	}{
		{
			name:         "greater than",
			contextValue: 30.0,
			condValue:    20.0,
			orEqual:      false,
			want:         true,
		},
		{
			name:         "not greater than",
			contextValue: 10.0,
			condValue:    20.0,
			orEqual:      false,
			want:         false,
		},
		{
			name:         "equal not greater than",
			contextValue: 20.0,
			condValue:    20.0,
			orEqual:      false,
			want:         false,
		},
		{
			name:         "greater than or equal - equal",
			contextValue: 20.0,
			condValue:    20.0,
			orEqual:      true,
			want:         true,
		},
		{
			name:         "greater than or equal - greater",
			contextValue: 30.0,
			condValue:    20.0,
			orEqual:      true,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateNumericGreaterThan(tt.contextValue, tt.condValue, tt.orEqual)
			if err != nil {
				t.Errorf("evaluateNumericGreaterThan() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateNumericGreaterThan() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Date Operator Tests

func TestEvaluateDateEquals(t *testing.T) {
	time1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	time2 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	time3 := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		contextValue interface{}
		condValue    interface{}
		want         bool
		wantErr      bool
	}{
		{
			name:         "equal times",
			contextValue: time1,
			condValue:    time2,
			want:         true,
		},
		{
			name:         "not equal times",
			contextValue: time1,
			condValue:    time3,
			want:         false,
		},
		{
			name:         "string time",
			contextValue: time1,
			condValue:    "2026-01-01T12:00:00Z",
			want:         true,
		},
		{
			name:         "RFC3339 format",
			contextValue: "2026-01-01T12:00:00Z",
			condValue:    time1,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateDateEquals(tt.contextValue, tt.condValue)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluateDateEquals() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateDateEquals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateDateLessThan(t *testing.T) {
	earlier := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	later := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		contextValue interface{}
		condValue    interface{}
		orEqual      bool
		want         bool
	}{
		{
			name:         "before",
			contextValue: earlier,
			condValue:    later,
			orEqual:      false,
			want:         true,
		},
		{
			name:         "after",
			contextValue: later,
			condValue:    earlier,
			orEqual:      false,
			want:         false,
		},
		{
			name:         "equal not before",
			contextValue: earlier,
			condValue:    earlier,
			orEqual:      false,
			want:         false,
		},
		{
			name:         "before or equal - equal",
			contextValue: earlier,
			condValue:    earlier,
			orEqual:      true,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateDateLessThan(tt.contextValue, tt.condValue, tt.orEqual)
			if err != nil {
				t.Errorf("evaluateDateLessThan() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateDateLessThan() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateDateGreaterThan(t *testing.T) {
	earlier := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	later := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		contextValue interface{}
		condValue    interface{}
		orEqual      bool
		want         bool
	}{
		{
			name:         "after",
			contextValue: later,
			condValue:    earlier,
			orEqual:      false,
			want:         true,
		},
		{
			name:         "before",
			contextValue: earlier,
			condValue:    later,
			orEqual:      false,
			want:         false,
		},
		{
			name:         "equal not after",
			contextValue: earlier,
			condValue:    earlier,
			orEqual:      false,
			want:         false,
		},
		{
			name:         "after or equal - equal",
			contextValue: earlier,
			condValue:    earlier,
			orEqual:      true,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateDateGreaterThan(tt.contextValue, tt.condValue, tt.orEqual)
			if err != nil {
				t.Errorf("evaluateDateGreaterThan() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateDateGreaterThan() = %v, want %v", got, tt.want)
			}
		})
	}
}

// IP Address Operator Tests

func TestEvaluateIpAddress(t *testing.T) {
	tests := []struct {
		name         string
		contextValue interface{}
		cidr         interface{}
		negate       bool
		want         bool
		wantErr      bool
	}{
		// Standard matching (negate=false)
		{
			name:         "IP in CIDR range",
			contextValue: net.ParseIP("192.168.1.100"),
			cidr:         "192.168.1.0/24",
			negate:       false,
			want:         true,
		},
		{
			name:         "IP not in CIDR range",
			contextValue: net.ParseIP("192.168.2.100"),
			cidr:         "192.168.1.0/24",
			negate:       false,
			want:         false,
		},
		{
			name:         "exact IP match",
			contextValue: net.ParseIP("192.168.1.100"),
			cidr:         "192.168.1.100/32",
			negate:       false,
			want:         true,
		},
		{
			name:         "plain IP without CIDR",
			contextValue: net.ParseIP("192.168.1.100"),
			cidr:         "192.168.1.100",
			negate:       false,
			want:         true,
		},
		{
			name:         "IPv6 in range",
			contextValue: net.ParseIP("2001:db8::1"),
			cidr:         "2001:db8::/32",
			negate:       false,
			want:         true,
		},
		{
			name:         "IPv6 not in range",
			contextValue: net.ParseIP("2001:db9::1"),
			cidr:         "2001:db8::/32",
			negate:       false,
			want:         false,
		},
		{
			name:         "string IP in CIDR",
			contextValue: "192.168.1.100",
			cidr:         "192.168.1.0/24",
			negate:       false,
			want:         true,
		},
		{
			name:         "large network",
			contextValue: net.ParseIP("10.0.5.100"),
			cidr:         "10.0.0.0/8",
			negate:       false,
			want:         true,
		},
		// Negated matching (negate=true) - inverts results
		{
			name:         "negate: IP in CIDR should fail",
			contextValue: net.ParseIP("192.168.1.100"),
			cidr:         "192.168.1.0/24",
			negate:       true,
			want:         false,
		},
		{
			name:         "negate: IP not in CIDR should succeed",
			contextValue: net.ParseIP("192.168.2.100"),
			cidr:         "192.168.1.0/24",
			negate:       true,
			want:         true,
		},
		{
			name:         "negate: exact IP match should fail",
			contextValue: net.ParseIP("192.168.1.100"),
			cidr:         "192.168.1.100",
			negate:       true,
			want:         false,
		},
		{
			name:         "negate: different IP should succeed",
			contextValue: net.ParseIP("192.168.1.50"),
			cidr:         "192.168.1.100",
			negate:       true,
			want:         true,
		},
		{
			name:         "negate: IPv6 in range should fail",
			contextValue: net.ParseIP("2001:db8::1"),
			cidr:         "2001:db8::/32",
			negate:       true,
			want:         false,
		},
		{
			name:         "negate: IPv6 not in range should succeed",
			contextValue: net.ParseIP("2001:db9::1"),
			cidr:         "2001:db8::/32",
			negate:       true,
			want:         true,
		},
		{
			name:         "negate: outside large network should succeed",
			contextValue: net.ParseIP("192.168.1.100"),
			cidr:         "10.0.0.0/8",
			negate:       true,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateIpAddress(tt.contextValue, tt.cidr, tt.negate)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluateIpAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateIpAddress(negate=%v) = %v, want %v", tt.negate, got, tt.want)
			}
		})
	}
}

// Boolean Operator Tests

func TestEvaluateBool(t *testing.T) {
	tests := []struct {
		name         string
		contextValue interface{}
		condValue    interface{}
		want         bool
		wantErr      bool
	}{
		{
			name:         "true equals true",
			contextValue: true,
			condValue:    true,
			want:         true,
		},
		{
			name:         "false equals false",
			contextValue: false,
			condValue:    false,
			want:         true,
		},
		{
			name:         "true not equals false",
			contextValue: true,
			condValue:    false,
			want:         false,
		},
		{
			name:         "string true",
			contextValue: "true",
			condValue:    true,
			want:         true,
		},
		{
			name:         "string false",
			contextValue: "false",
			condValue:    false,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateBool(tt.contextValue, tt.condValue)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluateBool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateBool() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Null Operator Tests

func TestEvaluateNull(t *testing.T) {
	tests := []struct {
		name         string
		contextValue interface{}
		shouldBeNull interface{}
		want         bool
	}{
		{
			name:         "null is null",
			contextValue: nil,
			shouldBeNull: true,
			want:         true,
		},
		{
			name:         "empty string is null",
			contextValue: "",
			shouldBeNull: true,
			want:         true,
		},
		{
			name:         "value is not null",
			contextValue: "something",
			shouldBeNull: false,
			want:         true,
		},
		{
			name:         "null should not be null",
			contextValue: nil,
			shouldBeNull: false,
			want:         false,
		},
		{
			name:         "value should be null",
			contextValue: "something",
			shouldBeNull: true,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateNull(tt.contextValue, tt.shouldBeNull)
			if err != nil {
				t.Errorf("evaluateNull() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateNull() = %v, want %v", got, tt.want)
			}
		})
	}
}
