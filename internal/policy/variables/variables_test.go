package variables

import (
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContext_Substitute(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	testTime := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	testIP := net.ParseIP("192.168.1.100")

	ctx := &Context{
		Username:    "alice",
		UserID:      testUUID,
		SourceIP:    testIP,
		CurrentTime: testTime,
		UserAgent:   "aws-cli/2.0",
		S3Prefix:    "documents/",
		S3Delimiter: "/",
	}

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "username substitution",
			input:    "arn:aws:s3:::bucket/${aws:username}/*",
			expected: "arn:aws:s3:::bucket/alice/*",
			wantErr:  false,
		},
		{
			name:     "userid substitution",
			input:    "prefix/${aws:userid}/data",
			expected: "prefix/550e8400-e29b-41d4-a716-446655440000/data",
			wantErr:  false,
		},
		{
			name:     "source IP substitution",
			input:    "Allowed from ${aws:SourceIp}",
			expected: "Allowed from 192.168.1.100",
			wantErr:  false,
		},
		{
			name:     "current time substitution",
			input:    "Created at ${aws:CurrentTime}",
			expected: "Created at 2026-02-16T12:00:00Z",
			wantErr:  false,
		},
		{
			name:     "multiple variables",
			input:    "${aws:username}/${aws:userid}/file.txt",
			expected: "alice/550e8400-e29b-41d4-a716-446655440000/file.txt",
			wantErr:  false,
		},
		{
			name:     "no variables",
			input:    "arn:aws:s3:::bucket/static/*",
			expected: "arn:aws:s3:::bucket/static/*",
			wantErr:  false,
		},
		{
			name:     "s3 prefix",
			input:    "prefix=${s3:prefix}",
			expected: "prefix=documents/",
			wantErr:  false,
		},
		{
			name:     "unknown variable",
			input:    "${aws:unknown}",
			expected: "${aws:unknown}",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.Substitute(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContext_SubstituteSlice(t *testing.T) {
	ctx := &Context{
		Username: "alice",
		UserID:   uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
	}

	input := []string{
		"arn:aws:s3:::bucket/${aws:username}/*",
		"arn:aws:s3:::bucket/${aws:userid}/*",
		"arn:aws:s3:::bucket/public/*",
	}

	expected := []string{
		"arn:aws:s3:::bucket/alice/*",
		"arn:aws:s3:::bucket/550e8400-e29b-41d4-a716-446655440000/*",
		"arn:aws:s3:::bucket/public/*",
	}

	result, err := ctx.SubstituteSlice(input)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestHasVariables(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"arn:aws:s3:::bucket/${aws:username}/*", true},
		{"${aws:userid}", true},
		{"arn:aws:s3:::bucket/static/*", false},
		{"", false},
		{"${}", false}, // Empty variable name - still matches pattern
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasVariables(tt.input))
		})
	}
}

func TestExtractVariables(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "arn:aws:s3:::bucket/${aws:username}/*",
			expected: []string{"aws:username"},
		},
		{
			input:    "${aws:username}/${aws:userid}/file.txt",
			expected: []string{"aws:username", "aws:userid"},
		},
		{
			input:    "no variables here",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ExtractVariables(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContext_MissingValues(t *testing.T) {
	// Context with only username set
	ctx := &Context{
		Username: "alice",
	}

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"username available", "${aws:username}", false},
		{"userid missing", "${aws:userid}", true},
		{"sourceip missing", "${aws:SourceIp}", true},
		{"time missing", "${aws:CurrentTime}", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ctx.Substitute(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestContext_SubstituteInterface(t *testing.T) {
	ctx := &Context{
		Username: "alice",
	}

	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{
			name:     "string",
			input:    "arn:aws:s3:::bucket/${aws:username}/*",
			expected: "arn:aws:s3:::bucket/alice/*",
		},
		{
			name:     "string slice",
			input:    []string{"${aws:username}/a", "${aws:username}/b"},
			expected: []string{"alice/a", "alice/b"},
		},
		{
			name:     "interface slice",
			input:    []any{"${aws:username}/a", "${aws:username}/b"},
			expected: []string{"alice/a", "alice/b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.SubstituteInterface(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
