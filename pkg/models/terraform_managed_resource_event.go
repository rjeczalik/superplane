package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/database"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ManagedResourceEventCreated           = "resource_created"
	ManagedResourceEventUpdated           = "resource_updated"
	ManagedResourceEventReplaced          = "resource_replaced"
	ManagedResourceEventDeleted           = "resource_deleted"
	ManagedResourceEventForgotten         = "resource_forgotten"
	ManagedResourceEventMissing           = "resource_missing"
	ManagedResourceEventExternallyDeleted = "resource_externally_deleted"
	ManagedResourceEventRecovered         = "resource_recovered"

	ManagedResourceEventStatePending   = "pending"
	ManagedResourceEventStateProcessed = "processed"
)

type TerraformManagedResourceEvent struct {
	ID                uuid.UUID `gorm:"primary_key;default:uuid_generate_v4()"`
	ManagedResourceID uuid.UUID
	OrganizationID    uuid.UUID
	CanvasID          uuid.UUID
	IntegrationID     uuid.UUID
	ResourceType      string
	EventType         string
	State             string
	OutputsHash       *string
	Outputs           datatypes.JSONType[map[string]any]
	HashInput         datatypes.JSONType[map[string]any]
	Metadata          datatypes.JSONType[map[string]any]
	DispatchAttempts  int
	LastDispatchError *string
	ProcessedAt       *time.Time
	CreatedAt         *time.Time
	UpdatedAt         *time.Time
}

func (e *TerraformManagedResourceEvent) TableName() string {
	return "terraform_managed_resource_events"
}

func CreateManagedResourceEvent(tx *gorm.DB, resourceID uuid.UUID, eventType string, sanitizedOutputs, hashInput map[string]any, outputsHash *string, metadata map[string]any) (*TerraformManagedResourceEvent, error) {
	resource, err := FindManagedResourceInTransaction(tx, resourceID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	event := TerraformManagedResourceEvent{
		ManagedResourceID: resource.ManagedResourceID,
		OrganizationID:    resource.OrganizationID,
		CanvasID:          resource.CanvasID,
		IntegrationID:     resource.IntegrationID,
		ResourceType:      resource.ResourceType,
		EventType:         eventType,
		State:             ManagedResourceEventStatePending,
		OutputsHash:       outputsHash,
		Outputs:           datatypes.NewJSONType(sanitizedOutputs),
		HashInput:         datatypes.NewJSONType(hashInput),
		Metadata:          datatypes.NewJSONType(metadata),
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}

	err = tx.Create(&event).Error
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func ListPendingManagedResourceEvents(limit int) ([]TerraformManagedResourceEvent, error) {
	var events []TerraformManagedResourceEvent
	query := database.Conn().
		Where("state = ?", ManagedResourceEventStatePending).
		Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	return events, query.Find(&events).Error
}

func LockPendingManagedResourceEvent(tx *gorm.DB, eventID uuid.UUID) (*TerraformManagedResourceEvent, error) {
	var event TerraformManagedResourceEvent
	err := tx.
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("id = ?", eventID).
		Where("state = ?", ManagedResourceEventStatePending).
		First(&event).
		Error
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (e *TerraformManagedResourceEvent) MarkProcessed(tx *gorm.DB) error {
	now := time.Now()
	return tx.Model(e).Updates(map[string]any{
		"state":        ManagedResourceEventStateProcessed,
		"processed_at": now,
		"updated_at":   now,
	}).Error
}

func DeleteExpiredManagedResourceEvents(referenceTime time.Time, batchSize int) (int64, error) {
	query := database.Conn().Exec(`
		DELETE FROM terraform_managed_resource_events e
		USING organizations o
		WHERE e.organization_id = o.id
		  AND o.deleted_at IS NULL
		  AND o.usage_retention_window_days IS NOT NULL
		  AND o.usage_retention_window_days > 0
		  AND e.state = ?
		  AND e.created_at + (o.usage_retention_window_days * INTERVAL '1 day') < ?
		  AND e.id IN (
		    SELECT id
		    FROM terraform_managed_resource_events
		    WHERE state = ?
		    ORDER BY created_at ASC
		    LIMIT ?
		  )
	`, ManagedResourceEventStateProcessed, referenceTime.UTC(), ManagedResourceEventStateProcessed, batchSize)
	return query.RowsAffected, query.Error
}
