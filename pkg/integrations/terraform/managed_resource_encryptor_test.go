package terraform

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestManagedResourceAssociatedData(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	nonce := []byte("0123456789abcdef")

	first := managedResourceAssociatedData(nonce, id, "abc", "def")
	second := managedResourceAssociatedData(nonce, id, "ab", "cdef")
	again := managedResourceAssociatedData(nonce, id, "abc", "def")

	require.False(t, bytes.Equal(first, second))
	require.Equal(t, first, again)
}
