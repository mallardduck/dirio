package fields

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/internal/config/data"
)

func TestSortedKeys(t *testing.T) {
	keys := SortedKeys(SettableFields)
	require.NotEmpty(t, keys)
	for i := 1; i < len(keys); i++ {
		assert.LessOrEqual(t, keys[i-1], keys[i], "keys not sorted at index %d", i)
	}
}

func TestUnknownReadableKeyError(t *testing.T) {
	err := UnknownReadableKeyError("no-such-key")
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "no-such-key")
	// Error message must list at least one known key
	assert.Contains(t, msg, "region")
}

func TestUnknownSettableKeyError(t *testing.T) {
	err := UnknownSettableKeyError("no-such-key")
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "no-such-key")
	assert.Contains(t, msg, "region")
}

func TestSettableFields_StringGet(t *testing.T) {
	dc := &data.ConfigData{Region: "eu-west-1"}
	f, ok := SettableFields["region"]
	require.True(t, ok)
	assert.Equal(t, "eu-west-1", f.Get(dc))
}

func TestSettableFields_StringSet(t *testing.T) {
	dc := &data.ConfigData{}
	f, ok := SettableFields["region"]
	require.True(t, ok)
	require.NoError(t, f.Set(dc, "ap-southeast-1"))
	assert.Equal(t, "ap-southeast-1", dc.Region)
}

func TestSettableFields_BoolField(t *testing.T) {
	dc := &data.ConfigData{}
	f, ok := SettableFields["compression.enabled"]
	require.True(t, ok)

	assert.Equal(t, "false", f.Get(dc))

	require.NoError(t, f.Set(dc, "true"))
	assert.True(t, dc.Compression.Enabled)
	assert.Equal(t, "true", f.Get(dc))

	require.NoError(t, f.Set(dc, "false"))
	assert.False(t, dc.Compression.Enabled)

	err := f.Set(dc, "notabool")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "true or false")
}

func TestSettableFields_AllowEncryption(t *testing.T) {
	dc := &data.ConfigData{}
	f, ok := SettableFields["compression.allow-encryption"]
	require.True(t, ok)
	require.NoError(t, f.Set(dc, "1"))
	assert.True(t, dc.Compression.AllowEncryption)
}

func TestSettableFields_CSVField(t *testing.T) {
	dc := &data.ConfigData{}

	t.Run("extensions set", func(t *testing.T) {
		f := SettableFields["compression.extensions"]
		require.NoError(t, f.Set(dc, ".txt,.log,.json"))
		assert.Equal(t, []string{".txt", ".log", ".json"}, dc.Compression.Extensions)
		assert.Equal(t, ".txt,.log,.json", f.Get(dc))
	})

	t.Run("extensions clear with empty string", func(t *testing.T) {
		f := SettableFields["compression.extensions"]
		require.NoError(t, f.Set(dc, ""))
		assert.Nil(t, dc.Compression.Extensions)
	})

	t.Run("mime-types set with whitespace", func(t *testing.T) {
		f := SettableFields["compression.mime-types"]
		require.NoError(t, f.Set(dc, "text/*, application/json"))
		assert.Equal(t, []string{"text/*", "application/json"}, dc.Compression.MIMETypes)
	})
}

func TestSettableFields_StorageClass(t *testing.T) {
	dc := &data.ConfigData{}

	f := SettableFields["storage-class.standard"]
	require.NoError(t, f.Set(dc, "STANDARD"))
	assert.Equal(t, "STANDARD", f.Get(dc))

	f = SettableFields["storage-class.rrs"]
	require.NoError(t, f.Set(dc, "REDUCED_REDUNDANCY"))
	assert.Equal(t, "REDUCED_REDUNDANCY", f.Get(dc))
}

func TestReadableFields_ContainsAllSettable(t *testing.T) {
	for k := range SettableFields {
		_, ok := ReadableFields[k]
		assert.True(t, ok, "ReadableFields missing settable key %q", k)
	}
}

func TestReadableFields_ReadOnlyKeys(t *testing.T) {
	readOnlyKeys := []string{"version", "instance-id", "credentials.access-key", "created-at", "updated-at"}
	for _, k := range readOnlyKeys {
		_, ok := ReadableFields[k]
		assert.True(t, ok, "ReadableFields missing read-only key %q", k)
		_, settable := SettableFields[k]
		assert.False(t, settable, "key %q should not be settable", k)
	}
}

func TestParseBoolValue(t *testing.T) {
	tests := []struct {
		input   string
		want    bool
		wantErr bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"1", true, false},
		{"0", false, false},
		{"T", true, false},
		{"F", false, false},
		{"yes", false, true},
		{"", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseBoolValue(tc.input)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestSplitCSVValue(t *testing.T) {
	assert.Nil(t, splitCSVValue(""))
	assert.Equal(t, []string{"a"}, splitCSVValue("a"))
	assert.Equal(t, []string{"a", "b", "c"}, splitCSVValue("a,b,c"))
	got := splitCSVValue("  a , b , c  ")
	for _, v := range got {
		assert.False(t, strings.HasPrefix(v, " "), "leading space in %q", v)
		assert.False(t, strings.HasSuffix(v, " "), "trailing space in %q", v)
	}
}
