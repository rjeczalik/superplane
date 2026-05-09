package terraform

import (
	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/models"
	"gorm.io/gorm"
)

func RecordManagedResourceEvent(tx *gorm.DB, resourceID uuid.UUID, eventType string, sanitizedOutputs, hashInput map[string]any, outputsHash *string, metadata map[string]any) (*models.TerraformManagedResourceEvent, error) {
	return models.CreateManagedResourceEvent(tx, resourceID, eventType, sanitizedOutputs, hashInput, outputsHash, metadata)
}

func DetectAndRecordManagedResourceChange(tx *gorm.DB, resource models.TerraformManagedResource, previousHash *string, sanitizedOutputs, hashInput map[string]any, changedFields []string, metadata map[string]any) (*models.TerraformManagedResourceEvent, string, bool, error) {
	outputsHash, err := ComputeOutputsHash(hashInput, changedFields)
	if err != nil {
		return nil, "", false, err
	}
	if previousHash != nil && *previousHash == outputsHash {
		return nil, outputsHash, false, nil
	}
	event, err := RecordManagedResourceEvent(tx, resource.ManagedResourceID, models.ManagedResourceEventUpdated, sanitizedOutputs, hashInput, &outputsHash, metadata)
	if err != nil {
		return nil, "", false, err
	}
	return event, outputsHash, true, nil
}
