package terraform

import (
	"context"
	"errors"
	"fmt"

	"github.com/superplanehq/superplane/pkg/crypto"
)

const (
	TerraformStateEncryptionV1 = "raw-key-ad-v1"
	TerraformStateEncryptionV2 = "hkdf-ad-v2"

	terraformStateHKDFSalt = "superplane-terraform-state-salt-v1"
	terraformStateHKDFInfo = "superplane-terraform-state-v2"
)

var ErrTerraformStateMigrationRequired = errors.New("terraform state migration required")

type TerraformStateEncryptors struct {
	Legacy  crypto.Encryptor
	Current crypto.Encryptor
}

func NewTerraformStateEncryptors(inputKeyMaterial []byte) (TerraformStateEncryptors, error) {
	derived, err := crypto.DeriveKey(inputKeyMaterial, []byte(terraformStateHKDFSalt), []byte(terraformStateHKDFInfo))
	if err != nil {
		return TerraformStateEncryptors{}, err
	}
	return TerraformStateEncryptors{
		Legacy:  crypto.NewAESGCMEncryptor(inputKeyMaterial),
		Current: crypto.NewAESGCMEncryptor(derived),
	}, nil
}

func TerraformStateEncryptorsFromLegacy(encryptor crypto.Encryptor) TerraformStateEncryptors {
	return TerraformStateEncryptors{Legacy: encryptor, Current: encryptor}
}

func (e TerraformStateEncryptors) Encrypt(ctx context.Context, plaintext []byte, associatedData []byte, version string) ([]byte, error) {
	switch version {
	case TerraformStateEncryptionV1:
		return e.Legacy.Encrypt(ctx, plaintext, associatedData)
	case TerraformStateEncryptionV2:
		return e.Current.Encrypt(ctx, plaintext, associatedData)
	default:
		return nil, fmt.Errorf("unsupported terraform state encryption version %q", version)
	}
}

func (e TerraformStateEncryptors) Decrypt(ctx context.Context, ciphertext []byte, associatedData []byte, version string) ([]byte, error) {
	switch version {
	case TerraformStateEncryptionV1:
		return e.Legacy.Decrypt(ctx, ciphertext, associatedData)
	case TerraformStateEncryptionV2:
		return e.Current.Decrypt(ctx, ciphertext, associatedData)
	default:
		return nil, fmt.Errorf("unsupported terraform state encryption version %q", version)
	}
}

func (e TerraformStateEncryptors) DecryptCurrent(ctx context.Context, ciphertext []byte, associatedData []byte, version string) ([]byte, error) {
	if version != TerraformStateEncryptionV2 {
		return nil, ErrTerraformStateMigrationRequired
	}
	return e.Current.Decrypt(ctx, ciphertext, associatedData)
}
