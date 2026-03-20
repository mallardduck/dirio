package hostname

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentity(t *testing.T) {
	// Reset the sync.Once for testing
	onceIdentity = sync.Once{}
	identity = ""

	t.Run("returns consistent identity", func(t *testing.T) {
		id1 := Identity()
		id2 := Identity()
		assert.Equal(t, id1, id2, "Identity should return the same value on multiple calls")
	})

	t.Run("identity is not empty", func(t *testing.T) {
		id := Identity()
		assert.NotEmpty(t, id)
	})

	t.Run("identity is at least one label", func(t *testing.T) {
		id := Identity()
		assert.NotEmpty(t, id)
		assert.NotContains(t, id, " ", "Identity should have no spaces")
	})

	t.Run("identity is mDNS safe", func(t *testing.T) {
		id := Identity()
		// Check that it only contains allowed characters
		for _, r := range id {
			assert.True(t,
				(r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-',
				"Identity should only contain lowercase letters, numbers, and hyphens")
		}
	})
}
