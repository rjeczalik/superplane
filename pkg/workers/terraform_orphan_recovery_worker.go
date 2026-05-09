package workers

import (
	"context"
	"time"

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
	terraformOrphanRecoveryBatchSize = 10
	terraformOrphanRecoveryEvery     = 5 * time.Minute
	terraformOrphanRecoveryOlderThan = 10 * time.Minute
)

type TerraformOrphanRecoveryWorker struct {
	semaphore            *semaphore.Weighted
	logger               *log.Entry
	store                terraformintegration.ManagedResourceStore
	runtimeFactory       terraformintegration.ConfiguredRuntimeFactory
	providerConfigLoader TerraformProviderConfigLoader
	timeout              time.Duration
}

func NewTerraformOrphanRecoveryWorker(store terraformintegration.ManagedResourceStore, runtimeFactory terraformintegration.ConfiguredRuntimeFactory, providerConfigLoader TerraformProviderConfigLoader) *TerraformOrphanRecoveryWorker {
	return &TerraformOrphanRecoveryWorker{
		semaphore:            semaphore.NewWeighted(2),
		logger:               log.WithFields(log.Fields{"worker": "TerraformOrphanRecoveryWorker"}),
		store:                store,
		runtimeFactory:       runtimeFactory,
		providerConfigLoader: providerConfigLoader,
		timeout:              30 * time.Minute,
	}
}

func (w *TerraformOrphanRecoveryWorker) Start(ctx context.Context) {
	w.tick(ctx)
	ticker := time.NewTicker(terraformOrphanRecoveryEvery)
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

func (w *TerraformOrphanRecoveryWorker) tick(ctx context.Context) {
	resources, err := models.ListOrphanRiskManagedResources(time.Now().Add(-terraformOrphanRecoveryOlderThan), terraformOrphanRecoveryBatchSize)
	if err != nil {
		w.logger.Errorf("Error listing Terraform orphan-risk managed resources: %v", err)
		return
	}
	for _, resource := range resources {
		if err := w.semaphore.Acquire(ctx, 1); err != nil {
			if ctx.Err() != nil {
				return
			}
			w.logger.Errorf("Error acquiring semaphore: %v", err)
			continue
		}
		go func(resource models.TerraformManagedResource) {
			defer w.semaphore.Release(1)
			if err := w.RecoverResource(ctx, resource); err != nil {
				w.logger.Errorf("Error claiming Terraform orphan-risk managed resource %s: %v", resource.ManagedResourceID, err)
			}
		}(resource)
	}
}

func (w *TerraformOrphanRecoveryWorker) ClaimForRecovery(ctx context.Context, resource models.TerraformManagedResource) error {
	auth := terraformintegration.ManagedResourceSystemAuth(resource.OrganizationID, resource.CanvasID, resource.IntegrationID, "orphan_recovery")
	_, err := w.store.ClaimOperation(ctx, auth, resource.ManagedResourceID, "recover", time.Now().Add(30*time.Minute), []string{models.ManagedResourceStatusCreating, models.ManagedResourceStatusUpdating, models.ManagedResourceStatusDeleting})
	return err
}

func (w *TerraformOrphanRecoveryWorker) RecoverResource(ctx context.Context, resource models.TerraformManagedResource) error {
	if w.runtimeFactory == nil || w.providerConfigLoader == nil {
		return w.ClaimForRecovery(ctx, resource)
	}
	auth := terraformintegration.ManagedResourceSystemAuth(resource.OrganizationID, resource.CanvasID, resource.IntegrationID, "orphan_recovery")
	callCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	operationID, err := w.store.ClaimOperation(callCtx, auth, resource.ManagedResourceID, "recover", time.Now().Add(w.timeout+time.Minute), []string{
		models.ManagedResourceStatusCreating,
		models.ManagedResourceStatusUpdating,
		models.ManagedResourceStatusDeleting,
	})
	if err != nil {
		return err
	}
	loaded, err := w.store.LoadForOperation(callCtx, auth, resource.ManagedResourceID, operationID)
	if err != nil {
		return err
	}
	providerConfig, err := w.providerConfigLoader.LoadProviderConfig(callCtx, resource.IntegrationID.String())
	if err != nil {
		return err
	}
	providerConfigValue, err := runtimeDynamicValue(providerConfig)
	if err != nil {
		return err
	}
	provider, err := w.runtimeFactory.RuntimeForProvider(callCtx, config.TerraformProviderIntegration{
		Name:    resource.ProviderName,
		Source:  resource.ProviderSource,
		Version: resource.ProviderVersion,
	})
	if err != nil {
		return err
	}
	defer provider.Close()
	if err := provider.Configure(callCtx, &runtime.ConfigureRequest{Config: providerConfigValue}); err != nil {
		return err
	}
	result, err := provider.ReadResourceState(callCtx, &runtime.ReadResourceStateRequest{
		TypeName:   resource.ResourceType,
		PriorState: runtime.ProviderState{Envelope: loaded.StatePayload},
		SchemaHash: loaded.State.SchemaHash,
	})
	if err != nil {
		return err
	}
	return database.Conn().Transaction(func(tx *gorm.DB) error {
		if err := models.RecoverManagedResource(tx, resource.ManagedResourceID, operationID, !result.NotFound); err != nil {
			return err
		}
		eventType := models.ManagedResourceEventRecovered
		if result.NotFound {
			eventType = models.ManagedResourceEventDeleted
		}
		_, err := terraformintegration.RecordManagedResourceEvent(tx, resource.ManagedResourceID, eventType, map[string]any{}, map[string]any{}, nil, map[string]any{"source": "orphan_recovery"})
		return err
	})
}
