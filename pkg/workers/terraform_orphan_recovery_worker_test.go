package workers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	terraformintegration "github.com/superplanehq/superplane/pkg/integrations/terraform"
	"github.com/superplanehq/superplane/pkg/models"
)

func TestTerraformOrphanRecoveryWorkerClaimsOrphanRiskResource(t *testing.T) {
	resource := models.TerraformManagedResource{
		ManagedResourceID: uuid.New(),
		OrganizationID:    uuid.New(),
		CanvasID:          uuid.New(),
		IntegrationID:     uuid.New(),
		Status:            models.ManagedResourceStatusCreating,
		OrphanRisk:        true,
	}
	store := &orphanRecoveryStore{}
	worker := NewTerraformOrphanRecoveryWorker(store, nil, nil)

	require.NoError(t, worker.ClaimForRecovery(context.Background(), resource))
	assert.Equal(t, resource.ManagedResourceID, store.claimedResourceID)
	assert.Equal(t, "recover", store.claimedOperation)
}

type orphanRecoveryStore struct {
	pollingStore
	claimedResourceID uuid.UUID
	claimedOperation  string
}

func (s *orphanRecoveryStore) ClaimOperation(ctx context.Context, auth terraformintegration.ManagedResourceAuthContext, managedResourceID uuid.UUID, operation string, leaseUntil time.Time, allowedStatuses []string) (uuid.UUID, error) {
	s.claimedResourceID = managedResourceID
	s.claimedOperation = operation
	return uuid.New(), nil
}
