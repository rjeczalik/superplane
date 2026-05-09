package terraform

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTerraformStateEncryptorsRejectsShortKey(t *testing.T) {
	_, err := NewTerraformStateEncryptors([]byte("short"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 16 bytes")
}

func TestTerraformStateEncryptorsLegacyDecryptsV1(t *testing.T) {
	encryptors, err := NewTerraformStateEncryptors([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	ad := []byte("legacy-ad")
	ciphertext, err := encryptors.Encrypt(context.Background(), []byte("state"), ad, TerraformStateEncryptionV1)
	require.NoError(t, err)
	plaintext, err := encryptors.Decrypt(context.Background(), ciphertext, ad, TerraformStateEncryptionV1)
	require.NoError(t, err)
	assert.Equal(t, []byte("state"), plaintext)
}

func TestTerraformStateEncryptorsCurrentUsesHKDFKey(t *testing.T) {
	encryptors, err := NewTerraformStateEncryptors([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	ad := []byte("expanded-ad")
	ciphertext, err := encryptors.Encrypt(context.Background(), []byte("state-v2"), ad, TerraformStateEncryptionV2)
	require.NoError(t, err)
	plaintext, err := encryptors.Decrypt(context.Background(), ciphertext, ad, TerraformStateEncryptionV2)
	require.NoError(t, err)
	assert.Equal(t, []byte("state-v2"), plaintext)

	_, err = encryptors.Decrypt(context.Background(), ciphertext, []byte("other-provider-source"), TerraformStateEncryptionV2)
	require.Error(t, err)
}

func TestTerraformStateEncryptorsCurrentRejectsLegacyVersionWithMigrationError(t *testing.T) {
	encryptors, err := NewTerraformStateEncryptors([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	_, err = encryptors.DecryptCurrent(context.Background(), []byte("ciphertext"), []byte("ad"), TerraformStateEncryptionV1)
	require.ErrorIs(t, err, ErrTerraformStateMigrationRequired)
}
