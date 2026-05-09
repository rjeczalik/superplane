package registry

import (
	"bytes"
	"crypto"
	"fmt"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

func VerifySignature(sha256sums, signature []byte, trustedKeys openpgp.EntityList) error {
	if len(sha256sums) == 0 {
		return fmt.Errorf("SHA256SUMS content is required")
	}
	if len(signature) == 0 {
		return fmt.Errorf("SHA256SUMS signature is required")
	}
	if len(trustedKeys) == 0 {
		return fmt.Errorf("no trusted GPG keys configured for provider signature verification")
	}

	if _, err := openpgp.CheckArmoredDetachedSignature(
		trustedKeys,
		bytes.NewReader(sha256sums),
		bytes.NewReader(signature),
		&packet.Config{DefaultHash: crypto.SHA256},
	); err != nil {
		return fmt.Errorf("verify SHA256SUMS signature: %w", err)
	}

	return nil
}
