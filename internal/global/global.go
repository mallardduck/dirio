package global

import (
	"sync/atomic"

	"github.com/google/uuid"
)

var (
	// globalInstanceIDPtr - unique per deployment ID
	globalInstanceIDPtr atomic.Pointer[uuid.UUID]
	SetGlobalInstanceID = func(instanceID uuid.UUID) {
		globalInstanceIDPtr.Store(&instanceID)
	}
	GlobalInstanceID = func() uuid.UUID {
		ptr := globalInstanceIDPtr.Load()
		if ptr == nil {
			return uuid.Nil
		}
		return *ptr
	}
)
