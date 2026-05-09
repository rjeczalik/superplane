package crypto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveKeyReturnsAES256Key(t *testing.T) {
	key, err := DeriveKey([]byte("0123456789abcdef"), []byte("salt"), []byte("terraform-state"))
	require.NoError(t, err)
	assert.Len(t, key, 32)
}

func TestDeriveKeyDeterministic(t *testing.T) {
	ikm := []byte("0123456789abcdef")
	key1, err := DeriveKey(ikm, []byte("salt"), []byte("info"))
	require.NoError(t, err)
	key2, err := DeriveKey(ikm, []byte("salt"), []byte("info"))
	require.NoError(t, err)
	assert.Equal(t, key1, key2)
}

func TestDeriveKeyDifferentSaltChangesOutput(t *testing.T) {
	ikm := []byte("0123456789abcdef")
	key1, err := DeriveKey(ikm, []byte("salt-1"), []byte("info"))
	require.NoError(t, err)
	key2, err := DeriveKey(ikm, []byte("salt-2"), []byte("info"))
	require.NoError(t, err)
	assert.False(t, bytes.Equal(key1, key2))
}

func TestDeriveKeyRejectsShortInputKeyMaterial(t *testing.T) {
	_, err := DeriveKey([]byte("too-short"), []byte("salt"), []byte("info"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 16 bytes")
}
