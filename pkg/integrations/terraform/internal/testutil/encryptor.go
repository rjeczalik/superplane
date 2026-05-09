package testutil

import (
	"github.com/superplanehq/superplane/pkg/crypto"
)

// Encryptor returns a deterministic crypto.Encryptor suitable for tests.
// Currently uses crypto.NoOpEncryptor; swap to AES-GCM with fixed key if
// real encryption is required by callers.
func Encryptor() crypto.Encryptor {
	return crypto.NewNoOpEncryptor()
}
