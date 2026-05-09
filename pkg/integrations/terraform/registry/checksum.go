package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

func VerifyChecksum(archive io.Reader, expectedHex string) error {
	expected, err := hex.DecodeString(expectedHex)
	if err != nil {
		return fmt.Errorf("invalid expected sha256 checksum: %w", err)
	}
	if len(expected) != sha256.Size {
		return fmt.Errorf("invalid expected sha256 checksum length: got %d bytes, want %d", len(expected), sha256.Size)
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, archive); err != nil {
		return fmt.Errorf("read archive for checksum: %w", err)
	}

	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expectedHex) {
		return fmt.Errorf("sha256 checksum mismatch: got %s, want %s", actual, expectedHex)
	}

	return nil
}
