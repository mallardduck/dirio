package minio

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyList_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected PolicyList
		wantErr  bool
	}{
		{
			name:     "single policy as string",
			input:    `{"policy": "readwrite"}`,
			expected: PolicyList{"readwrite"},
			wantErr:  false,
		},
		{
			name:     "multiple policies as comma-separated string",
			input:    `{"policy": "alpha-rw,beta-rw"}`,
			expected: PolicyList{"alpha-rw", "beta-rw"},
			wantErr:  false,
		},
		{
			name:     "multiple policies with spaces",
			input:    `{"policy": "alpha-rw, beta-rw, gamma-ro"}`,
			expected: PolicyList{"alpha-rw", "beta-rw", "gamma-ro"},
			wantErr:  false,
		},
		{
			name:     "single policy as array",
			input:    `{"policy": ["readwrite"]}`,
			expected: PolicyList{"readwrite"},
			wantErr:  false,
		},
		{
			name:     "multiple policies as array",
			input:    `{"policy": ["alpha-rw", "beta-rw"]}`,
			expected: PolicyList{"alpha-rw", "beta-rw"},
			wantErr:  false,
		},
		{
			name:     "empty string",
			input:    `{"policy": ""}`,
			expected: PolicyList{},
			wantErr:  false,
		},
		{
			name:     "empty array",
			input:    `{"policy": []}`,
			expected: PolicyList{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wrapper struct {
				Policy PolicyList `json:"policy"`
			}

			err := json.Unmarshal([]byte(tt.input), &wrapper)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, wrapper.Policy)
		})
	}
}

func TestPolicyList_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    PolicyList
		expected string
	}{
		{
			name:     "single policy",
			input:    PolicyList{"readwrite"},
			expected: `["readwrite"]`,
		},
		{
			name:     "multiple policies",
			input:    PolicyList{"alpha-rw", "beta-rw"},
			expected: `["alpha-rw","beta-rw"]`,
		},
		{
			name:     "empty list",
			input:    PolicyList{},
			expected: `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := json.Marshal(tt.input)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(result))
		})
	}
}

func TestPolicyList_String(t *testing.T) {
	tests := []struct {
		name     string
		input    PolicyList
		expected string
	}{
		{
			name:     "single policy",
			input:    PolicyList{"readwrite"},
			expected: "readwrite",
		},
		{
			name:     "multiple policies",
			input:    PolicyList{"alpha-rw", "beta-rw"},
			expected: "alpha-rw,beta-rw",
		},
		{
			name:     "empty list",
			input:    PolicyList{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}
