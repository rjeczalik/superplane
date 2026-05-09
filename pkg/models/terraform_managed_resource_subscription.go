package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/database"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TerraformManagedResourceSubscription struct {
	ID                uuid.UUID `gorm:"primary_key;default:uuid_generate_v4()"`
	OrganizationID    uuid.UUID
	CanvasID          uuid.UUID
	IntegrationID     uuid.UUID
	NodeID            string
	ResourceType      string
	ManagedResourceID *uuid.UUID
	IdempotencyKey    *string
	ChangedFields     datatypes.JSONType[[]string]
	PollIntervalSecs  int
	BackoffSecs       int
	LastPollAt        *time.Time
	Enabled           bool
	CreatedAt         *time.Time
	UpdatedAt         *time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

func (s *TerraformManagedResourceSubscription) TableName() string {
	return "terraform_managed_resource_subscriptions"
}

type TerraformManagedResourceSubscriptionCursor struct {
	ID                uuid.UUID `gorm:"primary_key;default:uuid_generate_v4()"`
	SubscriptionID    uuid.UUID
	ManagedResourceID uuid.UUID
	LastOutputsHash   *string
	LastEventID       *uuid.UUID
	LastEmittedAt     *time.Time
	CreatedAt         *time.Time
	UpdatedAt         *time.Time
}

func (c *TerraformManagedResourceSubscriptionCursor) TableName() string {
	return "terraform_managed_resource_subscription_cursors"
}

type TerraformManagedResourceSubscriptionConfig struct {
	ManagedResourceID *uuid.UUID
	IdempotencyKey    *string
	ChangedFields     []string
	PollIntervalSecs  int
	BackoffSecs       int
	Enabled           bool
}

type WatchedManagedResourceSchedule struct {
	ManagedResourceID uuid.UUID
	NextPollAt        time.Time
	PollIntervalSecs  int
}

func FindMatchingSubscriptions(tx *gorm.DB, canvasID, integrationID uuid.UUID, resourceType string, managedResourceID uuid.UUID, idempotencyKey *string) ([]TerraformManagedResourceSubscription, error) {
	var subscriptions []TerraformManagedResourceSubscription
	query := tx.
		Where("canvas_id = ?", canvasID).
		Where("integration_id = ?", integrationID).
		Where("resource_type = ?", resourceType).
		Where("enabled = ?", true).
		Where("deleted_at IS NULL").
		Where("(managed_resource_id IS NULL OR managed_resource_id = ?)", managedResourceID)
	if idempotencyKey != nil {
		query = query.Where("(idempotency_key IS NULL OR idempotency_key = ?)", *idempotencyKey)
	} else {
		query = query.Where("idempotency_key IS NULL")
	}

	return subscriptions, query.Find(&subscriptions).Error
}

func ListWatchedResourceSchedules(tx *gorm.DB, limit int) ([]WatchedManagedResourceSchedule, error) {
	var schedules []WatchedManagedResourceSchedule
	query := tx.
		Table("terraform_managed_resource_subscriptions AS s").
		Select(`
			s.managed_resource_id,
			COALESCE(MIN(s.last_poll_at + ((s.poll_interval_secs + s.backoff_secs) * INTERVAL '1 second')), now()) AS next_poll_at,
			MIN(s.poll_interval_secs) AS poll_interval_secs`).
		Joins("JOIN terraform_managed_resources r ON r.managed_resource_id = s.managed_resource_id").
		Where("s.enabled = ?", true).
		Where("s.deleted_at IS NULL").
		Where("s.managed_resource_id IS NOT NULL").
		Where("r.deleted_at IS NULL").
		Group("s.managed_resource_id").
		Order("next_poll_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	return schedules, query.Scan(&schedules).Error
}

func FindOrCreateManagedResourceSubscriptionCursor(tx *gorm.DB, subscriptionID, managedResourceID uuid.UUID) (*TerraformManagedResourceSubscriptionCursor, error) {
	now := time.Now()
	cursor := TerraformManagedResourceSubscriptionCursor{
		SubscriptionID:    subscriptionID,
		ManagedResourceID: managedResourceID,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}

	err := tx.
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "subscription_id"}, {Name: "managed_resource_id"}},
			DoNothing: true,
		}).
		Create(&cursor).
		Error
	if err != nil {
		return nil, err
	}

	err = tx.
		Where("subscription_id = ?", subscriptionID).
		Where("managed_resource_id = ?", managedResourceID).
		First(&cursor).
		Error
	if err != nil {
		return nil, err
	}

	return &cursor, nil
}

