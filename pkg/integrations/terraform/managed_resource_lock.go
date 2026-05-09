package terraform

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TryAdvisoryLockForIdempotency(tx *gorm.DB, canvasID, integrationID uuid.UUID, resourceType, key string) (bool, error) {
	scope := fmt.Sprintf("%s:%s:%s:%s", canvasID, integrationID, resourceType, key)
	sum := sha256.Sum256([]byte(scope))
	lockID := int64(binary.BigEndian.Uint64(sum[:8]))

	var acquired bool
	err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", lockID).Scan(&acquired).Error
	return acquired, err
}
