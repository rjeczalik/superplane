package workers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/database"
	"github.com/superplanehq/superplane/pkg/grpc/actions/messages"
	terraformintegration "github.com/superplanehq/superplane/pkg/integrations/terraform"
	"github.com/superplanehq/superplane/pkg/models"
	"golang.org/x/sync/semaphore"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	terraformEventDispatcherBatchSize = 50
	terraformEventDispatcherEvery     = 5 * time.Second
)

type TerraformEventDispatcherWorker struct {
	semaphore *semaphore.Weighted
	logger    *log.Entry
	publish   func(*models.CanvasEvent) error
}

func NewTerraformEventDispatcherWorker() *TerraformEventDispatcherWorker {
	return &TerraformEventDispatcherWorker{
		semaphore: semaphore.NewWeighted(10),
		logger:    log.WithFields(log.Fields{"worker": "TerraformEventDispatcherWorker"}),
		publish:   messages.PublishCanvasEventCreatedMessage,
	}
}

func (w *TerraformEventDispatcherWorker) Start(ctx context.Context) {
	w.tick(ctx)
	ticker := time.NewTicker(terraformEventDispatcherEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *TerraformEventDispatcherWorker) tick(ctx context.Context) {
	events, err := models.ListPendingManagedResourceEvents(terraformEventDispatcherBatchSize)
	if err != nil {
		w.logger.Errorf("Error listing pending Terraform managed resource events: %v", err)
		return
	}

	for _, event := range events {
		if err := w.semaphore.Acquire(ctx, 1); err != nil {
			if ctx.Err() != nil {
				return
			}
			w.logger.Errorf("Error acquiring semaphore: %v", err)
			continue
		}

		go func(event models.TerraformManagedResourceEvent) {
			defer w.semaphore.Release(1)
			if _, err := w.LockAndProcessEvent(event.ID); err != nil {
				w.logger.Errorf("Error dispatching Terraform managed resource event %s: %v", event.ID, err)
			}
		}(event)
	}
}

func (w *TerraformEventDispatcherWorker) LockAndProcessEvent(eventID uuid.UUID) ([]models.CanvasEvent, error) {
	var created []models.CanvasEvent
	err := database.Conn().Transaction(func(tx *gorm.DB) error {
		event, err := models.LockPendingManagedResourceEvent(tx, eventID)
		if err != nil {
			return err
		}
		created, err = w.processEvent(tx, event)
		return err
	})
	if err != nil {
		return nil, err
	}

	for i := range created {
		if err := w.publish(&created[i]); err != nil {
			w.logger.Errorf("Error publishing Terraform fanout workflow event %s: %v", created[i].ID, err)
		}
	}
	return created, nil
}

func (w *TerraformEventDispatcherWorker) processEvent(tx *gorm.DB, event *models.TerraformManagedResourceEvent) ([]models.CanvasEvent, error) {
	resource, err := models.FindManagedResourceInTransaction(tx, event.ManagedResourceID)
	if err != nil {
		return nil, fmt.Errorf("find managed resource: %w", err)
	}

	subscriptions, err := models.FindMatchingSubscriptions(tx, resource.CanvasID, resource.IntegrationID, resource.ResourceType, resource.ManagedResourceID, resource.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("find matching subscriptions: %w", err)
	}

	var created []models.CanvasEvent
	for _, subscription := range subscriptions {
		hash, err := terraformintegration.ComputeOutputsHash(event.HashInput.Data(), subscription.ChangedFields.Data())
		if err != nil {
			return nil, err
		}

		cursor, err := models.FindOrCreateManagedResourceSubscriptionCursor(tx, subscription.ID, resource.ManagedResourceID)
		if err != nil {
			return nil, err
		}
		if cursor.LastOutputsHash != nil && *cursor.LastOutputsHash == hash {
			if event.EventType == models.ManagedResourceEventUpdated {
				continue
			}
		}
		cursorHash := terraformEventCursorHash(event.EventType, hash)
		if cursor.LastOutputsHash != nil && *cursor.LastOutputsHash == cursorHash {
			continue
		}

		now := time.Now()
		workflowEvent := models.CanvasEvent{
			ID:         uuid.New(),
			WorkflowID: resource.CanvasID,
			NodeID:     subscription.NodeID,
			Channel:    core.DefaultOutputChannel.Name,
			State:      models.CanvasEventStatePending,
			Data: datatypes.NewJSONType(any(map[string]any{
				"event_type":          event.EventType,
				"managed_resource_id": resource.ManagedResourceID.String(),
				"resource_type":       resource.ResourceType,
				"outputs":             event.Outputs.Data(),
				"metadata":            event.Metadata.Data(),
			})),
			CreatedAt: &now,
		}
		if err := tx.Create(&workflowEvent).Error; err != nil {
			return nil, err
		}
		if err := cursor.Update(tx, &cursorHash, event.ID); err != nil {
			return nil, err
		}
		created = append(created, workflowEvent)
	}

	return created, event.MarkProcessed(tx)
}

func terraformEventCursorHash(eventType, outputsHash string) string {
	return strings.Join([]string{eventType, outputsHash}, ":")
}
