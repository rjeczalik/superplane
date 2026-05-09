package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superplanehq/superplane/pkg/crypto"
)

func TestBuildEncryptorRejectsNoEncryptionInProduction(t *testing.T) {
	_, err := buildEncryptor("production", "0123456789abcdef0123456789abcdef", "yes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NO_ENCRYPTION=yes is not allowed in production")
}

func TestBuildEncryptorAllowsNoEncryptionOutsideProduction(t *testing.T) {
	encryptor, err := buildEncryptor("development", "0123456789abcdef0123456789abcdef", "yes")
	require.NoError(t, err)
	assert.IsType(t, &crypto.NoOpEncryptor{}, encryptor)
}

func TestBuildEncryptorUsesAESGCMByDefault(t *testing.T) {
	encryptor, err := buildEncryptor("production", "0123456789abcdef0123456789abcdef", "")
	require.NoError(t, err)
	assert.NotNil(t, encryptor)
	_, isNoOp := encryptor.(*crypto.NoOpEncryptor)
	assert.False(t, isNoOp)
}
