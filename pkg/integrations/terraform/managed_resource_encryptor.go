package terraform

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"

	"github.com/google/uuid"
)

func managedResourceAssociatedData(nonce []byte, fields ...any) []byte {
	h := sha256.New()
	for _, field := range fields {
		switch value := field.(type) {
		case uuid.UUID:
			h.Write(value[:])
		case string:
			writeLengthPrefixedString(h, value)
		}
	}
	h.Write(nonce)
	return h.Sum(nil)
}

func writeLengthPrefixedString(h hash.Hash, value string) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	h.Write(length[:])
	h.Write([]byte(value))
}
