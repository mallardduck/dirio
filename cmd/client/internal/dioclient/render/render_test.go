package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		input    string
		wantMode OutputMode
		wantOK   bool
	}{
		{"tui", ModeTUI, true},
		{"", ModeTUI, true},
		{"plain", ModePlain, true},
		{"json", ModeJSON, true},
		{"unknown", ModeTUI, false},
		{"JSON", ModeTUI, false},
		{"PLAIN", ModeTUI, false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			mode, ok := ParseMode(tc.input)
			assert.Equal(t, tc.wantMode, mode)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}

func TestDetectMode_ReturnsValidMode(t *testing.T) {
	// In test environments stdout is not a TTY, so we expect ModePlain.
	// The important thing is that it returns a defined mode without panicking.
	mode := DetectMode()
	assert.True(t, mode == ModeTUI || mode == ModePlain, "DetectMode returned unexpected value %d", mode)
}

func TestJSON(t *testing.T) {
	t.Run("marshals struct to single JSON line", func(t *testing.T) {
		var out, errOut bytes.Buffer
		type row struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		JSON(&out, &errOut, row{Name: "alice", Age: 30})

		require.Empty(t, errOut.String())
		line := strings.TrimSpace(out.String())
		var got row
		require.NoError(t, json.Unmarshal([]byte(line), &got))
		assert.Equal(t, "alice", got.Name)
		assert.Equal(t, 30, got.Age)
	})

	t.Run("writes error to errW on unmarshalable value", func(t *testing.T) {
		var out, errOut bytes.Buffer
		// channels cannot be JSON-marshalled
		JSON(&out, &errOut, make(chan int))

		assert.Empty(t, out.String())
		assert.Contains(t, errOut.String(), "error:")
	})
}

func TestTable(t *testing.T) {
	headers := []string{"Name", "Status"}
	rows := [][]string{
		{"alice", "active"},
		{"bob", "inactive"},
	}

	t.Run("plain mode writes header and rows", func(t *testing.T) {
		var buf bytes.Buffer
		Table(&buf, headers, rows, ModePlain)
		out := buf.String()
		assert.Contains(t, out, "Name")
		assert.Contains(t, out, "Status")
		assert.Contains(t, out, "alice")
		assert.Contains(t, out, "bob")
		// divider line
		assert.Contains(t, out, "---")
	})

	t.Run("TUI mode writes header and rows", func(t *testing.T) {
		var buf bytes.Buffer
		Table(&buf, headers, rows, ModeTUI)
		out := buf.String()
		assert.Contains(t, out, "alice")
		assert.Contains(t, out, "bob")
	})

	t.Run("empty rows plain mode", func(t *testing.T) {
		var buf bytes.Buffer
		Table(&buf, headers, nil, ModePlain)
		assert.Contains(t, buf.String(), "(no results)")
	})

	t.Run("empty rows TUI mode", func(t *testing.T) {
		var buf bytes.Buffer
		Table(&buf, headers, nil, ModeTUI)
		assert.Contains(t, buf.String(), "(no results)")
	})

	t.Run("row with fewer cells than headers", func(t *testing.T) {
		var buf bytes.Buffer
		Table(&buf, headers, [][]string{{"alice"}}, ModePlain)
		// should not panic; second column rendered as empty
		assert.Contains(t, buf.String(), "alice")
	})
}
