package crypto

import (
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const DerivedKeyLength = 32

func DeriveKey(inputKeyMaterial []byte, salt []byte, info []byte) ([]byte, error) {
	if len(inputKeyMaterial) < 16 {
		return nil, fmt.Errorf("input key material must be at least 16 bytes")
	}
	reader := hkdf.New(sha256.New, inputKeyMaterial, salt, info)
	key := make([]byte, DerivedKeyLength)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return key, nil
}
