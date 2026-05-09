package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/database"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ManagedResourceStatusCreating        = "creating"
	ManagedResourceStatusReady           = "ready"
	ManagedResourceStatusUpdating        = "updating"
	ManagedResourceStatusDeleting        = "deleting"
	ManagedResourceStatusMissing         = "missing"
	ManagedResourceStatusDeleted         = "deleted"
	ManagedResourceStatusDeletedExternal = "deleted_external"

	ManagedResourceHealthHealthy     = "healthy"
	ManagedResourceHealthDegraded    = "degraded"
	ManagedResourceHealthUnreachable = "unreachable"
)

var ErrManagedResourceOperationInProgress = errors.New("managed resource operation in progress")

type TerraformManagedResource struct {
	ID                   uuid.UUID `gorm:"primary_key;default:uuid_generate_v4()"`
	ManagedResourceID    uuid.UUID
	OrganizationID       uuid.UUID
	IntegrationID        uuid.UUID
	CanvasID             uuid.UUID
	CreatedByNodeID      string
	CreatedByExecutionID *uuid.UUID
	CreatedByEventID     *uuid.UUID
	RootEventID          *uuid.UUID
	ProviderName         string
	ProviderSource       string
	ProviderVersion      string
	ResourceType         string
	IdempotencyKey       *string
	RemoteID             *string
	DisplayName          *string
	Status               string
	Health               string
	LastOperation        *string
	RetentionPolicy      datatypes.JSONType[map[string]any]
	RecoveryHints        datatypes.JSONType[map[string]any]
	LastRefreshedAt      *time.Time
	MissingCount         int
	ErrorCount           int
	OrphanRisk           bool
	LastError            *string
	LastErrorAt          *time.Time
	CurrentOperationID   *uuid.UUID
	OperationStartedAt   *time.Time
	OperationExpiresAt   *time.Time
	CreatedAt            *time.Time
	UpdatedAt            *time.Time
	DeletedAt            gorm.DeletedAt `gorm:"index"`
}

func (r *TerraformManagedResource) TableName() string {
	return "terraform_managed_resources"
}

