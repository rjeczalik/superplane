package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TerraformManagedResourceState struct {
	ID                   uuid.UUID `gorm:"primary_key;default:uuid_generate_v4()"`
	ManagedResourceID    uuid.UUID
	StateCiphertext      []byte
	StateNonce           []byte
	LastConfigCiphertext []byte
	LastConfigNonce      []byte
	SchemaHash           string
	EncryptionVersion    int
	StateFormat          string
	LockVersion          int64
	CreatedAt            *time.Time
	UpdatedAt            *time.Time
}

func (s *TerraformManagedResourceState) TableName() string {
	return "terraform_managed_resource_states"
}

func FindManagedResourceState(tx *gorm.DB, managedResourceID uuid.UUID) (*TerraformManagedResourceState, error) {
	var state TerraformManagedResourceState
	err := tx.Where("managed_resource_id = ?", managedResourceID).First(&state).Error
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *TerraformManagedResourceState) ZeroAndDelete(tx *gorm.DB) error {
	if err := tx.Model(s).Updates(map[string]any{
		"state_ciphertext":       []byte{},
		"state_nonce":            []byte{},
		"last_config_ciphertext": []byte{},
		"last_config_nonce":      []byte{},
		"updated_at":             time.Now(),
	}).Error; err != nil {
		return err
	}
	return tx.Delete(s).Error
}
