package workers

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/superplanehq/superplane/pkg/models"
	"golang.org/x/sync/semaphore"
)

const (
	terraformEventRetentionBatchSize = 100
	terraformEventRetentionEvery     = time.Hour
)

type TerraformEventRetentionWorker struct {
	semaphore *semaphore.Weighted
	logger    *log.Entry
}

func NewTerraformEventRetentionWorker() *TerraformEventRetentionWorker {
	return &TerraformEventRetentionWorker{
		semaphore: semaphore.NewWeighted(1),
		logger:    log.WithFields(log.Fields{"worker": "TerraformEventRetentionWorker"}),
	}
}

func (w *TerraformEventRetentionWorker) Start(ctx context.Context) {
	w.tick(ctx)
	ticker := time.NewTicker(terraformEventRetentionEvery)
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

func (w *TerraformEventRetentionWorker) tick(ctx context.Context) {
	if err := w.semaphore.Acquire(ctx, 1); err != nil {
		if ctx.Err() == nil {
			w.logger.Errorf("Error acquiring semaphore: %v", err)
		}
		return
	}
	defer w.semaphore.Release(1)

	if err := w.Cleanup(time.Now().UTC()); err != nil {
		w.logger.Errorf("Error cleaning up Terraform managed resource retention data: %v", err)
	}
}

func (w *TerraformEventRetentionWorker) Cleanup(referenceTime time.Time) error {
	if _, err := models.DeleteExpiredManagedResourceEvents(referenceTime, terraformEventRetentionBatchSize); err != nil {
		return err
	}
	if _, err := models.DeleteExpiredDeletedManagedResources(referenceTime, terraformEventRetentionBatchSize); err != nil {
		return err
	}
	return nil
}
