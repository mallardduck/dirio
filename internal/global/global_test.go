package global

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGlobalInstanceID_DefaultsToNil(t *testing.T) {
	// Reset to nil before testing the zero state.
	globalInstanceIDPtr.Store(nil)
	assert.Equal(t, uuid.Nil, GlobalInstanceID())
}

func TestSetAndGetGlobalInstanceID(t *testing.T) {
	id := uuid.MustParse("12345678-1234-5678-1234-567812345678")
	SetGlobalInstanceID(id)
	assert.Equal(t, id, GlobalInstanceID())
}

func TestSetGlobalInstanceID_Overwrite(t *testing.T) {
	first := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	second := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	SetGlobalInstanceID(first)
	assert.Equal(t, first, GlobalInstanceID())

	SetGlobalInstanceID(second)
	assert.Equal(t, second, GlobalInstanceID())
}
