package registry_test

import (
	"bytes"
	"crypto"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
)

func TestVerifySignature(t *testing.T) {
	entity := newTestEntity(t)
	sums := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  provider.zip\n")
	signature := detachedSignature(t, entity, sums)

	tests := []struct {
		name        string
		sums        []byte
		signature   []byte
		trustedKeys openpgp.EntityList
		wantErr     bool
	}{
		{name: "valid HashiCorp signature", sums: sums, signature: signature, trustedKeys: openpgp.EntityList{entity}},
		{name: "invalid signature", sums: sums, signature: corruptSignature(signature), trustedKeys: openpgp.EntityList{entity}, wantErr: true},
		{name: "missing signature", sums: sums, trustedKeys: openpgp.EntityList{entity}, wantErr: true},
		{name: "corrupted SHA256SUMS", sums: append([]byte("x"), sums...), signature: signature, trustedKeys: openpgp.EntityList{entity}, wantErr: true},
		{name: "community provider TOFU without trusted key", sums: sums, signature: signature, trustedKeys: nil, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.VerifySignature(tt.sums, tt.signature, tt.trustedKeys)
			if (err != nil) != tt.wantErr {
				t.Fatalf("VerifySignature() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func corruptSignature(signature []byte) []byte {
	corrupted := append([]byte(nil), signature...)
	corrupted[len(corrupted)/2] = '!'
	return corrupted
}

func newTestEntity(t *testing.T) *openpgp.Entity {
	t.Helper()

	entity, err := openpgp.NewEntity("HashiCorp", "test", "security@hashicorp.com", &packet.Config{
		DefaultHash: crypto.SHA256,
	})
	if err != nil {
		t.Fatalf("NewEntity() error = %v", err)
	}

	return entity
}

func detachedSignature(t *testing.T, entity *openpgp.Entity, message []byte) []byte {
	t.Helper()

	var signature bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&signature, entity, bytes.NewReader(message), &packet.Config{
		DefaultHash: crypto.SHA256,
	}); err != nil {
		t.Fatalf("ArmoredDetachSign() error = %v", err)
	}

	return signature.Bytes()
}
