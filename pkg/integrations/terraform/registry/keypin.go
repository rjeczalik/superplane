package registry

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/gorm"
)

const (
	TrustModeOperator = "operator"
	TrustModeTOFU     = "tofu"
)

type KeyPinAuditEvent struct {
	ProviderSource string
	Fingerprint    string
	TrustMode      string
}

type KeyPinPolicy struct {
	Production           bool
	AllowTOFU            bool
	OperatorFingerprints []string
	Audit                func(KeyPinAuditEvent)
}

type KeyPinStore struct {
	db *gorm.DB
}

func NewKeyPinStore(db *gorm.DB) *KeyPinStore {
	return &KeyPinStore{db: db}
}

func (s *KeyPinStore) ResolveKey(source ProviderSource, registryKeys openpgp.EntityList, policy KeyPinPolicy) (openpgp.EntityList, error) {
	if source.Namespace() == "hashicorp" {
		if len(registryKeys) == 0 {
			return nil, fmt.Errorf("no HashiCorp trust anchor keys configured")
		}
		return registryKeys, nil
	}
	if len(registryKeys) == 0 {
		return nil, fmt.Errorf("registry did not return a provider signing key")
	}

	operatorFingerprints := normalizedFingerprints(policy.OperatorFingerprints)
	if len(operatorFingerprints) > 0 {
		return keysMatchingFingerprints(registryKeys, operatorFingerprints)
	}

	var existing models.TerraformProviderGPGKey
	err := s.db.Where("provider_source = ?", source.String()).First(&existing).Error
	if err == nil {
		return keysMatchingFingerprints(registryKeys, map[string]struct{}{normalizeFingerprint(existing.Fingerprint): {}})
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("load provider GPG key pin: %w", err)
	}
	if policy.Production && !policy.AllowTOFU {
		return nil, fmt.Errorf("provider %q requires a preconfigured GPG key fingerprint in production", source.String())
	}
	if !policy.AllowTOFU {
		return nil, fmt.Errorf("TOFU key pinning is disabled for provider %q", source.String())
	}

	entity := registryKeys[0]
	fingerprint := FingerprintForEntity(entity)
	armored, err := armorEntity(entity)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row := models.TerraformProviderGPGKey{
		ProviderSource: source.String(),
		KeyID:          fmt.Sprintf("%X", entity.PrimaryKey.KeyId),
		Fingerprint:    fingerprint,
		ASCIIArmor:     armored,
		TrustMode:      TrustModeTOFU,
		PinnedAt:       &now,
	}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, fmt.Errorf("pin provider GPG key: %w", err)
	}
	if policy.Audit != nil {
		policy.Audit(KeyPinAuditEvent{
			ProviderSource: source.String(),
			Fingerprint:    fingerprint,
			TrustMode:      TrustModeTOFU,
		})
	}

	return openpgp.EntityList{entity}, nil
}

func armorEntity(entity *openpgp.Entity) (string, error) {
	var out bytes.Buffer
	writer, err := armor.Encode(&out, openpgp.PublicKeyType, nil)
	if err != nil {
		return "", fmt.Errorf("armor provider GPG key: %w", err)
	}
	if err := entity.Serialize(writer); err != nil {
		return "", fmt.Errorf("serialize provider GPG key: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close armored provider GPG key: %w", err)
	}
	return out.String(), nil
}

func ParseArmoredKey(armored []byte) (openpgp.EntityList, error) {
	keys, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(armored))
	if err != nil {
		return nil, fmt.Errorf("parse armored GPG key: %w", err)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("armored GPG keyring is empty")
	}

	return keys, nil
}

func FingerprintForEntity(entity *openpgp.Entity) string {
	return strings.ToUpper(hex.EncodeToString(entity.PrimaryKey.Fingerprint[:]))
}

func keysMatchingFingerprints(keys openpgp.EntityList, fingerprints map[string]struct{}) (openpgp.EntityList, error) {
	for _, key := range keys {
		if _, ok := fingerprints[normalizeFingerprint(FingerprintForEntity(key))]; ok {
			return openpgp.EntityList{key}, nil
		}
	}

	return nil, fmt.Errorf("registry signing key fingerprint does not match configured pin")
}

func normalizedFingerprints(fingerprints []string) map[string]struct{} {
	out := make(map[string]struct{}, len(fingerprints))
	for _, fingerprint := range fingerprints {
		out[normalizeFingerprint(fingerprint)] = struct{}{}
	}

	return out
}

func normalizeFingerprint(fingerprint string) string {
	fingerprint = strings.ReplaceAll(fingerprint, ":", "")
	fingerprint = strings.ReplaceAll(fingerprint, " ", "")
	return strings.ToUpper(fingerprint)
}