func MarkManagedResourceSubscriptionsPolled(tx *gorm.DB, managedResourceID uuid.UUID, at time.Time) error {
	return tx.
		Model(&TerraformManagedResourceSubscription{}).
		Where("managed_resource_id = ?", managedResourceID).
		Where("enabled = ?", true).
		Where("deleted_at IS NULL").
		Updates(map[string]any{"last_poll_at": at, "updated_at": at}).Error
}

func (c *TerraformManagedResourceSubscriptionCursor) Update(tx *gorm.DB, outputsHash *string, eventID uuid.UUID) error {
	now := time.Now()
	return tx.Model(c).Updates(map[string]any{
		"last_outputs_hash": outputsHash,
		"last_event_id":     eventID,
		"last_emitted_at":   now,
		"updated_at":        now,
	}).Error
}

func UpsertSubscriptionForNode(tx *gorm.DB, orgID, canvasID, integrationID uuid.UUID, nodeID, resourceType string, config TerraformManagedResourceSubscriptionConfig) (*TerraformManagedResourceSubscription, error) {
	now := time.Now()
	enabled := config.Enabled
	if config.PollIntervalSecs == 0 {
		config.PollIntervalSecs = 300
	}
	subscription := TerraformManagedResourceSubscription{
		OrganizationID:    orgID,
		CanvasID:          canvasID,
		IntegrationID:     integrationID,
		NodeID:            nodeID,
		ResourceType:      resourceType,
		ManagedResourceID: config.ManagedResourceID,
		IdempotencyKey:    config.IdempotencyKey,
		ChangedFields:     datatypes.NewJSONType(config.ChangedFields),
		PollIntervalSecs:  config.PollIntervalSecs,
		BackoffSecs:       config.BackoffSecs,
		Enabled:           enabled,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}

	err := tx.
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "node_id"},
				{Name: "canvas_id"},
				{Name: "integration_id"},
				{Name: "resource_type"},
			},
			TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
			DoUpdates: clause.Assignments(map[string]any{
				"managed_resource_id": config.ManagedResourceID,
				"idempotency_key":     config.IdempotencyKey,
				"changed_fields":      subscription.ChangedFields,
				"poll_interval_secs":  config.PollIntervalSecs,
				"backoff_secs":        config.BackoffSecs,
				"enabled":             enabled,
				"updated_at":          now,
			}),
		}).
		Create(&subscription).
		Error
	if err != nil {
		return nil, err
	}

	err = tx.
		Where("canvas_id = ?", canvasID).
		Where("integration_id = ?", integrationID).
		Where("node_id = ?", nodeID).
		Where("resource_type = ?", resourceType).
		First(&subscription).
		Error
	if err != nil {
		return nil, err
	}

	return &subscription, nil
}

func DisableSubscriptionForNode(tx *gorm.DB, canvasID uuid.UUID, nodeID, resourceType string) error {
	return tx.
		Model(&TerraformManagedResourceSubscription{}).
		Where("canvas_id = ?", canvasID).
		Where("node_id = ?", nodeID).
		Where("resource_type = ?", resourceType).
		Updates(map[string]any{"enabled": false, "updated_at": time.Now()}).Error
}

func DisableManagedResourceSubscriptionsForNodeInTransaction(tx *gorm.DB, canvasID uuid.UUID, nodeID string) error {
	return tx.
		Model(&TerraformManagedResourceSubscription{}).
		Where("canvas_id = ?", canvasID).
		Where("node_id = ?", nodeID).
		Updates(map[string]any{"enabled": false, "updated_at": time.Now()}).Error
}

func FindMatchingManagedResourceSubscriptions(canvasID, integrationID uuid.UUID, resourceType string, managedResourceID uuid.UUID, idempotencyKey *string) ([]TerraformManagedResourceSubscription, error) {
	return FindMatchingSubscriptions(database.Conn(), canvasID, integrationID, resourceType, managedResourceID, idempotencyKey)
}
