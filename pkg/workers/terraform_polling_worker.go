package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/superplanehq/superplane/pkg/config"
	"github.com/superplanehq/superplane/pkg/database"
	terraformintegration "github.com/superplanehq/superplane/pkg/integrations/terraform"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/runtime"
	"github.com/superplanehq/superplane/pkg/models"
	"golang.org/x/sync/semaphore"
	"gorm.io/gorm"
)

const (
	terraformPollingBatchSize = 100
	terraformPollingEvery     = 30 * time.Second
	terraformMissingThreshold = 2
	terraformErrorThreshold   = 3
)

type TerraformProviderConfigLoader interface {
	LoadProviderConfig(ctx context.Context, integrationID string) (map[string]any, error)
}

type TerraformPollingWorker struct {
	semaphore            *semaphore.Weighted
	logger               *log.Entry
	store                terraformintegration.ManagedResourceStore
	runtimeFactory       terraformintegration.ConfiguredRuntimeFactory
	providerConfigLoader TerraformProviderConfigLoader
	timeout              time.Duration
}

func NewTerraformPollingWorker(store terraformintegration.ManagedResourceStore, runtimeFactory terraformintegration.ConfiguredRuntimeFactory, providerConfigLoader TerraformProviderConfigLoader) *TerraformPollingWorker {
	return &TerraformPollingWorker{
		semaphore:            semaphore.NewWeighted(10),
		logger:               log.WithFields(log.Fields{"worker": "TerraformPollingWorker"}),
		store:                store,
		runtimeFactory:       runtimeFactory,
		providerConfigLoader: providerConfigLoader,
		timeout:              30 * time.Minute,
	}
}

