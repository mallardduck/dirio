package hostname

import (
	"sync"
)

var (
	onceIdentity sync.Once
	identity     string
)

// Identity returns the stable, canonical host identity used as the base
// for mDNS hostnames. When a hostname is explicitly configured (via env var
// or OS hostname), returns just the base. Otherwise appends a stable unique
// suffix to disambiguate: <base>-<unique-id> (e.g., "dirio-s3-abc123").
func Identity() string {
	onceIdentity.Do(func() {
		base := Base()
		if baseIsExplicit() {
			identity = base
		} else {
			identity = base + "-" + stableID()
		}
	})
	return identity
}
