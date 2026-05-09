package models

import (
	"time"

	"github.com/google/uuid"
)

type TerraformProviderGPGKey struct {
	ID             uuid.UUID `gorm:"primary_key;default:uuid_generate_v4()"`
	ProviderSource string
	KeyID          string
	Fingerprint    string
	ASCIIArmor     string
	TrustMode      string
	PinnedAt       *time.Time
}

func (k *TerraformProviderGPGKey) TableName() string {
	return "terraform_provider_gpg_keys"
}
