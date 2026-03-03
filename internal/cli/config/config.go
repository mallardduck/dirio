package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mallardduck/dirio/internal/config/data"
)

// configField defines how to read and write a single data-config key via the CLI.
type configField struct {
	description string
	Get         func(dc *data.ConfigData) string
	Set         func(dc *data.ConfigData, value string) error
}

// SettableFields maps CLI key names (hyphen-separated, dot-nested) to
// ConfigData accessors.  Admin credentials are intentionally absent — use
// "dirio credentials Set".
var SettableFields = map[string]configField{
	"region": {
		description: "Geographic/logical region (e.g. us-east-1)",
		Get:         func(dc *data.ConfigData) string { return dc.Region },
		Set:         func(dc *data.ConfigData, v string) error { dc.Region = v; return nil },
	},
	"compression.enabled": {
		description: "Enable server-side compression (true/false)",
		Get:         func(dc *data.ConfigData) string { return strconv.FormatBool(dc.Compression.Enabled) },
		Set: func(dc *data.ConfigData, v string) error {
			b, err := parseBoolValue(v)
			if err != nil {
				return err
			}
			dc.Compression.Enabled = b
			return nil
		},
	},
	"compression.allow-encryption": {
		description: "Allow compression of encrypted objects (true/false)",
		Get:         func(dc *data.ConfigData) string { return strconv.FormatBool(dc.Compression.AllowEncryption) },
		Set: func(dc *data.ConfigData, v string) error {
			b, err := parseBoolValue(v)
			if err != nil {
				return err
			}
			dc.Compression.AllowEncryption = b
			return nil
		},
	},
	"compression.extensions": {
		description: "Comma-separated file extensions to compress (e.g. .txt,.log,.json; empty to clear)",
		Get:         func(dc *data.ConfigData) string { return strings.Join(dc.Compression.Extensions, ",") },
		Set: func(dc *data.ConfigData, v string) error {
			dc.Compression.Extensions = splitCSVValue(v)
			return nil
		},
	},
	"compression.mime-types": {
		description: "Comma-separated MIME types to compress (e.g. text/*,application/json; empty to clear)",
		Get:         func(dc *data.ConfigData) string { return strings.Join(dc.Compression.MIMETypes, ",") },
		Set: func(dc *data.ConfigData, v string) error {
			dc.Compression.MIMETypes = splitCSVValue(v)
			return nil
		},
	},
	"storage-class.standard": {
		description: "Standard storage class label",
		Get:         func(dc *data.ConfigData) string { return dc.StorageClass.Standard },
		Set:         func(dc *data.ConfigData, v string) error { dc.StorageClass.Standard = v; return nil },
	},
	"storage-class.rrs": {
		description: "Reduced Redundancy Storage class label",
		Get:         func(dc *data.ConfigData) string { return dc.StorageClass.RRS },
		Set:         func(dc *data.ConfigData, v string) error { dc.StorageClass.RRS = v; return nil },
	},
}

var ReadableFields = func() map[string]configField {
	m := make(map[string]configField, len(SettableFields)+5)
	for k, v := range SettableFields {
		m[k] = v
	}
	m["version"] = configField{description: "Config format version", Get: func(dc *data.ConfigData) string { return dc.Version }}
	m["instance-id"] = configField{description: "Unique instance identifier", Get: func(dc *data.ConfigData) string { return dc.InstanceID.String() }}
	m["credentials.access-key"] = configField{description: `Admin access key (use "dirio credentials Set" to change)`, Get: func(dc *data.ConfigData) string { return dc.Credentials.AccessKey }}
	m["created-at"] = configField{description: "Config creation timestamp", Get: func(dc *data.ConfigData) string { return dc.CreatedAt.Format(time.RFC3339) }}
	m["updated-at"] = configField{description: "Config last updated timestamp", Get: func(dc *data.ConfigData) string { return dc.UpdatedAt.Format(time.RFC3339) }}
	return m
}()

func SortedKeys(m map[string]configField) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func UnknownReadableKeyError(key string) error {
	return fmt.Errorf("unknown config key %q\n\nReadable keys:\n  %s", key, strings.Join(SortedKeys(ReadableFields), "\n  "))
}

func UnknownSettableKeyError(key string) error {
	return fmt.Errorf("unknown config key %q\n\nSettable keys:\n  %s", key, strings.Join(SortedKeys(SettableFields), "\n  "))
}

func parseBoolValue(v string) (bool, error) {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("expected true or false, got %q", v)
	}
	return b, nil
}

func splitCSVValue(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}
