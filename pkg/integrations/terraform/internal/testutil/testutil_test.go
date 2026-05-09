package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptor(t *testing.T) {
	enc := Encryptor()
	require.NotNil(t, enc)

	ctx := t.Context()
	plaintext := []byte("hello")
	ad := []byte("world")

	ciphertext, err := enc.Encrypt(ctx, plaintext, ad)
	require.NoError(t, err)
	assert.Equal(t, plaintext, ciphertext)

	decrypted, err := enc.Decrypt(ctx, ciphertext, ad)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestSetEnvJSON(t *testing.T) {
	SetEnvJSON(t, "TESTUTIL_KEY", map[string]any{"a": 1})
	assert.Equal(t, `{"a":1}`, GetEnv(t, "TESTUTIL_KEY"))
}

func TestLoadFixture(t *testing.T) {
	// This test requires at least one fixture to exist. We will create a dummy
	// fixture in testdata for this purpose.
	b := LoadFixture(t, "dummy.txt")
	assert.Equal(t, "dummy\n", string(b))
}