func LockManagedResource(tx *gorm.DB, id uuid.UUID) (*TerraformManagedResource, error) {
	var resource TerraformManagedResource
	err := tx.
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("managed_resource_id = ?", id).
		First(&resource).
		Error
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

func LockManagedResourceForIdempotency(tx *gorm.DB, canvasID, integrationID uuid.UUID, resourceType, key string) (*TerraformManagedResource, error) {
	var resource TerraformManagedResource
	err := tx.
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("canvas_id = ?", canvasID).
		Where("integration_id = ?", integrationID).
		Where("resource_type = ?", resourceType).
		Where("idempotency_key = ?", key).
		First(&resource).
		Error
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

func FindManagedResource(id uuid.UUID) (*TerraformManagedResource, error) {
	return FindManagedResourceInTransaction(database.Conn(), id)
}

func FindManagedResourceInTransaction(tx *gorm.DB, id uuid.UUID) (*TerraformManagedResource, error) {
	var resource TerraformManagedResource
	err := tx.Where("managed_resource_id = ?", id).First(&resource).Error
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

func ListManagedResourcesForCanvas(canvasID uuid.UUID, limit int) ([]TerraformManagedResource, error) {
	var resources []TerraformManagedResource
	query := database.Conn().
		Where("canvas_id = ?", canvasID).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	return resources, query.Find(&resources).Error
}

func ListManagedResourcesForPolling(limit int) ([]TerraformManagedResource, error) {
	var resources []TerraformManagedResource
	query := database.Conn().
		Where("deleted_at IS NULL").
		Where("status IN ?", []string{ManagedResourceStatusReady, ManagedResourceStatusMissing}).
		Order("last_refreshed_at ASC NULLS FIRST")
	if limit > 0 {
		query = query.Limit(limit)
	}
	return resources, query.Find(&resources).Error
}

func (r *TerraformManagedResource) TransitionStatus(tx *gorm.DB, status string) error {
	now := time.Now()
	r.Status = status
	r.UpdatedAt = &now
	return tx.Model(r).Updates(map[string]any{"status": status, "updated_at": now}).Error
}

func (r *TerraformManagedResource) SetHealth(tx *gorm.DB, health string) error {
	now := time.Now()
	r.Health = health
	r.UpdatedAt = &now
	return tx.Model(r).Updates(map[string]any{"health": health, "updated_at": now}).Error
}

func (r *TerraformManagedResource) RecordError(tx *gorm.DB, msg string, threshold int) error {
	now := time.Now()
	health := ManagedResourceHealthDegraded
	if threshold > 0 && r.ErrorCount+1 >= threshold {
		health = ManagedResourceHealthUnreachable
	}
	return tx.Model(r).Updates(map[string]any{
		"error_count":   gorm.Expr("error_count + 1"),
		"health":        health,
		"last_error":    msg,
		"last_error_at": now,
		"updated_at":    now,
	}).Error
}

func (r *TerraformManagedResource) ClearErrors(tx *gorm.DB) error {
	now := time.Now()
	return tx.Model(r).Updates(map[string]any{
		"error_count":   0,
		"health":        ManagedResourceHealthHealthy,
		"last_error":    nil,
		"last_error_at": nil,
		"updated_at":    now,
	}).Error
}

func RecordManagedResourcePollError(tx *gorm.DB, resourceID uuid.UUID, msg string, threshold int) error {
	resource, err := LockManagedResource(tx, resourceID)
	if err != nil {
		return err
	}
	return resource.RecordError(tx, msg, threshold)
}

func RecordManagedResourceMissing(tx *gorm.DB, resourceID uuid.UUID, operationID uuid.UUID, threshold int) (string, error) {
	resource, err := LockManagedResource(tx, resourceID)
	if err != nil {
		return "", err
	}
	if resource.CurrentOperationID == nil || *resource.CurrentOperationID != operationID {
		return "", ErrManagedResourceOperationInProgress
	}

	missingCount := resource.MissingCount + 1
	eventType := ManagedResourceEventMissing
	updates := map[string]any{
		"status":            ManagedResourceStatusMissing,
		"missing_count":     missingCount,
		"last_refreshed_at": time.Now(),
	}
	if threshold > 0 && missingCount >= threshold {
		eventType = ManagedResourceEventExternallyDeleted
		updates["status"] = ManagedResourceStatusDeletedExternal
		updates["deleted_at"] = time.Now()
		if state, err := FindManagedResourceState(tx, resourceID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		} else if err == nil {
			if err := state.ZeroAndDelete(tx); err != nil {
				return "", err
			}
		}
	}
	if err := CompleteManagedResourceOperation(tx, resourceID, operationID, updates); err != nil {
		return "", err
	}
	return eventType, nil
}

func RecoverManagedResource(tx *gorm.DB, resourceID uuid.UUID, operationID uuid.UUID, found bool) error {
	resource, err := LockManagedResource(tx, resourceID)
	if err != nil {
		return err
	}
	if resource.CurrentOperationID == nil || *resource.CurrentOperationID != operationID {
		return ErrManagedResourceOperationInProgress
	}
	updates := map[string]any{
		"orphan_risk":       false,
		"last_refreshed_at": time.Now(),
	}
	if found {
		updates["status"] = ManagedResourceStatusReady
		updates["health"] = ManagedResourceHealthHealthy
		updates["missing_count"] = 0
		updates["error_count"] = 0
	} else {
		updates["status"] = ManagedResourceStatusDeleted
		updates["deleted_at"] = time.Now()
		if state, err := FindManagedResourceState(tx, resourceID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else if err == nil {
			if err := state.ZeroAndDelete(tx); err != nil {
				return err
			}
		}
	}
	return CompleteManagedResourceOperation(tx, resourceID, operationID, updates)
}

func ClaimManagedResourceOperation(tx *gorm.DB, resourceID uuid.UUID, operation string, leaseUntil time.Time, allowedStatuses []string) (uuid.UUID, error) {
	resource, err := LockManagedResource(tx, resourceID)
	if err != nil {
		return uuid.Nil, err
	}
	if resource.CurrentOperationID != nil && resource.OperationExpiresAt != nil && resource.OperationExpiresAt.After(time.Now()) {
		return uuid.Nil, ErrManagedResourceOperationInProgress
	}
	if len(allowedStatuses) > 0 && !containsString(allowedStatuses, resource.Status) {
		return uuid.Nil, gorm.ErrRecordNotFound
	}

	now := time.Now()
	operationID := uuid.New()
	updates := map[string]any{
		"current_operation_id": operationID,
		"operation_started_at": now,
		"operation_expires_at": leaseUntil,
		"last_operation":       operation,
		"updated_at":           now,
	}
	if status := transientStatusForOperation(operation); status != "" {
		updates["status"] = status
	}

	return operationID, tx.Model(resource).Updates(updates).Error
}

func CompleteManagedResourceOperation(tx *gorm.DB, resourceID, operationID uuid.UUID, updates map[string]any) error {
	if updates == nil {
		updates = map[string]any{}
	}
	updates["current_operation_id"] = nil
	updates["operation_started_at"] = nil
	updates["operation_expires_at"] = nil
	updates["updated_at"] = time.Now()
	result := tx.Model(&TerraformManagedResource{}).
		Where("managed_resource_id = ? AND current_operation_id = ?", resourceID, operationID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func ClearManagedResourceOperationLease(tx *gorm.DB, resourceID, operationID uuid.UUID) error {
	return CompleteManagedResourceOperation(tx, resourceID, operationID, nil)
}

func ListExpiredOperationLeases(olderThan time.Time, limit int) ([]TerraformManagedResource, error) {
	var resources []TerraformManagedResource
	query := database.Conn().
		Where("current_operation_id IS NOT NULL").
		Where("operation_expires_at < ?", olderThan).
		Order("operation_expires_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	return resources, query.Find(&resources).Error
}

func (r *TerraformManagedResource) SoftDelete(tx *gorm.DB) error {
	now := time.Now()
	return tx.Model(r).Updates(map[string]any{
		"status":     ManagedResourceStatusDeleted,
		"deleted_at": now,
		"updated_at": now,
	}).Error
}

func ValidateManagedResourceOperation(op string) bool {
	return op == "create" || op == "read" || op == "update" || op == "delete" || op == "force_forget" || op == "poll" || op == "recover"
}

func (r *TerraformManagedResource) IsTransient() bool {
	return r.Status == ManagedResourceStatusCreating || r.Status == ManagedResourceStatusUpdating || r.Status == ManagedResourceStatusDeleting
}

func CountActiveManagedResourcesForIntegration(id uuid.UUID) (int64, error) {
	return CountActiveManagedResourcesForIntegrationInTransaction(database.Conn(), id)
}

func CountActiveManagedResourcesForIntegrationInTransaction(tx *gorm.DB, id uuid.UUID) (int64, error) {
	return countManagedResources(tx.Where("integration_id = ?", id), true)
}

func CountActiveManagedResourcesForCanvas(id uuid.UUID) (int64, error) {
	return CountActiveManagedResourcesForCanvasInTransaction(database.Conn(), id)
}

func CountActiveManagedResourcesForCanvasInTransaction(tx *gorm.DB, id uuid.UUID) (int64, error) {
	return countManagedResources(tx.Where("canvas_id = ?", id), true)
}

func CountActiveManagedResourcesForOrganization(id uuid.UUID) (int64, error) {
	return CountActiveManagedResourcesForOrganizationInTransaction(database.Conn(), id)
}

func CountActiveManagedResourcesForOrganizationInTransaction(tx *gorm.DB, id uuid.UUID) (int64, error) {
	return countManagedResources(tx.Where("organization_id = ?", id), true)
}

func CountRetainedManagedResourceRowsForIntegration(id uuid.UUID) (int64, error) {
	return countManagedResources(database.Conn().Unscoped().Where("integration_id = ?", id), false)
}

func CountRetainedManagedResourceRowsForCanvas(id uuid.UUID) (int64, error) {
	return countManagedResources(database.Conn().Unscoped().Where("canvas_id = ?", id), false)
}

func CountRetainedManagedResourceRowsForOrganization(id uuid.UUID) (int64, error) {
	return countManagedResources(database.Conn().Unscoped().Where("organization_id = ?", id), false)
}

func DeleteExpiredDeletedManagedResources(referenceTime time.Time, batchSize int) (int64, error) {
	query := database.Conn().Exec(`
		DELETE FROM terraform_managed_resources r
		USING organizations o
		WHERE r.organization_id = o.id
		  AND r.deleted_at IS NOT NULL
		  AND r.status IN (?, ?)
		  AND o.deleted_at IS NULL
		  AND o.usage_retention_window_days IS NOT NULL
		  AND o.usage_retention_window_days > 0
		  AND r.deleted_at + (o.usage_retention_window_days * INTERVAL '1 day') < ?
		  AND NOT EXISTS (
		    SELECT 1 FROM terraform_managed_resource_states s
		    WHERE s.managed_resource_id = r.managed_resource_id
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM terraform_managed_resource_events e
		    WHERE e.managed_resource_id = r.managed_resource_id AND e.state = 'pending'
		  )
		  AND r.managed_resource_id IN (
		    SELECT managed_resource_id
		    FROM terraform_managed_resources
		    WHERE deleted_at IS NOT NULL
		    ORDER BY deleted_at ASC
		    LIMIT ?
		  )
	`, ManagedResourceStatusDeleted, ManagedResourceStatusDeletedExternal, referenceTime.UTC(), batchSize)
	return query.RowsAffected, query.Error
}

func ListOrphanRiskManagedResources(olderThan time.Time, limit int) ([]TerraformManagedResource, error) {
	var resources []TerraformManagedResource
	query := database.Conn().
		Where("orphan_risk = ?", true).
		Where("created_at < ?", olderThan).
		Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	return resources, query.Find(&resources).Error
}

func countManagedResources(query *gorm.DB, activeOnly bool) (int64, error) {
	var count int64
	query = query.Model(&TerraformManagedResource{})
	if activeOnly {
		query = query.
			Where("deleted_at IS NULL").
			Where("status NOT IN ?", []string{ManagedResourceStatusDeleted, ManagedResourceStatusDeletedExternal})
	}
	return count, query.Count(&count).Error
}

func transientStatusForOperation(operation string) string {
	switch operation {
	case "create":
		return ManagedResourceStatusCreating
	case "update":
		return ManagedResourceStatusUpdating
	case "delete", "force_forget":
		return ManagedResourceStatusDeleting
	default:
		return ""
	}
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