func (w *TerraformPollingWorker) Start(ctx context.Context) {
	w.tick(ctx)
	ticker := time.NewTicker(terraformPollingEvery)
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

func (w *TerraformPollingWorker) tick(ctx context.Context) {
	schedules, err := models.ListWatchedResourceSchedules(database.Conn(), terraformPollingBatchSize)
	if err != nil {
		w.logger.Errorf("Error listing Terraform polling schedules: %v", err)
		return
	}

	now := time.Now()
	for _, schedule := range schedules {
		if schedule.NextPollAt.After(now) {
			continue
		}
		if err := w.semaphore.Acquire(ctx, 1); err != nil {
			if ctx.Err() != nil {
				return
			}
			w.logger.Errorf("Error acquiring semaphore: %v", err)
			continue
		}
		go func(schedule models.WatchedManagedResourceSchedule) {
			defer w.semaphore.Release(1)
			if err := w.RefreshResource(ctx, schedule.ManagedResourceID); err != nil {
				w.logger.Errorf("Error polling Terraform managed resource %s: %v", schedule.ManagedResourceID, err)
			}
		}(schedule)
	}
}

func (w *TerraformPollingWorker) RefreshResource(ctx context.Context, resourceID uuid.UUID) error {
	resource, err := models.FindManagedResource(resourceID)
	if err != nil {
		return err
	}
	if resource.IsTransient() || resource.CurrentOperationID != nil {
		return nil
	}
	auth := terraformintegration.ManagedResourceSystemAuth(resource.OrganizationID, resource.CanvasID, resource.IntegrationID, "polling_worker")
	callCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	operationID, err := w.store.ClaimOperation(callCtx, auth, resource.ManagedResourceID, "poll", time.Now().Add(w.timeout+time.Minute), []string{models.ManagedResourceStatusReady, models.ManagedResourceStatusMissing})
	if err != nil {
		return err
	}
	loaded, err := w.store.LoadForOperation(callCtx, auth, resource.ManagedResourceID, operationID)
	if err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		return err
	}
	providerConfig, err := w.providerConfigLoader.LoadProviderConfig(callCtx, resource.IntegrationID.String())
	if err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		return err
	}
	providerConfigValue, err := runtimeDynamicValue(providerConfig)
	if err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		return err
	}
	provider, err := w.runtimeFactory.RuntimeForProvider(callCtx, config.TerraformProviderIntegration{
		Name:    resource.ProviderName,
		Source:  resource.ProviderSource,
		Version: resource.ProviderVersion,
	})
	if err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		return err
	}
	defer provider.Close()
	if err := provider.Configure(callCtx, &runtime.ConfigureRequest{Config: providerConfigValue}); err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		return err
	}
	result, err := provider.ReadResourceState(callCtx, &runtime.ReadResourceStateRequest{
		TypeName:   resource.ResourceType,
		PriorState: runtime.ProviderState{Envelope: loaded.StatePayload},
		SchemaHash: loaded.State.SchemaHash,
	})
	if err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		_ = database.Conn().Transaction(func(tx *gorm.DB) error {
			if err := models.RecordManagedResourcePollError(tx, resource.ManagedResourceID, err.Error(), terraformErrorThreshold); err != nil {
				return err
			}
			return models.MarkManagedResourceSubscriptionsPolled(tx, resource.ManagedResourceID, time.Now())
		})
		return err
	}
	if result.NotFound {
		return database.Conn().Transaction(func(tx *gorm.DB) error {
			eventType, err := models.RecordManagedResourceMissing(tx, resource.ManagedResourceID, operationID, terraformMissingThreshold)
			if err != nil {
				return err
			}
			if _, err := terraformintegration.RecordManagedResourceEvent(tx, resource.ManagedResourceID, eventType, map[string]any{}, map[string]any{}, nil, map[string]any{"source": "polling_worker"}); err != nil {
				return err
			}
			return models.MarkManagedResourceSubscriptionsPolled(tx, resource.ManagedResourceID, time.Now())
		})
	}
	payload, err := terraformintegration.PayloadFromProviderState(result.NewState)
	if err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		return err
	}
	outputsHash, err := terraformintegration.ComputeOutputsHash(payload, nil)
	if err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		return err
	}
	priorPayload, err := terraformintegration.PayloadFromProviderState(runtime.ProviderState{Envelope: loaded.StatePayload})
	if err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		return err
	}
	priorOutputsHash, err := terraformintegration.ComputeOutputsHash(priorPayload, nil)
	if err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		return err
	}
	if err := w.store.SaveRefreshedState(context.Background(), auth, terraformintegration.SaveManagedResourceStateInput{
		ManagedResourceID:   resource.ManagedResourceID,
		OperationID:         operationID,
		ExpectedLockVersion: loaded.State.LockVersion,
		StatePayload:        result.NewState.Envelope,
		SchemaHash:          loaded.State.SchemaHash,
		StateFormat:         loaded.State.StateFormat,
	}); err != nil {
		w.markOperationFailed(auth, resource, operationID, err)
		return err
	}

	return database.Conn().Transaction(func(tx *gorm.DB) error {
		if priorOutputsHash != outputsHash {
			if _, err := terraformintegration.RecordManagedResourceEvent(tx, resource.ManagedResourceID, models.ManagedResourceEventUpdated, payload, payload, &outputsHash, map[string]any{"source": "polling_worker"}); err != nil {
				return err
			}
		}
		return models.MarkManagedResourceSubscriptionsPolled(tx, resource.ManagedResourceID, time.Now())
	})
}

func (w *TerraformPollingWorker) markOperationFailed(auth terraformintegration.ManagedResourceAuthContext, resource *models.TerraformManagedResource, operationID uuid.UUID, err error) {
	if err == nil {
		return
	}
	_ = w.store.MarkOperationFailed(context.Background(), auth, resource.ManagedResourceID, operationID, resource.Status, err.Error())
}

func runtimeDynamicValue(payload map[string]any) (runtime.DynamicValue, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return runtime.DynamicValue{}, fmt.Errorf("marshal terraform provider config: %w", err)
	}
	return runtime.DynamicValue{JSON: raw}, nil
}
