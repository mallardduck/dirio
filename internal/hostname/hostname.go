package hostname

import (
	"sync"
)

var (
	onceIdentity sync.Once
	identity     string
)

// Identity returns the stable, canonical host identity used as the base
// for mDNS hostnames. Format: <base>-<unique-id> (e.g., "macbook-abc123").
func Identity() string {
	onceIdentity.Do(func() {
		identity = Base() + "-" + stableID()
	})
	return identity
}
