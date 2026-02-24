package jsonutil

import (
	"io"
	"os"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testStruct struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Tags  []string `json:"tags"`
}

func TestIsDebugMode(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{
			name:     "debug mode enabled with 1",
			envValue: "1",
			expected: true,
		},
		{
			name:     "debug mode enabled with true",
			envValue: "true",
			expected: true,
		},
		{
			name:     "debug mode disabled with 0",
			envValue: "0",
			expected: false,
		},
		{
			name:     "debug mode disabled with false",
			envValue: "false",
			expected: false,
		},
		{
			name:     "debug mode disabled when not set",
			envValue: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original env
			original := os.Getenv("DIRIO_DEBUG")
			defer os.Setenv("DIRIO_DEBUG", original)

			if tt.envValue == "" {
				os.Unsetenv("DIRIO_DEBUG")
			} else {
				os.Setenv("DIRIO_DEBUG", tt.envValue)
			}

			result := isDebugMode()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMarshal_ProductionMode(t *testing.T) {
	// Save and restore original env
	original := os.Getenv("DIRIO_DEBUG")
	defer os.Setenv("DIRIO_DEBUG", original)

	// Set production mode
	os.Unsetenv("DIRIO_DEBUG")

	data := testStruct{
		Name:  "test",
		Count: 42,
		Tags:  []string{"tag1", "tag2"},
	}

	result, err := Marshal(data)
	require.NoError(t, err)

	// Should be compact (no newlines or indentation)
	expected := `{"name":"test","count":42,"tags":["tag1","tag2"]}`
	assert.JSONEq(t, expected, string(result))
	assert.NotContains(t, string(result), "\n", "production mode should produce compact JSON")
}

func TestMarshal_DebugMode(t *testing.T) {
	// Save and restore original env
	original := os.Getenv("DIRIO_DEBUG")
	defer os.Setenv("DIRIO_DEBUG", original)

	// Set debug mode
	os.Setenv("DIRIO_DEBUG", "1")

	data := testStruct{
		Name:  "test",
		Count: 42,
		Tags:  []string{"tag1", "tag2"},
	}

	result, err := Marshal(data)
	require.NoError(t, err)

	// Should be pretty-printed (with newlines and indentation)
	expected := `{
  "name": "test",
  "count": 42,
  "tags": [
    "tag1",
    "tag2"
  ]
}`
	assert.Equal(t, expected, string(result))
	assert.Contains(t, string(result), "\n", "debug mode should produce pretty JSON")
	assert.Contains(t, string(result), "  ", "debug mode should include indentation")
}

func TestMarshalToFile_ProductionMode(t *testing.T) {
	// Save and restore original env
	original := os.Getenv("DIRIO_DEBUG")
	defer os.Setenv("DIRIO_DEBUG", original)

	// Set production mode
	os.Unsetenv("DIRIO_DEBUG")

	fs := memfs.New()
	path := "test.json"

	data := testStruct{
		Name:  "test",
		Count: 42,
		Tags:  []string{"tag1", "tag2"},
	}

	err := MarshalToFile(fs, path, data)
	require.NoError(t, err)

	// Read back the file
	file, err := fs.Open(path)
	require.NoError(t, err)
	defer file.Close()

	content, err := io.ReadAll(file)
	require.NoError(t, err)

	// Should be compact
	expected := `{"name":"test","count":42,"tags":["tag1","tag2"]}`
	assert.JSONEq(t, expected, string(content))
	assert.NotContains(t, string(content), "\n", "production mode should produce compact JSON")
}

func TestMarshalToFile_DebugMode(t *testing.T) {
	// Save and restore original env
	original := os.Getenv("DIRIO_DEBUG")
	defer os.Setenv("DIRIO_DEBUG", original)

	// Set debug mode
	os.Setenv("DIRIO_DEBUG", "true")

	fs := memfs.New()
	path := "test.json"

	data := testStruct{
		Name:  "test",
		Count: 42,
		Tags:  []string{"tag1", "tag2"},
	}

	err := MarshalToFile(fs, path, data)
	require.NoError(t, err)

	// Read back the file
	file, err := fs.Open(path)
	require.NoError(t, err)
	defer file.Close()

	content, err := io.ReadAll(file)
	require.NoError(t, err)

	// Should be pretty-printed
	expected := `{
  "name": "test",
  "count": 42,
  "tags": [
    "tag1",
    "tag2"
  ]
}`
	assert.Equal(t, expected, string(content))
	assert.Contains(t, string(content), "\n", "debug mode should produce pretty JSON")
}

func TestUnmarshal(t *testing.T) {
	jsonData := []byte(`{"name":"test","count":42,"tags":["tag1","tag2"]}`)

	var result testStruct
	err := Unmarshal(jsonData, &result)
	require.NoError(t, err)

	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 42, result.Count)
	assert.Equal(t, []string{"tag1", "tag2"}, result.Tags)
}

func TestMarshal_InvalidInput(t *testing.T) {
	// Channel cannot be marshaled to JSON
	invalidData := make(chan int)

	_, err := Marshal(invalidData)
	require.Error(t, err)
}

func TestMarshalToFile_MarshalError(t *testing.T) {
	fs := memfs.New()
	path := "test.json"

	// Channel cannot be marshaled to JSON
	invalidData := make(chan int)

	err := MarshalToFile(fs, path, invalidData)
	require.Error(t, err)
}

func TestMarshalToFile_FileSystemError(t *testing.T) {
	fs := memfs.New()

	// Create a directory with the same name to cause a conflict
	err := fs.MkdirAll("test.json", 0o755)
	require.NoError(t, err)

	data := testStruct{Name: "test"}

	// Should fail because path exists as directory
	err = MarshalToFile(fs, "test.json", data)
	require.Error(t, err)
}

// Note: Config-based debug mode testing requires integration tests
// since config.GetConfig() is a global singleton.
// The behavior is tested indirectly through the environment variable tests above.
// In real usage:
// - --debug flag sets config.Settings.Debug = true
// - --log-level=debug AND --verbosity=verbose both set to enable debug mode
