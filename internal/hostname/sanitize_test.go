package hostname

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase letters and numbers",
			input:    "abc123",
			expected: "abc123",
		},
		{
			name:     "uppercase converted to lowercase",
			input:    "ABC123",
			expected: "abc123",
		},
		{
			name:     "hyphens preserved",
			input:    "my-host-name",
			expected: "my-host-name",
		},
		{
			name:     "spaces replaced with hyphens",
			input:    "my host name",
			expected: "my-host-name",
		},
		{
			name:     "special characters replaced",
			input:    "host@name#test",
			expected: "host-name-test",
		},
		{
			name:     "leading hyphens trimmed",
			input:    "---hostname",
			expected: "hostname",
		},
		{
			name:     "trailing hyphens trimmed",
			input:    "hostname---",
			expected: "hostname",
		},
		{
			name:     "multiple consecutive hyphens",
			input:    "host---name",
			expected: "host---name",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special characters",
			input:    "@#$%",
			expected: "",
		},
		{
			name:     "exceeds max length",
			input:    strings.Repeat("a", 100),
			expected: strings.Repeat("a", 63),
		},
		{
			name:     "unicode characters replaced",
			input:    "host-名前-name",
			expected: "host----name",
		},
		{
			name:     "dots replaced",
			input:    "host.example.com",
			expected: "host-example-com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeMaxLength(t *testing.T) {
	input := strings.Repeat("a", 100)
	result := sanitize(input)
	assert.Len(t, result, maxLabelLen)
	assert.Equal(t, strings.Repeat("a", maxLabelLen), result)
}
